package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/databufflabs/databuff-diag/internal/skill"
)

func TestSkillsHandler(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "test-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillMD := `---
name: test-skill
description: test description
---
# Test
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		t.Fatal(err)
	}

	loader := skill.NewLoader([]string{root})
	if err := loader.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	h := &SkillsHandler{Loader: loader}
	req := httptest.NewRequest(http.MethodGet, "/api/skills", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body skillsResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Skills) != 1 {
		t.Fatalf("skills len = %d, want 1", len(body.Skills))
	}
	if body.Skills[0].Name != "test-skill" {
		t.Fatalf("name = %q", body.Skills[0].Name)
	}
}
