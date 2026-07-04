package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/databufflabs/databuff-diag/internal/store"
)

func testHandlerRaw(t *testing.T) http.Handler {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgStore := store.NewConfigStoreAt(filepath.Join(home, ".databuff-diag", "config.yaml"))
	return NewWithStore(cfgStore)
}

func TestLoginWithDefaultCredentials(t *testing.T) {
	handler := testHandlerRaw(t)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"Admin","password":"Databuff@123"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != sessionCookieName {
		t.Fatalf("expected session cookie, got %#v", cookies)
	}
}

func TestLoginRejectsInvalidCredentials(t *testing.T) {
	handler := testHandlerRaw(t)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"Admin","password":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestMultipleConcurrentSessions(t *testing.T) {
	handler := testHandlerRaw(t)

	cookie1 := testLoginCookie(t, handler)
	cookie2 := testLoginCookie(t, handler)
	if cookie1.Value == cookie2.Value {
		t.Fatal("expected distinct session tokens for concurrent logins")
	}

	for _, cookie := range []*http.Cookie{cookie1, cookie2} {
		req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d for cookie %q", rec.Code, http.StatusOK, cookie.Value)
		}
	}
}

func TestProtectedAPIRequiresAuth(t *testing.T) {
	handler := testHandlerRaw(t)

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAuthMeReturnsUsername(t *testing.T) {
	handler := testHandlerRaw(t)
	cookie := testLoginCookie(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body authMeResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Username != "Admin" {
		t.Fatalf("username = %q, want Admin", body.Username)
	}
}

func TestLogoutInvalidatesSession(t *testing.T) {
	handler := testHandlerRaw(t)
	cookie := testLoginCookie(t, handler)

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	logoutReq.AddCookie(cookie)
	logoutRec := httptest.NewRecorder()
	handler.ServeHTTP(logoutRec, logoutReq)
	if logoutRec.Code != http.StatusOK {
		t.Fatalf("logout status = %d, want %d", logoutRec.Code, http.StatusOK)
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meReq.AddCookie(cookie)
	meRec := httptest.NewRecorder()
	handler.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusUnauthorized {
		t.Fatalf("status after logout = %d, want %d", meRec.Code, http.StatusUnauthorized)
	}
}

func TestCustomAuthFromConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgStore := store.NewConfigStoreAt(filepath.Join(home, ".databuff-diag", "config.yaml"))
	cfg, err := cfgStore.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.Auth.Username = "ops"
	cfg.Auth.Password = "secret-pass"
	if err := cfgStore.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := os.Chmod(cfgStore.Path(), 0o600); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	handler := NewWithStore(cfgStore)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"ops","password":"secret-pass"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestProtectedStaticRequiresAuth(t *testing.T) {
	handler := testHandlerRaw(t)

	req := httptest.NewRequest(http.MethodGet, "/preview.html", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestPublicStaticAllowedWithoutAuth(t *testing.T) {
	handler := testHandlerRaw(t)

	for _, path := range []string{"/", "/app.js", "/app.css"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d", path, rec.Code, http.StatusOK)
		}
	}
}
