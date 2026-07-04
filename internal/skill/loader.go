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

// SystemPromptContext formats loaded skills for the agent system prompt (Pi-compatible).
func (l *Loader) SystemPromptContext() string {
	if len(l.skills) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("The following skills provide specialized instructions for specific diagnostic tasks.\n")
	b.WriteString("Use the read tool to load a skill's SKILL.md when the task matches its description.\n")
	b.WriteString("When a skill file references a relative path, resolve it against the skill directory.\n")
	b.WriteString("\n<available_skills>\n")
	for _, sk := range l.skills {
		skillPath := filepath.Join(sk.Path, skillFileName)
		desc := sk.Description
		if len(sk.Runbooks) > 0 {
			ids := make([]string, len(sk.Runbooks))
			for i, rb := range sk.Runbooks {
				ids[i] = rb.ID
			}
			runbookNote := "runbooks: " + strings.Join(ids, ", ")
			if desc == "" {
				desc = runbookNote
			} else {
				desc += " (" + runbookNote + ")"
			}
		}
		fmt.Fprintf(&b, "  <skill>\n    <name>%s</name>\n", escapeXML(sk.Name))
		fmt.Fprintf(&b, "    <description>%s</description>\n", escapeXML(desc))
		fmt.Fprintf(&b, "    <location>%s</location>\n  </skill>\n", escapeXML(skillPath))
	}
	b.WriteString("</available_skills>")
	return strings.TrimSpace(b.String())
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return strings.ReplaceAll(s, "'", "&apos;")
}
