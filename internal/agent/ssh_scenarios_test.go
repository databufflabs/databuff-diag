package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/databufflabs/databuff-diag/internal/exec"
	"github.com/databufflabs/databuff-diag/internal/llm"
	"github.com/databufflabs/databuff-diag/internal/policy"
	"github.com/databufflabs/databuff-diag/internal/store"
)

// sshScenarioRecorder captures the exact ssh/sshpass command line for inspection.
type sshScenarioRecorder struct {
	lastCmd string
}

func (r *sshScenarioRecorder) run(ctx context.Context, toolCall ToolCall, session *store.Session, cfgStore *store.ConfigStore) (*exec.Result, string, error) {
	ag := &Agent{ConfigStore: cfgStore, Sessions: store.NewSessionStoreAt("")}
	resolved, err := ag.resolveSSH(session, toolCall.SSHTool)
	if err != nil {
		return nil, "", err
	}
	display := buildSSHToolDisplay(*toolCall.SSHTool, resolved)

	runner := exec.NewSSH(exec.SSHConfig{
		Host:     resolved.Host,
		User:     resolved.User,
		Port:     resolved.Port,
		Password: resolved.Password,
	})

	if resolved.Password != "" {
		r.lastCmd = fmt.Sprintf("sshpass -e ssh %s %q", runner.Target(), toolCall.SSHTool.RemoteCommand)
	} else {
		r.lastCmd = fmt.Sprintf("ssh %s %q", runner.Target(), toolCall.SSHTool.RemoteCommand)
	}

	result, err := runner.Run(ctx, toolCall.SSHTool.RemoteCommand)
	return result, display, err
}

func messagesContainSecret(messages []store.SessionMessage, secret string) bool {
	for _, msg := range messages {
		if msg.Role == "user" {
			continue
		}
		if strings.Contains(msg.Content, secret) || strings.Contains(msg.Command, secret) {
			return true
		}
	}
	return false
}

func TestSSHScenario_SavedHostCredentials(t *testing.T) {
	dir := t.TempDir()
	cfgStore := store.NewConfigStoreAt(filepath.Join(dir, "config.yaml"))
	hostID := "host-prod-case-a"
	cfg := store.DefaultConfig()
	cfg.SSH.Hosts = store.SSHHostsList{
		{
			ID:       hostID,
			Name:     "prod-vm",
			Host:     "127.0.0.1",
			User:     "root",
			Password: "SavedHostPass123",
		},
	}
	if err := cfgStore.Save(cfg); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	raw, err := os.ReadFile(cfgStore.Path())
	if err != nil {
		t.Fatalf("ReadFile config: %v", err)
	}
	if strings.Contains(string(raw), "SavedHostPass123") {
		t.Fatal("case A setup: host password must be encrypted on disk")
	}
	t.Logf("[Case A] config saved; host password encrypted on disk (enc:v1 present: %v)", strings.Contains(string(raw), "enc:v1:"))

	sessions := store.NewSessionStoreAt(filepath.Join(dir, "sessions"))
	session, err := sessions.Create(policy.Open)
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}

	remoteCmd := "echo CASE_A_SAVED_HOST"
	llmResponse := fmt.Sprintf(`{"tool":"ssh","host_id":%q,"command":%q}`, hostID, remoteCmd)

	ag := &Agent{
		LLM: &mockLLM{
			responses: []string{llmResponse, "remote check done"},
		},
		Policy:      &policy.Engine{},
		Executor:    exec.NewLocal(exec.LocalConfig{}),
		Sessions:    sessions,
		ConfigStore: cfgStore,
	}
	provider := llm.MergedProvider{ProviderCode: "test", BaseURL: "http://test", Model: "m"}

	userMsg := "请 SSH 登录 prod-vm 查看主机名"
	if err := ag.HandleUserMessage(context.Background(), session, userMsg, provider); err != nil {
		t.Fatalf("HandleUserMessage: %v", err)
	}

	reloaded, err := sessions.Load(session.ID)
	if err != nil {
		t.Fatalf("Load session: %v", err)
	}

	var toolMsg *store.SessionMessage
	for i := range reloaded.Messages {
		if reloaded.Messages[i].Role == "tool" {
			toolMsg = &reloaded.Messages[i]
			break
		}
	}
	if toolMsg == nil {
		t.Fatalf("case A: expected tool message, got %+v", reloaded.Messages)
	}

	t.Logf("[Case A] display command: %s", toolMsg.Command)
	t.Logf("[Case A] tool output excerpt: %s", truncateForLog(toolMsg.Content, 400))

	if !strings.HasPrefix(toolMsg.Command, "ssh prod-vm (root@127.0.0.1)") {
		t.Fatalf("case A: unexpected display command: %q", toolMsg.Command)
	}
	if messagesContainSecret(reloaded.Messages, "SavedHostPass123") {
		t.Fatal("case A: saved host password leaked into session messages")
	}
	if strings.Contains(toolMsg.Command, "sshpass") {
		t.Fatal("case A: sshpass must not appear in display command")
	}

	rec := &sshScenarioRecorder{}
	toolCall, ok := ParseTool(llmResponse)
	if !ok {
		t.Fatal("case A: failed to parse ssh tool")
	}
	result, display, runErr := rec.run(context.Background(), toolCall, reloaded, cfgStore)
	t.Logf("[Case A] resolved exec: %s", rec.lastCmd)
	t.Logf("[Case A] resolved display: %s", display)
	if !strings.Contains(rec.lastCmd, "sshpass -e ssh root@127.0.0.1") {
		t.Fatalf("case A: expected sshpass with saved password, got %q", rec.lastCmd)
	}
	if result != nil {
		t.Logf("[Case A] ssh exit_code=%d stderr=%q", result.ExitCode, truncateForLog(result.Stderr, 200))
	}
	if runErr != nil {
		t.Logf("[Case A] ssh run note: %v (expected if no sshd on 127.0.0.1)", runErr)
	}
}

