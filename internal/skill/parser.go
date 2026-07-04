package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const skillFileName = "SKILL.md"

// FrontMatter holds YAML front matter fields from SKILL.md.
type FrontMatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

type runbookYAML struct {
	ID       string   `yaml:"id"`
	Skill    string   `yaml:"skill"`
	Symptoms []string `yaml:"symptoms"`
	Checks   []struct {
		ID   string `yaml:"id"`
		Cmd  string `yaml:"cmd"`
		Risk string `yaml:"risk"`
	} `yaml:"checks"`
	Hints []string `yaml:"hints"`
}

// ParseSKILLFile reads and parses a SKILL.md file.
func ParseSKILLFile(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return ParseSKILL(data, filepath.Dir(path))
}

// ParseSKILL parses SKILL.md bytes. dir is the skill directory path.
func ParseSKILL(data []byte, dir string) (*Skill, error) {
	fm, body, err := parseFrontMatter(data)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(fm.Name) == "" {
		return nil, fmt.Errorf("SKILL.md missing required front matter field %q", "name")
	}
	return &Skill{
		Name:        strings.TrimSpace(fm.Name),
		Description: strings.TrimSpace(fm.Description),
		Path:        dir,
		Body:        strings.TrimSpace(body),
	}, nil
}

func parseFrontMatter(data []byte) (FrontMatter, string, error) {
	content := string(data)
	if !strings.HasPrefix(content, "---") {
		return FrontMatter{}, content, nil
	}

	rest := content[3:]
	if strings.HasPrefix(rest, "\n") {
		rest = rest[1:]
	} else if strings.HasPrefix(rest, "\r\n") {
		rest = rest[2:]
	} else {
		return FrontMatter{}, content, nil
	}

	end := strings.Index(rest, "\n---")
	if end < 0 {
		return FrontMatter{}, "", fmt.Errorf("unclosed YAML front matter")
	}

	fmRaw := rest[:end]
	body := strings.TrimPrefix(rest[end+4:], "\n")
	body = strings.TrimPrefix(body, "\r\n")

	var fm FrontMatter
	if err := yaml.Unmarshal([]byte(fmRaw), &fm); err != nil {
		return FrontMatter{}, "", fmt.Errorf("parse SKILL.md front matter: %w", err)
	}
	return fm, body, nil
}

// LoadRunbooks reads all *.yaml files from dir/runbooks/.
func LoadRunbooks(skillDir string, skillName string) ([]Runbook, error) {
	runbookDir := filepath.Join(skillDir, "runbooks")
	entries, err := os.ReadDir(runbookDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read runbooks dir: %w", err)
	}

	var out []Runbook
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		path := filepath.Join(runbookDir, name)
		rb, err := ParseRunbookFile(path, skillName)
		if err != nil {
			return nil, err
		}
		out = append(out, *rb)
	}
	return out, nil
}

// ParseRunbookFile reads and parses a runbook YAML file.
func ParseRunbookFile(path, skillName string) (*Runbook, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read runbook %s: %w", path, err)
	}
	return ParseRunbook(data, path, skillName)
}

// ParseRunbook parses runbook YAML bytes.
func ParseRunbook(data []byte, path, skillName string) (*Runbook, error) {
	var raw runbookYAML
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse runbook %s: %w", path, err)
	}

	id := strings.TrimSpace(raw.ID)
	if id == "" {
		id = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}

	rb := &Runbook{
		ID:       id,
		Skill:    strings.TrimSpace(raw.Skill),
		FilePath: path,
		Symptoms: append([]string(nil), raw.Symptoms...),
		Hints:    append([]string(nil), raw.Hints...),
	}
	if rb.Skill == "" {
		rb.Skill = skillName
	}

	for _, c := range raw.Checks {
		rb.Checks = append(rb.Checks, RunbookCheck{
			ID:   strings.TrimSpace(c.ID),
			Cmd:  strings.TrimSpace(c.Cmd),
			Risk: strings.TrimSpace(c.Risk),
		})
	}
	return rb, nil
}
