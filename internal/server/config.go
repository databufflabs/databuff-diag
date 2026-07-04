package server

import (
	"encoding/json"
	"net/http"

	"github.com/databufflabs/databuff-diag/internal/llm"
	"github.com/databufflabs/databuff-diag/internal/store"
)

// ConfigHandler serves GET/PUT /api/config.
type ConfigHandler struct {
	Store *store.ConfigStore
}

type providersResponse struct {
	Providers []providerListItem `json:"providers"`
}

type providerListItem struct {
	ProviderCode   string `json:"provider_code"`
	DisplayName    string `json:"display_name"`
	DefaultBaseURL string `json:"default_base_url"`
	DefaultWireAPI string `json:"default_wire_api"`
	DefaultModel   string `json:"default_model,omitempty"`
}

// ProvidersHandler serves GET /api/providers.
type ProvidersHandler struct{}

func (h *ProvidersHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	catalog, err := llm.LoadCatalog()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	items := make([]providerListItem, 0, len(catalog))
	for _, entry := range catalog {
		items = append(items, providerListItem{
			ProviderCode:   entry.ProviderCode,
			DisplayName:    entry.DisplayName,
			DefaultBaseURL: entry.DefaultBaseURL,
			DefaultWireAPI: entry.DefaultWireAPI,
			DefaultModel:   entry.DefaultModel,
		})
	}

	writeJSON(w, http.StatusOK, providersResponse{Providers: items})
}

func (h *ConfigHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.get(w, r)
	case http.MethodPut:
		h.put(w, r)
	default:
		methodNotAllowed(w)
	}
}

func (h *ConfigHandler) get(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.Store.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sanitizeConfigForResponse(cfg)
	writeJSON(w, http.StatusOK, cfg)
}

func (h *ConfigHandler) put(w http.ResponseWriter, r *http.Request) {
	var incoming store.Config
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	existing, err := h.Store.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	mergeEmptyAPIKeys(&incoming, existing)
	mergeEmptyAuthPassword(&incoming, existing)
	mergeEmptyHostPasswords(&incoming, existing)

	if incoming.LLM.Providers == nil {
		incoming.LLM.Providers = map[string]store.ProviderInstance{}
	}
	if incoming.SSH.Hosts == nil {
		incoming.SSH.Hosts = store.SSHHostsList{}
	}
	if incoming.Skills.Dirs == nil {
		incoming.Skills.Dirs = []string{}
	}

	if err := h.Store.Save(&incoming); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sanitizeConfigForResponse(&incoming)
	writeJSON(w, http.StatusOK, &incoming)
}

// mergeEmptyAPIKeys copies api_key from existing when incoming omits or clears it,
// so PUT /api/config can update other fields without wiping saved keys.
func mergeEmptyAuthPassword(incoming, existing *store.Config) {
	if incoming == nil || existing == nil {
		return
	}
	if incoming.Auth.Password == "" && existing.Auth.Password != "" {
		incoming.Auth.Password = existing.Auth.Password
	}
	if incoming.Auth.Username == "" && existing.Auth.Username != "" {
		incoming.Auth.Username = existing.Auth.Username
	}
}

func mergeEmptyAPIKeys(incoming, existing *store.Config) {
	if incoming == nil || existing == nil || incoming.LLM.Providers == nil || existing.LLM.Providers == nil {
		return
	}
	for code, p := range incoming.LLM.Providers {
		if p.APIKey != "" {
			continue
		}
		if prev, ok := existing.LLM.Providers[code]; ok && prev.APIKey != "" {
			p.APIKey = prev.APIKey
			incoming.LLM.Providers[code] = p
		}
	}
}

// sanitizeConfigForResponse masks secrets before returning config to the client.
func sanitizeConfigForResponse(cfg *store.Config) {
	if cfg == nil {
		return
	}
	for i := range cfg.SSH.Hosts {
		h := &cfg.SSH.Hosts[i]
		h.PasswordConfigured = h.Password != ""
		h.Password = ""
	}
}

// mergeEmptyHostPasswords preserves saved passwords when PUT omits or clears them.
func mergeEmptyHostPasswords(incoming, existing *store.Config) {
	if incoming == nil || existing == nil {
		return
	}
	prevByID := make(map[string]string, len(existing.SSH.Hosts))
	for _, h := range existing.SSH.Hosts {
		if h.ID != "" && h.Password != "" {
			prevByID[h.ID] = h.Password
		}
	}
	for i := range incoming.SSH.Hosts {
		h := &incoming.SSH.Hosts[i]
		if h.ID == "" {
			h.ID = store.NewSSHHostID()
		}
		if h.Password == "" {
			if pw, ok := prevByID[h.ID]; ok {
				h.Password = pw
			}
		}
		h.PasswordConfigured = false
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}
