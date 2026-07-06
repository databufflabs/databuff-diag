package llm

import (
	"fmt"
	"strings"
)

// UsesUltraGateway reports whether outbound requests need ultra AIS
// message shaping (no system role; last message must be user).
func UsesUltraGateway(provider MergedProvider) bool {
	return provider.ResponseProcessor == "databuff_ultra_result"
}

func prepareUltraChatRequest(provider MergedProvider, req *ChatRequest) {
	if !UsesUltraGateway(provider) {
		return
	}
	req.Stream = false
	req.Messages = AdaptUltraMessages(req.Messages)
}

// AdaptUltraMessages reshapes messages for ultra AIS gateways that reject the
// system role and require the final message to be from the user.
func AdaptUltraMessages(messages []ChatMessage) []ChatMessage {
	if len(messages) == 0 {
		return messages
	}

	var pendingSystem []string
	out := make([]ChatMessage, 0, len(messages))

	flushSystem := func() {
		if len(pendingSystem) == 0 {
			return
		}
		out = append(out, ChatMessage{
			Role:    "user",
			Content: strings.Join(pendingSystem, "\n\n"),
		})
		pendingSystem = nil
	}

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			if text := messageTextContent(msg.Content); text != "" {
				pendingSystem = append(pendingSystem, text)
			}
		case "user":
			text := messageTextContent(msg.Content)
			if len(pendingSystem) > 0 {
				text = strings.Join(pendingSystem, "\n\n") + "\n\n" + text
				pendingSystem = nil
			}
			out = append(out, ChatMessage{Role: "user", Content: text})
		case "assistant":
			flushSystem()
			out = append(out, ChatMessage{
				Role:      "assistant",
				Content:   msg.Content,
				ToolCalls: msg.ToolCalls,
			})
		case "tool":
			flushSystem()
			out = append(out, ChatMessage{
				Role:    "user",
				Content: "[tool result]\n" + messageTextContent(msg.Content),
			})
		default:
			flushSystem()
			out = append(out, ChatMessage{
				Role:    "user",
				Content: messageTextContent(msg.Content),
			})
		}
	}
	flushSystem()

	if len(out) == 0 {
		return out
	}
	if out[len(out)-1].Role != "user" {
		out = append(out, ChatMessage{Role: "user", Content: "请继续。"})
	}
	return mergeConsecutiveUltraUsers(out)
}

func mergeConsecutiveUltraUsers(messages []ChatMessage) []ChatMessage {
	if len(messages) == 0 {
		return messages
	}
	out := []ChatMessage{messages[0]}
	for i := 1; i < len(messages); i++ {
		cur := messages[i]
		if cur.Role == "user" && out[len(out)-1].Role == "user" {
			prev := messageTextContent(out[len(out)-1].Content)
			next := messageTextContent(cur.Content)
			if prev == "" {
				out[len(out)-1].Content = next
			} else if next == "" {
				out[len(out)-1].Content = prev
			} else {
				out[len(out)-1].Content = prev + "\n\n" + next
			}
			continue
		}
		out = append(out, cur)
	}
	return out
}

func messageTextContent(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []ContentPart:
		var b strings.Builder
		for _, part := range v {
			if part.Type == "text" && part.Text != "" {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(part.Text)
			}
		}
		return b.String()
	default:
		if content == nil {
			return ""
		}
		return fmt.Sprint(content)
	}
}
