package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/databufflabs/databuff-diag/internal/agent"
	"github.com/databufflabs/databuff-diag/internal/llm"
	"github.com/databufflabs/databuff-diag/internal/policy"
	"github.com/databufflabs/databuff-diag/internal/store"
	"github.com/go-chi/chi/v5"
)

const timeRFC3339 = time.RFC3339

type createSessionRequest struct {
	PolicyMode string `json:"policy_mode,omitempty"`
}

// SessionsHandler serves session CRUD and message/approve endpoints.
type SessionsHandler struct {
	ConfigStore  *store.ConfigStore
	SessionStore *store.SessionStore
	Agent        *agent.Agent
}

func (h *SessionsHandler) agent() *agent.Agent {
	if h.Agent != nil {
		return h.Agent
	}
	return agent.New(h.SessionStore)
}

func (h *SessionsHandler) resolveProvider() (*llm.MergedProvider, error) {
	cfg, err := h.ConfigStore.Load()
	if err != nil {
		return nil, err
	}
	catalog, err := llm.LoadCatalog()
	if err != nil {
		return nil, err
	}
	return llm.ActiveProvider(catalog, cfg)
}

type sessionSummary struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	MessageCount int    `json:"message_count"`
	PolicyMode   string `json:"policy_mode"`
}

// List handles GET /api/sessions.
func (h *SessionsHandler) List(w http.ResponseWriter, r *http.Request) {
	sessions, err := h.SessionStore.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	out := make([]sessionSummary, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, sessionSummary{
			ID:           session.ID,
			Title:        store.SessionTitle(session),
			CreatedAt:    session.CreatedAt.Format(timeRFC3339),
			UpdatedAt:    session.UpdatedAt.Format(timeRFC3339),
			MessageCount: len(session.Messages),
			PolicyMode:   string(session.PolicyMode),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

// Create handles POST /api/sessions.
func (h *SessionsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createSessionRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	mode := policy.Mode(req.PolicyMode)
	if mode == "" {
		cfg, err := h.ConfigStore.Load()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		mode = policy.Mode(cfg.Policy.Default)
		if mode == "" {
			mode = policy.WriteApproval
		}
	}

	session, err := h.SessionStore.Create(mode)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, session)
}

type patchSessionRequest struct {
	PolicyMode string `json:"policy_mode"`
}

// Patch handles PATCH /api/sessions/{id}.
func (h *SessionsHandler) Patch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req patchSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	mode := policy.Mode(req.PolicyMode)
	if !policy.IsValidMode(mode) {
		writeError(w, http.StatusBadRequest, "invalid policy_mode")
		return
	}

	session, err := h.SessionStore.SetPolicyMode(id, mode)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, session)
}

// Get handles GET /api/sessions/{id}.
func (h *SessionsHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	session, err := h.SessionStore.Load(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, session)
}

// Delete handles DELETE /api/sessions/{id}.
func (h *SessionsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := h.SessionStore.Load(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if err := h.SessionStore.Delete(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

type sessionMessageRequest struct {
	Content string `json:"content"`
}

// Message handles POST /api/sessions/{id}/message.
func (h *SessionsHandler) Message(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	session, err := h.SessionStore.Load(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req sessionMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}
	if err := store.ValidateChatMessage(req.Content); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if len(session.PendingApprovals) > 0 {
		writeError(w, http.StatusConflict, "请先处理待审批命令")
		return
	}

	provider, err := h.resolveProvider()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.agent().HandleUserMessage(r.Context(), session, req.Content, *provider); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	updated, err := h.SessionStore.Load(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

type approveRequest struct {
	ApprovalID string `json:"approval_id"`
	Approved   bool   `json:"approved"`
}

// Approve handles POST /api/sessions/{id}/approve with SSE streaming.
func (h *SessionsHandler) Approve(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	session, err := h.SessionStore.Load(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req approveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if req.ApprovalID == "" {
		writeError(w, http.StatusBadRequest, "approval_id is required")
		return
	}

	provider, err := h.resolveProvider()
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	writeSSE := func(event string, payload any) error {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	if err := writeSSE("started", map[string]string{"approval_id": req.ApprovalID}); err != nil {
		return
	}

	callbacks := &agent.LoopCallbacks{
		OnBeforeExecute: func(command string) error {
			return writeSSE("executing", map[string]string{"command": command})
		},
		AfterResolve: func(_ *store.Session) error {
			reloaded, err := h.SessionStore.Load(id)
			if err != nil {
				return err
			}
			return writeSSE("session", reloaded)
		},
		OnTurnStart: func() error {
			return writeSSE("turn_start", map[string]string{})
		},
		OnChunk: func(content string) error {
			return writeSSE("chunk", map[string]string{"content": content})
		},
	}

	if err := h.agent().ApproveWithCallbacks(r.Context(), session, req.ApprovalID, req.Approved, *provider, callbacks); err != nil {
		_ = writeSSE("error", map[string]string{"error": err.Error()})
		return
	}

	reloaded, err := h.SessionStore.Load(id)
	if err != nil {
		_ = writeSSE("error", map[string]string{"error": err.Error()})
		return
	}
	_ = writeSSE("done", map[string]any{
		"session_id":        id,
		"pending_approvals": reloaded.PendingApprovals,
	})
}
