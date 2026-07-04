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

const maxReactIterations = 1000

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
		if err := a.rejectAndObserve(session, toolCall); err != nil {
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
	_ = a.maybeCompactSession(ctx, session, provider)

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
		assistantText, llmCalls, err := a.completeAssistant(ctx, provider, messages, onChunk)
		if err != nil {
			return fmt.Errorf("llm chat: %w", err)
		}

		assistantText = strings.TrimSpace(assistantText)
		toolCalls, hasTools := ToolCallsFromCompletion(assistantText, llmCalls)

		if assistantText == "" && !hasTools {
			if err := a.Sessions.AppendMessage(session, store.SessionMessage{
				Role:    "system",
				Content: emptyResponseNudgeMessage,
			}); err != nil {
				return err
			}
			continue
		}

		if hasTools {
			assistantMsg := a.buildAssistantToolMessage(assistantText, toolCalls, llmCalls)
			if err := a.Sessions.AppendMessage(session, assistantMsg); err != nil {
				return err
			}
			if err := a.emitProgress(cb, session); err != nil {
				return err
			}
			for _, toolCall := range toolCalls {
				tc := toolCall
				if err := a.finalizeToolCall(session, &tc); err != nil {
					if appendErr := a.Sessions.AppendMessage(session, store.SessionMessage{
						Role:    "system",
						Content: fmt.Sprintf("工具解析失败：%v", err),
					}); appendErr != nil {
						return appendErr
					}
					if err := a.emitProgress(cb, session); err != nil {
						return err
					}
					continue
				}
				if err := a.maybeEmitBeforeExecute(cb, session, tc); err != nil {
					return err
				}
				if err := a.HandleProposedTool(ctx, session, tc); err != nil {
					return err
				}
				if err := a.emitProgress(cb, session); err != nil {
					return err
				}
				if len(session.PendingApprovals) > 0 {
					return nil
				}
			}
			continue
		}

		assistantMsg := store.SessionMessage{
			Role:    "assistant",
			Content: assistantText,
		}
		if err := a.Sessions.AppendMessage(session, assistantMsg); err != nil {
			return err
		}

		if LooksInlineScriptTool(assistantText) {
			if err := a.Sessions.AppendMessage(session, store.SessionMessage{
				Role:    "system",
				Content: inlineScriptNudgeMessage,
			}); err != nil {
				return err
			}
			continue
		}
		if looksMalformedToolJSON(assistantText) {
			if err := a.Sessions.AppendMessage(session, store.SessionMessage{
				Role:    "system",
				Content: "工具调用格式无效。请使用 function calling（read/write/edit/bash/ssh）或输出合法 tool JSON。",
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
	return nil
}

func (a *Agent) buildAssistantToolMessage(text string, toolCalls []ToolCall, llmCalls []llm.FunctionToolCall) store.SessionMessage {
	msg := store.SessionMessage{Role: "assistant", Content: text}
	if len(llmCalls) > 0 {
		stored := make([]store.StoredToolCall, len(llmCalls))
		for i, c := range llmCalls {
			id := c.ID
			if id == "" {
				id = newApprovalID()
			}
			stored[i] = store.StoredToolCall{ID: id, Name: c.Function.Name, Arguments: c.Function.Arguments}
		}
		msg.ToolCalls = stored
	}
	if len(toolCalls) > 0 {
		tc := toolCalls[0]
		msg.Command = tc.DisplayCommand
		if text == "" {
			msg.Content = ProposalTextForTool("", tc)
		} else {
			msg.Content = ProposalTextForTool(text, tc)
		}
		if risk, err := a.policy().Classify(tc.PolicyCommand()); err == nil {
			msg.Risk = string(risk)
		}
	}
	return msg
}

func (a *Agent) finalizeToolCall(session *store.Session, toolCall *ToolCall) error {
	if toolCall.DisplayCommand == "" {
		toolCall.DisplayCommand = displayForTool(*toolCall)
	}
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

func (a *Agent) emitProgress(cb *LoopCallbacks, session *store.Session) error {
	if cb == nil || cb.AfterResolve == nil {
		return nil
	}
	return cb.AfterResolve(session)
}

func (a *Agent) maybeEmitBeforeExecute(cb *LoopCallbacks, session *store.Session, toolCall ToolCall) error {
	if cb == nil || cb.OnBeforeExecute == nil {
		return nil
	}
	risk, err := a.policy().Classify(toolCall.PolicyCommand())
	if err != nil || risk == policy.RiskBlocked {
		return nil
	}
	if policy.NeedsApproval(risk, session.PolicyMode) {
		return nil
	}
	cmd := toolCall.DisplayCommand
	if cmd == "" {
		cmd = displayForTool(toolCall)
	}
	return cb.OnBeforeExecute(cmd)
}

func (a *Agent) completeAssistant(ctx context.Context, provider llm.MergedProvider, messages []llm.ChatMessage, onChunk func(string) error) (string, []llm.FunctionToolCall, error) {
	req := llm.ChatRequest{
		Messages:   messages,
		Tools:      llm.AgentTools(),
		ToolChoice: "auto",
	}

	if onChunk != nil {
		if streamer, ok := a.LLM.(streamChatClient); ok {
			var full strings.Builder
			var toolCalls []llm.FunctionToolCall
			err := streamer.ChatStream(ctx, provider, req, func(chunk llm.StreamChunk) error {
				if chunk.Done {
					toolCalls = chunk.ToolCalls
					return nil
				}
				if chunk.Content != "" {
					full.WriteString(chunk.Content)
					return onChunk(chunk.Content)
				}
				return nil
			})
			return full.String(), toolCalls, err
		}
	}

	resp, err := a.LLM.Chat(ctx, provider, req)
	if err != nil {
		return "", nil, err
	}
	if onChunk != nil && resp.Content != "" {
		if err := onChunk(resp.Content); err != nil {
			return "", nil, err
		}
	}
	return resp.Content, resp.ToolCalls, nil
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
			Content: fmt.Sprintf("策略已拦截该操作：%s", toolCall.DisplayCommand),
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
	if displayCmd == "" {
		displayCmd = displayForTool(toolCall)
	}

	workspace := ""
	if a.Sessions != nil && session != nil {
		workspace = a.Sessions.WorkspaceDir(session.ID)
	}

	var observation string
	var stdout, stderr string
	var exitCode *int

	switch toolCall.Kind {
	case ToolRead:
		text, err := readWorkspaceFile(workspace, toolCall.ReadPath, toolCall.ReadOffset, toolCall.ReadLimit)
		observation = formatTextToolObservation(displayCmd, text, err)
		if err == nil {
			stdout = text
			code := 0
			exitCode = &code
		} else {
			stderr = err.Error()
			code := 1
			exitCode = &code
		}
	case ToolWrite:
		err := writeWorkspaceFile(workspace, toolCall.WritePath, toolCall.WriteContent)
		msg := fmt.Sprintf("Successfully wrote %s (%d bytes)", toolCall.WritePath, len(toolCall.WriteContent))
		observation = formatTextToolObservation(displayCmd, msg, err)
		code := 0
		if err != nil {
			stderr = err.Error()
			code = 1
		} else {
			stdout = msg
		}
		exitCode = &code
	case ToolEdit:
		msg, err := editWorkspaceFile(workspace, toolCall.EditPath, toolCall.Edits)
		observation = formatTextToolObservation(displayCmd, msg, err)
		code := 0
		if err != nil {
			stderr = err.Error()
			code = 1
		} else {
			stdout = msg
		}
		exitCode = &code
	case ToolSSH:
		result, err := a.executeSSH(ctx, session, toolCall.SSHTool)
		if err != nil {
			return fmt.Errorf("execute ssh: %w", err)
		}
		code := result.ExitCode
		exitCode = &code
		stdout = result.Stdout
		stderr = result.Stderr
		observation = formatObservation(displayCmd, result)
	default:
		result, err := a.executor().Run(ctx, toolCall.ShellCommand)
		if err != nil {
			return fmt.Errorf("execute command: %w", err)
		}
		code := result.ExitCode
		exitCode = &code
		stdout = result.Stdout
		stderr = result.Stderr
		observation = formatObservation(displayCmd, result)
	}

	return a.Sessions.AppendMessage(session, store.SessionMessage{
		Role:       "tool",
		Content:    observation,
		Command:    displayCmd,
		Stdout:     stdout,
		Stderr:     stderr,
		ExitCode:   exitCode,
		Risk:       string(risk),
		ToolCallID: toolCall.ID,
		ToolName:   toolCall.Kind,
	})
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

func (a *Agent) rejectAndObserve(session *store.Session, toolCall ToolCall) error {
	risk, _ := a.policy().Classify(toolCall.PolicyCommand())
	observation := formatRejectionObservation(toolCall.DisplayCommand)
	return a.Sessions.AppendMessage(session, store.SessionMessage{
		Role:       "tool",
		Content:    observation,
		Command:    toolCall.DisplayCommand,
		Risk:       string(risk),
		ToolCallID: toolCall.ID,
		ToolName:   toolCall.Kind,
	})
}

func formatRejectionObservation(cmd string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Command: %s\n", cmd)
	b.WriteString("Status: rejected by user\n")
	b.WriteString("Message: 用户拒绝执行该命令。请勿重试该命令，改用文字说明或提出其他方案。")
	return strings.TrimSpace(b.String())
}

func formatTextToolObservation(cmd, output string, err error) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Tool: %s\n", cmd)
	if err != nil {
		fmt.Fprintf(&b, "Exit code: 1\nStderr:\n%s\n", err.Error())
	} else {
		b.WriteString("Exit code: 0\n")
		if output != "" {
			fmt.Fprintf(&b, "Output:\n%s\n", output)
		}
	}
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
	sys := buildSystemPromptBase()
	if session != nil && session.CompactionSummary != "" {
		sys += "\n\n<conversation_summary>\n" + session.CompactionSummary + "\n</conversation_summary>"
	}
	if cfg, err := a.loadConfig(); err == nil && cfg != nil {
		sys += "\n\n" + sshresolve.FormatHostCatalog(cfg.SSH.Hosts)
	}
	if a.Sessions != nil && session != nil && session.ID != "" {
		workspace := a.Sessions.WorkspaceDir(session.ID)
		sys += fmt.Sprintf("\n\nCurrent date: %s", formatPromptDate())
		sys += fmt.Sprintf("\nCurrent session workspace directory: %s", workspace)
		sys += fmt.Sprintf("\nSession ID: %s", session.ID)
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
		Kind:           ToolBash,
		ShellCommand:   cmd,
		DisplayCommand: cmd,
	})
}
