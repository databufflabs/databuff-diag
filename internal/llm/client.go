package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/databufflabs/databuff-diag/internal/llm/processor"
)

// ChatMessage is a single message in a chat request.
// Content is a string for plain text, or []ContentPart for multimodal messages.
type ChatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// ChatRequest is the OpenAI-shaped chat completion payload.
type ChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

// ChatResponse is the extracted assistant reply from a chat call.
type ChatResponse struct {
	Content string
}

// Client sends chat requests to configured LLM providers.
type Client struct {
	HTTPClient *http.Client
}

// NewClient returns a client with sensible defaults.
func NewClient() *Client {
	return &Client{
		HTTPClient: &http.Client{Timeout: 120 * time.Second},
	}
}

func (c *Client) http() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

// ProcessorFor returns the response processor configured on the provider.
func ProcessorFor(provider MergedProvider) (processor.Processor, error) {
	return processor.Resolve(provider.ResponseProcessor, provider.WireAPI)
}

// Chat posts a non-streaming chat completion and extracts assistant text
// using the provider's response_processor (or wire_api default).
func (c *Client) Chat(ctx context.Context, provider MergedProvider, req ChatRequest) (*ChatResponse, error) {
	if provider.BaseURL == "" {
		return nil, fmt.Errorf("provider %q missing base_url", provider.ProviderCode)
	}
	if req.Model == "" {
		req.Model = provider.Model
	}
	if req.Model == "" {
		return nil, fmt.Errorf("provider %q missing model", provider.ProviderCode)
	}
	req.Stream = false

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal chat request: %w", err)
	}

	url := strings.TrimRight(provider.BaseURL, "/")
	if !strings.HasSuffix(url, "/chat/completions") {
		url += "/chat/completions"
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if provider.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}

	resp, err := c.http().Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("chat request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chat request failed: status %d %s", resp.StatusCode, string(body))
	}

	proc, err := ProcessorFor(provider)
	if err != nil {
		return nil, err
	}

	content, err := proc.Extract(body)
	if err != nil {
		return nil, fmt.Errorf("extract response: %w", err)
	}
	return &ChatResponse{Content: content}, nil
}

// ExtractResponse parses an HTTP response body with the provider's processor.
func ExtractResponse(provider MergedProvider, statusCode int, body []byte) (string, error) {
	if statusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d: %s", statusCode, string(body))
	}
	proc, err := ProcessorFor(provider)
	if err != nil {
		return "", err
	}
	return proc.Extract(body)
}
