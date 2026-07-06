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

func TestApplyProviderChatOptions_StripsToolsWhenDisabled(t *testing.T) {
	no := false
	provider := MergedProvider{ToolsEnabled: &no}
	req := ChatRequest{
		Tools:      AgentTools(),
		ToolChoice: "auto",
		Messages:   []ChatMessage{{Role: "user", Content: "hi"}},
	}
	ApplyProviderChatOptions(provider, &req)
	if len(req.Tools) != 0 || req.ToolChoice != nil {
		t.Fatalf("tools should be stripped: tools=%d choice=%v", len(req.Tools), req.ToolChoice)
	}
}

func TestApplyProviderChatOptions_KeepsToolsByDefault(t *testing.T) {
	provider := MergedProvider{}
	req := ChatRequest{
		Tools:      AgentTools(),
		ToolChoice: "auto",
		Messages:   []ChatMessage{{Role: "user", Content: "hi"}},
	}
	ApplyProviderChatOptions(provider, &req)
	if len(req.Tools) == 0 || req.ToolChoice == nil {
		t.Fatalf("tools should be kept by default: tools=%d choice=%v", len(req.Tools), req.ToolChoice)
	}
}

func TestClient_Chat_StripsToolsWhenDisabled(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	no := false
	client := NewClient()
	provider := MergedProvider{
		BaseURL:      srv.URL,
		Model:        "test",
		ToolsEnabled: &no,
	}

	_, err := client.Chat(context.Background(), provider, ChatRequest{
		Messages:   []ChatMessage{{Role: "user", Content: "hi"}},
		Tools:      AgentTools(),
		ToolChoice: "auto",
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if strings.Contains(gotBody, `"tools"`) {
		t.Fatalf("body should not contain tools: %s", gotBody)
	}
	if strings.Contains(gotBody, `"tool_choice"`) {
		t.Fatalf("body should not contain tool_choice: %s", gotBody)
	}
}

func TestClient_Chat_MarshalsToolsEnabledFalse(t *testing.T) {
	no := false
	provider := MergedProvider{ToolsEnabled: &no}
	req := ChatRequest{
		Messages:   []ChatMessage{{Role: "user", Content: "hi"}},
		Tools:      AgentTools(),
		ToolChoice: "auto",
	}
	ApplyProviderChatOptions(provider, &req)
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if strings.Contains(body, `"tools"`) {
		t.Fatalf("unexpected tools in payload: %s", body)
	}
}
