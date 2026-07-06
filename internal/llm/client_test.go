package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_Chat_OpenAICompat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}`))
	}))
	defer srv.Close()

	client := NewClient()
	provider := MergedProvider{
		ProviderCode: "test",
		BaseURL:      srv.URL + "/v1",
		Model:        "test-model",
		WireAPI:      "openai_compat",
	}

	resp, err := client.Chat(context.Background(), provider, ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "pong" {
		t.Fatalf("content = %q, want pong", resp.Content)
	}
}

func TestClient_Chat_DatabuffUltraResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apis/ais/qwen-72b" {
			t.Fatalf("path = %q, want /apis/ais/qwen-72b", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"ultra-ok"}`))
	}))
	defer srv.Close()

	client := NewClient()
	provider := MergedProvider{
		ProviderCode:      "boc-gateway",
		BaseURL:           srv.URL + "/apis/ais/qwen-72b",
		Model:             "qwen-72b",
		WireAPI:           "openai_compat",
		ResponseProcessor: "databuff_ultra_result",
	}

	resp, err := client.Chat(context.Background(), provider, ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "ultra-ok" {
		t.Fatalf("content = %q, want ultra-ok", resp.Content)
	}
}

func TestClient_Chat_DatabuffUltraResult_Escaped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apis/ais/qwen-72b" {
			t.Fatalf("path = %q, want /apis/ais/qwen-72b", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{\"result\":\"escaped-ok\"}`))
	}))
	defer srv.Close()

	client := NewClient()
	provider := MergedProvider{
		ProviderCode:      "boc-gateway",
		BaseURL:           srv.URL + "/apis/ais/qwen-72b",
		Model:             "qwen-72b",
		ResponseProcessor: "databuff_ultra_result",
	}

	resp, err := client.Chat(context.Background(), provider, ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: "ping"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "escaped-ok" {
		t.Fatalf("content = %q, want escaped-ok", resp.Content)
	}
}

func TestProcessorFor_AnthropicDefault(t *testing.T) {
	proc, err := ProcessorFor(MergedProvider{WireAPI: "anthropic"})
	if err != nil {
		t.Fatalf("ProcessorFor: %v", err)
	}
	if proc.ID() != "anthropic_messages" {
		t.Fatalf("processor = %q, want anthropic_messages", proc.ID())
	}
}

func TestExtractResponse_NonOK(t *testing.T) {
	_, err := ExtractResponse(MergedProvider{}, 500, []byte(`error`))
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}
