package exec

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

const (
	// DefaultTimeout is the maximum time a local command may run.
	DefaultTimeout = 30 * time.Second
	// DefaultMaxOutputBytes is the per-stream output cap before truncation.
	DefaultMaxOutputBytes = 64 * 1024
)

// LocalConfig configures a Local executor.
type LocalConfig struct {
	Timeout        time.Duration
	MaxOutputBytes int
}

// Local runs shell commands on the host machine.
type Local struct {
	Timeout        time.Duration
	MaxOutputBytes int
}

// NewLocal returns a Local executor with defaults applied.
func NewLocal(cfg LocalConfig) *Local {
	l := &Local{
		Timeout:        cfg.Timeout,
		MaxOutputBytes: cfg.MaxOutputBytes,
	}
	if l.Timeout <= 0 {
		l.Timeout = DefaultTimeout
	}
	if l.MaxOutputBytes <= 0 {
		l.MaxOutputBytes = DefaultMaxOutputBytes
	}
	return l
}

// Result holds captured command output and execution metadata.
type Result struct {
	Stdout          string
	Stderr          string
	ExitCode        int
	DurationMS      int64
	StdoutTruncated bool
	StderrTruncated bool
	TimedOut        bool
}

// Run executes command via sh -c with timeout and output truncation.
func (l *Local) Run(ctx context.Context, command string) (*Result, error) {
	if command == "" {
		return nil, fmt.Errorf("command is required")
	}

	timeout := l.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	maxBytes := l.MaxOutputBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxOutputBytes
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(runCtx, "sh", "-c", command)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	durationMS := time.Since(start).Milliseconds()

	result := &Result{
		DurationMS: durationMS,
		TimedOut:   runCtx.Err() == context.DeadlineExceeded,
	}

	stdout, stdoutTrunc := truncateBytes(stdoutBuf.Bytes(), maxBytes)
	stderr, stderrTrunc := truncateBytes(stderrBuf.Bytes(), maxBytes)
	result.Stdout = stdout
	result.Stderr = stderr
	result.StdoutTruncated = stdoutTrunc
	result.StderrTruncated = stderrTrunc

	if err == nil {
		result.ExitCode = 0
		return result, nil
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}

	if result.TimedOut {
		result.ExitCode = -1
		return result, nil
	}

	return result, fmt.Errorf("run command: %w", err)
}

func truncateBytes(b []byte, max int) (string, bool) {
	if len(b) <= max {
		return string(b), false
	}
	return string(b[:max]), true
}
