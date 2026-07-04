package exec

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// SSHConfig configures remote command execution via the system ssh binary.
type SSHConfig struct {
	Host           string
	User           string
	Port           int
	Password       string
	Timeout        time.Duration
	MaxOutputBytes int
	ExtraArgs      []string
}

// SSH runs shell commands on a remote host using `ssh` (sshpass when available, or built-in password auth).
type SSH struct {
	Host           string
	User           string
	Port           int
	Password       string
	Timeout        time.Duration
	MaxOutputBytes int
	ExtraArgs      []string
}

// NewSSH returns an SSH executor with defaults applied.
func NewSSH(cfg SSHConfig) *SSH {
	s := &SSH{
		Host:           cfg.Host,
		User:           cfg.User,
		Port:           cfg.Port,
		Password:       cfg.Password,
		Timeout:        cfg.Timeout,
		MaxOutputBytes: cfg.MaxOutputBytes,
		ExtraArgs:      cfg.ExtraArgs,
	}
	if s.Timeout <= 0 {
		s.Timeout = DefaultTimeout
	}
	if s.MaxOutputBytes <= 0 {
		s.MaxOutputBytes = DefaultMaxOutputBytes
	}
	return s
}

// Target returns user@host or host when user is empty.
func (s *SSH) Target() string {
	host := s.Host
	if s.Port > 0 && s.Port != 22 {
		host = netJoinHostPort(host, s.Port)
	}
	if s.User != "" {
		return s.User + "@" + host
	}
	return host
}

func netJoinHostPort(host string, port int) string {
	if strings.Contains(host, ":") {
		return host
	}
	return host + ":" + strconv.Itoa(port)
}

func (s *SSH) sshArgs(remoteCommand string) []string {
	args := append([]string{}, s.ExtraArgs...)
	if s.Port > 0 && s.Port != 22 {
		args = append(args, "-p", strconv.Itoa(s.Port))
	}
	args = append(args,
		"-o", "StrictHostKeyChecking=no",
		"-o", "BatchMode=no",
	)
	args = append(args, s.Target(), remoteCommand)
	return args
}

// Run executes command on the remote host via ssh.
func (s *SSH) Run(ctx context.Context, command string) (*Result, error) {
	if s.Host == "" {
		return nil, fmt.Errorf("ssh host is required")
	}
	if command == "" {
		return nil, fmt.Errorf("command is required")
	}

	timeout := s.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := s.sshArgs(command)
	start := time.Now()

	if s.Password != "" {
		if _, err := exec.LookPath("sshpass"); err != nil {
			return s.runNativePassword(runCtx, command)
		}
		sshArgs := append([]string{"-e", "ssh"}, args...)
		cmd := exec.CommandContext(runCtx, "sshpass", sshArgs...)
		cmd.Env = append(os.Environ(), "SSHPASS="+s.Password)
		return s.runSSHCmd(runCtx, cmd, start, s.MaxOutputBytes)
	}

	cmd := exec.CommandContext(runCtx, "ssh", args...)

	return s.runSSHCmd(runCtx, cmd, start, s.MaxOutputBytes)
}

func (s *SSH) runSSHCmd(runCtx context.Context, cmd *exec.Cmd, start time.Time, maxOutputBytes int) (*Result, error) {
	var stdoutBuf, stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	durationMS := time.Since(start).Milliseconds()

	result := &Result{
		DurationMS: durationMS,
		TimedOut:   runCtx.Err() == context.DeadlineExceeded,
	}

	maxBytes := maxOutputBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxOutputBytes
	}
	stdout, stdoutTrunc := truncateBytes([]byte(stdoutBuf.String()), maxBytes)
	stderr, stderrTrunc := truncateBytes([]byte(stderrBuf.String()), maxBytes)
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

	return result, fmt.Errorf("ssh run: %w", err)
}
