package daemon

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/databufflabs/databuff-diag/internal/server"
)

const childEnv = "DATABUFF_DIAG_DAEMON"

// Options configures daemon startup.
type Options struct {
	Listen  string
	PIDFile string
	LogFile string
}

// IsChild reports whether the current process is the daemon child.
func IsChild() bool {
	return os.Getenv(childEnv) == "1"
}

// Fork starts a detached child process and waits until it is healthy.
func Fork(opts Options) error {
	if err := checkNotRunning(opts.PIDFile); err != nil {
		return err
	}
	if err := ensureParentDir(opts.PIDFile); err != nil {
		return err
	}
	if err := ensureParentDir(opts.LogFile); err != nil {
		return err
	}
	if err := startDetached(opts); err != nil {
		return err
	}

	url := server.ServeURL(opts.Listen)
	if err := waitHealthy(url, 15*time.Second); err != nil {
		return fmt.Errorf("后台启动失败: %w（查看日志 %s）", err, opts.LogFile)
	}

	pid, _ := readPID(opts.PIDFile)
	fmt.Printf("✓ databuff-diag 启动成功，访问 %s\n", url)
	fmt.Printf("  日志: %s\n", opts.LogFile)
	if pid > 0 {
		fmt.Printf("  PID:  %d\n", pid)
	}
	return nil
}

// SetupChild redirects output to the log file and writes the PID file.
func SetupChild(pidFile, logFile string) error {
	if err := ensureParentDir(pidFile); err != nil {
		return err
	}
	if err := ensureParentDir(logFile); err != nil {
		return err
	}

	log, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	os.Stdout = log
	os.Stderr = log

	pid := os.Getpid()
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", pid)), 0o644); err != nil {
		return fmt.Errorf("write pid file: %w", err)
	}
	return nil
}

// Cleanup removes the PID file on shutdown.
func Cleanup(pidFile string) {
	_ = os.Remove(pidFile)
}

func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0o700)
}

func checkNotRunning(pidFile string) error {
	pid, err := readPID(pidFile)
	if err != nil || pid <= 0 {
		_ = os.Remove(pidFile)
		return nil
	}
	if processAlive(pid) {
		return fmt.Errorf("databuff-diag 已在运行 (PID %d)", pid)
	}
	_ = os.Remove(pidFile)
	return nil
}

func readPID(pidFile string) (int, error) {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, err
	}
	return pid, nil
}

func waitHealthy(baseURL string, timeout time.Duration) error {
	healthURL := strings.TrimRight(baseURL, "/") + "/health"
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}

	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(healthURL)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			lastErr = fmt.Errorf("health status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(200 * time.Millisecond)
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("timeout after %s", timeout)
}
