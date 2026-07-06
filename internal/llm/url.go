package llm

import "strings"

// chatEndpointURL returns the POST URL for a chat request.
//
// OpenAI-compatible cloud APIs use base_url + /chat/completions unless the suffix
// is already present.
//
// DataBuff ultra gateways follow OpenAIService.sendMessage in
// ultra_2.10.2-databuff/.../OpenAIService.java: the configured URL is posted
// as-is (e.g. https://host/apis/ais/qwen2-72b), never with /chat/completions
// appended.
func chatEndpointURL(provider MergedProvider) string {
	url := strings.TrimRight(provider.BaseURL, "/")
	if strings.HasSuffix(url, "/chat/completions") {
		return url
	}
	if provider.ResponseProcessor == "databuff_ultra_result" {
		return url
	}
	return url + "/chat/completions"
}
