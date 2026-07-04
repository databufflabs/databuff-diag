package agent

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/databufflabs/databuff-diag/internal/sshresolve"
	"mvdan.cc/sh/v3/syntax"
)

var (
	bashFenceRE       = regexp.MustCompile("(?s)```(?:json|bash|sh|shell)?\\s*\\n([^`]+)```")
	jsonToolRE        = regexp.MustCompile(`(?s)\{\s*"tool"\s*:\s*"shell"\s*,\s*"command"\s*:\s*"([^"]+)"\s*\}`)
	shellToolRE       = regexp.MustCompile(`\{\s*"tool"\s*:\s*"shell"`)
	sshToolRE         = regexp.MustCompile(`\{\s*"tool"\s*:\s*"ssh"`)
	fenceOpenerRE     = regexp.MustCompile("(?s)\\n?```(?:json|bash|sh|shell)?\\s*\\n?\\s*$")
	orphanFenceLineRE = regexp.MustCompile("(?m)^```(?:json|bash|sh|shell)?\\s*$")
)

// shellTool is the JSON tool format from the LLM.
type shellTool struct {
	Tool    string `json:"tool"`
	Command string `json:"command"`
}

// sshToolJSON is the JSON SSH tool format from the LLM.
type sshToolJSON struct {
	Tool     string `json:"tool"`
	HostID   string `json:"host_id,omitempty"`
	Host     string `json:"host,omitempty"`
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
	Port     int    `json:"port,omitempty"`
	Command  string `json:"command"`
}

// ProposalText returns the assistant portion to persist for a command-proposal turn,
// keeping intro text before the command block and dropping fabricated output that
// may follow the proposed command.
func ProposalText(full, cmd string) string {
	return ProposalTextForTool(full, ToolCall{Kind: ToolShell, ShellCommand: cmd, DisplayCommand: cmd})
}

// ProposalTextForTool returns the assistant portion to persist for a tool-proposal turn.
func ProposalTextForTool(full string, tool ToolCall) string {
	full = strings.TrimSpace(full)
	cmd := tool.DisplayCommand
	if full == "" || cmd == "" {
		return full
	}

	if loc := shellToolRE.FindStringIndex(full); loc != nil {
		if before := trimTrailingFenceOpener(strings.TrimSpace(full[:loc[0]])); before != "" {
			return sanitizeProposalContent(before)
		}
		return proposalLead()
	}
	if loc := sshToolRE.FindStringIndex(full); loc != nil {
		if before := trimTrailingFenceOpener(strings.TrimSpace(full[:loc[0]])); before != "" {
			return sanitizeProposalContent(before)
		}
		return proposalLead()
	}

	searchFrom := 0
	for searchFrom < len(full) {
		loc := bashFenceRE.FindStringSubmatchIndex(full[searchFrom:])
		if loc == nil {
			break
		}
		start := searchFrom + loc[0]
		end := searchFrom + loc[1]
		fenced := strings.TrimSpace(full[searchFrom+loc[2] : searchFrom+loc[3]])
		if fenced == cmd || strings.Contains(fenced, cmd) {
			if before := trimTrailingFenceOpener(strings.TrimSpace(full[:start])); before != "" {
				return sanitizeProposalContent(before)
			}
			return proposalLead()
		}
		searchFrom = end
	}

	return sanitizeProposalContent(full)
}

func proposalLead() string {
	return "将执行命令"
}

func trimTrailingFenceOpener(s string) string {
	return strings.TrimSpace(fenceOpenerRE.ReplaceAllString(s, ""))
}

// sanitizeProposalContent removes orphan markdown code fences left after stripping tool JSON.
func sanitizeProposalContent(s string) string {
	s = strings.TrimSpace(s)
	for {
		next := strings.TrimSpace(fenceOpenerRE.ReplaceAllString(s, ""))
		next = strings.TrimSpace(orphanFenceLineRE.ReplaceAllString(next, ""))
		if next == s {
			break
		}
		s = next
	}
	return s
}

// ParseCommand extracts a local shell command from assistant text.
// Supports {"tool":"shell","command":"..."} JSON (optionally fenced) or ```bash blocks.
// Bare identifier lists (e.g. container names) are rejected — only plausible shell commands pass.
func ParseCommand(text string) (string, bool) {
	tool, ok := ParseTool(text)
	if !ok || tool.Kind != ToolShell {
		return "", false
	}
	return tool.ShellCommand, true
}

// ParseTool extracts a shell or SSH tool call from assistant text.
func ParseTool(text string) (ToolCall, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return ToolCall{}, false
	}

	if tool, ok := parseJSONSSHTool(text); ok {
		return tool, true
	}
	if cmd, ok := parseJSONTool(text); ok && looksLikeShellCommand(cmd) {
		return ToolCall{
			Kind:           ToolShell,
			ShellCommand:   cmd,
			DisplayCommand: cmd,
		}, true
	}

	if matches := bashFenceRE.FindStringSubmatch(text); len(matches) == 2 {
		fenced := strings.TrimSpace(matches[1])
		if tool, ok := parseJSONSSHTool(fenced); ok {
			return tool, true
		}
		if looksLikeShellCommand(fenced) {
			return ToolCall{
				Kind:           ToolShell,
				ShellCommand:   fenced,
				DisplayCommand: fenced,
			}, true
		}
	}

	// Single-line bare shell command when the whole message is clearly a command line.
	if isBareShellLine(text) {
		return ToolCall{
			Kind:           ToolShell,
			ShellCommand:   text,
			DisplayCommand: text,
		}, true
	}

	return ToolCall{}, false
}

