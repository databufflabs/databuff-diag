package report

import (
	"strings"
	"testing"
	"time"

	"github.com/databufflabs/databuff-diag/internal/policy"
	"github.com/databufflabs/databuff-diag/internal/store"
)

func TestRenderMarkdown_Basic(t *testing.T) {
	exit := 0
	session := &store.Session{
		ID:         "abc123",
		CreatedAt:  time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 7, 4, 10, 5, 0, 0, time.UTC),
		PolicyMode: policy.WriteApproval,
		Messages: []store.SessionMessage{
			{Role: "user", Content: "check docker", Timestamp: time.Date(2026, 7, 4, 10, 1, 0, 0, time.UTC)},
			{
				Role:      "assistant",
				Command:   "docker ps",
				Stdout:    "CONTAINER ID\n",
				Stderr:    "",
				ExitCode:  &exit,
				Risk:      "readonly",
				Timestamp: time.Date(2026, 7, 4, 10, 2, 0, 0, time.UTC),
			},
		},
	}

	md := RenderMarkdown(session)
	for _, want := range []string{
		"# Diagnostic Report",
		"abc123",
		"write_approval",
		"check docker",
		"docker ps",
		"CONTAINER ID",
		"## Command Audit",
		"readonly",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
}

func TestRenderMarkdown_EmptySession(t *testing.T) {
	md := RenderMarkdown(&store.Session{ID: "x"})
	if !strings.Contains(md, "No messages") {
		t.Fatalf("markdown = %q", md)
	}
}

func TestRenderMarkdown_Nil(t *testing.T) {
	if RenderMarkdown(nil) != "" {
		t.Fatal("expected empty string for nil session")
	}
}
