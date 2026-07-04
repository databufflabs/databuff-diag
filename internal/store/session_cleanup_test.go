package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/databufflabs/databuff-diag/internal/policy"
)

func writeSessionFile(t *testing.T, s *SessionStore, session *Session) {
	t.Helper()
	if err := os.MkdirAll(s.sessionDir(session.ID), 0o700); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(s.metaPath(session.ID), data, sessionFileMode); err != nil {
		t.Fatalf("write session: %v", err)
	}
}

func TestSessionStore_PurgeBefore(t *testing.T) {
	dir := t.TempDir()
	s := NewSessionStoreAt(filepath.Join(dir, "sessions"))

	fresh, err := s.Create(policy.WriteApproval)
	if err != nil {
		t.Fatalf("Create fresh: %v", err)
	}
	if err := os.WriteFile(filepath.Join(s.WorkspaceDir(fresh.ID), "keep.txt"), []byte("ok"), 0o600); err != nil {
		t.Fatalf("write workspace file: %v", err)
	}

	oldID := "old-session-id"
	old := &Session{
		ID:         oldID,
		CreatedAt:  time.Now().UTC().Add(-40 * 24 * time.Hour),
		UpdatedAt:  time.Now().UTC().Add(-40 * 24 * time.Hour),
		PolicyMode: policy.WriteApproval,
		Messages:   []SessionMessage{},
	}
	writeSessionFile(t, s, old)
	if err := os.WriteFile(filepath.Join(s.WorkspaceDir(oldID), "gone.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write old workspace file: %v", err)
	}

	cutoff := time.Now().UTC().Add(-30 * 24 * time.Hour)
	purged, err := s.PurgeBefore(cutoff)
	if err != nil {
		t.Fatalf("PurgeBefore: %v", err)
	}
	if len(purged) != 1 || purged[0] != oldID {
		t.Fatalf("purged = %v, want [%s]", purged, oldID)
	}
	if _, err := s.Load(oldID); err == nil {
		t.Fatal("expected old session removed")
	}
	if _, err := os.Stat(s.WorkspaceDir(oldID)); !os.IsNotExist(err) {
		t.Fatalf("expected old workspace removed: %v", err)
	}
	if _, err := s.Load(fresh.ID); err != nil {
		t.Fatalf("fresh session removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.WorkspaceDir(fresh.ID), "keep.txt")); err != nil {
		t.Fatalf("fresh workspace file missing: %v", err)
	}
}
