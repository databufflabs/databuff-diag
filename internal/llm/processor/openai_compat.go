package processor

import (
	"encoding/json"
	"fmt"
)

// OpenAICompat extracts choices[0].message.content from OpenAI-compatible responses.
type OpenAICompat struct{}

func (OpenAICompat) ID() string          { return "openai_compat" }
func (OpenAICompat) Description() string { return "OpenAI-compatible chat completions (choices[0].message.content)" }

func (OpenAICompat) Extract(body []byte) (string, error) {
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("openai_compat: parse response: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("openai_compat: missing choices")
	}
	content := resp.Choices[0].Message.Content
	if content == "" {
		content = resp.Choices[0].Delta.Content
	}
	if content == "" {
		return "", fmt.Errorf("openai_compat: missing message content")
	}
	return content, nil
}
