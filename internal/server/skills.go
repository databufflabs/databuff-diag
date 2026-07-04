package server

import (
	"encoding/json"
	"net/http"

	"github.com/databufflabs/databuff-diag/internal/skill"
)

type skillsResponse struct {
	Skills []skill.Summary `json:"skills"`
}

// SkillsHandler serves GET /api/skills.
type SkillsHandler struct {
	Loader *skill.Loader
}

func (h *SkillsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.Loader == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(skillsResponse{Skills: []skill.Summary{}})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(skillsResponse{Skills: h.Loader.Summaries()})
}
