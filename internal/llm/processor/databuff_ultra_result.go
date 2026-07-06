package processor

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DatabuffUltraResult extracts the top-level "result" field from DataBuff ultra gateways.
// Body may be double-escaped (starts with {\"); aligned with OpenAIService.parseResponseBodyToMap.
type DatabuffUltraResult struct{}

func (DatabuffUltraResult) ID() string          { return "databuff_ultra_result" }
func (DatabuffUltraResult) Description() string { return "DataBuff ultra gateway (top-level result field)" }

func (DatabuffUltraResult) Extract(body []byte) (string, error) {
	c, err := DatabuffUltraResult{}.ExtractCompletion(body)
	if err != nil {
		return "", err
	}
	if c.Content == "" && len(c.ToolCalls) > 0 {
		return "", nil
	}
	if c.Content == "" && len(c.ToolCalls) == 0 {
		return "", fmt.Errorf("databuff_ultra_result: missing result field")
	}
	return c.Content, nil
}

func (DatabuffUltraResult) ExtractCompletion(body []byte) (Completion, error) {
	result, err := extractUltraResultString(body)
	if err != nil {
		return Completion{}, err
	}
	trimmed := strings.TrimSpace(result)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		if c, err := (OpenAICompat{}).ExtractCompletion([]byte(trimmed)); err == nil {
			if c.Content != "" || len(c.ToolCalls) > 0 {
				return c, nil
			}
		}
	}
	return Completion{Content: result}, nil
}

func extractUltraResultString(body []byte) (string, error) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "", fmt.Errorf("databuff_ultra_result: empty response body")
	}

	toParse := trimmed
	if isEscapedUltraJSON(trimmed) {
		toParse = unescapeUltraJSON(trimmed)
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(toParse), &m); err != nil {
		return "", fmt.Errorf("databuff_ultra_result: parse response: %w", err)
	}

	raw, ok := m["result"]
	if !ok || raw == nil {
		return "", fmt.Errorf("databuff_ultra_result: missing result field")
	}

	content, ok := raw.(string)
	if !ok {
		content = fmt.Sprint(raw)
	}
	return content, nil
}

func isEscapedUltraJSON(trimmed string) bool {
	return strings.HasPrefix(trimmed, `{\"`)
}

func unescapeUltraJSON(s string) string {
	const ph = "\x01"
	return strings.ReplaceAll(
		strings.ReplaceAll(
			strings.ReplaceAll(s, `\\`, ph),
			`\"`, `"`),
		ph, `\`)
}