func isBareShellLine(text string) bool {
	if strings.Contains(text, "\n") || !looksLikeShellCommand(text) {
		return false
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return false
	}
	first := strings.ToLower(words[0])
	if strings.ContainsAny(words[0], "/.") {
		return true
	}
	_, known := knownCommands[first]
	return known
}

func parseJSONSSHTool(text string) (ToolCall, bool) {
	candidates := collectJSONCandidates(text, sshToolRE)
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if !strings.Contains(candidate, `"tool"`) {
			continue
		}
		var tool sshToolJSON
		if err := json.Unmarshal([]byte(candidate), &tool); err != nil {
			continue
		}
		if tool.Tool != ToolSSH || strings.TrimSpace(tool.Command) == "" {
			continue
		}
		spec := &SSHToolCall{
			HostID:        strings.TrimSpace(tool.HostID),
			Host:          strings.TrimSpace(tool.Host),
			User:          strings.TrimSpace(tool.User),
			Password:      strings.TrimSpace(tool.Password),
			Port:          tool.Port,
			RemoteCommand: strings.TrimSpace(tool.Command),
		}
		if err := validateSSHTool(spec); err != nil {
			continue
		}
		display := spec.RemoteCommand
		if spec.Host != "" || spec.HostID != "" {
			display = buildSSHToolDisplay(*spec, sshresolve.Resolved{
				Host:        spec.Host,
				User:        spec.User,
				Port:        spec.Port,
				DisplayName: spec.HostID,
			})
		}
		return ToolCall{
			Kind:           ToolSSH,
			SSHTool:        spec,
			DisplayCommand: display,
		}, true
	}
	return ToolCall{}, false
}

func collectJSONCandidates(text string, toolRE *regexp.Regexp) []string {
	candidates := []string{text}
	if matches := bashFenceRE.FindStringSubmatch(text); len(matches) == 2 {
		candidates = append([]string{strings.TrimSpace(matches[1])}, candidates...)
	}
	if loc := toolRE.FindStringIndex(text); loc != nil {
		fragment := extractJSONObject(text[loc[0]:])
		if fragment != "" {
			candidates = append([]string{fragment}, candidates...)
		}
	}
	return candidates
}

func parseJSONTool(text string) (string, bool) {
	candidates := collectJSONCandidates(text, shellToolRE)
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if !strings.Contains(candidate, `"tool"`) {
			continue
		}
		var tool shellTool
		if err := json.Unmarshal([]byte(candidate), &tool); err != nil {
			if m := jsonToolRE.FindStringSubmatch(candidate); len(m) == 2 {
				return strings.TrimSpace(m[1]), true
			}
			continue
		}
		if tool.Tool == "shell" && strings.TrimSpace(tool.Command) != "" {
			return strings.TrimSpace(tool.Command), true
		}
	}
	return "", false
}

// knownCommands is the set of common single-word shell builtins/binaries we accept
// without additional arguments (e.g. "ls", not bare container names like "ai-apm-demo").
var knownCommands = map[string]struct{}{
	"awk": {}, "bash": {}, "cat": {}, "curl": {}, "df": {}, "dig": {}, "docker": {},
	"du": {}, "echo": {}, "env": {}, "false": {}, "find": {}, "free": {}, "grep": {},
	"head": {}, "host": {}, "hostname": {}, "id": {}, "iptables": {}, "journalctl": {},
	"kubectl": {}, "ls": {}, "mount": {}, "netstat": {}, "nginx": {}, "ping": {},
	"printenv": {}, "ps": {}, "pwd": {}, "rm": {}, "sed": {}, "sh": {}, "sort": {},
	"ss": {}, "stat": {}, "systemctl": {}, "tail": {}, "tee": {}, "test": {}, "top": {},
	"traceroute": {}, "true": {}, "uname": {}, "uniq": {}, "uptime": {}, "wc": {},
	"which": {}, "whoami": {}, "xargs": {},
}

func looksLikeShellCommand(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return false
	}

	parser := syntax.NewParser()
	file, err := parser.Parse(strings.NewReader(cmd), "")
	if err != nil {
		return false
	}

	hasCall := false
	onlyBareNames := true
	syntax.Walk(file, func(node syntax.Node) bool {
		call, ok := node.(*syntax.CallExpr)
		if !ok {
			return true
		}
		hasCall = true
		if plausibleCall(callWords(call)) {
			onlyBareNames = false
		}
		return true
	})

	return hasCall && !onlyBareNames
}

func plausibleCall(words []string) bool {
	if len(words) == 0 {
		return false
	}
	name := words[0]
	if len(words) > 1 {
		return true
	}
	if strings.ContainsAny(name, "/.") {
		return true
	}
	_, known := knownCommands[strings.ToLower(name)]
	return known
}

func callWords(call *syntax.CallExpr) []string {
	var words []string
	for _, arg := range call.Args {
		if lit := arg.Lit(); lit != "" {
			words = append(words, lit)
		}
	}
	return words
}

func extractJSONObject(s string) string {
	depth := 0
	for i, ch := range s {
		if ch == '{' {
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 {
				return s[:i+1]
			}
		}
	}
	return ""
}
