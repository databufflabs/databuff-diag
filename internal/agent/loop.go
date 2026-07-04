package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/databufflabs/databuff-diag/internal/exec"
	"github.com/databufflabs/databuff-diag/internal/llm"
	"github.com/databufflabs/databuff-diag/internal/policy"
	"github.com/databufflabs/databuff-diag/internal/skill"
	"github.com/databufflabs/databuff-diag/internal/sshresolve"
	"github.com/databufflabs/databuff-diag/internal/store"
)

const maxReactIterations = 8

const systemPrompt = `You are databuff-diag, a site reliability assistant that helps diagnose deployment and infrastructure issues on the host machine.

When you need to run a command on the local machine, propose exactly one JSON tool block:
{"tool":"shell","command":"your full command here"}

When you need to run a command on a remote host via SSH, propose exactly one JSON tool block:
{"tool":"ssh","host_id":"host-xxx","command":"remote command here"}
or
{"tool":"ssh","host":"192.168.1.10","user":"root","command":"remote command here"}

Saved SSH hosts have passwords injected by the system. Never include passwords in tool calls for saved hosts.
Do not use sshpass or embed passwords in shell commands. Use {"tool":"ssh",...} for all remote inspection.
Do not run a local substitute command when the user asked to inspect a remote host.

Do not wrap JSON tool blocks in markdown code fences.

Use complete shell commands (e.g. "docker inspect ai-apm-demo", "docker ps -a"). Never put bare container names, hostnames, or identifiers in code blocks as if they were commands.

Only propose commands you actually need. After command output is provided, analyze it and either propose another command or give your final answer.

Do not invent command output. Wait for real execution results before claiming what a command returned.

Non-interactive execution: never use -it or -t with docker exec; use docker exec <container> sh -c "command" instead.

When you intend to run a command, you MUST include the tool JSON in the same response. Never end with only a transitional sentence (e.g. "接下来查看…：" or "let me check…") without the tool block. Either propose the command or deliver your full analysis.

Each conversation has an isolated session workspace directory. All files you create during this session (reports, scripts, notes, exported data, etc.) must be written only under that directory. Prefer absolute paths rooted at the session workspace in shell commands. Do not write session artifacts to /tmp or other locations outside the workspace unless the user explicitly asks.`

// LoopCallbacks hooks optional streaming progress during approve / agent loop.
type LoopCallbacks struct {
	OnBeforeExecute func(command string) error
	AfterResolve    func(session *store.Session) error
	OnTurnStart     func() error
	OnChunk         func(content string) error
}

// ChatClient is the LLM surface used by the agent loop.
type ChatClient interface {
	Chat(ctx context.Context, provider llm.MergedProvider, req llm.ChatRequest) (*llm.ChatResponse, error)
}

type streamChatClient interface {
	ChatStream(ctx context.Context, provider llm.MergedProvider, req llm.ChatRequest, onChunk func(llm.StreamChunk) error) error
}

// Agent coordinates LLM reasoning, policy checks, and command execution.
type Agent struct {
	LLM         ChatClient
	Policy      *policy.Engine
	Executor    *exec.Local
	Sessions    *store.SessionStore
	Attachments *store.AttachmentStore
	Skills      *skill.Loader
	ConfigStore *store.ConfigStore
}

// New returns an agent with sensible defaults.
func New(sessions *store.SessionStore) *Agent {
	return &Agent{
		LLM:      llm.NewClient(),
		Policy:   &policy.Engine{},
		Executor: exec.NewLocal(exec.LocalConfig{}),
		Sessions: sessions,
	}
}

func (a *Agent) policy() *policy.Engine {
	if a.Policy != nil {
		return a.Policy
	}
	return &policy.Engine{}
}

func (a *Agent) executor() *exec.Local {
	if a.Executor != nil {
		return a.Executor
	}
	return exec.NewLocal(exec.LocalConfig{})
}

func (a *Agent) loadConfig() (*store.Config, error) {
	if a.ConfigStore == nil {
		return store.DefaultConfig(), nil
	}
	return a.ConfigStore.Load()
}

