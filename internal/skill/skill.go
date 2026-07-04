// Package skill loads SKILL.md runbooks from configured directories.
package skill

// Skill is a parsed skill directory with optional runbooks.
type Skill struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Path        string    `json:"path"`
	Body        string    `json:"-"`
	Runbooks    []Runbook `json:"runbooks"`
}

// Runbook is a YAML troubleshooting playbook under runbooks/.
type Runbook struct {
	ID       string         `json:"id"`
	Skill    string         `json:"skill,omitempty"`
	FilePath string         `json:"-"`
	Symptoms []string       `json:"symptoms,omitempty"`
	Checks   []RunbookCheck `json:"checks,omitempty"`
	Hints    []string       `json:"hints,omitempty"`
}

// RunbookCheck is a single command step inside a runbook.
type RunbookCheck struct {
	ID   string `json:"id"`
	Cmd  string `json:"cmd"`
	Risk string `json:"risk,omitempty"`
}

// Summary is the API-facing view of a loaded skill.
type Summary struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Path        string   `json:"path"`
	Runbooks    []string `json:"runbooks"`
}
