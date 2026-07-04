package report

import (
	"fmt"
	"strings"
	"time"

	"github.com/databufflabs/databuff-diag/internal/store"
)

// RenderMarkdown builds a diagnostic report from a session.
func RenderMarkdown(session *store.Session) string {
	if session == nil {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Diagnostic Report\n\n")
	fmt.Fprintf(&b, "| Field | Value |\n")
	fmt.Fprintf(&b, "|-------|-------|\n")
	fmt.Fprintf(&b, "| Session ID | `%s` |\n", session.ID)
	fmt.Fprintf(&b, "| Created | %s |\n", formatTime(session.CreatedAt))
	fmt.Fprintf(&b, "| Updated | %s |\n", formatTime(session.UpdatedAt))
	fmt.Fprintf(&b, "| Policy | %s |\n", session.PolicyMode)
	fmt.Fprintf(&b, "\n")

	if len(session.Messages) == 0 {
		b.WriteString("_No messages in this session._\n")
		return b.String()
	}

	b.WriteString("## Timeline\n\n")
	for _, msg := range session.Messages {
		writeMessage(&b, msg)
	}

	if len(session.PendingApprovals) > 0 {
		b.WriteString("## Pending Approvals\n\n")
		for _, p := range session.PendingApprovals {
			fmt.Fprintf(&b, "- **%s** (`%s`): `%s`\n", p.ID, p.Risk, p.Command)
			if p.Reason != "" {
				fmt.Fprintf(&b, "  - Reason: %s\n", p.Reason)
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("## Command Audit\n\n")
	commands := 0
	for _, msg := range session.Messages {
		if msg.Command == "" {
			continue
		}
		commands++
		fmt.Fprintf(&b, "### `%s`\n\n", msg.Command)
		if msg.Risk != "" {
			fmt.Fprintf(&b, "- **Risk:** %s\n", msg.Risk)
		}
		if msg.ExitCode != nil {
			fmt.Fprintf(&b, "- **Exit code:** %d\n", *msg.ExitCode)
		}
		fmt.Fprintf(&b, "- **Time:** %s\n\n", formatTime(msg.Timestamp))
		if msg.Stdout != "" {
			b.WriteString("```text\n")
			b.WriteString(msg.Stdout)
			if !strings.HasSuffix(msg.Stdout, "\n") {
				b.WriteByte('\n')
			}
			b.WriteString("```\n\n")
		}
		if msg.Stderr != "" {
			b.WriteString("**stderr:**\n\n```text\n")
			b.WriteString(msg.Stderr)
			if !strings.HasSuffix(msg.Stderr, "\n") {
				b.WriteByte('\n')
			}
			b.WriteString("```\n\n")
		}
	}
	if commands == 0 {
		b.WriteString("_No commands executed in this session._\n")
	}

	return b.String()
}

func writeMessage(b *strings.Builder, msg store.SessionMessage) {
	title := roleTitle(msg.Role)
	if msg.Command != "" {
		title = fmt.Sprintf("Command (%s)", msg.Role)
	}
	fmt.Fprintf(b, "### %s — %s\n\n", title, formatTime(msg.Timestamp))
	if msg.Content != "" {
		b.WriteString(msg.Content)
		if !strings.HasSuffix(msg.Content, "\n") {
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	if msg.Command != "" && msg.Content == "" {
		fmt.Fprintf(b, "`%s`\n\n", msg.Command)
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format(time.RFC3339)
}

func roleTitle(role string) string {
	if role == "" {
		return "Message"
	}
	return strings.ToUpper(role[:1]) + role[1:]
}
