package server

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrintStartupBanner(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	var buf bytes.Buffer
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	PrintStartupBanner(":8787")

	w.Close()
	os.Stdout = old
	_, _ = io.Copy(&buf, r)

	out := buf.String()
	if !strings.Contains(out, "启动成功") {
		t.Fatalf("expected startup message, got: %q", out)
	}
	if !strings.Contains(out, "用户名: Admin") {
		t.Fatalf("expected username in output, got: %q", out)
	}
	if !strings.Contains(out, "密码:   Databuff@123") {
		t.Fatalf("expected password in output, got: %q", out)
	}
}

func TestPrintStartupBannerCustomCredentials(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgPath := filepath.Join(home, ".databuff-diag", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(`auth:
  username: ops
  password: secret
`), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	PrintStartupBanner(":8787")

	w.Close()
	os.Stdout = old
	_, _ = io.Copy(&buf, r)

	out := buf.String()
	if !strings.Contains(out, "用户名: ops") {
		t.Fatalf("expected custom username, got: %q", out)
	}
	if !strings.Contains(out, "密码:   secret") {
		t.Fatalf("expected custom password, got: %q", out)
	}
}
