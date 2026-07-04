package processor

import (
	"encoding/json"
	"fmt"
)

// AnthropicMessages extracts content[0].text from Anthropic Messages API responses.
type AnthropicMessages struct{}

func (AnthropicMessages) ID() string          { return "anthropic_messages" }
func (AnthropicMessages) Description() string { return "Anthropic Messages API (content[0].text)" }

func (AnthropicMessages) Extract(body []byte) (string, error) {
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("anthropic_messages: parse response: %w", err)
	}
	if len(resp.Content) == 0 {
		return "", fmt.Errorf("anthropic_messages: missing content")
	}
	text := resp.Content[0].Text
	if text == "" {
		return "", fmt.Errorf("anthropic_messages: missing text in content[0]")
	}
	return text, nil
}