// HandleUserMessage appends the user turn and runs the ReAct loop until no command,
// approval is required, or max iterations is reached.
func (a *Agent) HandleUserMessage(ctx context.Context, session *store.Session, content string, provider llm.MergedProvider) error {
	content = strings.TrimSpace(content)
	sshresolve.ApplyMessageOverrides(session, content)
	if len(session.SSHOverrides) > 0 {
		if err := a.Sessions.Save(session); err != nil {
			return err
		}
	}
	if err := a.Sessions.AppendMessage(session, store.SessionMessage{
		Role:    "user",
		Content: content,
	}); err != nil {
		return err
	}
	return a.runLoop(ctx, session, provider)
}

// Approve resolves a pending approval and continues the loop when approved.
func (a *Agent) Approve(ctx context.Context, session *store.Session, approvalID string, approved bool, provider llm.MergedProvider) error {
	return a.ApproveWithCallbacks(ctx, session, approvalID, approved, provider, nil)
}

// ApproveWithCallbacks is like Approve but can stream loop progress to the UI.
func (a *Agent) ApproveWithCallbacks(ctx context.Context, session *store.Session, approvalID string, approved bool, provider llm.MergedProvider, cb *LoopCallbacks) error {
	idx := -1
	for i, p := range session.PendingApprovals {
		if p.ID == approvalID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("approval %q not found", approvalID)
	}

	pending := session.PendingApprovals[idx]
	session.PendingApprovals = append(session.PendingApprovals[:idx], session.PendingApprovals[idx+1:]...)

	toolCall, ok := ToolCallFromPending(pending)
	if !ok {
		return fmt.Errorf("approval %q has no executable tool", approvalID)
	}

	if !approved {
		if err := a.rejectAndObserve(session, toolCall.DisplayCommand); err != nil {
			return err
		}
	} else {
		if cb != nil && cb.OnBeforeExecute != nil {
			if err := cb.OnBeforeExecute(toolCall.DisplayCommand); err != nil {
				return err
			}
		}
		if err := a.executeToolAndObserve(ctx, session, toolCall); err != nil {
			return err
		}
	}

	if cb != nil && cb.AfterResolve != nil {
		if err := cb.AfterResolve(session); err != nil {
			return err
		}
	}
	return a.runLoopWithCallbacks(ctx, session, provider, cb)
}

func (a *Agent) runLoop(ctx context.Context, session *store.Session, provider llm.MergedProvider) error {
	return a.runLoopWithCallbacks(ctx, session, provider, nil)
}

// RunLoopStream continues the ReAct loop for a session whose latest user turn is
// already persisted. Optional callbacks stream progress to the UI.
func (a *Agent) RunLoopStream(ctx context.Context, session *store.Session, provider llm.MergedProvider, cb *LoopCallbacks) error {
	return a.runLoopWithCallbacks(ctx, session, provider, cb)
}

func (a *Agent) runLoopWithCallbacks(ctx context.Context, session *store.Session, provider llm.MergedProvider, cb *LoopCallbacks) error {
	for i := 0; i < maxReactIterations; i++ {
		if len(session.PendingApprovals) > 0 {
			return a.Sessions.Save(session)
		}

		messages, err := a.buildChatMessages(session, provider)
		if err != nil {
			return fmt.Errorf("build chat messages: %w", err)
		}

		if cb != nil && cb.OnTurnStart != nil {
			if err := cb.OnTurnStart(); err != nil {
				return err
			}
		}

		var onChunk func(string) error
		if cb != nil {
			onChunk = cb.OnChunk
		}
		assistantText, err := a.completeAssistant(ctx, provider, messages, onChunk)
		if err != nil {
			return fmt.Errorf("llm chat: %w", err)
		}

		assistantText = strings.TrimSpace(assistantText)
		if assistantText == "" {
			if err := a.Sessions.AppendMessage(session, store.SessionMessage{
				Role:    "system",
				Content: emptyResponseNudgeMessage,
			}); err != nil {
				return err
			}
			continue
		}
		toolCall, ok := ParseTool(assistantText)
		assistantMsg := store.SessionMessage{
			Role:    "assistant",
			Content: assistantText,
		}
		if ok {
			if err := a.finalizeToolCall(session, &toolCall); err != nil {
				if appendErr := a.Sessions.AppendMessage(session, store.SessionMessage{
					Role:    "system",
					Content: fmt.Sprintf("SSH 工具解析失败：%v", err),
				}); appendErr != nil {
					return appendErr
				}
				continue
			}
			assistantMsg.Content = ProposalTextForTool(assistantText, toolCall)
			assistantMsg.Command = toolCall.DisplayCommand
			if risk, classifyErr := a.policy().Classify(toolCall.PolicyCommand()); classifyErr == nil {
				assistantMsg.Risk = string(risk)
			}
		}
		if err := a.Sessions.AppendMessage(session, assistantMsg); err != nil {
			return err
		}

		if !ok {
			if looksMalformedToolJSON(assistantText) {
				if err := a.Sessions.AppendMessage(session, store.SessionMessage{
					Role:    "system",
					Content: "tool JSON 格式无效或混入多余 shell 片段。请只输出一行合法 JSON，例如 {\"tool\":\"shell\",\"command\":\"docker ps -a\"}，不要把 2>/dev/null 等重定向写在 JSON 外面。",
				}); err != nil {
					return err
				}
				continue
			}
			if looksIncompleteAssistant(assistantText) {
				if err := a.Sessions.AppendMessage(session, store.SessionMessage{
					Role:    "system",
					Content: incompleteNudgeMessage,
				}); err != nil {
					return err
				}
				continue
			}
			return nil
		}

		if err := a.HandleProposedTool(ctx, session, toolCall); err != nil {
			return err
		}
		if len(session.PendingApprovals) > 0 {
			return nil
		}
	}
	return nil
}

