package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/databufflabs/databuff-diag/internal/policy"
)

func TestSessionStore_WorkspaceDirUnderSessionDir(t *testing.T) {
	dir := t.TempDir()
	s := NewSessionStoreAt(filepath.Join(dir, "sessions"))

	created, err := s.Create(policy.WriteApproval)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	want := filepath.Join(s.Dir(), created.ID)
	if got := s.WorkspaceDir(created.ID); got != want {
		t.Fatalf("WorkspaceDir = %q, want %q", got, want)
	}
}

func TestSessionStore_CreateLoadPersist(t *testing.T) {
	dir := t.TempDir()
	s := NewSessionStoreAt(filepath.Join(dir, "sessions"))

	created, err := s.Create(policy.WriteApproval)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected session id")
	}
	if st, err := os.Stat(s.WorkspaceDir(created.ID)); err != nil || !st.IsDir() {
		t.Fatalf("workspace dir missing: %v", err)
	}

	loaded, err := s.Load(created.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.PolicyMode != policy.WriteApproval {
		t.Fatalf("policy_mode = %q", loaded.PolicyMode)
	}

	// Simulate restart: new store instance, same directory.
	restarted := NewSessionStoreAt(s.Dir())
	reloaded, err := restarted.Load(created.ID)
	if err != nil {
		t.Fatalf("reload after restart: %v", err)
	}
	if reloaded.ID != created.ID {
		t.Fatalf("id = %q, want %q", reloaded.ID, created.ID)
	}
}

func TestSessionStore_AppendMessage(t *testing.T) {
	dir := t.TempDir()
	s := NewSessionStoreAt(filepath.Join(dir, "sessions"))
	session, err := s.Create(policy.Open)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.AppendMessage(session, SessionMessage{Role: "user", Content: "hello"}); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}

	loaded, err := s.Load(session.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Messages) != 1 || loaded.Messages[0].Content != "hello" {
		t.Fatalf("messages = %+v", loaded.Messages)
	}
}

func TestSessionStore_SetPolicyMode(t *testing.T) {
	dir := t.TempDir()
	s := NewSessionStoreAt(filepath.Join(dir, "sessions"))

	created, err := s.Create(policy.WriteApproval)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := s.SetPolicyMode(created.ID, policy.Open)
	if err != nil {
		t.Fatalf("SetPolicyMode: %v", err)
	}
	if updated.PolicyMode != policy.Open {
		t.Fatalf("policy_mode = %q, want open", updated.PolicyMode)
	}

	loaded, err := s.Load(created.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.PolicyMode != policy.Open {
		t.Fatalf("persisted policy_mode = %q, want open", loaded.PolicyMode)
	}

	if _, err := s.SetPolicyMode(created.ID, policy.Mode("invalid")); err == nil {
		t.Fatal("expected error for invalid policy_mode")
	}
}

func TestValidateChatMessage(t *testing.T) {
	if err := ValidateChatMessage(strings.Repeat("a", MaxChatMessageRunes)); err != nil {
		t.Fatalf("at limit: %v", err)
	}
	if err := ValidateChatMessage(strings.Repeat("a", MaxChatMessageRunes+1)); err == nil {
		t.Fatal("expected error above limit")
	}
}
