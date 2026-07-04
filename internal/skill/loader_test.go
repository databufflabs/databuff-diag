package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSKILL(t *testing.T) {
	data := []byte(`---
name: generic-infra
description: Linux, Docker, and Kubernetes read-only checks
---

# Generic infra

## Workflow
1. Check docker ps
`)

	sk, err := ParseSKILL(data, "/tmp/generic-infra")
	if err != nil {
		t.Fatalf("ParseSKILL: %v", err)
	}
	if sk.Name != "generic-infra" {
		t.Fatalf("name = %q, want generic-infra", sk.Name)
	}
	if sk.Description == "" {
		t.Fatal("expected description")
	}
	if !contains(sk.Body, "Generic infra") {
		t.Fatalf("body = %q", sk.Body)
	}
}

func TestParseRunbook(t *testing.T) {
	data := []byte(`
id: docker-health
skill: generic-infra
symptoms: ["container unhealthy"]
checks:
  - id: compose_ps
    cmd: "docker ps"
    risk: readonly
hints:
  - "check restart count"
`)

	rb, err := ParseRunbook(data, "/tmp/runbooks/docker-health.yaml", "generic-infra")
	if err != nil {
		t.Fatalf("ParseRunbook: %v", err)
	}
	if rb.ID != "docker-health" {
		t.Fatalf("id = %q", rb.ID)
	}
	if len(rb.Checks) != 1 || rb.Checks[0].Cmd != "docker ps" {
		t.Fatalf("checks = %+v", rb.Checks)
	}
}

func TestLoader_Load(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "generic-infra")
	runbookDir := filepath.Join(skillDir, "runbooks")
	if err := os.MkdirAll(runbookDir, 0o755); err != nil {
		t.Fatal(err)
	}

	skillMD := `---
name: generic-infra
description: infra checks
---
# Generic
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		t.Fatal(err)
	}
	runbook := `
id: docker-health
symptoms: ["unhealthy"]
checks:
  - id: ps
    cmd: "docker ps"
    risk: readonly
`
	if err := os.WriteFile(filepath.Join(runbookDir, "docker-health.yaml"), []byte(runbook), 0o644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader([]string{root})
	if err := loader.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	skills := loader.Skills()
	if len(skills) != 1 {
		t.Fatalf("skills len = %d, want 1", len(skills))
	}
	if len(skills[0].Runbooks) != 1 {
		t.Fatalf("runbooks len = %d, want 1", len(skills[0].Runbooks))
	}

	ctx := loader.SystemPromptContext()
	if !contains(ctx, "generic-infra") || !contains(ctx, "<available_skills>") || !contains(ctx, "SKILL.md") {
		t.Fatalf("system context missing skill data: %q", ctx)
	}

	sums := loader.Summaries()
	if len(sums) != 1 || sums[0].Name != "generic-infra" {
		t.Fatalf("summaries = %+v", sums)
	}
	if len(sums[0].Runbooks) != 1 || sums[0].Runbooks[0] != "docker-health" {
		t.Fatalf("runbook ids = %+v", sums[0].Runbooks)
	}
}

func TestLoader_LoadRepoGenericInfra(t *testing.T) {
	root := repoRoot(t)
	skillsDir := filepath.Join(root, "deploy", "skills")

	loader := NewLoader([]string{skillsDir})
	if err := loader.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	sk := findSkill(t, loader.Skills(), "generic-infra")
	if sk.Description == "" {
		t.Fatal("expected description")
	}

	wantRunbooks := map[string]bool{
		"docker-health":  false,
		"k8s-pod-crash":  false,
		"host-resources": false,
	}
	for _, rb := range sk.Runbooks {
		if _, ok := wantRunbooks[rb.ID]; ok {
			wantRunbooks[rb.ID] = true
		}
	}
	for id, found := range wantRunbooks {
		if !found {
			t.Fatalf("missing runbook %q", id)
		}
	}

	ctx := loader.SystemPromptContext()
	for _, needle := range []string{"generic-infra", "<available_skills>", "SKILL.md", "docker-health"} {
		if !contains(ctx, needle) {
			t.Fatalf("system context missing %q", needle)
		}
	}
}

func TestLoader_LoadRepoDatabuffOSS(t *testing.T) {
	root := repoRoot(t)
	skillsDir := filepath.Join(root, "deploy", "skills")

	loader := NewLoader([]string{skillsDir})
	if err := loader.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	sk := findSkill(t, loader.Skills(), "databuff-oss")
	if sk.Description == "" {
		t.Fatal("expected description")
	}
	if len(sk.Runbooks) < 5 {
		t.Fatalf("runbooks len = %d, want >= 5", len(sk.Runbooks))
	}

	wantRunbooks := map[string]bool{
		"ai-apm-install-failure":  false,
		"doris-fe-unhealthy":      false,
		"doris-be-unhealthy":      false,
		"ingest-4318-unreachable": false,
		"web-27403-unreachable":   false,
		"compose-logs-triage":     false,
	}
	for _, rb := range sk.Runbooks {
		if _, ok := wantRunbooks[rb.ID]; ok {
			wantRunbooks[rb.ID] = true
		}
	}
	for id, found := range wantRunbooks {
		if !found {
			t.Fatalf("missing runbook %q", id)
		}
	}

	ctx := loader.SystemPromptContext()
	for _, needle := range []string{
		"databuff-oss",
		"<available_skills>",
		"SKILL.md",
		"ai-apm-install-failure",
	} {
		if !contains(ctx, needle) {
			t.Fatalf("system context missing %q", needle)
		}
	}

	sums := loader.Summaries()
	var ossSum *Summary
	for i := range sums {
		if sums[i].Name == "databuff-oss" {
			ossSum = &sums[i]
			break
		}
	}
	if ossSum == nil {
		t.Fatal("databuff-oss summary not found")
	}
	if len(ossSum.Runbooks) < 5 {
		t.Fatalf("summary runbooks len = %d, want >= 5", len(ossSum.Runbooks))
	}
}

func findSkill(t *testing.T, skills []Skill, name string) Skill {
	t.Helper()
	for _, sk := range skills {
		if sk.Name == name {
			return sk
		}
	}
	t.Fatalf("skill %q not found among %d loaded skills", name, len(skills))
	return Skill{}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (go.mod)")
		}
		dir = parent
	}
}

func TestLoader_MissingDirIsOK(t *testing.T) {
	loader := NewLoader([]string{filepath.Join(t.TempDir(), "missing")})
	if err := loader.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loader.Skills()) != 0 {
		t.Fatalf("expected no skills")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
