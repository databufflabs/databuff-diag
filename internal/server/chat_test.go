package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/databufflabs/databuff-diag/internal/store"
)

func TestChat_SSEStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hel\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgStore := store.NewConfigStoreAt(filepath.Join(home, ".databuff-diag", "config.yaml"))
	sessionStore := store.NewSessionStoreAt(filepath.Join(home, ".databuff-diag", "sessions"))

	cfg, _ := cfgStore.Load()
	cfg.LLM.Providers["deepseek"] = store.ProviderInstance{
		Enabled: true,
		BaseURL: srv.URL + "/v1",
		Model:   "test-model",
	}
	cfg.LLM.Active = "deepseek"
	_ = cfgStore.Save(cfg)

	handler := &ChatHandler{
		ConfigStore:  cfgStore,
		SessionStore: sessionStore,
	}

	payload, _ := json.Marshal(map[string]string{"message": "hi"})
	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: chunk") {
		t.Fatalf("missing chunk events: %s", body)
	}
	if !strings.Contains(body, "hel") || !strings.Contains(body, "lo") {
		t.Fatalf("missing streamed content: %s", body)
	}
	if !strings.Contains(body, "event: done") {
		t.Fatalf("missing done event: %s", body)
	}
}

func TestChat_MessageTooLong(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgStore := store.NewConfigStoreAt(filepath.Join(home, ".databuff-diag", "config.yaml"))
	sessionStore := store.NewSessionStoreAt(filepath.Join(home, ".databuff-diag", "sessions"))

	cfg, _ := cfgStore.Load()
	cfg.LLM.Providers["deepseek"] = store.ProviderInstance{
		Enabled: true,
		BaseURL: "http://example.invalid/v1",
		Model:   "test-model",
	}
	cfg.LLM.Active = "deepseek"
	_ = cfgStore.Save(cfg)

	handler := &ChatHandler{
		ConfigStore:  cfgStore,
		SessionStore: sessionStore,
	}

	payload, _ := json.Marshal(map[string]string{
		"message": strings.Repeat("x", store.MaxChatMessageRunes+1),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "100000") {
		t.Fatalf("expected length error, body=%s", rec.Body.String())
	}
}

func TestChat_NonStreamingFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"plain"}}]}`))
	}))
	defer srv.Close()

	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgStore := store.NewConfigStoreAt(filepath.Join(home, ".databuff-diag", "config.yaml"))
	sessionStore := store.NewSessionStoreAt(filepath.Join(home, ".databuff-diag", "sessions"))

	cfg, _ := cfgStore.Load()
	cfg.LLM.Providers["deepseek"] = store.ProviderInstance{
		Enabled: true,
		BaseURL: srv.URL + "/v1",
		Model:   "test-model",
	}
	cfg.LLM.Active = "deepseek"
	_ = cfgStore.Save(cfg)

	handler := &ChatHandler{ConfigStore: cfgStore, SessionStore: sessionStore}
	payload, _ := json.Marshal(map[string]string{"message": "hi"})
	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "plain") {
		t.Fatalf("expected plain content in stream: %s", body)
	}
}
