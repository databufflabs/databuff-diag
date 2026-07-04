package processor

import (
	"encoding/json"
	"fmt"
)

// OpenAICompat extracts choices[0].message from OpenAI-compatible responses.
type OpenAICompat struct{}

func (OpenAICompat) ID() string          { return "openai_compat" }
func (OpenAICompat) Description() string { return "OpenAI-compatible chat completions (choices[0].message.content)" }

func (OpenAICompat) Extract(body []byte) (string, error) {
	c, err := OpenAICompat{}.ExtractCompletion(body)
	if err != nil {
		return "", err
	}
	if c.Content == "" && len(c.ToolCalls) > 0 {
		return "", nil
	}
	if c.Content == "" && len(c.ToolCalls) == 0 {
		return "", fmt.Errorf("openai_compat: missing message content")
	}
	return c.Content, nil
}

func (OpenAICompat) ExtractCompletion(body []byte) (Completion, error) {
	var resp struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return Completion{}, fmt.Errorf("openai_compat: parse response: %w", err)
	}
	if len(resp.Choices) == 0 {
		return Completion{}, fmt.Errorf("openai_compat: missing choices")
	}

	msg := resp.Choices[0].Message
	content := msg.Content
	if content == "" {
		content = resp.Choices[0].Delta.Content
	}

	var toolCalls []ToolCall
	for _, tc := range msg.ToolCalls {
		if tc.Function.Name == "" {
			continue
		}
		toolCalls = append(toolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}

	return Completion{Content: content, ToolCalls: toolCalls}, nil
}
