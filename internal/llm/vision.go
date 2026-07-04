package llm

import (
	"strings"
)

// ProviderSupportsVision reports whether the active provider/model accepts
// OpenAI-style image_url content parts in chat messages.
func ProviderSupportsVision(p MergedProvider) bool {
	if p.SupportsVision != nil {
		return *p.SupportsVision
	}
	return modelSupportsVision(p.ProviderCode, p.Model)
}

func modelSupportsVision(providerCode, model string) bool {
	code := strings.ToLower(strings.TrimSpace(providerCode))
	model = strings.ToLower(strings.TrimSpace(model))

	switch code {
	case "deepseek", "groq", "together", "minimax", "qianfan", "moonshot":
		return false
	case "ollama":
		return ollamaModelSupportsVision(model)
	case "openai", "openrouter":
		return openAIModelSupportsVision(model)
	case "anthropic":
		return anthropicModelSupportsVision(model)
	case "bailian":
		return bailianModelSupportsVision(model)
	case "zhipu":
		return zhipuModelSupportsVision(model)
	case "custom":
		return customModelSupportsVision(model)
	default:
		return genericModelSupportsVision(model)
	}
}

func genericModelSupportsVision(model string) bool {
	if model == "" {
		return false
	}
	return strings.Contains(model, "vision") ||
		strings.Contains(model, "vl") ||
		strings.Contains(model, "4o") ||
		strings.Contains(model, "llava")
}

func openAIModelSupportsVision(model string) bool {
	if model == "" {
		return true
	}
	if strings.Contains(model, "gpt-3.5") {
		return false
	}
	return genericModelSupportsVision(model) ||
		strings.HasPrefix(model, "gpt-4") ||
		strings.HasPrefix(model, "gpt-5") ||
		strings.HasPrefix(model, "o1") ||
		strings.HasPrefix(model, "o3") ||
		strings.HasPrefix(model, "o4")
}

func anthropicModelSupportsVision(model string) bool {
	if model == "" {
		return true
	}
	return strings.Contains(model, "claude-3") ||
		strings.Contains(model, "claude-sonnet") ||
		strings.Contains(model, "claude-opus") ||
		strings.Contains(model, "claude-haiku")
}

func bailianModelSupportsVision(model string) bool {
	return strings.Contains(model, "vl") || strings.Contains(model, "vision")
}

func zhipuModelSupportsVision(model string) bool {
	return strings.Contains(model, "glm-4v") ||
		strings.Contains(model, "glm-5v") ||
		strings.Contains(model, "vision")
}

func ollamaModelSupportsVision(model string) bool {
	return strings.Contains(model, "vision") ||
		strings.Contains(model, "llava") ||
		strings.Contains(model, "moondream") ||
		strings.Contains(model, "bakllava")
}

func customModelSupportsVision(model string) bool {
	return genericModelSupportsVision(model)
}

// VisionUnsupportedError is returned when a chat request includes images but the
// active provider/model only accepts text content.
func VisionUnsupportedError(provider MergedProvider) error {
	model := provider.Model
	if model == "" {
		model = provider.ProviderCode
	}
	return &visionUnsupportedError{model: model}
}

type visionUnsupportedError struct {
	model string
}

func (e *visionUnsupportedError) Error() string {
	return "当前模型 " + e.model + " 不支持图片输入，请在设置中切换到支持视觉的模型（如 gpt-5.5、claude-sonnet-5 或 qwen-vl）"
}
