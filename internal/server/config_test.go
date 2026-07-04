package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/databufflabs/databuff-diag/internal/store"
)

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgStore := store.NewConfigStoreAt(filepath.Join(home, ".databuff-diag", "config.yaml"))
	base := NewWithStore(cfgStore)
	cookie := testLoginCookie(t, base)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") == "" {
			r.AddCookie(cookie)
		}
		base.ServeHTTP(w, r)
	})
}

func testLoginCookie(t *testing.T, handler http.Handler) *http.Cookie {
	t.Helper()
	body := `{"username":"Admin","password":"Databuff@123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("test login status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	for _, c := range cookies {
		if c.Name == sessionCookieName {
			return c
		}
	}
	t.Fatal("test login: session cookie not set")
	return nil
}

func TestGetConfigReturnsAPIKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgStore := store.NewConfigStoreAt(filepath.Join(home, ".databuff-diag", "config.yaml"))

	cfg, err := cfgStore.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.LLM.Providers["deepseek"] = store.ProviderInstance{
		Enabled: true,
		APIKey:  "sk-must-not-leak",
		Model:   "deepseek-chat",
	}
	if err := cfgStore.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	handler := NewWithStore(cfgStore)
	cookie := testLoginCookie(t, handler)
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "sk-must-not-leak") {
		t.Fatalf("response missing api_key: %s", body)
	}
}

func TestPutConfigPreservesAPIKeyWhenEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgStore := store.NewConfigStoreAt(filepath.Join(home, ".databuff-diag", "config.yaml"))

	cfg, err := cfgStore.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.LLM.Providers["deepseek"] = store.ProviderInstance{
		Enabled: true,
		APIKey:  "sk-persisted",
		Model:   "deepseek-chat",
	}
	if err := cfgStore.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	handler := NewWithStore(cfgStore)
	cookie := testLoginCookie(t, handler)
	payload := map[string]any{
		"llm": map[string]any{
			"active": "deepseek",
			"providers": map[string]any{
				"deepseek": map[string]any{
					"enabled": true,
					"api_key": "",
					"model":   "deepseek-chat",
				},
			},
		},
		"policy": map[string]any{"default": "write_approval"},
		"ssh":    map[string]any{"hosts": []any{}},
		"skills": map[string]any{"dirs": []string{}},
	}
	raw, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(raw))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	reloaded, err := cfgStore.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if reloaded.LLM.Providers["deepseek"].APIKey != "sk-persisted" {
		t.Fatalf("api_key = %q, want sk-persisted", reloaded.LLM.Providers["deepseek"].APIKey)
	}

	disk, err := os.ReadFile(cfgStore.Path())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(disk), "sk-persisted") {
		t.Fatal("api_key must not be stored in plaintext on disk")
	}
	if !strings.Contains(string(disk), "enc:v1:") {
		t.Fatal("expected encrypted api_key on disk after merge")
	}
}

func TestGetProvidersListsCatalog(t *testing.T) {
	handler := testHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/providers", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body struct {
		Providers []struct {
			ProviderCode string `json:"provider_code"`
		} `json:"providers"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Providers) < 10 {
		t.Fatalf("providers len = %d, want >= 10", len(body.Providers))
	}
}

func TestConfigAPIKeyInResponse(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgStore := store.NewConfigStoreAt(filepath.Join(home, ".databuff-diag", "config.yaml"))
	base := NewWithStore(cfgStore)
	cookie := testLoginCookie(t, base)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") == "" {
			r.AddCookie(cookie)
		}
		base.ServeHTTP(w, r)
	})

	secret := "sk-log-leak-test"
	payload := map[string]any{
		"llm": map[string]any{
			"active": "openai",
			"providers": map[string]any{
				"openai": map[string]any{
					"enabled": true,
					"api_key": secret,
					"model":   "gpt-4o",
				},
			},
		},
		"policy": map[string]any{"default": "write_approval"},
		"ssh":    map[string]any{"hosts": []any{}},
		"skills": map[string]any{"dirs": []string{}},
	}
	raw, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), secret) {
		t.Fatal("PUT response missing api_key")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), secret) {
		t.Fatal("GET response missing api_key")
	}
}

func TestPutConfigPreservesHostPasswordWhenEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgStore := store.NewConfigStoreAt(filepath.Join(home, ".databuff-diag", "config.yaml"))

	cfg, err := cfgStore.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	hostID := store.NewSSHHostID()
	cfg.SSH.Hosts = store.SSHHostsList{
		{ID: hostID, Host: "10.0.0.1", User: "root", Password: "secret-pass"},
	}
	if err := cfgStore.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	handler := NewWithStore(cfgStore)
	cookie := testLoginCookie(t, handler)
	payload := map[string]any{
		"llm":    map[string]any{"active": "deepseek", "providers": map[string]any{}},
		"policy": map[string]any{"default": "write_approval"},
		"ssh": map[string]any{
			"hosts": []any{
				map[string]any{
					"id":   hostID,
					"host": "10.0.0.1",
					"user": "root",
				},
			},
		},
		"skills": map[string]any{"dirs": []string{}},
	}
	raw, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(raw))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	reloaded, err := cfgStore.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(reloaded.SSH.Hosts) != 1 || reloaded.SSH.Hosts[0].Password != "secret-pass" {
		t.Fatalf("password = %q, want secret-pass", reloaded.SSH.Hosts[0].Password)
	}
}

func TestGetConfigMasksHostPassword(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgStore := store.NewConfigStoreAt(filepath.Join(home, ".databuff-diag", "config.yaml"))

	cfg, err := cfgStore.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.SSH.Hosts = store.SSHHostsList{
		{ID: store.NewSSHHostID(), Host: "10.0.0.2", User: "admin", Password: "ssh-secret"},
	}
	if err := cfgStore.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	handler := NewWithStore(cfgStore)
	cookie := testLoginCookie(t, handler)
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if strings.Contains(body, "ssh-secret") {
		t.Fatalf("response leaked password: %s", body)
	}
	if !strings.Contains(body, `"password_configured":true`) {
		t.Fatalf("response missing password_configured: %s", body)
	}
}
