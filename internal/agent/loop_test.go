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
	"github.com/databufflabs/databuff-diag/internal/skill"
	"github.com/databufflabs/databuff-diag/internal/store"
)

type mockLLM struct {
	responses []string
	calls     int
}

func (m *mockLLM) Chat(_ context.Context, _ llm.MergedProvider, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	idx := m.calls
	m.calls++
	if idx >= len(m.responses) {
		return &llm.ChatResponse{Content: "done"}, nil
	}
	return &llm.ChatResponse{Content: m.responses[idx]}, nil
}

func TestAgent_AutoExecReadonly(t *testing.T) {
	dir := t.TempDir()
	sessions := store.NewSessionStoreAt(filepath.Join(dir, "sessions"))
	session, err := sessions.Create(policy.WriteApproval)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ag := &Agent{
		LLM: &mockLLM{
			responses: []string{
				`Checking hostname: {"tool":"shell","command":"ls"}`,
				"The listing is complete.",
			},
		},
		Policy:   &policy.Engine{},
		Executor: exec.NewLocal(exec.LocalConfig{}),
		Sessions: sessions,
	}

	provider := llm.MergedProvider{ProviderCode: "test", BaseURL: "http://test", Model: "m"}
	if err := ag.HandleUserMessage(context.Background(), session, "check hostname", provider); err != nil {
		t.Fatalf("HandleUserMessage: %v", err)
	}

	reloaded, err := sessions.Load(session.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var sawTool bool
	for _, msg := range reloaded.Messages {
		if msg.Role == "tool" && msg.Command == "ls" {
			sawTool = true
		}
	}
	if !sawTool {
		t.Fatalf("expected tool message with command output, got %+v", reloaded.Messages)
	}
	if len(reloaded.PendingApprovals) != 0 {
		t.Fatalf("pending approvals = %d, want 0", len(reloaded.PendingApprovals))
	}
}

func TestAgent_PendingApprovalForWrite(t *testing.T) {
	dir := t.TempDir()
	sessions := store.NewSessionStoreAt(filepath.Join(dir, "sessions"))
	session, err := sessions.Create(policy.WriteApproval)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ag := &Agent{
		LLM: &mockLLM{
			responses: []string{`{"tool":"shell","command":"sed -i 's/a/b/' /tmp/x"}`},
		},
		Policy:   &policy.Engine{},
		Executor: exec.NewLocal(exec.LocalConfig{}),
		Sessions: sessions,
	}

	provider := llm.MergedProvider{ProviderCode: "test", BaseURL: "http://test", Model: "m"}
	if err := ag.HandleUserMessage(context.Background(), session, "edit file", provider); err != nil {
		t.Fatalf("HandleUserMessage: %v", err)
	}

	reloaded, err := sessions.Load(session.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(reloaded.PendingApprovals) != 1 {
		t.Fatalf("pending approvals = %d, want 1", len(reloaded.PendingApprovals))
	}
}

func TestAgent_ApproveExecutesCommand(t *testing.T) {
	dir := t.TempDir()
	sessions := store.NewSessionStoreAt(filepath.Join(dir, "sessions"))
	session, err := sessions.Create(policy.WriteApproval)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ag := &Agent{
		LLM: &mockLLM{
			responses: []string{
				`{"tool":"shell","command":"sed -i 's/a/b/' /tmp/x"}`,
				"edit complete",
			},
		},
		Policy:   &policy.Engine{},
		Executor: exec.NewLocal(exec.LocalConfig{}),
		Sessions: sessions,
	}
	provider := llm.MergedProvider{ProviderCode: "test", BaseURL: "http://test", Model: "m"}

	if err := ag.HandleUserMessage(context.Background(), session, "edit file", provider); err != nil {
		t.Fatalf("HandleUserMessage: %v", err)
	}
	reloaded, _ := sessions.Load(session.ID)
	if len(reloaded.PendingApprovals) != 1 {
		t.Fatalf("expected pending approval")
	}
	approvalID := reloaded.PendingApprovals[0].ID

	if err := ag.Approve(context.Background(), reloaded, approvalID, true, provider); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	final, _ := sessions.Load(session.ID)
	if len(final.PendingApprovals) != 0 {
		t.Fatalf("pending approvals should be cleared")
	}
}

func TestAgent_RejectRecordsToolMessage(t *testing.T) {
	dir := t.TempDir()
	sessions := store.NewSessionStoreAt(filepath.Join(dir, "sessions"))
	session, err := sessions.Create(policy.WriteApproval)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	cmd := `sed -i 's/a/b/' /tmp/x`
	ag := &Agent{
		LLM: &mockLLM{
			responses: []string{
				fmt.Sprintf(`{"tool":"shell","command":%q}`, cmd),
				"Understood, I will explain without running that command.",
			},
		},
		Policy:   &policy.Engine{},
		Executor: exec.NewLocal(exec.LocalConfig{}),
		Sessions: sessions,
	}
	provider := llm.MergedProvider{ProviderCode: "test", BaseURL: "http://test", Model: "m"}

	if err := ag.HandleUserMessage(context.Background(), session, "edit file", provider); err != nil {
		t.Fatalf("HandleUserMessage: %v", err)
	}
	reloaded, _ := sessions.Load(session.ID)
	approvalID := reloaded.PendingApprovals[0].ID

	if err := ag.Approve(context.Background(), reloaded, approvalID, false, provider); err != nil {
		t.Fatalf("Approve reject: %v", err)
	}

	final, _ := sessions.Load(session.ID)
	var sawReject bool
	for _, msg := range final.Messages {
		if msg.Role == "tool" && msg.Command == cmd && strings.Contains(msg.Content, "Status: rejected by user") {
			sawReject = true
		}
	}
	if !sawReject {
		t.Fatalf("expected tool rejection message, got %+v", final.Messages)
	}

	messages, err := ag.buildChatMessages(final, provider)
	if err != nil {
		t.Fatalf("buildChatMessages: %v", err)
	}
	var sawRejectInLLM bool
	for _, msg := range messages {
		content, _ := msg.Content.(string)
		if strings.Contains(content, "Status: rejected by user") {
			sawRejectInLLM = true
			if msg.Role != "user" {
				t.Fatalf("tool observation should be sent as user role to LLM, got %q", msg.Role)
			}
		}
	}
	if !sawRejectInLLM {
		t.Fatalf("expected rejection observation in LLM messages, got %+v", messages)
	}
}

func TestAgent_IncludesSessionWorkspaceInSystemPrompt(t *testing.T) {
	dir := t.TempDir()
	sessions := store.NewSessionStoreAt(filepath.Join(dir, "sessions"))
	session, err := sessions.Create(policy.WriteApproval)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var captured []llm.ChatMessage
	ag := &Agent{
		LLM:      &capturingLLM{messages: &captured},
		Policy:   &policy.Engine{},
		Executor: exec.NewLocal(exec.LocalConfig{}),
		Sessions: sessions,
	}

	provider := llm.MergedProvider{ProviderCode: "test", BaseURL: "http://test", Model: "m"}
	if err := ag.HandleUserMessage(context.Background(), session, "write a report", provider); err != nil {
		t.Fatalf("HandleUserMessage: %v", err)
	}
	if len(captured) == 0 {
		t.Fatal("expected LLM call")
	}
	sys, _ := captured[0].Content.(string)
	wantWorkspace := sessions.WorkspaceDir(session.ID)
	if !strings.Contains(sys, wantWorkspace) {
		t.Fatalf("system prompt missing workspace dir %q: %q", wantWorkspace, sys)
	}
	if !strings.Contains(sys, session.ID) {
		t.Fatalf("system prompt missing session id %q: %q", session.ID, sys)
	}
}

func TestAgent_IncludesSkillContextInSystemPrompt(t *testing.T) {
	dir := t.TempDir()
	skillRoot := filepath.Join(dir, "skills")
	skillDir := filepath.Join(skillRoot, "generic-infra")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillMD := `---
name: generic-infra
description: infra checks
---
# Generic infra guidance
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		t.Fatal(err)
	}

	loader := skill.NewLoader([]string{skillRoot})
	if err := loader.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	sessions := store.NewSessionStoreAt(filepath.Join(dir, "sessions"))
	session, err := sessions.Create(policy.WriteApproval)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var captured []llm.ChatMessage
	ag := &Agent{
		LLM: &capturingLLM{messages: &captured},
		Policy:   &policy.Engine{},
		Executor: exec.NewLocal(exec.LocalConfig{}),
		Sessions: sessions,
		Skills:   loader,
	}

	provider := llm.MergedProvider{ProviderCode: "test", BaseURL: "http://test", Model: "m"}
	if err := ag.HandleUserMessage(context.Background(), session, "check docker", provider); err != nil {
		t.Fatalf("HandleUserMessage: %v", err)
	}
	if len(captured) == 0 {
		t.Fatal("expected LLM call")
	}
	sys, _ := captured[0].Content.(string)
	if !strings.Contains(sys, "generic-infra") || !strings.Contains(sys, "<available_skills>") || !strings.Contains(sys, "SKILL.md") {
		t.Fatalf("system prompt missing skill context: %q", sys)
	}
}

func TestAgent_IncludesHostCatalogInSystemPrompt(t *testing.T) {
	dir := t.TempDir()
	cfgStore := store.NewConfigStoreAt(filepath.Join(dir, "config.yaml"))
	cfg := store.DefaultConfig()
	cfg.SSH.Hosts = store.SSHHostsList{
		{ID: "host-test", Name: "prod", Host: "192.168.1.10", User: "root", Password: "secret"},
	}
	if err := cfgStore.Save(cfg); err != nil {
		t.Fatal(err)
	}

	sessions := store.NewSessionStoreAt(filepath.Join(dir, "sessions"))
	session, err := sessions.Create(policy.WriteApproval)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var captured []llm.ChatMessage
	ag := &Agent{
		LLM:         &capturingLLM{messages: &captured},
		Policy:      &policy.Engine{},
		Executor:    exec.NewLocal(exec.LocalConfig{}),
		Sessions:    sessions,
		ConfigStore: cfgStore,
	}

	provider := llm.MergedProvider{ProviderCode: "test", BaseURL: "http://test", Model: "m"}
	if err := ag.HandleUserMessage(context.Background(), session, "check remote", provider); err != nil {
		t.Fatalf("HandleUserMessage: %v", err)
	}
	if len(captured) == 0 {
		t.Fatal("expected LLM call")
	}
	sys, _ := captured[0].Content.(string)
	if !strings.Contains(sys, "host-test") || !strings.Contains(sys, "prod") {
		t.Fatalf("system prompt missing host catalog: %q", sys)
	}
	if strings.Contains(sys, "secret") {
		t.Fatalf("system prompt must not contain password: %q", sys)
	}
}

func TestAgent_SSHPendingApproval(t *testing.T) {
	dir := t.TempDir()
	sessions := store.NewSessionStoreAt(filepath.Join(dir, "sessions"))
	session, err := sessions.Create(policy.WriteApproval)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ag := &Agent{
		LLM: &mockLLM{
			responses: []string{
				`{"tool":"ssh","host":"10.0.0.2","user":"root","command":"sed -i 's/a/b/' /tmp/x"}`,
			},
		},
		Policy:   &policy.Engine{},
		Executor: exec.NewLocal(exec.LocalConfig{}),
		Sessions: sessions,
	}

	provider := llm.MergedProvider{ProviderCode: "test", BaseURL: "http://test", Model: "m"}
	if err := ag.HandleUserMessage(context.Background(), session, "fix remote file", provider); err != nil {
		t.Fatalf("HandleUserMessage: %v", err)
	}

	reloaded, err := sessions.Load(session.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(reloaded.PendingApprovals) != 1 {
		t.Fatalf("pending approvals = %d, want 1", len(reloaded.PendingApprovals))
	}
	pending := reloaded.PendingApprovals[0]
	if pending.ToolKind != ToolSSH || pending.SSHTool == nil {
		t.Fatalf("pending = %+v, want ssh tool", pending)
	}
	if pending.SSHTool.Host != "10.0.0.2" {
		t.Fatalf("ssh host = %q", pending.SSHTool.Host)
	}
	if strings.Contains(pending.Command, "sshpass") || strings.Contains(pending.Command, "password") {
		t.Fatalf("display command leaked credentials: %q", pending.Command)
	}
}

func TestAgent_AppliesSSHOverridesFromUserMessage(t *testing.T) {
	dir := t.TempDir()
	sessions := store.NewSessionStoreAt(filepath.Join(dir, "sessions"))
	session, err := sessions.Create(policy.WriteApproval)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ag := &Agent{
		LLM:      &mockLLM{responses: []string{"done"}},
		Policy:   &policy.Engine{},
		Executor: exec.NewLocal(exec.LocalConfig{}),
		Sessions: sessions,
	}
	provider := llm.MergedProvider{ProviderCode: "test", BaseURL: "http://test", Model: "m"}

	msg := "ssh root@192.168.1.50 password: TempPass999 看一下 docker"
	if err := ag.HandleUserMessage(context.Background(), session, msg, provider); err != nil {
		t.Fatalf("HandleUserMessage: %v", err)
	}

	reloaded, err := sessions.Load(session.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	o, ok := reloaded.SSHOverrides["192.168.1.50"]
	if !ok {
		t.Fatalf("overrides = %+v", reloaded.SSHOverrides)
	}
	if o.Password != "TempPass999" || o.User != "root" {
		t.Fatalf("override = %+v", o)
	}
}

type capturingLLM struct {
	messages *[]llm.ChatMessage
}

func (c *capturingLLM) Chat(_ context.Context, _ llm.MergedProvider, req llm.ChatRequest) (*llm.ChatResponse, error) {
	*c.messages = req.Messages
	return &llm.ChatResponse{Content: "done"}, nil
}
