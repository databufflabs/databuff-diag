package processor

import "testing"

func TestOpenAICompat_ExtractCompletion_ToolCalls(t *testing.T) {
	body := []byte(`{
		"choices":[{"message":{
			"content":"",
			"tool_calls":[{"id":"call_1","type":"function","function":{"name":"bash","arguments":"{\"command\":\"docker ps\"}"}}]
		}}]
	}`)
	c, err := OpenAICompat{}.ExtractCompletion(body)
	if err != nil {
		t.Fatalf("ExtractCompletion: %v", err)
	}
	if len(c.ToolCalls) != 1 {
		t.Fatalf("tool calls = %+v", c.ToolCalls)
	}
	if c.ToolCalls[0].Name != "bash" || c.ToolCalls[0].Arguments == "" {
		t.Fatalf("tool call = %+v", c.ToolCalls[0])
	}
}
