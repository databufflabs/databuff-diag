package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/databufflabs/databuff-diag/internal/policy"
	"github.com/databufflabs/databuff-diag/internal/store"
)

func TestNextDailyRun(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	tests := []struct {
		name string
		now  time.Time
		hour int
		want time.Time
	}{
		{
			name: "before hour today",
			now:  time.Date(2026, 7, 4, 0, 30, 0, 0, loc),
			hour: 1,
			want: time.Date(2026, 7, 4, 1, 0, 0, 0, loc),
		},
		{
			name: "after hour today",
			now:  time.Date(2026, 7, 4, 2, 0, 0, 0, loc),
			hour: 1,
			want: time.Date(2026, 7, 5, 1, 0, 0, 0, loc),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := nextDailyRun(tc.now, tc.hour, loc)
			if !got.Equal(tc.want) {
				t.Fatalf("nextDailyRun = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSessionCleanup_Run(t *testing.T) {
	dir := t.TempDir()
	cfgStore := store.NewConfigStoreAt(filepath.Join(dir, "config.yaml"))
	sessionStore := store.NewSessionStoreAt(filepath.Join(dir, "sessions"))

	retention := 30
	if err := cfgStore.Save(&store.Config{
		Sessions: store.SessionsConfig{RetentionDays: &retention},
	}); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	old := &store.Session{
		ID:         "expired-session",
		CreatedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		PolicyMode: policy.WriteApproval,
	}
	if err := writeSessionFile(sessionStore, old); err != nil {
		t.Fatalf("write old session: %v", err)
	}

	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	cleanup := &SessionCleanup{
		Sessions: sessionStore,
		Config:   cfgStore,
		Now:      func() time.Time { return now },
	}

	count, err := cleanup.Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	if _, err := sessionStore.Load("expired-session"); err == nil {
		t.Fatal("expected expired session removed")
	}
}

func TestSessionCleanup_DisabledWhenRetentionZero(t *testing.T) {
	dir := t.TempDir()
	cfgStore := store.NewConfigStoreAt(filepath.Join(dir, "config.yaml"))
	sessionStore := store.NewSessionStoreAt(filepath.Join(dir, "sessions"))

	zero := 0
	if err := cfgStore.Save(&store.Config{
		Sessions: store.SessionsConfig{RetentionDays: &zero},
	}); err != nil {
		t.Fatalf("Save config: %v", err)
	}

	old := &store.Session{
		ID:         "keep-me",
		CreatedAt:  time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		PolicyMode: policy.WriteApproval,
	}
	if err := writeSessionFile(sessionStore, old); err != nil {
		t.Fatalf("write session: %v", err)
	}

	cleanup := &SessionCleanup{Sessions: sessionStore, Config: cfgStore}
	count, err := cleanup.Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
	if _, err := sessionStore.Load("keep-me"); err != nil {
		t.Fatalf("session removed while cleanup disabled: %v", err)
	}
}

func writeSessionFile(s *store.SessionStore, session *store.Session) error {
	if err := os.MkdirAll(s.Dir(), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.Dir(), session.ID+".json"), data, 0o600)
}