func (a *Agent) finalizeToolCall(session *store.Session, toolCall *ToolCall) error {
	if toolCall.Kind != ToolSSH || toolCall.SSHTool == nil {
		return nil
	}
	resolved, err := a.resolveSSH(session, toolCall.SSHTool)
	if err != nil {
		return err
	}
	toolCall.DisplayCommand = buildSSHToolDisplay(*toolCall.SSHTool, resolved)
	return nil
}

func (a *Agent) resolveSSH(session *store.Session, spec *SSHToolCall) (sshresolve.Resolved, error) {
	cfg, err := a.loadConfig()
	if err != nil {
		return sshresolve.Resolved{}, err
	}
	return sshresolve.Resolve(sshresolve.Request{
		HostID:   spec.HostID,
		Host:     spec.Host,
		User:     spec.User,
		Password: spec.Password,
		Port:     spec.Port,
	}, cfg, session)
}

func (a *Agent) completeAssistant(ctx context.Context, provider llm.MergedProvider, messages []llm.ChatMessage, onChunk func(string) error) (string, error) {
	if onChunk != nil {
		if streamer, ok := a.LLM.(streamChatClient); ok {
			var full strings.Builder
			err := streamer.ChatStream(ctx, provider, llm.ChatRequest{Messages: messages}, func(chunk llm.StreamChunk) error {
				if chunk.Done {
					return nil
				}
				full.WriteString(chunk.Content)
				return onChunk(chunk.Content)
			})
			return full.String(), err
		}
	}

	resp, err := a.LLM.Chat(ctx, provider, llm.ChatRequest{Messages: messages})
	if err != nil {
		return "", err
	}
	if onChunk != nil && resp.Content != "" {
		if err := onChunk(resp.Content); err != nil {
			return "", err
		}
	}
	return resp.Content, nil
}

// HandleProposedTool classifies a tool call, queues approval, or executes it.
func (a *Agent) HandleProposedTool(ctx context.Context, session *store.Session, toolCall ToolCall) error {
	policyCmd := toolCall.PolicyCommand()
	risk, err := a.policy().Classify(policyCmd)
	if err != nil {
		return fmt.Errorf("policy classify: %w", err)
	}

	if risk == policy.RiskBlocked {
		return a.Sessions.AppendMessage(session, store.SessionMessage{
			Role:    "system",
			Content: fmt.Sprintf("策略已拦截该命令：%s", toolCall.DisplayCommand),
			Command: toolCall.DisplayCommand,
			Risk:    string(risk),
		})
	}

	if policy.NeedsApproval(risk, session.PolicyMode) {
		session.PendingApprovals = append(session.PendingApprovals, toolCall.ToPendingApproval(newApprovalID(), risk))
		return a.Sessions.Save(session)
	}

	return a.executeToolAndObserve(ctx, session, toolCall)
}

