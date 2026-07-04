package processor

// ToolCall is a function tool invocation parsed from an API response.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// Completion is the parsed result of a chat completion response.
type Completion struct {
	Content   string
	ToolCalls []ToolCall
}
