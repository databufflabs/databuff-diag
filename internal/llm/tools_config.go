package llm

// ApplyProviderChatOptions adjusts the outbound chat request for the provider (ultra messages, tools toggle).
func ApplyProviderChatOptions(provider MergedProvider, req *ChatRequest) {
	prepareUltraChatRequest(provider, req)
	if provider.ToolsEnabled != nil && !*provider.ToolsEnabled {
		req.Tools = nil
		req.ToolChoice = nil
	}
}
