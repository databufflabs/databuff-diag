package policy

import (
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Risk is the classified risk level of a shell command.
type Risk string

const (
	RiskReadonly  Risk = "readonly"
	RiskWrite     Risk = "write"
	RiskDangerous Risk = "dangerous"
	RiskBlocked   Risk = "blocked"
)

// Engine classifies shell commands using an AST built by mvdan/sh.
type Engine struct{}

var (
	blacklistPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)rm\s+.*(-rf|-fr)\s+/?\s*$`),
		regexp.MustCompile(`(?i)rm\s+(-rf|-fr)\s+/\s`),
		regexp.MustCompile(`:\s*\(\s*\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;\s*:`),
	}

	forkBombPattern = regexp.MustCompile(`:\s*\(\s*\)\s*\{`)
)

// Classify parses cmd and returns the highest risk found across pipelines,
// subshells, and command substitutions.
func (e *Engine) Classify(cmd string) (Risk, error) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return RiskReadonly, nil
	}

	if matchesBlacklist(cmd) {
		return RiskBlocked, nil
	}

	parser := syntax.NewParser()
	file, err := parser.Parse(strings.NewReader(cmd), "")
	if err != nil {
		return classifyHeuristic(cmd), nil
	}

	risk := RiskReadonly
	syntax.Walk(file, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.CallExpr:
			risk = maxRisk(risk, classifyCall(n))
		case *syntax.Redirect:
			if isWriteRedirect(n) {
				risk = maxRisk(risk, RiskWrite)
			}
		}
		return true
	})

	if risk != RiskBlocked && matchesBlacklist(cmd) {
		return RiskBlocked, nil
	}

	return risk, nil
}

func matchesBlacklist(cmd string) bool {
	for _, re := range blacklistPatterns {
		if re.MatchString(cmd) {
			return true
		}
	}
	return false
}

func classifyHeuristic(cmd string) Risk {
	lower := strings.ToLower(cmd)
	if matchesBlacklist(cmd) || forkBombPattern.MatchString(cmd) {
		return RiskBlocked
	}
	if strings.Contains(lower, "rm -rf") || strings.Contains(lower, "rm -fr") {
		return RiskDangerous
	}
	if strings.Contains(lower, "mkfs") || strings.Contains(lower, " dd ") || strings.HasPrefix(lower, "dd ") {
		return RiskDangerous
	}
	if strings.Contains(lower, "iptables -f") || strings.Contains(lower, "iptables -F") {
		return RiskDangerous
	}
	if strings.Contains(lower, "sed -i") || strings.Contains(lower, "systemctl restart") ||
		strings.Contains(lower, "kubectl apply") || strings.Contains(lower, "kubectl delete") ||
		strings.Contains(lower, "docker compose") {
		return RiskWrite
	}
	return RiskWrite
}

func classifyCall(call *syntax.CallExpr) Risk {
	words := callWords(call)
	if len(words) == 0 {
		return RiskReadonly
	}

	full := strings.Join(words, " ")
	if matchesBlacklist(full) || forkBombPattern.MatchString(full) {
		return RiskBlocked
	}

	cmd := strings.ToLower(words[0])

	if strings.HasPrefix(cmd, "mkfs") {
		return RiskDangerous
	}

	switch cmd {
	case "ls", "cat", "grep", "journalctl", "df", "free":
		return RiskReadonly
	case "curl":
		return RiskReadonly
	case "docker":
		if len(words) > 1 {
			switch strings.ToLower(words[1]) {
			case "ps":
				return RiskReadonly
			case "compose":
				return RiskWrite
			}
		}
	case "kubectl":
		if len(words) > 1 {
			switch strings.ToLower(words[1]) {
			case "get":
				return RiskReadonly
			case "apply", "delete":
				return RiskWrite
			}
		}
	case "systemctl":
		if len(words) > 1 {
			switch strings.ToLower(words[1]) {
			case "restart", "stop", "start", "enable", "disable":
				return RiskWrite
			}
		}
	case "sed":
		for _, w := range words[1:] {
			if strings.HasPrefix(w, "-i") {
				return RiskWrite
			}
		}
	case "tee":
		return RiskWrite
	case "rm":
		if hasFlag(words, "-rf", "-fr") {
			if targetsRoot(words) {
				return RiskBlocked
			}
			return RiskDangerous
		}
	case "dd":
		return RiskDangerous
	case "iptables":
		if len(words) > 1 && strings.EqualFold(words[1], "-F") {
			return RiskDangerous
		}
	}

	return RiskWrite
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

func hasFlag(words []string, flags ...string) bool {
	for _, w := range words[1:] {
		for _, f := range flags {
			if w == f || strings.HasPrefix(w, f) {
				return true
			}
		}
	}
	return false
}

func targetsRoot(words []string) bool {
	for _, w := range words[1:] {
		if strings.HasPrefix(w, "-") {
			continue
		}
		clean := strings.TrimSpace(w)
		if clean == "/" || clean == "/*" {
			return true
		}
	}
	return false
}

func isWriteRedirect(r *syntax.Redirect) bool {
	switch r.Op {
	case syntax.RdrOut, syntax.AppOut, syntax.DplOut, syntax.RdrInOut, syntax.ClbOut, syntax.RdrAll, syntax.AppAll:
		return true
	default:
		return false
	}
}

func maxRisk(a, b Risk) Risk {
	order := map[Risk]int{
		RiskReadonly:  0,
		RiskWrite:     1,
		RiskDangerous: 2,
		RiskBlocked:   3,
	}
	if order[a] >= order[b] {
		return a
	}
	return b
}
