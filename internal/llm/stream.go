package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// StreamChunk is one assistant text delta from a streaming chat call.
type StreamChunk struct {
	Content string
	Done    bool
}

// ChatStream posts a streaming chat completion and invokes onChunk for each delta.
func (c *Client) ChatStream(ctx context.Context, provider MergedProvider, req ChatRequest, onChunk func(StreamChunk) error) error {
	if provider.BaseURL == "" {
		return fmt.Errorf("provider %q missing base_url", provider.ProviderCode)
	}
	if req.Model == "" {
		req.Model = provider.Model
	}
	if req.Model == "" {
		return fmt.Errorf("provider %q missing model", provider.ProviderCode)
	}
	req.Stream = true

	payload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal chat request: %w", err)
	}

	url := strings.TrimRight(provider.BaseURL, "/")
	if !strings.HasSuffix(url, "/chat/completions") {
		url += "/chat/completions"
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if provider.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}

	resp, err := c.http().Do(httpReq)
	if err != nil {
		return fmt.Errorf("chat stream request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("chat stream failed: status %d %s", resp.StatusCode, string(body))
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read response: %w", err)
		}
		content, err := ExtractResponse(provider, resp.StatusCode, body)
		if err != nil {
			return err
		}
		if content != "" {
			if err := onChunk(StreamChunk{Content: content}); err != nil {
				return err
			}
		}
		return onChunk(StreamChunk{Done: true})
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		delta, err := extractStreamDelta(data)
		if err != nil || delta == "" {
			continue
		}
		if err := onChunk(StreamChunk{Content: delta}); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read stream: %w", err)
	}
	return onChunk(StreamChunk{Done: true})
}

func extractStreamDelta(data string) (string, error) {
	var chunk struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return "", err
	}
	if len(chunk.Choices) == 0 {
		return "", nil
	}
	if chunk.Choices[0].Delta.Content != "" {
		return chunk.Choices[0].Delta.Content, nil
	}
	return chunk.Choices[0].Message.Content, nil
}
