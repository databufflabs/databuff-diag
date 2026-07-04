package llm

import (
	"encoding/json"
)

// ParseStreamToolCalls merges tool_call deltas from a streaming chunk into acc.
func ParseStreamToolCalls(data string, acc map[int]*FunctionToolCall) (content string, err error) {
	var chunk struct {
		Choices []struct {
			Delta struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					Index    int    `json:"index"`
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return "", err
	}
	if len(chunk.Choices) == 0 {
		return "", nil
	}
	delta := chunk.Choices[0].Delta
	for _, tc := range delta.ToolCalls {
		slot, ok := acc[tc.Index]
		if !ok {
			slot = &FunctionToolCall{Type: "function"}
			acc[tc.Index] = slot
		}
		if tc.ID != "" {
			slot.ID = tc.ID
		}
		if tc.Type != "" {
			slot.Type = tc.Type
		}
		if slot.Type == "" {
			slot.Type = "function"
		}
		if tc.Function.Name != "" {
			slot.Function.Name = tc.Function.Name
		}
		slot.Function.Arguments += tc.Function.Arguments
	}
	return delta.Content, nil
}

// ToolCallsFromAccumulator returns ordered tool calls from a stream accumulator.
func ToolCallsFromAccumulator(acc map[int]*FunctionToolCall) []FunctionToolCall {
	if len(acc) == 0 {
		return nil
	}
	max := -1
	for i := range acc {
		if i > max {
			max = i
		}
	}
	out := make([]FunctionToolCall, 0, len(acc))
	for i := 0; i <= max; i++ {
		if tc, ok := acc[i]; ok && tc.Function.Name != "" {
			if tc.Type == "" {
				tc.Type = "function"
			}
			out = append(out, *tc)
		}
	}
	return out
}
