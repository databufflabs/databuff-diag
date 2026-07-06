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

// Regression for customer ultra gateway: URL must be posted as-is, matching
// OpenAIService.sendMessage (no /chat/completions suffix).
func TestUltraGateway_RegressionExactUserScenario(t *testing.T) {
	const model = "qwen2-72b"
	const aisPath = "/apis/ais/" + model

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == aisPath+"/chat/completions" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"code":"invalid_model","message":"` + aisPath + `/chat/completions not found, api not registered"}}`))
			return
		}
		if r.URL.Path != aisPath {
			t.Fatalf("unexpected path %q, want %q", r.URL.Path, aisPath)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %q, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization = %q", got)
		}
		if ct := r.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Fatalf("content-type = %q", ct)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		if payload["model"] != model {
			t.Fatalf("model = %v, want %q", payload["model"], model)
		}
		if payload["stream"] != false {
			t.Fatalf("stream = %v, want false", payload["stream"])
		}
		msgs, ok := payload["messages"].([]any)
		if !ok || len(msgs) == 0 {
			t.Fatalf("messages = %v", payload["messages"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"gateway-ok"}`))
	}))
	defer srv.Close()

	provider := MergedProvider{
		ProviderCode:      "custom",
		BaseURL:           srv.URL + aisPath,
		Model:             model,
		APIKey:            "test-key",
		ResponseProcessor: "databuff_ultra_result",
	}

	if got := chatEndpointURL(provider); got != srv.URL+aisPath {
		t.Fatalf("chatEndpointURL = %q, want %q", got, srv.URL+aisPath)
	}

	resp, err := NewClient().Chat(context.Background(), provider, ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: "reply ok"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Content != "gateway-ok" {
		t.Fatalf("content = %q, want gateway-ok", resp.Content)
	}
}

func TestUltraGateway_OpenAICompatWouldStillAppendSuffix(t *testing.T) {
	provider := MergedProvider{
		BaseURL:           "https://gateway.example/apis/ais/qwen2-72b",
		ResponseProcessor: "openai_compat",
	}
	want := "https://gateway.example/apis/ais/qwen2-72b/chat/completions"
	if got := chatEndpointURL(provider); got != want {
		t.Fatalf("openai_compat still appends suffix: got %q want %q", got, want)
	}
}
