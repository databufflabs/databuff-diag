package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/databufflabs/databuff-diag/internal/exec"
	"github.com/databufflabs/databuff-diag/internal/policy"
	"github.com/databufflabs/databuff-diag/internal/store"
)

func testHandlerWithSessions(t *testing.T) (http.Handler, *store.SessionStore) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgStore := store.NewConfigStoreAt(filepath.Join(home, ".databuff-diag", "config.yaml"))
	sessionStore := store.NewSessionStoreAt(filepath.Join(home, ".databuff-diag", "sessions"))
	attachmentStore := store.NewAttachmentStoreAt(filepath.Join(home, ".databuff-diag", "uploads"))
	base := NewWithStores(cfgStore, sessionStore, attachmentStore)
	cookie := testLoginCookie(t, base)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") == "" {
			r.AddCookie(cookie)
		}
		base.ServeHTTP(w, r)
	})
	return handler, sessionStore
}

func TestReportExport_GET(t *testing.T) {
	handler, sessions := testHandlerWithSessions(t)
	session, err := sessions.Create(policy.WriteApproval)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	exit := 0
	_ = sessions.AppendMessage(session, store.SessionMessage{
		Role:      "assistant",
		Command:   "uname -a",
		Stdout:    "Linux test",
		ExitCode:  &exit,
		Risk:      "readonly",
		Timestamp: time.Now().UTC(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/report/export?session_id="+session.ID, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/markdown") {
		t.Fatalf("content-type = %q, want text/markdown", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Diagnostic Report") {
		t.Fatalf("body missing title: %s", body)
	}
	if !strings.Contains(body, "uname -a") {
		t.Fatalf("body missing command: %s", body)
	}
}

func TestReportExport_POST(t *testing.T) {
	handler, sessions := testHandlerWithSessions(t)
	session, _ := sessions.Create(policy.WriteApproval)

	payload, _ := json.Marshal(map[string]string{"session_id": session.ID})
	req := httptest.NewRequest(http.MethodPost, "/api/report/export", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestReportExport_MissingSession(t *testing.T) {
	handler, _ := testHandlerWithSessions(t)
	req := httptest.NewRequest(http.MethodGet, "/api/report/export?session_id=missing", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestEnvBundle_CollectAndDownload(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	reportsDir := filepath.Join(home, ".databuff-diag", "reports")
	cfgStore := store.NewConfigStoreAt(filepath.Join(home, ".databuff-diag", "config.yaml"))
	sessionStore := store.NewSessionStoreAt(filepath.Join(home, ".databuff-diag", "sessions"))

	attachmentStore := store.NewAttachmentStoreAt(filepath.Join(home, ".databuff-diag", "uploads"))
	base := NewWithStores(cfgStore, sessionStore, attachmentStore)
	cookie := testLoginCookie(t, base)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") == "" {
			r.AddCookie(cookie)
		}
		base.ServeHTTP(w, r)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/collect/env-bundle", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("collect status = %d, body=%s", rec.Code, rec.Body.String())
	}

	var bundle exec.EnvBundleResult
	if err := json.NewDecoder(rec.Body).Decode(&bundle); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if bundle.Filename == "" {
		t.Fatal("missing filename")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/collect/env-bundle/"+bundle.Filename, nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("download status = %d, body=%s", rec.Code, rec.Body.String())
	}

	gr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	if _, err := tr.Next(); err != nil {
		t.Fatalf("tar entry: %v", err)
	}

	if _, err := os.Stat(filepath.Join(reportsDir, bundle.Filename)); err != nil {
		t.Fatalf("bundle on disk: %v", err)
	}
}

func TestExecSSH_MissingHost(t *testing.T) {
	handler := testHandler(t)
	payload, _ := json.Marshal(map[string]string{"command": "echo hi"})
	req := httptest.NewRequest(http.MethodPost, "/api/exec/ssh", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestExecSSH_BlockedCommand(t *testing.T) {
	handler := testHandler(t)
	payload, _ := json.Marshal(map[string]string{
		"host":    "example.com",
		"command": "rm -rf /",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/exec/ssh", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestExecSSH_Validation(t *testing.T) {
	handler := testHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/exec/ssh", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestReportExport_NoSessionID(t *testing.T) {
	handler, _ := testHandlerWithSessions(t)
	req := httptest.NewRequest(http.MethodGet, "/api/report/export", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
