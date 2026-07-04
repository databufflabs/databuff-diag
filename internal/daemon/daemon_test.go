package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckNotRunning_missingPIDFile(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "databuff-diag.pid")
	if err := checkNotRunning(pidFile); err != nil {
		t.Fatalf("checkNotRunning() = %v, want nil", err)
	}
}

func TestCheckNotRunning_stalePIDFile(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "databuff-diag.pid")
	if err := os.WriteFile(pidFile, []byte("999999999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkNotRunning(pidFile); err != nil {
		t.Fatalf("checkNotRunning() = %v, want nil for stale pid", err)
	}
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Fatalf("stale pid file should be removed, stat err = %v", err)
	}
}

func TestDefaultPaths(t *testing.T) {
	pid, err := DefaultPIDFile()
	if err != nil {
		t.Fatal(err)
	}
	log, err := DefaultLogFile()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(pid) != "databuff-diag.pid" {
		t.Fatalf("DefaultPIDFile() = %q", pid)
	}
	if filepath.Base(log) != "databuff-diag.log" {
		t.Fatalf("DefaultLogFile() = %q", log)
	}
}
