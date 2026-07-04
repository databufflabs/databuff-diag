package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/databufflabs/databuff-diag/internal/agent"
	"github.com/databufflabs/databuff-diag/internal/llm"
	"github.com/databufflabs/databuff-diag/internal/policy"
	"github.com/databufflabs/databuff-diag/internal/sshresolve"
	"github.com/databufflabs/databuff-diag/internal/store"
)

type chatRequest struct {
	SessionID   string   `json:"session_id,omitempty"`
	Message     string   `json:"message"`
	Attachments []string `json:"attachments,omitempty"`
}

// ChatHandler serves POST /api/chat with SSE streaming.
type ChatHandler struct {
	ConfigStore     *store.ConfigStore
	SessionStore    *store.SessionStore
	AttachmentStore *store.AttachmentStore
	LLMClient       *llm.Client
	Agent           *agent.Agent
}

func (h *ChatHandler) client() *llm.Client {
	if h.LLMClient != nil {
		return h.LLMClient
	}
	return llm.NewClient()
}

func (h *ChatHandler) agent() *agent.Agent {
	if h.Agent != nil {
		if h.Agent.LLM == nil {
			h.Agent.LLM = h.client()
		}
		if h.Agent.Attachments == nil {
			h.Agent.Attachments = h.AttachmentStore
		}
		return h.Agent
	}
	ag := agent.New(h.SessionStore)
	ag.LLM = h.client()
	ag.Attachments = h.AttachmentStore
	return ag
}

func (h *ChatHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if strings.TrimSpace(req.Message) == "" && len(req.Attachments) == 0 {
		writeError(w, http.StatusBadRequest, "message or attachments required")
		return
	}
	if err := store.ValidateChatMessage(req.Message); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	cfg, err := h.ConfigStore.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	catalog, err := llm.LoadCatalog()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	provider, err := llm.ActiveProvider(catalog, cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var session *store.Session
	if req.SessionID != "" {
		session, err = h.SessionStore.Load(req.SessionID)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
	} else {
		mode := policy.Mode(cfg.Policy.Default)
		if mode == "" {
			mode = policy.Open
		}
		session, err = h.SessionStore.Create(mode)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	var msgAttachments []store.MessageAttachment
	if len(req.Attachments) > 0 {
		if h.AttachmentStore == nil {
			writeError(w, http.StatusInternalServerError, "attachment store not configured")
			return
		}
		var err error
		msgAttachments, err = h.AttachmentStore.ResolveMany(req.Attachments)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if !llm.ProviderSupportsVision(*provider) && hasImageAttachments(msgAttachments) {
			writeError(w, http.StatusBadRequest, llm.VisionUnsupportedError(*provider).Error())
			return
		}
	}

	if len(session.PendingApprovals) > 0 {
		writeError(w, http.StatusConflict, "请先处理待审批命令")
		return
	}

	if repaired := llm.RepairToolCallSequences(session.Messages); len(repaired) != len(session.Messages) {
		session.Messages = repaired
		if err := h.SessionStore.Save(session); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	message := strings.TrimSpace(req.Message)
	sshresolve.ApplyMessageOverrides(session, message)
	if len(session.SSHOverrides) > 0 {
		if err := h.SessionStore.Save(session); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if err := h.SessionStore.AppendMessage(session, store.SessionMessage{
		Role:        "user",
		Content:     message,
		Attachments: msgAttachments,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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

	sessionID := session.ID
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

	callbacks := &agent.LoopCallbacks{
		OnTurnStart: func() error {
			return writeSSE("turn_start", map[string]string{})
		},
		OnChunk: func(content string) error {
			return writeSSE("chunk", map[string]string{"content": content})
		},
		OnBeforeExecute: func(command string) error {
			return writeSSE("executing", map[string]string{"command": command})
		},
		AfterResolve: func(_ *store.Session) error {
			reloaded, err := h.SessionStore.Load(sessionID)
			if err != nil {
				return err
			}
			return writeSSE("session", reloaded)
		},
	}

	if err := h.agent().RunLoopStream(r.Context(), session, *provider, callbacks); err != nil {
		payload, _ := json.Marshal(map[string]string{"error": err.Error()})
		_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", payload)
		flusher.Flush()
		return
	}

	reloaded, err := h.SessionStore.Load(sessionID)
	if err != nil {
		payload, _ := json.Marshal(map[string]string{"error": err.Error()})
		_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", payload)
		flusher.Flush()
		return
	}

	donePayload, _ := json.Marshal(map[string]any{
		"session_id":        sessionID,
		"pending_approvals": reloaded.PendingApprovals,
	})
	_, _ = fmt.Fprintf(w, "event: done\ndata: %s\n\n", donePayload)
	flusher.Flush()
}

func hasImageAttachments(attachments []store.MessageAttachment) bool {
	for _, att := range attachments {
		if store.IsImageMime(att.MimeType) {
			return true
		}
	}
	return false
}
