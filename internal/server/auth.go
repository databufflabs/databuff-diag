package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/databufflabs/databuff-diag/internal/store"
)

const (
	sessionCookieName = "diag_session"
	sessionTTL        = 7 * 24 * time.Hour
)

// AuthManager tracks login sessions and validates credentials from config.
// Multiple concurrent sessions per account are allowed (each login gets its own token).
type AuthManager struct {
	cfgStore *store.ConfigStore

	mu       sync.RWMutex
	sessions map[string]time.Time
}

// NewAuthManager creates an auth manager backed by the config store.
func NewAuthManager(cfgStore *store.ConfigStore) *AuthManager {
	return &AuthManager{
		cfgStore: cfgStore,
		sessions: make(map[string]time.Time),
	}
}

// AuthHandler serves login, logout, and session status endpoints.
type AuthHandler struct {
	Auth *AuthManager
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type authMeResponse struct {
	Username string `json:"username"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	if !h.Auth.VerifyCredentials(strings.TrimSpace(req.Username), req.Password) {
		writeError(w, http.StatusUnauthorized, "用户名或密码错误")
		return
	}

	token := h.Auth.CreateSession()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		h.Auth.DeleteSession(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	username, ok := h.Auth.AuthenticatedUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "未登录")
		return
	}
	writeJSON(w, http.StatusOK, authMeResponse{Username: username})
}

// RequireAuth rejects requests without a valid session cookie.
func (a *AuthManager) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := a.AuthenticatedUser(r); !ok {
			writeError(w, http.StatusUnauthorized, "未登录")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// AuthenticatedUser returns the configured username when the session is valid.
func (a *AuthManager) AuthenticatedUser(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	if !a.ValidateSession(cookie.Value) {
		return "", false
	}
	cfg, err := a.cfgStore.Load()
	if err != nil {
		return "", false
	}
	return cfg.Auth.Username, true
}

// VerifyCredentials checks username and password against config.
func (a *AuthManager) VerifyCredentials(username, password string) bool {
	cfg, err := a.cfgStore.Load()
	if err != nil {
		return false
	}
	return subtleEqual(username, cfg.Auth.Username) && subtleEqual(password, cfg.Auth.Password)
}

// CreateSession registers a new session token and returns it.
func (a *AuthManager) CreateSession() string {
	token := randomToken()
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cleanupLocked(time.Now())
	a.sessions[token] = time.Now().Add(sessionTTL)
	return token
}

// ValidateSession reports whether the token is still active.
func (a *AuthManager) ValidateSession(token string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	a.cleanupLocked(now)
	expiry, ok := a.sessions[token]
	return ok && now.Before(expiry)
}

// DeleteSession removes a session token.
func (a *AuthManager) DeleteSession(token string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.sessions, token)
}

func (a *AuthManager) cleanupLocked(now time.Time) {
	for token, expiry := range a.sessions {
		if !now.Before(expiry) {
			delete(a.sessions, token)
		}
	}
}

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func subtleEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

// publicStaticPaths are served without authentication (login page assets).
var publicStaticPaths = map[string]bool{
	"":            true,
	"index.html":  true,
	"app.css":     true,
	"app.js":      true,
	"favicon.svg": true,
	"favicon.png": true,
}

func authenticatedStaticHandler(auth *AuthManager) http.Handler {
	base := staticHandler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if publicStaticPaths[path] {
			base.ServeHTTP(w, r)
			return
		}
		if _, ok := auth.AuthenticatedUser(r); !ok {
			writeError(w, http.StatusUnauthorized, "未登录")
			return
		}
		base.ServeHTTP(w, r)
	})
}
