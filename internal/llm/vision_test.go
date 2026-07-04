package llm

import "testing"

func TestProviderSupportsVision_DeepSeek(t *testing.T) {
	p := MergedProvider{ProviderCode: "deepseek", Model: "deepseek-v4-flash"}
	if ProviderSupportsVision(p) {
		t.Fatal("deepseek-v4-flash should not support vision")
	}
}

func TestProviderSupportsVision_OpenAI(t *testing.T) {
	p := MergedProvider{ProviderCode: "openai", Model: "gpt-4o"}
	if !ProviderSupportsVision(p) {
		t.Fatal("gpt-4o should support vision")
	}
}

func TestProviderSupportsVision_Anthropic(t *testing.T) {
	p := MergedProvider{ProviderCode: "anthropic", Model: "claude-sonnet-4-20250514"}
	if !ProviderSupportsVision(p) {
		t.Fatal("claude-sonnet should support vision")
	}
}

func TestProviderSupportsVision_ConfigOverride(t *testing.T) {
	yes := true
	no := false

	p := MergedProvider{ProviderCode: "deepseek", Model: "deepseek-v4-flash", SupportsVision: &yes}
	if !ProviderSupportsVision(p) {
		t.Fatal("supports_vision=true should override heuristic")
	}

	p = MergedProvider{ProviderCode: "openai", Model: "gpt-4o", SupportsVision: &no}
	if ProviderSupportsVision(p) {
		t.Fatal("supports_vision=false should override heuristic")
	}
}

func TestVisionUnsupportedError(t *testing.T) {
	err := VisionUnsupportedError(MergedProvider{Model: "deepseek-v4-flash"})
	if err.Error() == "" {
		t.Fatal("expected non-empty error")
	}
}
