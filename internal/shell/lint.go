package shell

import (
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Diagnostic is a shell syntax issue at a specific source position.
type Diagnostic struct {
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

// IsShellFile reports whether path or content looks like a shell script.
func IsShellFile(path, content string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".sh", ".bash", ".zsh":
		return true
	}
	return hasShellShebang(content)
}

// DetectDialect picks a shell language variant from path and shebang.
func DetectDialect(path, content string) syntax.LangVariant {
	line := strings.ToLower(firstLine(content))
	if strings.Contains(line, "bash") || strings.Contains(line, "zsh") {
		return syntax.LangBash
	}
	if strings.Contains(line, "/bin/sh") || strings.Contains(line, " dash") {
		return syntax.LangPOSIX
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".bash" || ext == ".zsh" {
		return syntax.LangBash
	}
	if ext == ".sh" {
		return syntax.LangPOSIX
	}
	return syntax.LangBash
}

// Lint parses shell content and returns syntax diagnostics. Returns nil when valid.
// This is parse-only: it never invokes a shell or runs any command from the script.
func Lint(content, path string) []Diagnostic {
	if !IsShellFile(path, content) {
		return nil
	}

	dialect := DetectDialect(path, content)
	parser := syntax.NewParser(syntax.Variant(dialect))
	_, err := parser.Parse(strings.NewReader(content), path)
	if err == nil {
		return nil
	}
	return parseErrorToDiagnostics(err)
}

func hasShellShebang(content string) bool {
	line := strings.ToLower(firstLine(content))
	if !strings.HasPrefix(line, "#!") {
		return false
	}
	return strings.Contains(line, "sh")
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return strings.TrimSpace(s[:idx])
	}
	return strings.TrimSpace(s)
}

func parseErrorToDiagnostics(err error) []Diagnostic {
	switch e := err.(type) {
	case syntax.ParseError:
		return []Diagnostic{diagnosticFromParseError(e)}
	case *syntax.ParseError:
		return []Diagnostic{diagnosticFromParseError(*e)}
	case syntax.LangError:
		return []Diagnostic{{
			Line:     int(e.Pos.Line()),
			Column:   int(e.Pos.Col()),
			Message:  e.Feature + " is not supported in this shell dialect",
			Severity: "error",
		}}
	default:
		return []Diagnostic{{
			Line:     1,
			Column:   1,
			Message:  err.Error(),
			Severity: "error",
		}}
	}
}

func diagnosticFromParseError(e syntax.ParseError) Diagnostic {
	return Diagnostic{
		Line:     int(e.Pos.Line()),
		Column:   int(e.Pos.Col()),
		Message:  e.Text,
		Severity: "error",
	}
}
