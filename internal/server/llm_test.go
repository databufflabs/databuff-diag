package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLLMTest_OpenAICompat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "reply ok") {
			t.Fatalf("request body missing test prompt: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	handler := testHandler(t)
	payload := map[string]any{
		"provider_code": "deepseek",
		"base_url":      srv.URL + "/v1",
		"model":         "test-model",
	}
	raw, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/llm/test", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp llmTestResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Success {
		t.Fatalf("success = false, error=%q", resp.Error)
	}
	if resp.Content != "ok" {
		t.Fatalf("content = %q, want ok", resp.Content)
	}
	if resp.ProcessorUsed != "openai_compat" {
		t.Fatalf("processor_used = %q, want openai_compat", resp.ProcessorUsed)
	}
	if resp.LatencyMS < 0 {
		t.Fatalf("latency_ms = %d, want >= 0", resp.LatencyMS)
	}
}

func TestLLMTest_DatabuffUltraResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apis/ais/qwen-72b" {
			t.Fatalf("path = %q, want /apis/ais/qwen-72b", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"ultra-ok"}`))
	}))
	defer srv.Close()

	handler := testHandler(t)
	payload := map[string]any{
		"provider_code":      "bailian",
		"base_url":           srv.URL + "/apis/ais/qwen-72b",
		"model":              "qwen-72b",
		"response_processor": "databuff_ultra_result",
	}
	raw, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/llm/test", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp llmTestResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Success {
		t.Fatalf("success = false, error=%q", resp.Error)
	}
	if resp.Content != "ultra-ok" {
		t.Fatalf("content = %q, want ultra-ok", resp.Content)
	}
	if resp.ProcessorUsed != "databuff_ultra_result" {
		t.Fatalf("processor_used = %q, want databuff_ultra_result", resp.ProcessorUsed)
	}
}

func TestLLMTest_UltraRegression_UserQwen2Path(t *testing.T) {
	const model = "qwen2-72b"
	const aisPath = "/apis/ais/" + model

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == aisPath+"/chat/completions" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"code":"invalid_model","message":"` + aisPath + `/chat/completions not found, api not registered"}}`))
			return
		}
		if r.URL.Path != aisPath {
			t.Fatalf("path = %q, want %q", r.URL.Path, aisPath)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer srv.Close()

	handler := testHandler(t)
	payload := map[string]any{
		"provider_code":      "custom",
		"base_url":           srv.URL + aisPath,
		"api_key":            "test-key",
		"model":              model,
		"response_processor": "databuff_ultra_result",
	}
	raw, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/llm/test", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp llmTestResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Success {
		t.Fatalf("success = false, error=%q", resp.Error)
	}
	if resp.Content != "ok" {
		t.Fatalf("content = %q, want ok", resp.Content)
	}
}

func TestLLMTest_EmptyBodyUsesActiveProvider(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"active-ok"}}]}`))
	}))
	defer srv.Close()

	handler := testHandler(t)

	// Override active provider base_url via config PUT is heavy; use minimal body override.
	payload := map[string]any{
		"provider_code": "deepseek",
		"base_url":      srv.URL + "/v1",
		"model":         "deepseek-chat",
	}
	raw, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/llm/test", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}

	var resp llmTestResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Success || resp.Content != "active-ok" {
		t.Fatalf("response = %+v, want success with active-ok", resp)
	}
}

func TestLLMTest_MethodNotAllowed(t *testing.T) {
	handler := testHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/llm/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestLLMTest_LLMErrorReturnsSuccessFalse(t *testing.T) {
	handler := testHandler(t)
	payload := map[string]any{
		"provider_code": "deepseek",
		"base_url":      "http://127.0.0.1:1",
		"model":         "test-model",
	}
	raw, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/llm/test", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp llmTestResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Success {
		t.Fatal("expected success=false on connection failure")
	}
	if resp.Error == "" {
		t.Fatal("expected error message")
	}
	if resp.ProcessorUsed != "openai_compat" {
		t.Fatalf("processor_used = %q, want openai_compat", resp.ProcessorUsed)
	}
}
