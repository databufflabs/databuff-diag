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
	Content   string
	Done      bool
	ToolCalls []FunctionToolCall
}

// StreamResult is the assembled result of a streaming chat completion.
type StreamResult struct {
	Content   string
	ToolCalls []FunctionToolCall
}

// ChatStream posts a streaming chat completion and invokes onChunk for each text delta.
// Tool calls are accumulated and returned via the final done chunk metadata in onChunk
// through StreamChunk.ToolCalls when Done is true.
func (c *Client) ChatStream(ctx context.Context, provider MergedProvider, req ChatRequest, onChunk func(StreamChunk) error) error {
	_, err := c.ChatStreamCollect(ctx, provider, req, onChunk)
	return err
}

// ChatStreamCollect streams a chat completion and returns the full assembled response.
func (c *Client) ChatStreamCollect(ctx context.Context, provider MergedProvider, req ChatRequest, onChunk func(StreamChunk) error) (StreamResult, error) {
	if provider.BaseURL == "" {
		return StreamResult{}, fmt.Errorf("provider %q missing base_url", provider.ProviderCode)
	}
	if req.Model == "" {
		req.Model = provider.Model
	}
	if req.Model == "" {
		return StreamResult{}, fmt.Errorf("provider %q missing model", provider.ProviderCode)
	}
	req.Stream = true

	payload, err := json.Marshal(req)
	if err != nil {
		return StreamResult{}, fmt.Errorf("marshal chat request: %w", err)
	}

	url := strings.TrimRight(provider.BaseURL, "/")
	if !strings.HasSuffix(url, "/chat/completions") {
		url += "/chat/completions"
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return StreamResult{}, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if provider.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}

	resp, err := c.http().Do(httpReq)
	if err != nil {
		return StreamResult{}, fmt.Errorf("chat stream request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return StreamResult{}, fmt.Errorf("chat stream failed: status %d %s", resp.StatusCode, string(body))
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return StreamResult{}, fmt.Errorf("read response: %w", err)
		}
		completion, err := ExtractCompletion(provider, body)
		if err != nil {
			return StreamResult{}, err
		}
		if completion.Content != "" && onChunk != nil {
			if err := onChunk(StreamChunk{Content: completion.Content}); err != nil {
				return StreamResult{}, err
			}
		}
		if onChunk != nil {
			_ = onChunk(StreamChunk{Done: true, ToolCalls: completion.ToolCalls})
		}
		return StreamResult{Content: completion.Content, ToolCalls: completion.ToolCalls}, nil
	}

	var full strings.Builder
	toolAcc := make(map[int]*FunctionToolCall)

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
		delta, err := ParseStreamToolCalls(data, toolAcc)
		if err != nil {
			continue
		}
		if delta != "" {
			full.WriteString(delta)
			if onChunk != nil {
				if err := onChunk(StreamChunk{Content: delta}); err != nil {
					return StreamResult{}, err
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return StreamResult{}, fmt.Errorf("read stream: %w", err)
	}

	toolCalls := ToolCallsFromAccumulator(toolAcc)
	if onChunk != nil {
		if err := onChunk(StreamChunk{Done: true, ToolCalls: toolCalls}); err != nil {
			return StreamResult{}, err
		}
	}
	return StreamResult{Content: full.String(), ToolCalls: toolCalls}, nil
}
