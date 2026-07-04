package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsShellFile(t *testing.T) {
	cases := []struct {
		path string
		body string
		want bool
	}{
		{"deploy.sh", "#!/bin/sh\necho hi", true},
		{"script.bash", "echo hi", true},
		{"run.zsh", "echo hi", true},
		{"Makefile", "all:\n\techo hi", false},
		{"tool", "#!/usr/bin/env python3\nprint(1)", false},
		{"entry", "#!/usr/bin/env bash\necho hi", true},
	}
	for _, tc := range cases {
		if got := IsShellFile(tc.path, tc.body); got != tc.want {
			t.Errorf("IsShellFile(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestLint_validScript(t *testing.T) {
	content := "#!/bin/bash\nif true; then\n  echo ok\nfi\n"
	if diags := Lint(content, "check.sh"); len(diags) != 0 {
		t.Fatalf("Lint valid script: got %+v", diags)
	}
}

func TestLint_missingFi(t *testing.T) {
	content := "#!/bin/bash\nif true; then\n  echo ok\n"
	diags := Lint(content, "bad.sh")
	if len(diags) != 1 {
		t.Fatalf("Lint missing fi: got %d diagnostics: %+v", len(diags), diags)
	}
	if diags[0].Line < 1 || diags[0].Column < 1 {
		t.Fatalf("unexpected position: %+v", diags[0])
	}
	if !strings.Contains(diags[0].Message, "fi") {
		t.Fatalf("expected fi-related message, got %q", diags[0].Message)
	}
}

func TestLint_nonShellSkipped(t *testing.T) {
	if diags := Lint("not{valid json", "data.json"); diags != nil {
		t.Fatalf("expected nil for non-shell file, got %+v", diags)
	}
}

func TestLint_doesNotExecute(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "lint-must-not-run")
	content := "#!/bin/sh\ntouch " + marker + "\n"
	Lint(content, "preview.sh")
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("Lint must not execute shell script side effects")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat marker: %v", err)
	}
}
