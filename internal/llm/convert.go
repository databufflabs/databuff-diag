package llm

import "github.com/databufflabs/databuff-diag/internal/llm/processor"

func completionFromProcessor(c processor.Completion) ChatCompletion {
	out := ChatCompletion{Content: c.Content}
	if len(c.ToolCalls) > 0 {
		out.ToolCalls = make([]FunctionToolCall, len(c.ToolCalls))
		for i, tc := range c.ToolCalls {
			out.ToolCalls[i] = NewFunctionToolCall(tc.ID, tc.Name, tc.Arguments)
		}
	}
	return out
}
