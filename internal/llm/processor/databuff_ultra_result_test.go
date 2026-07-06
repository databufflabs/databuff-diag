package processor

import (
	"encoding/json"
	"testing"
)

func TestDatabuffUltraResult_Standard(t *testing.T) {
	body := []byte(`{"result":"ok"}`)
	got, err := (&DatabuffUltraResult{}).Extract(body)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if got != "ok" {
		t.Fatalf("content = %q, want ok", got)
	}
}

func TestDatabuffUltraResult_Escaped(t *testing.T) {
	body := []byte(`{\"result\":\"ok\"}`)
	got, err := (&DatabuffUltraResult{}).Extract(body)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if got != "ok" {
		t.Fatalf("content = %q, want ok", got)
	}
}

func TestDatabuffUltraResult_MissingResult(t *testing.T) {
	body := []byte(`{"code":0}`)
	_, err := (&DatabuffUltraResult{}).Extract(body)
	if err == nil {
		t.Fatal("expected error for missing result field")
	}
}

func TestDatabuffUltraResult_EmptyBody(t *testing.T) {
	_, err := (&DatabuffUltraResult{}).Extract([]byte("  "))
	if err == nil {
		t.Fatal("expected error for empty body")
	}
}

func TestDatabuffUltraResult_ToolCallsInResultJSON(t *testing.T) {
	inner := `{"choices":[{"message":{"tool_calls":[{"id":"call-1","type":"function","function":{"name":"bash","arguments":"{\"command\":\"docker ps\"}"}}]}}]}`
	body := []byte(`{"result":` + jsonString(inner) + `}`)
	c, err := (&DatabuffUltraResult{}).ExtractCompletion(body)
	if err != nil {
		t.Fatalf("ExtractCompletion: %v", err)
	}
	if len(c.ToolCalls) != 1 || c.ToolCalls[0].Name != "bash" {
		t.Fatalf("tool_calls = %#v", c.ToolCalls)
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestOpenAICompat_Extract(t *testing.T) {
	body := []byte(`{"choices":[{"message":{"content":"hello"}}]}`)
	got, err := (&OpenAICompat{}).Extract(body)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if got != "hello" {
		t.Fatalf("content = %q, want hello", got)
	}
}

func TestAnthropicMessages_Extract(t *testing.T) {
	body := []byte(`{"content":[{"type":"text","text":"hi"}]}`)
	got, err := (&AnthropicMessages{}).Extract(body)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if got != "hi" {
		t.Fatalf("content = %q, want hi", got)
	}
}

func TestRegistry_ListAndGet(t *testing.T) {
	list := List()
	if len(list) < 3 {
		t.Fatalf("List len = %d, want >= 3", len(list))
	}

	for _, id := range []string{"openai_compat", "anthropic_messages", "databuff_ultra_result"} {
		p, err := Get(id)
		if err != nil {
			t.Fatalf("Get(%q): %v", id, err)
		}
		if p.ID() != id {
			t.Fatalf("ID = %q, want %q", p.ID(), id)
		}
	}
}

func TestResolve_Defaults(t *testing.T) {
	p, err := Resolve("", "openai_compat")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.ID() != "openai_compat" {
		t.Fatalf("processor = %q, want openai_compat", p.ID())
	}

	p, err = Resolve("", "anthropic")
	if err != nil {
		t.Fatalf("Resolve anthropic: %v", err)
	}
	if p.ID() != "anthropic_messages" {
		t.Fatalf("processor = %q, want anthropic_messages", p.ID())
	}
}

func TestResolve_Explicit(t *testing.T) {
	p, err := Resolve("databuff_ultra_result", "openai_compat")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.ID() != "databuff_ultra_result" {
		t.Fatalf("processor = %q, want databuff_ultra_result", p.ID())
	}
}

func TestGet_Unknown(t *testing.T) {
	_, err := Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown processor")
	}
}
