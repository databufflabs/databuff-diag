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

// BuildMessagesFromSession converts stored session messages to LLM chat messages.
// When vision is false, image attachments are replaced with text placeholders so
// text-only providers (e.g. DeepSeek) do not receive image_url content parts.
func BuildMessagesFromSession(session *store.Session, systemPrompt string, attachments *store.AttachmentStore, vision bool) ([]ChatMessage, error) {
	out := []ChatMessage{{Role: "system", Content: systemPrompt}}
	for _, msg := range session.Messages {
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
		role = "user"
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
