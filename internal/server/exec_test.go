package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExecLocal_Echo(t *testing.T) {
	handler := testHandler(t)
	payload := map[string]string{"command": "echo hello"}
	raw, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/exec/local", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp execLocalResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ExitCode != 0 {
		t.Fatalf("exit_code = %d, want 0", resp.ExitCode)
	}
	if strings.TrimSpace(resp.Stdout) != "hello" {
		t.Fatalf("stdout = %q, want hello", resp.Stdout)
	}
	if resp.Command != "echo hello" {
		t.Fatalf("command = %q, want echo hello", resp.Command)
	}
	if resp.Risk == "" {
		t.Fatal("risk audit field is empty")
	}
	if resp.DurationMS < 0 {
		t.Fatalf("duration_ms = %d, want >= 0", resp.DurationMS)
	}
}

func TestExecLocal_False(t *testing.T) {
	handler := testHandler(t)
	payload := map[string]string{"command": "false"}
	raw, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/exec/local", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}

	var resp execLocalResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ExitCode == 0 {
		t.Fatalf("exit_code = %d, want non-zero", resp.ExitCode)
	}
}

func TestExecLocal_LongOutput(t *testing.T) {
	handler := testHandler(t)
	payload := map[string]string{"command": "python3 -c \"print('y' * 70000)\""}
	raw, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/exec/local", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}

	var resp execLocalResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.StdoutTruncated {
		t.Fatal("stdout_truncated = false, want true")
	}
	if len(resp.Stdout) != 64*1024 {
		t.Fatalf("stdout len = %d, want %d", len(resp.Stdout), 64*1024)
	}
}

func TestExecLocal_DockerPS(t *testing.T) {
	handler := testHandler(t)
	payload := map[string]string{"command": "docker ps"}
	raw, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/exec/local", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}

	var resp execLocalResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Risk != "readonly" {
		t.Fatalf("risk = %q, want readonly", resp.Risk)
	}
	// docker may be missing; either success or "command not found" in stderr.
	if resp.ExitCode == 0 {
		if !strings.Contains(resp.Stdout, "CONTAINER") && !strings.Contains(resp.Stdout, "NAMES") {
			t.Logf("docker ps succeeded with unexpected stdout: %q", resp.Stdout)
		}
	}
}

func TestExecLocal_BlockedCommand(t *testing.T) {
	handler := testHandler(t)
	payload := map[string]string{"command": "rm -rf /"}
	raw, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/exec/local", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestExecLocal_EmptyCommand(t *testing.T) {
	handler := testHandler(t)
	payload := map[string]string{"command": ""}
	raw, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/exec/local", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestExecLocal_MethodNotAllowed(t *testing.T) {
	handler := testHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/exec/local", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestExecLocal_AuditFields(t *testing.T) {
	handler := testHandler(t)
	payload := map[string]string{"command": "echo audit"}
	raw, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/exec/local", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var resp execLocalResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	for _, field := range []struct {
		name string
		ok   bool
	}{
		{"command", resp.Command != ""},
		{"stdout", resp.Stdout != ""},
		{"exit_code", true},
		{"duration_ms", resp.DurationMS >= 0},
		{"risk", resp.Risk != ""},
	} {
		if !field.ok {
			t.Fatalf("missing audit field %q in %+v", field.name, resp)
		}
	}
}
