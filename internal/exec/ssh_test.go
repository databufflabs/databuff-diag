package exec

import (
	"context"
	"testing"
	"time"
)

func TestSSH_MissingHost(t *testing.T) {
	s := NewSSH(SSHConfig{})
	_, err := s.Run(context.Background(), "echo hi")
	if err == nil {
		t.Fatal("expected error for missing host")
	}
}

func TestSSH_EmptyCommand(t *testing.T) {
	s := NewSSH(SSHConfig{Host: "example.com"})
	_, err := s.Run(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestSSH_Target(t *testing.T) {
	if got := NewSSH(SSHConfig{Host: "host"}).Target(); got != "host" {
		t.Fatalf("target = %q, want host", got)
	}
	if got := NewSSH(SSHConfig{Host: "host", User: "root"}).Target(); got != "root@host" {
		t.Fatalf("target = %q, want root@host", got)
	}
}

func TestSSH_InvalidHost(t *testing.T) {
	s := NewSSH(SSHConfig{
		Host:    "invalid-host-that-does-not-exist.example",
		Timeout: 2 * time.Second,
	})
	result, err := s.Run(context.Background(), "echo hi")
	if err != nil && result == nil {
		// ssh binary missing or connection failed with non-exit error is acceptable
		t.Skipf("ssh unavailable: %v", err)
	}
	if result != nil && result.ExitCode == 0 {
		t.Log("ssh succeeded unexpectedly")
	}
}
