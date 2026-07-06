package llm

import "testing"

func TestChatEndpointURL(t *testing.T) {
	tests := []struct {
		name     string
		provider MergedProvider
		want     string
	}{
		{
			name: "openai appends chat completions",
			provider: MergedProvider{
				BaseURL:           "https://api.example.com/v1",
				ResponseProcessor: "openai_compat",
			},
			want: "https://api.example.com/v1/chat/completions",
		},
		{
			name: "openai keeps full endpoint",
			provider: MergedProvider{
				BaseURL: "https://api.example.com/v1/chat/completions",
			},
			want: "https://api.example.com/v1/chat/completions",
		},
		{
			name: "ultra ais endpoint as-is",
			provider: MergedProvider{
				BaseURL:           "https://gateway.example/apis/ais/qwen2-72b",
				ResponseProcessor: "databuff_ultra_result",
			},
			want: "https://gateway.example/apis/ais/qwen2-72b",
		},
		{
			name: "ultra full chat completions endpoint",
			provider: MergedProvider{
				BaseURL:           "https://internal-llm/v1/chat/completions",
				ResponseProcessor: "databuff_ultra_result",
			},
			want: "https://internal-llm/v1/chat/completions",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := chatEndpointURL(tt.provider); got != tt.want {
				t.Fatalf("chatEndpointURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
