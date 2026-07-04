//go:build windows

package exec

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestLocal_Echo(t *testing.T) {
	l := NewLocal(LocalConfig{})
	result, err := l.Run(context.Background(), "echo hello")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit_code = %d, want 0", result.ExitCode)
	}
	if strings.TrimSpace(result.Stdout) != "hello" {
		t.Fatalf("stdout = %q, want hello", result.Stdout)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
	if result.TimedOut {
		t.Fatal("timed_out = true, want false")
	}
	if result.StdoutTruncated || result.StderrTruncated {
		t.Fatal("unexpected truncation")
	}
	if result.DurationMS < 0 {
		t.Fatalf("duration_ms = %d, want >= 0", result.DurationMS)
	}
}

func TestLocal_False(t *testing.T) {
	l := NewLocal(LocalConfig{})
	result, err := l.Run(context.Background(), "exit 1")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ExitCode == 0 {
		t.Fatalf("exit_code = %d, want non-zero", result.ExitCode)
	}
}

func TestLocal_LongOutputTruncated(t *testing.T) {
	const maxBytes = 1024
	l := NewLocal(LocalConfig{MaxOutputBytes: maxBytes})
	result, err := l.Run(context.Background(), "powershell -NoProfile -Command \"'x' * 5000\"")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit_code = %d, want 0", result.ExitCode)
	}
	if !result.StdoutTruncated {
		t.Fatal("stdout_truncated = false, want true")
	}
	if len(result.Stdout) != maxBytes {
		t.Fatalf("stdout len = %d, want %d", len(result.Stdout), maxBytes)
	}
}

func TestLocal_Timeout(t *testing.T) {
	l := NewLocal(LocalConfig{Timeout: 100 * time.Millisecond})
	result, err := l.Run(context.Background(), "powershell -NoProfile -Command \"Start-Sleep -Seconds 5\"")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.TimedOut {
		t.Fatal("timed_out = false, want true")
	}
	if result.ExitCode != -1 {
		t.Fatalf("exit_code = %d, want -1", result.ExitCode)
	}
}

func TestLocal_EmptyCommand(t *testing.T) {
	l := NewLocal(LocalConfig{})
	_, err := l.Run(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}
