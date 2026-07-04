package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Loader discovers and parses skills from configured root directories.
type Loader struct {
	dirs   []string
	skills []Skill
}

// NewLoader returns a loader that scans the given skill root directories.
func NewLoader(dirs []string) *Loader {
	return &Loader{dirs: append([]string(nil), dirs...)}
}

// Load scans skill roots and parses SKILL.md plus runbooks/*.yaml.
func (l *Loader) Load() error {
	seen := make(map[string]struct{})
	var loaded []Skill

	for _, root := range l.dirs {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		skills, err := scanRoot(root)
		if err != nil {
			return err
		}
		for _, sk := range skills {
			if _, ok := seen[sk.Name]; ok {
				continue
			}
			seen[sk.Name] = struct{}{}
			loaded = append(loaded, sk)
		}
	}

	l.skills = loaded
	return nil
}

func scanRoot(root string) ([]Skill, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat skills dir %s: %w", root, err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("skills path is not a directory: %s", root)
	}

	// Support a root that itself is a skill directory.
	if skill, ok, err := loadSkillDir(root); err != nil {
		return nil, err
	} else if ok {
		return []Skill{*skill}, nil
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read skills dir %s: %w", root, err)
	}

	var out []Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillDir := filepath.Join(root, entry.Name())
		skill, ok, err := loadSkillDir(skillDir)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, *skill)
		}
	}
	return out, nil
}

func loadSkillDir(dir string) (*Skill, bool, error) {
	skillPath := filepath.Join(dir, skillFileName)
	if _, err := os.Stat(skillPath); err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("stat %s: %w", skillPath, err)
	}

	skill, err := ParseSKILLFile(skillPath)
	if err != nil {
		return nil, false, err
	}

	runbooks, err := LoadRunbooks(dir, skill.Name)
	if err != nil {
		return nil, false, err
	}
	skill.Runbooks = runbooks
	return skill, true, nil
}

// Skills returns loaded skills in discovery order.
func (l *Loader) Skills() []Skill {
	out := make([]Skill, len(l.skills))
	copy(out, l.skills)
	return out
}

// Summaries returns API-facing skill summaries.
func (l *Loader) Summaries() []Summary {
	out := make([]Summary, 0, len(l.skills))
	for _, sk := range l.skills {
		sum := Summary{
			Name:        sk.Name,
			Description: sk.Description,
			Path:        sk.Path,
		}
		for _, rb := range sk.Runbooks {
			sum.Runbooks = append(sum.Runbooks, rb.ID)
		}
		out = append(out, sum)
	}
	return out
}

// SystemPromptContext formats loaded skills for the agent system prompt.
func (l *Loader) SystemPromptContext() string {
	if len(l.skills) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Loaded diagnostic skills\n\n")
	b.WriteString("Use the following skills and runbooks as reference when diagnosing issues. ")
	b.WriteString("Follow runbook checks in order when symptoms match; do not invent command output.\n")

	for _, sk := range l.skills {
		fmt.Fprintf(&b, "\n### %s\n", sk.Name)
		if sk.Description != "" {
			fmt.Fprintf(&b, "%s\n", sk.Description)
		}
		if sk.Body != "" {
			fmt.Fprintf(&b, "\n%s\n", sk.Body)
		}
		for _, rb := range sk.Runbooks {
			fmt.Fprintf(&b, "\nRunbook %s", rb.ID)
			if len(rb.Symptoms) > 0 {
				fmt.Fprintf(&b, " (symptoms: %s)", strings.Join(rb.Symptoms, "; "))
			}
			b.WriteString(":\n")
			for i, check := range rb.Checks {
				fmt.Fprintf(&b, "  %d. [%s] %s\n", i+1, check.ID, check.Cmd)
			}
			for _, hint := range rb.Hints {
				fmt.Fprintf(&b, "  hint: %s\n", hint)
			}
		}
	}
	return strings.TrimSpace(b.String())
}
