package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdaptUltraMessages_FirstTurnMergesSystemIntoUser(t *testing.T) {
	in := []ChatMessage{
		{Role: "system", Content: "你是排障助手"},
		{Role: "user", Content: "你是谁"},
	}
	out := AdaptUltraMessages(in)
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1", len(out))
	}
	if out[0].Role != "user" {
		t.Fatalf("role = %q, want user", out[0].Role)
	}
	text, ok := out[0].Content.(string)
	if !ok || !strings.Contains(text, "你是排障助手") || !strings.Contains(text, "你是谁") {
		t.Fatalf("content = %v", out[0].Content)
	}
}

func TestAdaptUltraMessages_NudgeAfterAssistantEndsWithUser(t *testing.T) {
	in := []ChatMessage{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "检查 docker"},
		{Role: "assistant", Content: "接下来查看："},
		{Role: "system", Content: "请给出 tool JSON"},
	}
	out := AdaptUltraMessages(in)
	if len(out) != 3 {
		t.Fatalf("len = %d, want 3; %#v", len(out), out)
	}
	if out[len(out)-1].Role != "user" {
		t.Fatalf("last role = %q, want user", out[len(out)-1].Role)
	}
	if out[len(out)-1].Content != "请给出 tool JSON" {
		t.Fatalf("last content = %v", out[len(out)-1].Content)
	}
}

func TestAdaptUltraMessages_MergesConsecutiveUsers(t *testing.T) {
	in := []ChatMessage{
		{Role: "user", Content: "a"},
		{Role: "assistant", Content: "b"},
		{Role: "user", Content: "c"},
		{Role: "user", Content: "d"},
	}
	out := AdaptUltraMessages(in)
	if len(out) != 3 {
		t.Fatalf("len = %d, want 3; %#v", len(out), out)
	}
	if out[2].Role != "user" || out[2].Content != "c\n\nd" {
		t.Fatalf("merged tail = %#v", out[2])
	}
}

func TestAdaptUltraMessages_ToolResultBecomesUser(t *testing.T) {
	in := []ChatMessage{
		{Role: "user", Content: "run"},
		{Role: "assistant", Content: "ok"},
		{Role: "tool", Content: "output", ToolCallID: "call-1", Name: "bash"},
	}
	out := AdaptUltraMessages(in)
	if out[len(out)-1].Role != "user" {
		t.Fatalf("last role = %q, want user", out[len(out)-1].Role)
	}
}

func TestAdaptUltraMessages_PreservesAssistantToolCalls(t *testing.T) {
	in := []ChatMessage{
		{Role: "user", Content: "hi"},
		{
			Role:    "assistant",
			Content: "",
			ToolCalls: []FunctionToolCall{
				NewFunctionToolCall("call-1", "bash", `{"command":"docker ps"}`),
			},
		},
	}
	out := AdaptUltraMessages(in)
	if len(out) != 3 {
		t.Fatalf("len = %d, want 3 (assistant + trailing user)", len(out))
	}
	if len(out[1].ToolCalls) != 1 || out[1].ToolCalls[0].Function.Name != "bash" {
		t.Fatalf("tool_calls not preserved: %#v", out[1].ToolCalls)
	}
	if out[2].Role != "user" {
		t.Fatalf("last role = %q, want user", out[2].Role)
	}
}

func TestPrepareUltraChatRequest_KeepsTools(t *testing.T) {
	req := ChatRequest{
		Stream:     true,
		ToolChoice: "auto",
		Tools:      AgentTools(),
		Messages: []ChatMessage{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "hi"},
		},
	}
	provider := MergedProvider{ResponseProcessor: "databuff_ultra_result"}
	ApplyProviderChatOptions(provider, &req)
	if len(req.Tools) == 0 || req.ToolChoice == nil {
		t.Fatalf("tools should be kept: tools=%d choice=%v", len(req.Tools), req.ToolChoice)
	}
	if req.Stream {
		t.Fatal("ultra gateway should force stream false")
	}
	if len(req.Messages) != 1 || req.Messages[0].Role != "user" {
		t.Fatalf("messages = %#v", req.Messages)
	}

	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, required := range []string{`"tools"`, `"tool_choice"`, `"stream":false`} {
		if !strings.Contains(body, required) {
			t.Fatalf("payload missing %s: %s", required, body)
		}
	}
	if strings.Contains(body, `"system"`) {
		t.Fatalf("payload should not contain system role: %s", body)
	}
}

func TestClient_Chat_UltraGatewayPayloadKeepsTools(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer srv.Close()

	client := NewClient()
	provider := MergedProvider{
		BaseURL:           srv.URL + "/apis/ais/qwen2-72b",
		Model:             "qwen2-72b",
		ResponseProcessor: "databuff_ultra_result",
	}

	_, err := client.Chat(context.Background(), provider, ChatRequest{
		Messages: []ChatMessage{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "你是谁"},
		},
		Tools:      AgentTools(),
		ToolChoice: "auto",
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if strings.Contains(gotBody, `"system"`) {
		t.Fatalf("body still has system role: %s", gotBody)
	}
	if !strings.Contains(gotBody, `"tools"`) {
		t.Fatalf("body missing tools: %s", gotBody)
	}
	if strings.Contains(gotBody, `"stream":true`) {
		t.Fatalf("ultra body should use stream false: %s", gotBody)
	}
}
