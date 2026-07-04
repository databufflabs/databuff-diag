package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/databufflabs/databuff-diag/internal/agent"
	"github.com/databufflabs/databuff-diag/internal/exec"
	"github.com/databufflabs/databuff-diag/internal/llm"
	"github.com/databufflabs/databuff-diag/internal/policy"
	"github.com/databufflabs/databuff-diag/internal/store"
	"github.com/go-chi/chi/v5"
)

type seqLLM struct {
	responses []string
	n         int
}

func (m *seqLLM) Chat(_ context.Context, _ llm.MergedProvider, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	idx := m.n
	m.n++
	if idx >= len(m.responses) {
		return &llm.ChatResponse{Content: "ok"}, nil
	}
	return &llm.ChatResponse{Content: m.responses[idx]}, nil
}

func TestSessions_CreateAndGet(t *testing.T) {
	handler := testHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/sessions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}

	var created store.Session
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.ID == "" {
		t.Fatal("missing session id")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/sessions/"+created.ID, nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestSessions_PatchPolicyMode(t *testing.T) {
	handler := testHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/sessions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s", rec.Code, rec.Body.String())
	}

	var created store.Session
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	payload, _ := json.Marshal(map[string]string{"policy_mode": "open"})
	req = httptest.NewRequest(http.MethodPatch, "/api/sessions/"+created.ID, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body=%s", rec.Code, rec.Body.String())
	}

	var updated store.Session
	if err := json.NewDecoder(rec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode patch: %v", err)
	}
	if updated.PolicyMode != policy.Open {
		t.Fatalf("policy_mode = %q, want open", updated.PolicyMode)
	}
	if len(updated.Messages) != 0 {
		t.Fatalf("policy change should not append messages, got %+v", updated.Messages)
	}
	if updated.ID != created.ID {
		t.Fatalf("session id changed: %q -> %q", created.ID, updated.ID)
	}
}

func TestSessions_Delete(t *testing.T) {
	handler := testHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/sessions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s", rec.Code, rec.Body.String())
	}

	var created store.Session
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/sessions/"+created.ID, nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/sessions/"+created.ID, nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete status = %d, want 404", rec.Code)
	}
}

func TestSessions_MessageAutoExec(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgStore := store.NewConfigStoreAt(filepath.Join(home, ".databuff-diag", "config.yaml"))
	sessionStore := store.NewSessionStoreAt(filepath.Join(home, ".databuff-diag", "sessions"))

	cfg, _ := cfgStore.Load()
	cfg.LLM.Providers["deepseek"] = store.ProviderInstance{Enabled: true, BaseURL: "http://x", Model: "m"}
	cfg.LLM.Active = "deepseek"
	_ = cfgStore.Save(cfg)

	mock := &seqLLM{responses: []string{
		`{"tool":"shell","command":"ls"}`,
		"done",
	}}

	sessions := &SessionsHandler{
		ConfigStore:  cfgStore,
		SessionStore: sessionStore,
		Agent: &agent.Agent{
			LLM:      mock,
			Policy:   &policy.Engine{},
			Executor: exec.NewLocal(exec.LocalConfig{}),
			Sessions: sessionStore,
		},
	}

	created, _ := sessionStore.Create(policy.WriteApproval)
	payload, _ := json.Marshal(map[string]string{"content": "list files"})
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+created.ID+"/message", bytes.NewReader(payload))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", created.ID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	sessions.Message(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}

	var resp store.Session
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	found := false
	for _, msg := range resp.Messages {
		if msg.Role == "tool" && msg.Command == "ls" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected executed command output in session: %+v", resp.Messages)
	}
}

func TestSessions_ReloadAfterRestart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".databuff-diag")
	sessionStore := store.NewSessionStoreAt(filepath.Join(dir, "sessions"))

	created, err := sessionStore.Create(policy.WriteApproval)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	restartedStore := store.NewSessionStoreAt(filepath.Join(dir, "sessions"))
	loaded, err := restartedStore.Load(created.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if loaded.ID != created.ID {
		t.Fatalf("id mismatch after restart")
	}
}
