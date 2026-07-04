package llm

// ToolFunction is the function payload inside an OpenAI tool_call.
type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// FunctionToolCall is one tool invocation in OpenAI wire format.
type FunctionToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// NewFunctionToolCall builds an OpenAI-compatible tool_call object.
func NewFunctionToolCall(id, name, arguments string) FunctionToolCall {
	return FunctionToolCall{
		ID:   id,
		Type: "function",
		Function: ToolFunction{
			Name:      name,
			Arguments: arguments,
		},
	}
}

// ChatCompletion is the parsed result of a chat completion response.
type ChatCompletion struct {
	Content   string
	ToolCalls []FunctionToolCall
}

// HasToolCalls reports whether the assistant requested tool execution.
func (c ChatCompletion) HasToolCalls() bool {
	return len(c.ToolCalls) > 0
}
