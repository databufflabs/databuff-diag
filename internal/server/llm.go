package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/databufflabs/databuff-diag/internal/llm"
	"github.com/databufflabs/databuff-diag/internal/store"
)

type llmTestRequest struct {
	ProviderCode      string `json:"provider_code,omitempty"`
	BaseURL           string `json:"base_url,omitempty"`
	APIKey            string `json:"api_key,omitempty"`
	Model             string `json:"model,omitempty"`
	WireAPI           string `json:"wire_api,omitempty"`
	ResponseProcessor string `json:"response_processor,omitempty"`
}

type llmTestResponse struct {
	Success       bool   `json:"success"`
	Content       string `json:"content,omitempty"`
	LatencyMS     int64  `json:"latency_ms,omitempty"`
	ProcessorUsed string `json:"processor_used,omitempty"`
	Error         string `json:"error,omitempty"`
}

// LLMTestHandler serves POST /api/llm/test.
type LLMTestHandler struct {
	Store  *store.ConfigStore
	Client *llm.Client
}

func (h *LLMTestHandler) client() *llm.Client {
	if h.Client != nil {
		return h.Client
	}
	return llm.NewClient()
}

func (h *LLMTestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var req llmTestRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
	}

	cfg, err := h.Store.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	catalog, err := llm.LoadCatalog()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	provider, err := resolveLLMTestProvider(catalog, cfg, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	proc, err := llm.ProcessorFor(*provider)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	processorUsed := proc.ID()

	start := time.Now()
	chatResp, err := h.client().Chat(r.Context(), *provider, llm.ChatRequest{
		Messages: []llm.ChatMessage{{Role: "user", Content: "reply ok"}},
	})
	latencyMS := time.Since(start).Milliseconds()

	if err != nil {
		writeJSON(w, http.StatusOK, llmTestResponse{
			Success:       false,
			Error:         err.Error(),
			LatencyMS:     latencyMS,
			ProcessorUsed: processorUsed,
		})
		return
	}

	writeJSON(w, http.StatusOK, llmTestResponse{
		Success:       true,
		Content:       chatResp.Content,
		LatencyMS:     latencyMS,
		ProcessorUsed: processorUsed,
	})
}

func resolveLLMTestProvider(catalog []llm.CatalogEntry, cfg *store.Config, req llmTestRequest) (*llm.MergedProvider, error) {
	var base llm.MergedProvider

	if req.ProviderCode != "" {
		found := false
		for _, p := range llm.MergeProviders(catalog, cfg) {
			if p.ProviderCode == req.ProviderCode {
				base = p
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("provider %q not found", req.ProviderCode)
		}
	} else {
		ap, err := llm.ActiveProvider(catalog, cfg)
		if err != nil {
			return nil, err
		}
		base = *ap
	}

	if req.BaseURL != "" {
		base.BaseURL = req.BaseURL
	}
	if req.APIKey != "" {
		base.APIKey = req.APIKey
	}
	if req.Model != "" {
		base.Model = req.Model
	}
	if req.WireAPI != "" {
		base.WireAPI = req.WireAPI
	}
	if req.ResponseProcessor != "" {
		base.ResponseProcessor = req.ResponseProcessor
	}

	return &base, nil
}
