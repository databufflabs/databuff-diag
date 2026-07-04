package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/databufflabs/databuff-diag/internal/llm"
	"github.com/databufflabs/databuff-diag/internal/store"
)

const (
	compactionKeepRecentMessages = 24
	compactionCharThreshold      = 240000 // ~60k tokens at 4 chars/token
)

// maybeCompactSession summarizes older messages when context grows too large.
func (a *Agent) maybeCompactSession(ctx context.Context, session *store.Session, provider llm.MergedProvider) error {
	if len(session.Messages) <= compactionKeepRecentMessages {
		return nil
	}
	total := estimateSessionChars(session)
	if total < compactionCharThreshold {
		return nil
	}

	split := len(session.Messages) - compactionKeepRecentMessages
	if split <= 0 {
		return nil
	}
	old := session.Messages[:split]
	recent := session.Messages[split:]

	var transcript strings.Builder
	for _, msg := range old {
		fmt.Fprintf(&transcript, "[%s] %s\n\n", msg.Role, truncateForSummary(msg.Content, 4000))
	}

	summaryPrompt := `Summarize the following diagnostic conversation for continuity. Preserve:
- hosts/IPs inspected, key commands run, and their outcomes
- identified root causes, hypotheses ruled out, and open questions
- file paths and artifacts created in the session workspace
Be factual and concise. Do not invent details.

` + transcript.String()

	resp, err := a.LLM.Chat(ctx, provider, llm.ChatRequest{
		Messages: []llm.ChatMessage{
			{Role: "system", Content: "You compress conversation history for an SRE diagnostic assistant."},
			{Role: "user", Content: summaryPrompt},
		},
	})
	if err != nil {
		return nil // non-fatal: continue without compaction
	}
	summary := strings.TrimSpace(resp.Content)
	if summary == "" {
		return nil
	}

	if session.CompactionSummary != "" {
		summary = session.CompactionSummary + "\n\n---\n\n" + summary
	}
	session.CompactionSummary = summary
	session.Messages = recent
	return a.Sessions.Save(session)
}

func estimateSessionChars(session *store.Session) int {
	n := len(session.CompactionSummary)
	for _, msg := range session.Messages {
		n += len(msg.Content) + len(msg.Stdout) + len(msg.Stderr)
	}
	return n
}

func truncateForSummary(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n...(truncated)"
}
