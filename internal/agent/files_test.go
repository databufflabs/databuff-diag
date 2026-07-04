package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadWriteEditWorkspace(t *testing.T) {
	dir := t.TempDir()
	path := "notes/report.md"

	if _, err := readWorkspaceFile(dir, path, 0, 0); err == nil {
		t.Fatal("expected read error for missing file")
	}

	content := "# report\nline2\n"
	if err := writeWorkspaceFile(dir, path, content); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := readWorkspaceFile(dir, path, 0, 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(got, "line2") {
		t.Fatalf("read output = %q", got)
	}

	msg, err := editWorkspaceFile(dir, path, []TextEdit{{OldText: "line2", NewText: "line-two"}})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if !strings.Contains(msg, "Successfully edited") {
		t.Fatalf("edit msg = %q", msg)
	}

	data, err := os.ReadFile(filepath.Join(dir, "notes", "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "line-two") {
		t.Fatalf("file after edit = %q", string(data))
	}
}

func TestResolveWorkspacePathBlocksEscape(t *testing.T) {
	dir := t.TempDir()
	if _, err := resolveWorkspacePath(dir, "../outside.txt"); err == nil {
		t.Fatal("expected path escape to fail")
	}
}

func TestResolveWorkspacePathAbsoluteInsideWorkspace(t *testing.T) {
	dir := t.TempDir()
	inside := filepath.Join(dir, "notes", "report.md")
	if err := os.MkdirAll(filepath.Dir(inside), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inside, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := resolveWorkspacePath(dir, inside)
	if err != nil {
		t.Fatalf("resolve inside workspace: %v", err)
	}
	if got != inside {
		t.Fatalf("resolve = %q, want %q", got, inside)
	}
}

func TestResolveWorkspacePathAbsoluteOutsideWorkspace(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := resolveWorkspacePath(dir, outside); err == nil {
		t.Fatal("expected outside workspace path to fail")
	}
}

func TestReadWorkspaceFileAbsoluteHostPath(t *testing.T) {
	dir := t.TempDir()
	hostFile := filepath.Join(t.TempDir(), "host-read.txt")
	if err := os.WriteFile(hostFile, []byte("host-content\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := readWorkspaceFile(dir, hostFile, 0, 0)
	if err != nil {
		t.Fatalf("read host file: %v", err)
	}
	if !strings.Contains(got, "host-content") {
		t.Fatalf("read output = %q", got)
	}
}