func (a *Agent) executeToolAndObserve(ctx context.Context, session *store.Session, toolCall ToolCall) error {
	policyCmd := toolCall.PolicyCommand()
	risk, _ := a.policy().Classify(policyCmd)
	displayCmd := toolCall.DisplayCommand

	var result *exec.Result
	var err error
	if toolCall.Kind == ToolSSH && toolCall.SSHTool != nil {
		result, err = a.executeSSH(ctx, session, toolCall.SSHTool)
	} else {
		result, err = a.executor().Run(ctx, toolCall.ShellCommand)
	}
	if err != nil {
		return fmt.Errorf("execute command: %w", err)
	}

	exitCode := result.ExitCode
	observation := formatObservation(displayCmd, result)
	if err := a.Sessions.AppendMessage(session, store.SessionMessage{
		Role:     "tool",
		Content:  observation,
		Command:  displayCmd,
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ExitCode: &exitCode,
		Risk:     string(risk),
	}); err != nil {
		return err
	}
	return nil
}

func (a *Agent) executeSSH(ctx context.Context, session *store.Session, spec *SSHToolCall) (*exec.Result, error) {
	resolved, err := a.resolveSSH(session, spec)
	if err != nil {
		return nil, err
	}
	runner := exec.NewSSH(exec.SSHConfig{
		Host:     resolved.Host,
		User:     resolved.User,
		Port:     resolved.Port,
		Password: resolved.Password,
	})
	return runner.Run(ctx, spec.RemoteCommand)
}

func (a *Agent) rejectAndObserve(session *store.Session, cmd string) error {
	risk, _ := a.policy().Classify(cmd)
	observation := formatRejectionObservation(cmd)
	if err := a.Sessions.AppendMessage(session, store.SessionMessage{
		Role:    "tool",
		Content: observation,
		Command: cmd,
		Risk:    string(risk),
	}); err != nil {
		return err
	}
	return nil
}

func formatRejectionObservation(cmd string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Command: %s\n", cmd)
	b.WriteString("Status: rejected by user\n")
	b.WriteString("Message: 用户拒绝执行该命令。请勿重试该命令，改用文字说明或提出其他方案。")
	return strings.TrimSpace(b.String())
}

func formatObservation(cmd string, result *exec.Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Command: %s\n", cmd)
	fmt.Fprintf(&b, "Exit code: %d\n", result.ExitCode)
	if result.Stdout != "" {
		fmt.Fprintf(&b, "Stdout:\n%s\n", result.Stdout)
	}
	if result.Stderr != "" {
		fmt.Fprintf(&b, "Stderr:\n%s\n", result.Stderr)
	}
	if result.TimedOut {
		b.WriteString("(timed out)\n")
	}
	if result.StdoutTruncated || result.StderrTruncated {
		b.WriteString("(output truncated)\n")
	}
	return strings.TrimSpace(b.String())
}

func (a *Agent) buildSystemPrompt(session *store.Session) string {
	sys := systemPrompt
	if cfg, err := a.loadConfig(); err == nil && cfg != nil {
		sys += "\n\n" + sshresolve.FormatHostCatalog(cfg.SSH.Hosts)
	}
	if a.Sessions != nil && session != nil && session.ID != "" {
		workspace := a.Sessions.WorkspaceDir(session.ID)
		sys += fmt.Sprintf(`

Current session workspace directory: %s
Session ID: %s`, workspace, session.ID)
	}
	if a.Skills != nil {
		if ctx := a.Skills.SystemPromptContext(); ctx != "" {
			sys = sys + "\n\n" + ctx
		}
	}
	return sys
}

func (a *Agent) buildChatMessages(session *store.Session, provider llm.MergedProvider) ([]llm.ChatMessage, error) {
	sys := a.buildSystemPrompt(session)
	vision := llm.ProviderSupportsVision(provider)
	return llm.BuildMessagesFromSession(session, sys, a.Attachments, vision)
}

func newApprovalID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "appr-" + hex.EncodeToString(b[:])
}

// HandleProposedCommand classifies a local shell command (legacy helper).
func (a *Agent) HandleProposedCommand(ctx context.Context, session *store.Session, cmd string, provider llm.MergedProvider) error {
	_ = provider
	return a.HandleProposedTool(ctx, session, ToolCall{
		Kind:           ToolShell,
		ShellCommand:   cmd,
		DisplayCommand: cmd,
	})
}
