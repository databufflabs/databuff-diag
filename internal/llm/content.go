package llm

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/databufflabs/databuff-diag/internal/store"
)

// ContentPart is one block in a multimodal chat message (OpenAI-compatible).
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL holds a data URL or remote image reference.
type ImageURL struct {
	URL string `json:"url"`
}

const interruptedToolResult = "Error: tool execution was interrupted before completion."

// RepairToolCallSequences inserts synthetic tool results when an assistant turn
// with tool_calls was not fully answered (e.g. client disconnect or a new user
// message arrived mid-batch). OpenAI-compatible APIs reject such histories.
func RepairToolCallSequences(messages []store.SessionMessage) []store.SessionMessage {
	if len(messages) == 0 {
		return messages
	}
	out := make([]store.SessionMessage, 0, len(messages)+4)
	for i := 0; i < len(messages); i++ {
		msg := messages[i]
		out = append(out, msg)
		if msg.Role != "assistant" || len(msg.ToolCalls) == 0 {
			continue
		}
		expected := make(map[string]store.StoredToolCall, len(msg.ToolCalls))
		order := make([]string, 0, len(msg.ToolCalls))
		for _, tc := range msg.ToolCalls {
			expected[tc.ID] = tc
			order = append(order, tc.ID)
		}
		i++
		responded := make(map[string]bool, len(order))
		for i < len(messages) && messages[i].Role == "tool" {
			out = append(out, messages[i])
			if id := messages[i].ToolCallID; id != "" {
				responded[id] = true
			}
			i++
		}
		for _, id := range order {
			if responded[id] {
				continue
			}
			tc := expected[id]
			out = append(out, store.SessionMessage{
				Role:       "tool",
				Content:    interruptedToolResult,
				ToolCallID: id,
				ToolName:   tc.Name,
			})
		}
		i--
	}
	return out
}

// BuildMessagesFromSession converts stored session messages to LLM chat messages.
// When vision is false, image attachments are replaced with text placeholders so
// text-only providers (e.g. DeepSeek) do not receive image_url content parts.
func BuildMessagesFromSession(session *store.Session, systemPrompt string, attachments *store.AttachmentStore, vision bool) ([]ChatMessage, error) {
	out := []ChatMessage{{Role: "system", Content: systemPrompt}}
	for _, msg := range RepairToolCallSequences(session.Messages) {
		chatMsg, err := MessageFromSession(msg, attachments, vision)
		if err != nil {
			return nil, err
		}
		out = append(out, chatMsg)
	}
	return out, nil
}

func MessageFromSession(msg store.SessionMessage, attachments *store.AttachmentStore, vision bool) (ChatMessage, error) {
	role := msg.Role
	if role == "tool" {
		chat := ChatMessage{
			Role:       "tool",
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
			Name:       msg.ToolName,
		}
		if chat.ToolCallID == "" {
			chat.Role = "user"
			chat.Content = "[tool result]\n" + msg.Content
		}
		return chat, nil
	}

	if role == "assistant" && len(msg.ToolCalls) > 0 {
		calls := make([]FunctionToolCall, len(msg.ToolCalls))
		for i, tc := range msg.ToolCalls {
			calls[i] = NewFunctionToolCall(tc.ID, tc.Name, tc.Arguments)
		}
		content := msg.Content
		return ChatMessage{Role: "assistant", Content: content, ToolCalls: calls}, nil
	}

	if len(msg.Attachments) == 0 || attachments == nil {
		return ChatMessage{Role: role, Content: msg.Content}, nil
	}

	var textBuilder strings.Builder
	if strings.TrimSpace(msg.Content) != "" {
		textBuilder.WriteString(strings.TrimSpace(msg.Content))
		textBuilder.WriteString("\n\n")
	}

	var imageParts []ContentPart
	for _, att := range msg.Attachments {
		data, meta, err := attachments.ReadAll(att.ID)
		if err != nil {
			return ChatMessage{}, fmt.Errorf("load attachment %q: %w", att.ID, err)
		}
		if store.IsImageMime(meta.MimeType) {
			if !vision {
				fmt.Fprintf(&textBuilder, "[Attached image: %s — current model does not support image input]\n\n", meta.Filename)
				continue
			}
			encoded := base64.StdEncoding.EncodeToString(data)
			dataURL := fmt.Sprintf("data:%s;base64,%s", meta.MimeType, encoded)
			imageParts = append(imageParts, ContentPart{
				Type:     "image_url",
				ImageURL: &ImageURL{URL: dataURL},
			})
			continue
		}
		content := string(data)
		truncated := false
		if len(data) > store.MaxTextEmbedBytes {
			content = string(data[:store.MaxTextEmbedBytes])
			truncated = true
		}
		fmt.Fprintf(&textBuilder, "[Attached file: %s]\n", meta.Filename)
		textBuilder.WriteString(content)
		if truncated {
			textBuilder.WriteString("\n...(truncated)")
		}
		textBuilder.WriteString("\n\n")
	}

	text := strings.TrimSpace(textBuilder.String())
	if len(imageParts) == 0 {
		return ChatMessage{Role: role, Content: text}, nil
	}

	parts := make([]ContentPart, 0, 1+len(imageParts))
	if text != "" {
		parts = append(parts, ContentPart{Type: "text", Text: text})
	}
	parts = append(parts, imageParts...)
	return ChatMessage{Role: role, Content: parts}, nil
}
