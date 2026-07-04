package exec

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"
)

func (s *SSH) runNativePassword(ctx context.Context, command string) (*Result, error) {
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	port := s.Port
	if port <= 0 {
		port = 22
	}
	addr := net.JoinHostPort(s.Host, strconv.Itoa(port))

	user := s.User
	if user == "" {
		user = "root"
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(s.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}

	start := time.Now()
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(runCtx, "tcp", addr)
	if err != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			return &Result{TimedOut: true, ExitCode: -1, DurationMS: time.Since(start).Milliseconds()}, nil
		}
		return nil, fmt.Errorf("ssh dial: %w", err)
	}
	defer conn.Close()

	clientConn, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			return &Result{TimedOut: true, ExitCode: -1, DurationMS: time.Since(start).Milliseconds()}, nil
		}
		return nil, fmt.Errorf("ssh handshake: %w", err)
	}
	client := ssh.NewClient(clientConn, chans, reqs)
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf

	runErr := session.Run(command)
	durationMS := time.Since(start).Milliseconds()

	result := &Result{
		DurationMS: durationMS,
		TimedOut:   runCtx.Err() == context.DeadlineExceeded,
	}

	maxBytes := s.MaxOutputBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxOutputBytes
	}
	stdout, stdoutTrunc := truncateBytes(stdoutBuf.Bytes(), maxBytes)
	stderr, stderrTrunc := truncateBytes(stderrBuf.Bytes(), maxBytes)
	result.Stdout = stdout
	result.Stderr = stderr
	result.StdoutTruncated = stdoutTrunc
	result.StderrTruncated = stderrTrunc

	if runErr == nil {
		result.ExitCode = 0
		return result, nil
	}

	if exitErr, ok := runErr.(*ssh.ExitError); ok {
		result.ExitCode = exitErr.ExitStatus()
		return result, nil
	}

	if result.TimedOut {
		result.ExitCode = -1
		return result, nil
	}

	return nil, fmt.Errorf("ssh run: %w", runErr)
}