func TestSSHScenario_UserMessageCredentialsWithoutSavedHost(t *testing.T) {
	dir := t.TempDir()
	cfgStore := store.NewConfigStoreAt(filepath.Join(dir, "config.yaml"))
	cfg := store.DefaultConfig()
	cfg.SSH.Hosts = store.SSHHostsList{}
	if err := cfgStore.Save(cfg); err != nil {
		t.Fatalf("Save config: %v", err)
	}
	t.Log("[Case B] config has no saved SSH hosts")

	sessions := store.NewSessionStoreAt(filepath.Join(dir, "sessions"))
	session, err := sessions.Create(policy.Open)
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}

	remoteCmd := "echo CASE_B_USER_PASS"
	llmResponse := fmt.Sprintf(`{"tool":"ssh","host":"127.0.0.1","user":"root","command":%q}`, remoteCmd)

	ag := &Agent{
		LLM: &mockLLM{
			responses: []string{llmResponse, "done"},
		},
		Policy:      &policy.Engine{},
		Executor:    exec.NewLocal(exec.LocalConfig{}),
		Sessions:    sessions,
		ConfigStore: cfgStore,
	}
	provider := llm.MergedProvider{ProviderCode: "test", BaseURL: "http://test", Model: "m"}

	userMsg := "ssh root@127.0.0.1 password: UserMsgPass456 帮我看下主机"
	if err := ag.HandleUserMessage(context.Background(), session, userMsg, provider); err != nil {
		t.Fatalf("HandleUserMessage: %v", err)
	}

	reloaded, err := sessions.Load(session.ID)
	if err != nil {
		t.Fatalf("Load session: %v", err)
	}

	override, ok := reloaded.SSHOverrides["127.0.0.1"]
	if !ok {
		t.Fatalf("case B: expected session ssh override, got %+v", reloaded.SSHOverrides)
	}
	if override.Password != "UserMsgPass456" || override.User != "root" {
		t.Fatalf("case B: override = %+v", override)
	}
	t.Logf("[Case B] session override parsed: user=%s host=%s (password present)", override.User, override.Host)

	var toolMsg *store.SessionMessage
	for i := range reloaded.Messages {
		if reloaded.Messages[i].Role == "tool" {
			toolMsg = &reloaded.Messages[i]
			break
		}
	}
	if toolMsg == nil {
		t.Fatalf("case B: expected tool message, got %+v", reloaded.Messages)
	}

	t.Logf("[Case B] display command: %s", toolMsg.Command)
	t.Logf("[Case B] tool output excerpt: %s", truncateForLog(toolMsg.Content, 400))

	if !strings.Contains(toolMsg.Command, "root@127.0.0.1") {
		t.Fatalf("case B: unexpected display command: %q", toolMsg.Command)
	}
	if messagesContainSecret(reloaded.Messages, "UserMsgPass456") {
		t.Fatal("case B: user-provided password leaked into assistant/tool messages")
	}

	rec := &sshScenarioRecorder{}
	toolCall, ok := ParseTool(llmResponse)
	if !ok {
		t.Fatal("case B: failed to parse ssh tool")
	}
	result, display, runErr := rec.run(context.Background(), toolCall, reloaded, cfgStore)
	t.Logf("[Case B] resolved exec: %s", rec.lastCmd)
	t.Logf("[Case B] resolved display: %s", display)
	if !strings.Contains(rec.lastCmd, "sshpass -e ssh root@127.0.0.1") {
		t.Fatalf("case B: expected sshpass with session override password, got %q", rec.lastCmd)
	}
	if result != nil {
		t.Logf("[Case B] ssh exit_code=%d stderr=%q", result.ExitCode, truncateForLog(result.Stderr, 200))
	}
	if runErr != nil {
		t.Logf("[Case B] ssh run note: %v (expected if no sshd on 127.0.0.1)", runErr)
	}
}

func truncateForLog(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
