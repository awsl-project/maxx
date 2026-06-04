package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/converter"
	"github.com/awsl-project/maxx/internal/testutil/mockserver"
	"github.com/tidwall/gjson"
)

// capturedRequest stores the last request received by a mock upstream.
type capturedRequest struct {
	mu      sync.Mutex
	Method  string
	Path    string
	Headers http.Header
	Body    []byte
}

func (c *capturedRequest) Set(r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Method = r.Method
	c.Path = r.URL.Path
	c.Headers = r.Header.Clone()
	c.Body, _ = io.ReadAll(r.Body)
}

func (c *capturedRequest) Get() (string, string, http.Header, []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Method, c.Path, c.Headers, c.Body
}

// createProvider creates a custom provider via admin API and returns the provider ID.
func createProvider(t *testing.T, env *ProxyTestEnv, name, baseURL string, supportedTypes []string) uint64 {
	t.Helper()
	resp := env.AdminPost("/api/admin/providers", map[string]any{
		"name": name,
		"type": "custom",
		"config": map[string]any{
			"custom": map[string]any{
				"baseURL": baseURL,
				"apiKey":  "sk-mock-test-key",
			},
		},
		"supportedClientTypes": supportedTypes,
	})
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to create provider %s: status=%d body=%s", name, resp.StatusCode, body)
	}
	var result struct {
		ID uint64 `json:"id"`
	}
	DecodeJSON(t, resp, &result)
	return result.ID
}

// createRoute creates a route via admin API and returns the route ID.
func createRoute(t *testing.T, env *ProxyTestEnv, clientType string, providerID uint64) uint64 {
	t.Helper()
	resp := env.AdminPost("/api/admin/routes", map[string]any{
		"isEnabled":  true,
		"clientType": clientType,
		"providerID": providerID,
		"position":   1,
	})
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to create route: status=%d body=%s", resp.StatusCode, body)
	}
	var result struct {
		ID uint64 `json:"id"`
	}
	DecodeJSON(t, resp, &result)
	return result.ID
}

// --- Mock Upstream Servers ---

// newMockClaudeUpstream creates a mock server that returns Claude-format responses.
func newMockClaudeUpstream(t *testing.T, captured *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.Set(r)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":    "msg_mock_001",
			"type":  "message",
			"role":  "assistant",
			"model": "claude-sonnet-4-20250514",
			"content": []map[string]any{
				{"type": "text", "text": "Hello from mock Claude!"},
			},
			"stop_reason": "end_turn",
			"usage": map[string]any{
				"input_tokens":  10,
				"output_tokens": 8,
			},
		})
	}))
}

// newMockOpenAIUpstream creates a mock server that returns OpenAI-format responses.
func newMockOpenAIUpstream(t *testing.T, captured *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.Set(r)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-mock-001",
			"object":  "chat.completion",
			"model":   "gpt-4o",
			"created": 1700000000,
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "Hello from mock OpenAI!",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 8,
				"total_tokens":      18,
			},
		})
	}))
}

// newMockCodexUpstream creates a mock server that returns Codex (Responses API) format responses.
func newMockCodexUpstream(t *testing.T, captured *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.Set(r)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":         "rsp_mock_001",
			"object":     "response",
			"created_at": 1700000000000,
			"model":      "gpt-4o",
			"output": []map[string]any{
				{
					"type": "message",
					"id":   "msg_mock_001",
					"role": "assistant",
					"content": []map[string]any{
						{"type": "output_text", "text": "Hello from mock Codex!"},
					},
					"status": "completed",
				},
			},
			"status": "completed",
			"usage": map[string]any{
				"input_tokens":  10,
				"output_tokens": 8,
				"total_tokens":  18,
			},
		})
	}))
}

// newMockGeminiUpstream creates a mock server that returns Gemini-format responses.
func newMockGeminiUpstream(t *testing.T, captured *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.Set(r)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"role": "model",
						"parts": []map[string]any{
							{"text": "Hello from mock Gemini!"},
						},
					},
					"finishReason": "STOP",
				},
			},
			"usageMetadata": map[string]any{
				"promptTokenCount":     10,
				"candidatesTokenCount": 8,
				"totalTokenCount":      18,
			},
		})
	}))
}

func newMockGeminiStreamUpstream(t *testing.T, captured *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.Set(r)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("streaming not supported")
		}

		chunk := map[string]any{
			"candidates": []map[string]any{{
				"content": map[string]any{
					"role":  "model",
					"parts": []map[string]any{{"text": "Hello from mock Gemini stream"}},
				},
			}},
			"usageMetadata": map[string]any{
				"promptTokenCount":     10,
				"candidatesTokenCount": 8,
				"totalTokenCount":      18,
			},
		}

		if _, err := w.Write(converter.FormatSSE("", chunk)); err != nil {
			t.Fatalf("write gemini stream chunk: %v", err)
		}
		flusher.Flush()
		if _, err := w.Write(converter.FormatDone()); err != nil {
			t.Fatalf("write gemini done chunk: %v", err)
		}
		flusher.Flush()
	}))
}

func newMockCodexStreamUpstream(t *testing.T, captured *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.Set(r)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("streaming not supported")
		}

		events := [][]byte{
			converter.FormatSSE("response.created", map[string]any{
				"type": "response.created",
				"response": map[string]any{
					"id":         "resp_mock_stream",
					"object":     "response",
					"created_at": 1700000000,
					"status":     "in_progress",
				},
			}),
			converter.FormatSSE("response.output_text.delta", map[string]any{
				"type": "response.output_text.delta",
				"delta": map[string]any{
					"type": "output_text_delta",
					"text": "Hello from mock Codex stream",
				},
			}),
			converter.FormatSSE("response.output_item.added", map[string]any{
				"type": "response.output_item.added",
				"item": map[string]any{
					"type":      "function_call",
					"name":      "lookup",
					"call_id":   "call_1",
					"arguments": `{"city":"Tokyo"}`,
					"status":    "completed",
				},
			}),
			converter.FormatSSE("response.completed", map[string]any{
				"type": "response.completed",
				"response": map[string]any{
					"id":         "resp_mock_stream",
					"object":     "response",
					"created_at": 1700000001,
					"status":     "completed",
					"usage": map[string]any{
						"input_tokens":  10,
						"output_tokens": 8,
						"total_tokens":  18,
					},
				},
			}),
			converter.FormatDone(),
		}

		for _, event := range events {
			if _, err := w.Write(event); err != nil {
				t.Fatalf("write codex stream event: %v", err)
			}
			flusher.Flush()
		}
	}))
}

func proxyStreamPost(t *testing.T, env *ProxyTestEnv, path string, body any, headers map[string]string) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("Failed to marshal stream body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, env.URL(path), bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to create stream request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Stream request failed: %v", err)
	}
	return resp
}

// --- Helper: assert response status ---

func assertStatus(t *testing.T, resp *http.Response, expected int) {
	t.Helper()
	if resp.StatusCode != expected {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected %d, got %d: %s", expected, resp.StatusCode, body)
	}
}

// --- Client request builders ---

func claudeRequest(model string) map[string]any {
	return map[string]any{
		"model":      model,
		"max_tokens": 1024,
		"messages": []map[string]any{
			{"role": "user", "content": "Hello"},
		},
	}
}

func openaiRequest(model string) map[string]any {
	return map[string]any{
		"model":      model,
		"max_tokens": 1024,
		"messages": []map[string]any{
			{"role": "user", "content": "Hello"},
		},
	}
}

func codexRequest(model string) map[string]any {
	return map[string]any{
		"model":             model,
		"max_output_tokens": 1024,
		"input": []map[string]any{
			{"type": "message", "role": "user", "content": "Hello"},
		},
	}
}

func geminiRequest() map[string]any {
	return map[string]any{
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]any{
					{"text": "Hello"},
				},
			},
		},
		"generationConfig": map[string]any{
			"maxOutputTokens": 1024,
		},
	}
}

// --- Passthrough Tests ---

func newMockOllamaUpstream(t *testing.T, captured *capturedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.Set(r)
		if r.URL.Path != "/api/chat" {
			t.Fatalf("unexpected Ollama path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"qwen3-coder-next:cloud","message":{"role":"assistant","content":"ollama claude ok"},"done":true,"done_reason":"stop","prompt_eval_count":11,"eval_count":7}`))
	}))
}

func createOllamaProvider(t *testing.T, env *ProxyTestEnv, name, baseURL string) uint64 {
	t.Helper()
	resp := env.AdminPost("/api/admin/providers", map[string]any{
		"name": name,
		"type": "custom",
		"config": map[string]any{
			"custom": map[string]any{
				"baseURL": baseURL,
				"backend": "ollama",
			},
		},
		"supportedClientTypes": []string{"claude"},
	})
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to create Ollama provider %s: status=%d body=%s", name, resp.StatusCode, body)
	}
	var result struct {
		ID uint64 `json:"id"`
	}
	DecodeJSON(t, resp, &result)
	return result.ID
}

func TestProxyClaudeToCustomOllamaBackendEndToEnd(t *testing.T) {
	env := NewProxyTestEnv(t)

	captured := &capturedRequest{}
	mock := newMockOllamaUpstream(t, captured)
	defer mock.Close()

	providerID := createOllamaProvider(t, env, "mock-ollama", mock.URL)
	createRoute(t, env, "claude", providerID)

	resp := env.ProxyPost("/v1/messages", map[string]any{
		"model":      "claude-sonnet-4-20250514",
		"max_tokens": 32,
		"thinking":   map[string]any{"type": "enabled", "budget_tokens": 16},
		"system":     []map[string]any{{"type": "text", "text": "be precise"}},
		"tools": []map[string]any{{
			"name":         "lookup",
			"description":  "lookup docs",
			"input_schema": map[string]any{"type": "object", "properties": map[string]any{"q": map[string]any{"type": "string"}}},
		}},
		"messages": []map[string]any{
			{"role": "user", "content": []map[string]any{{"type": "text", "text": "use the tool"}}},
			{"role": "assistant", "content": []map[string]any{{"type": "tool_use", "id": "toolu_1", "name": "lookup", "input": map[string]any{"q": "ollama"}}}},
			{"role": "user", "content": []map[string]any{{"type": "tool_result", "tool_use_id": "toolu_1", "content": "docs say OK"}}},
		},
	}, nil)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	if gjson.GetBytes(body, "type").String() != "message" || gjson.GetBytes(body, "content.0.text").String() != "ollama claude ok" {
		t.Fatalf("unexpected Claude response: %s", body)
	}
	if gjson.GetBytes(body, "usage.input_tokens").Int() != 11 || gjson.GetBytes(body, "usage.output_tokens").Int() != 7 {
		t.Fatalf("usage was not mapped from Ollama: %s", body)
	}

	_, path, _, upBody := captured.Get()
	if path != "/api/chat" {
		t.Fatalf("upstream path = %s", path)
	}
	if gjson.GetBytes(upBody, "model").String() != "claude-sonnet-4-20250514" {
		t.Fatalf("upstream model = %s body=%s", gjson.GetBytes(upBody, "model").String(), upBody)
	}
	if !gjson.GetBytes(upBody, "think").Bool() {
		t.Fatalf("upstream think not enabled: %s", upBody)
	}
	if gjson.GetBytes(upBody, "tools.0.function.name").String() != "lookup" {
		t.Fatalf("upstream tools not mapped: %s", upBody)
	}
	if gjson.GetBytes(upBody, "messages.2.tool_calls.0.function.name").String() != "lookup" {
		t.Fatalf("assistant tool_use not mapped to Ollama tool_calls: %s", upBody)
	}
	if gjson.GetBytes(upBody, "messages.3.role").String() != "tool" || gjson.GetBytes(upBody, "messages.3.tool_name").String() != "lookup" {
		t.Fatalf("tool_result not mapped to Ollama tool message: %s", upBody)
	}
}

func TestProxyClaudePassthrough(t *testing.T) {
	captured := &capturedRequest{}
	mock := newMockClaudeUpstream(t, captured)
	defer mock.Close()

	env := NewProxyTestEnv(t)
	providerID := createProvider(t, env, "mock-claude", mock.URL, []string{"claude"})
	createRoute(t, env, "claude", providerID)

	resp := env.ProxyPost("/v1/messages", claudeRequest("claude-sonnet-4-20250514"), nil)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	_, path, _, body := captured.Get()
	if path != "/v1/messages" {
		t.Errorf("Expected upstream path /v1/messages, got %s", path)
	}
	if gjson.GetBytes(body, "model").String() != "claude-sonnet-4-20250514" {
		t.Errorf("Upstream model mismatch: %s", gjson.GetBytes(body, "model").String())
	}

	respBody, _ := io.ReadAll(resp.Body)
	if gjson.GetBytes(respBody, "type").String() != "message" {
		t.Errorf("Response should be Claude format with type=message, got: %s", string(respBody))
	}
}

func TestProxyOpenAIPassthrough(t *testing.T) {
	captured := &capturedRequest{}
	mock := newMockOpenAIUpstream(t, captured)
	defer mock.Close()

	env := NewProxyTestEnv(t)
	providerID := createProvider(t, env, "mock-openai", mock.URL, []string{"openai"})
	createRoute(t, env, "openai", providerID)

	resp := env.ProxyPost("/v1/chat/completions", openaiRequest("gpt-4o"), nil)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	_, path, _, _ := captured.Get()
	if path != "/v1/chat/completions" {
		t.Errorf("Expected upstream path /v1/chat/completions, got %s", path)
	}

	respBody, _ := io.ReadAll(resp.Body)
	if gjson.GetBytes(respBody, "object").String() != "chat.completion" {
		t.Errorf("Response should have object=chat.completion, got: %s", string(respBody))
	}
}

func TestProxyCodexPassthrough(t *testing.T) {
	captured := &capturedRequest{}
	mock := newMockCodexUpstream(t, captured)
	defer mock.Close()

	env := NewProxyTestEnv(t)
	providerID := createProvider(t, env, "mock-codex", mock.URL, []string{"codex"})
	createRoute(t, env, "codex", providerID)

	resp := env.ProxyPost("/responses", codexRequest("gpt-4o"), nil)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	_, path, _, body := captured.Get()
	if path != "/responses" {
		t.Errorf("Expected upstream path /responses, got %s", path)
	}
	if !gjson.GetBytes(body, "input").Exists() {
		t.Error("Upstream request should have 'input' field (Codex format)")
	}

	respBody, _ := io.ReadAll(resp.Body)
	if gjson.GetBytes(respBody, "object").String() != "response" {
		t.Errorf("Response should have object=response, got: %s", string(respBody))
	}
}

func TestProxyGeminiPassthrough(t *testing.T) {
	captured := &capturedRequest{}
	mock := newMockGeminiUpstream(t, captured)
	defer mock.Close()

	env := NewProxyTestEnv(t)
	providerID := createProvider(t, env, "mock-gemini", mock.URL, []string{"gemini"})
	createRoute(t, env, "gemini", providerID)

	resp := env.ProxyPost("/v1beta/models/gemini-2.0-flash:generateContent", geminiRequest(), nil)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	_, path, _, body := captured.Get()
	if path != "/v1beta/models/gemini-2.0-flash:generateContent" {
		t.Errorf("Expected Gemini upstream path, got %s", path)
	}
	if !gjson.GetBytes(body, "contents").Exists() {
		t.Error("Upstream request should have 'contents' field (Gemini format)")
	}

	respBody, _ := io.ReadAll(resp.Body)
	if !gjson.GetBytes(respBody, "candidates").Exists() {
		t.Errorf("Response should have 'candidates' array (Gemini format), got: %s", string(respBody))
	}
}

// --- Cross-protocol conversion tests ---
// Test all 12 conversion pairs: Claude↔OpenAI, Claude↔Codex, Claude↔Gemini,
// OpenAI↔Codex, OpenAI↔Gemini, Codex↔Gemini

// conversionTestCase defines a cross-protocol conversion test.
type conversionTestCase struct {
	name string
	// Client side
	clientType string // route clientType
	clientPath string // URL path to send request to
	clientReq  map[string]any
	// Upstream side
	upstreamType    string // provider supportedClientTypes
	upstreamMock    func(t *testing.T, captured *capturedRequest) *httptest.Server
	expectedUpPath  string // expected upstream path prefix
	expectedUpField string // field that should exist in upstream request body
	// Response verification
	respAssert func(t *testing.T, respBody []byte)
}

func startStrictOpenAICompatibleProxyServer(t *testing.T, captured *capturedRequest) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.Set(r)
		if r.Method != http.MethodPost || r.URL.Path != "/compatible/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer ***" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "missing provider auth"}})
			return
		}

		var body map[string]any
		if err := json.Unmarshal(captured.Body, &body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": err.Error()}})
			return
		}
		messages, ok := body["messages"].([]any)
		if !ok || len(messages) < 2 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "messages missing"}})
			return
		}
		if leaked := findCodexContentPartType(body); leaked != "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "invalid OpenAI chat content part: " + leaked}})
			return
		}
		if !gjson.GetBytes(captured.Body, "messages.0.content.1.image_url.url").Exists() {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "image_url part missing"}})
			return
		}
		if got := gjson.GetBytes(captured.Body, "messages.1.content").String(); got != "previous answer" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": "assistant output_text was not normalized"}})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-openai-compatible",
			"object":  "chat.completion",
			"created": 1700000000,
			"model":   body["model"],
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "strict OpenAI-compatible provider ok",
				},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 4,
				"total_tokens":      14,
			},
		})
	}))
}

func findCodexContentPartType(value any) string {
	switch v := value.(type) {
	case map[string]any:
		if typ, _ := v["type"].(string); typ == "input_text" || typ == "output_text" || typ == "input_image" {
			return typ
		}
		for _, child := range v {
			if leaked := findCodexContentPartType(child); leaked != "" {
				return leaked
			}
		}
	case []any:
		for _, child := range v {
			if leaked := findCodexContentPartType(child); leaked != "" {
				return leaked
			}
		}
	}
	return ""
}

func TestProxyCodexRouteToLegacyOpenAICompatibleProviderConfig(t *testing.T) {
	captured := &capturedRequest{}
	mock := startStrictOpenAICompatibleProxyServer(t, captured)
	defer mock.Close()

	env := NewProxyTestEnv(t)
	legacyProviderConfig := map[string]any{
		"name": "Legacy OpenAI Compatible Provider",
		"type": "custom",
		"config": map[string]any{
			"custom": map[string]any{
				"baseURL": "http://unused.invalid/base",
				"apiKey":  "***",
				"clientBaseURL": map[string]any{
					"openai": mock.URL + "/compatible",
				},
			},
		},
		"supportedClientTypes": []string{"openai"},
		"supportModels":        []string{"*"},
	}
	providerResp := env.AdminPost("/api/admin/providers", legacyProviderConfig)
	AssertStatus(t, providerResp, http.StatusCreated)
	var provider map[string]any
	DecodeJSON(t, providerResp, &provider)
	providerID := uint64(provider["id"].(float64))
	createRoute(t, env, "codex", providerID)

	resp := env.ProxyPost("/responses?source=codex-cli", map[string]any{
		"model": "gpt-5-codex",
		"input": []map[string]any{{
			"type": "message",
			"role": "user",
			"content": []map[string]any{{
				"type": "input_text",
				"text": "describe this",
			}, {
				"type":      "input_image",
				"image_url": "data:image/png;base64,Zm9v",
			}},
		}, {
			"type": "message",
			"role": "assistant",
			"content": []map[string]any{{
				"type": "output_text",
				"text": "previous answer",
			}},
		}},
		"max_output_tokens": 128,
	}, map[string]string{
		"User-Agent":    "codex_cli_rs/0.50.0 (Mac OS 26.0.1; arm64)",
		"Authorization": "Bearer client-token-must-not-leak",
	})
	defer resp.Body.Close()
	AssertStatus(t, resp, http.StatusOK)

	_, path, headers, upstreamBody := captured.Get()
	if path != "/compatible/v1/chat/completions" {
		t.Fatalf("upstream path = %s, want /compatible/v1/chat/completions", path)
	}
	if got := headers.Get("Authorization"); got != "Bearer ***" {
		t.Fatalf("Authorization = %q, want provider credential", got)
	}
	if strings.Contains(string(upstreamBody), "client-token-must-not-leak") {
		t.Fatalf("client auth leaked to upstream body/headers")
	}
	if got := gjson.GetBytes(upstreamBody, "messages.0.content.0.type").String(); got != "text" {
		t.Fatalf("first OpenAI content part type = %q, want text; body=%s", got, string(upstreamBody))
	}
	if got := gjson.GetBytes(upstreamBody, "messages.0.content.1.type").String(); got != "image_url" {
		t.Fatalf("second OpenAI content part type = %q, want image_url; body=%s", got, string(upstreamBody))
	}
	if got := gjson.GetBytes(upstreamBody, "messages.1.content").String(); got != "previous answer" {
		t.Fatalf("assistant content = %q, want previous answer; body=%s", got, string(upstreamBody))
	}

	respBody := ReadBody(t, resp)
	if got := gjson.Get(respBody, "object").String(); got != "response" {
		t.Fatalf("response object = %q, want response; body=%s", got, respBody)
	}
	if got := gjson.Get(respBody, "output.0.content.0.text").String(); got != "strict OpenAI-compatible provider ok" {
		t.Fatalf("converted response text = %q; body=%s", got, respBody)
	}
}

func TestProxyCrossProtocolConversions(t *testing.T) {
	tests := []conversionTestCase{
		// === Claude ↔ OpenAI ===
		{
			name:            "OpenAI_client_to_Claude_upstream",
			clientType:      "openai",
			clientPath:      "/v1/chat/completions",
			clientReq:       openaiRequest("claude-sonnet-4-20250514"),
			upstreamType:    "claude",
			upstreamMock:    newMockClaudeUpstream,
			expectedUpPath:  "/v1/messages",
			expectedUpField: "messages",
			respAssert: func(t *testing.T, body []byte) {
				if gjson.GetBytes(body, "object").String() != "chat.completion" {
					t.Errorf("Response should be OpenAI format, got: %s", string(body))
				}
				content := gjson.GetBytes(body, "choices.0.message.content").String()
				if content != "Hello from mock Claude!" {
					t.Errorf("Content mismatch: %s", content)
				}
			},
		},
		{
			name:            "Claude_client_to_OpenAI_upstream",
			clientType:      "claude",
			clientPath:      "/v1/messages",
			clientReq:       claudeRequest("gpt-4o"),
			upstreamType:    "openai",
			upstreamMock:    newMockOpenAIUpstream,
			expectedUpPath:  "/v1/chat/completions",
			expectedUpField: "messages",
			respAssert: func(t *testing.T, body []byte) {
				if gjson.GetBytes(body, "type").String() != "message" {
					t.Errorf("Response should be Claude format, got: %s", string(body))
				}
				content := gjson.GetBytes(body, "content.0.text").String()
				if content != "Hello from mock OpenAI!" {
					t.Errorf("Content mismatch: %s", content)
				}
			},
		},

		// === Claude ↔ Codex ===
		{
			name:            "Claude_client_to_Codex_upstream",
			clientType:      "claude",
			clientPath:      "/v1/messages",
			clientReq:       claudeRequest("gpt-4o"),
			upstreamType:    "codex",
			upstreamMock:    newMockCodexUpstream,
			expectedUpPath:  "/responses",
			expectedUpField: "input",
			respAssert: func(t *testing.T, body []byte) {
				if gjson.GetBytes(body, "type").String() != "message" {
					t.Errorf("Response should be Claude format, got: %s", string(body))
				}
			},
		},
		{
			name:            "Codex_client_to_Claude_upstream",
			clientType:      "codex",
			clientPath:      "/responses",
			clientReq:       codexRequest("claude-sonnet-4-20250514"),
			upstreamType:    "claude",
			upstreamMock:    newMockClaudeUpstream,
			expectedUpPath:  "/v1/messages",
			expectedUpField: "messages",
			respAssert: func(t *testing.T, body []byte) {
				if gjson.GetBytes(body, "object").String() != "response" {
					t.Errorf("Response should be Codex format, got: %s", string(body))
				}
			},
		},

		// === OpenAI ↔ Codex ===
		{
			name:            "OpenAI_client_to_Codex_upstream",
			clientType:      "openai",
			clientPath:      "/v1/chat/completions",
			clientReq:       openaiRequest("gpt-4o"),
			upstreamType:    "codex",
			upstreamMock:    newMockCodexUpstream,
			expectedUpPath:  "/responses",
			expectedUpField: "input",
			respAssert: func(t *testing.T, body []byte) {
				if gjson.GetBytes(body, "object").String() != "chat.completion" {
					t.Errorf("Response should be OpenAI format, got: %s", string(body))
				}
				content := gjson.GetBytes(body, "choices.0.message.content").String()
				if content != "Hello from mock Codex!" {
					t.Errorf("Content mismatch: %s", content)
				}
			},
		},
		{
			name:            "Codex_client_to_OpenAI_upstream",
			clientType:      "codex",
			clientPath:      "/responses",
			clientReq:       codexRequest("gpt-4o"),
			upstreamType:    "openai",
			upstreamMock:    newMockOpenAIUpstream,
			expectedUpPath:  "/v1/chat/completions",
			expectedUpField: "messages",
			respAssert: func(t *testing.T, body []byte) {
				if gjson.GetBytes(body, "object").String() != "response" {
					t.Errorf("Response should be Codex format, got: %s", string(body))
				}
			},
		},

		// === Claude ↔ Gemini ===
		{
			name:            "Claude_client_to_Gemini_upstream",
			clientType:      "claude",
			clientPath:      "/v1/messages",
			clientReq:       claudeRequest("gemini-2.0-flash"),
			upstreamType:    "gemini",
			upstreamMock:    newMockGeminiUpstream,
			expectedUpPath:  "/v1beta/models/",
			expectedUpField: "contents",
			respAssert: func(t *testing.T, body []byte) {
				if gjson.GetBytes(body, "type").String() != "message" {
					t.Errorf("Response should be Claude format, got: %s", string(body))
				}
			},
		},
		{
			name:            "Gemini_client_to_Claude_upstream",
			clientType:      "gemini",
			clientPath:      "/v1beta/models/claude-sonnet-4-20250514:generateContent",
			clientReq:       geminiRequest(),
			upstreamType:    "claude",
			upstreamMock:    newMockClaudeUpstream,
			expectedUpPath:  "/v1/messages",
			expectedUpField: "messages",
			respAssert: func(t *testing.T, body []byte) {
				if !gjson.GetBytes(body, "candidates").Exists() {
					t.Errorf("Response should be Gemini format, got: %s", string(body))
				}
			},
		},

		// === OpenAI ↔ Gemini ===
		{
			name:            "OpenAI_client_to_Gemini_upstream",
			clientType:      "openai",
			clientPath:      "/v1/chat/completions",
			clientReq:       openaiRequest("gemini-2.0-flash"),
			upstreamType:    "gemini",
			upstreamMock:    newMockGeminiUpstream,
			expectedUpPath:  "/v1beta/models/",
			expectedUpField: "contents",
			respAssert: func(t *testing.T, body []byte) {
				if gjson.GetBytes(body, "object").String() != "chat.completion" {
					t.Errorf("Response should be OpenAI format, got: %s", string(body))
				}
				content := gjson.GetBytes(body, "choices.0.message.content").String()
				if content != "Hello from mock Gemini!" {
					t.Errorf("Content mismatch: %s", content)
				}
			},
		},
		{
			name:            "Gemini_client_to_OpenAI_upstream",
			clientType:      "gemini",
			clientPath:      "/v1beta/models/gpt-4o:generateContent",
			clientReq:       geminiRequest(),
			upstreamType:    "openai",
			upstreamMock:    newMockOpenAIUpstream,
			expectedUpPath:  "/v1/chat/completions",
			expectedUpField: "messages",
			respAssert: func(t *testing.T, body []byte) {
				if !gjson.GetBytes(body, "candidates").Exists() {
					t.Errorf("Response should be Gemini format, got: %s", string(body))
				}
			},
		},

		// === Codex ↔ Gemini ===
		{
			name:            "Codex_client_to_Gemini_upstream",
			clientType:      "codex",
			clientPath:      "/responses",
			clientReq:       codexRequest("gemini-2.0-flash"),
			upstreamType:    "gemini",
			upstreamMock:    newMockGeminiUpstream,
			expectedUpPath:  "/v1beta/models/",
			expectedUpField: "contents",
			respAssert: func(t *testing.T, body []byte) {
				if gjson.GetBytes(body, "object").String() != "response" {
					t.Errorf("Response should be Codex format, got: %s", string(body))
				}
			},
		},
		{
			name:            "Gemini_client_to_Codex_upstream",
			clientType:      "gemini",
			clientPath:      "/v1beta/models/gpt-4o:generateContent",
			clientReq:       geminiRequest(),
			upstreamType:    "codex",
			upstreamMock:    newMockCodexUpstream,
			expectedUpPath:  "/responses",
			expectedUpField: "input",
			respAssert: func(t *testing.T, body []byte) {
				if !gjson.GetBytes(body, "candidates").Exists() {
					t.Errorf("Response should be Gemini format, got: %s", string(body))
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			captured := &capturedRequest{}
			mock := tc.upstreamMock(t, captured)
			defer mock.Close()

			env := NewProxyTestEnv(t)
			providerID := createProvider(t, env, fmt.Sprintf("mock-%s", tc.upstreamType), mock.URL, []string{tc.upstreamType})
			createRoute(t, env, tc.clientType, providerID)

			resp := env.ProxyPost(tc.clientPath, tc.clientReq, nil)
			defer resp.Body.Close()
			assertStatus(t, resp, http.StatusOK)

			// Verify upstream received converted format
			_, path, _, body := captured.Get()
			if tc.expectedUpPath != "" {
				if path != tc.expectedUpPath && !hasPrefix(path, tc.expectedUpPath) {
					t.Errorf("Expected upstream path %s, got %s", tc.expectedUpPath, path)
				}
			}
			if tc.expectedUpField != "" && !gjson.GetBytes(body, tc.expectedUpField).Exists() {
				t.Errorf("Upstream request should have '%s' field, body: %s", tc.expectedUpField, string(body))
			}

			// Verify response is converted back to client format
			respBody, _ := io.ReadAll(resp.Body)
			if tc.respAssert != nil {
				tc.respAssert(t, respBody)
			}
		})
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// --- Error and edge case tests ---

func TestProxyUpstreamError(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "Rate limit exceeded",
				"type":    "rate_limit_error",
			},
		})
	}))
	defer mock.Close()

	env := NewProxyTestEnv(t)
	providerID := createProvider(t, env, "mock-error", mock.URL, []string{"openai"})
	createRoute(t, env, "openai", providerID)

	resp := env.ProxyPost("/v1/chat/completions", openaiRequest("gpt-4o"), nil)
	AssertStatus(t, resp, http.StatusTooManyRequests)

	var payload map[string]map[string]any
	DecodeJSON(t, resp, &payload)
	if got := payload["error"]["type"]; got != "upstream_error" {
		t.Fatalf("error.type = %v, want upstream_error", got)
	}
	msg, _ := payload["error"]["message"].(string)
	if !strings.Contains(msg, "Rate limit exceeded") {
		t.Fatalf("error.message = %q, want to contain Rate limit exceeded", msg)
	}
	if got := payload["error"]["retryable"]; got != true {
		t.Fatalf("error.retryable = %v, want true", got)
	}
}

func TestProxyNoMatchingRoute(t *testing.T) {
	env := NewProxyTestEnv(t)

	resp := env.ProxyPost("/v1/chat/completions", openaiRequest("gpt-4o"), nil)
	AssertStatus(t, resp, http.StatusBadGateway)

	var payload map[string]map[string]any
	DecodeJSON(t, resp, &payload)
	if got := payload["error"]["type"]; got != "upstream_error" {
		t.Fatalf("error.type = %v, want upstream_error", got)
	}
	msg, _ := payload["error"]["message"].(string)
	if !strings.Contains(msg, "route match failed") || !strings.Contains(msg, "no routes available") {
		t.Fatalf("error.message = %q, want to contain route match failed and no routes available", msg)
	}
	if got := payload["error"]["retryable"]; got != false {
		t.Fatalf("error.retryable = %v, want false", got)
	}
}

func TestProxyCodexToGeminiStreamingSSE(t *testing.T) {
	captured := &capturedRequest{}
	mock := newMockGeminiStreamUpstream(t, captured)
	defer mock.Close()

	env := NewProxyTestEnv(t)
	providerID := createProvider(t, env, "mock-gemini-stream", mock.URL, []string{"gemini"})
	createRoute(t, env, "codex", providerID)

	request := codexRequest("gemini-2.0-flash")
	request["stream"] = true

	resp := proxyStreamPost(t, env, "/responses", request, nil)
	AssertStatus(t, resp, http.StatusOK)
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		resp.Body.Close()
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	respBody := ReadBody(t, resp)

	_, path, headers, body := captured.Get()
	if path != "/v1beta/models/gemini-2.0-flash:streamGenerateContent" {
		t.Fatalf("upstream path = %q, want /v1beta/models/gemini-2.0-flash:streamGenerateContent", path)
	}
	if accept := headers.Get("Accept"); !strings.Contains(accept, "text/event-stream") {
		t.Fatalf("upstream Accept = %q, want text/event-stream", accept)
	}
	if !gjson.GetBytes(body, "contents").Exists() {
		t.Fatalf("expected Gemini upstream request to contain contents, body=%s", string(body))
	}

	if !strings.Contains(respBody, "response.created") {
		t.Fatalf("stream body missing response.created: %s", respBody)
	}
	if !strings.Contains(respBody, "response.output_text.delta") {
		t.Fatalf("stream body missing response.output_text.delta: %s", respBody)
	}
	if !strings.Contains(respBody, "Hello from mock Gemini stream") {
		t.Fatalf("stream body missing transformed Gemini text: %s", respBody)
	}
	if !strings.Contains(respBody, "response.completed") {
		t.Fatalf("stream body missing response.completed: %s", respBody)
	}
	if !strings.Contains(respBody, "data: [DONE]") {
		t.Fatalf("stream body missing [DONE]: %s", respBody)
	}
}

func TestProxyGeminiToCodexStreamingSSE(t *testing.T) {
	captured := &capturedRequest{}
	mock := newMockCodexStreamUpstream(t, captured)
	defer mock.Close()

	env := NewProxyTestEnv(t)
	providerID := createProvider(t, env, "mock-codex-stream", mock.URL, []string{"codex"})
	createRoute(t, env, "gemini", providerID)

	resp := proxyStreamPost(t, env, "/v1beta/models/gpt-4o:streamGenerateContent", geminiRequest(), nil)
	AssertStatus(t, resp, http.StatusOK)
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		resp.Body.Close()
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	respBody := ReadBody(t, resp)

	_, path, headers, body := captured.Get()
	if path != "/responses" {
		t.Fatalf("upstream path = %q, want /responses", path)
	}
	if accept := headers.Get("Accept"); !strings.Contains(accept, "text/event-stream") {
		t.Fatalf("upstream Accept = %q, want text/event-stream", accept)
	}
	if got := gjson.GetBytes(body, "model").String(); got != "gpt-4o" {
		t.Fatalf("upstream model = %q, want gpt-4o", got)
	}
	if !gjson.GetBytes(body, "input").Exists() {
		t.Fatalf("expected Codex upstream request to contain input, body=%s", string(body))
	}
	if !gjson.GetBytes(body, "stream").Bool() {
		t.Fatalf("expected Codex upstream request stream=true, body=%s", string(body))
	}

	if !strings.Contains(respBody, "Hello from mock Codex stream") {
		t.Fatalf("stream body missing transformed Codex text: %s", respBody)
	}
	if !strings.Contains(respBody, `"functionCall":{"name":"lookup_call_1"`) {
		t.Fatalf("stream body missing transformed functionCall: %s", respBody)
	}
	if !strings.Contains(respBody, `"finishReason":"STOP"`) {
		t.Fatalf("stream body missing finishReason STOP: %s", respBody)
	}
	if !strings.Contains(respBody, `"totalTokenCount":18`) {
		t.Fatalf("stream body missing usage metadata: %s", respBody)
	}
}

// TestProxyAllProtocolsCoexist verifies that all 4 protocol types can coexist
// in the same environment with cross-protocol routing.
func TestProxyAllProtocolsCoexist(t *testing.T) {
	capturedClaude := &capturedRequest{}
	capturedOpenAI := &capturedRequest{}
	capturedCodex := &capturedRequest{}
	capturedGemini := &capturedRequest{}

	mockClaude := newMockClaudeUpstream(t, capturedClaude)
	mockOpenAI := newMockOpenAIUpstream(t, capturedOpenAI)
	mockCodex := newMockCodexUpstream(t, capturedCodex)
	mockGemini := newMockGeminiUpstream(t, capturedGemini)
	defer mockClaude.Close()
	defer mockOpenAI.Close()
	defer mockCodex.Close()
	defer mockGemini.Close()

	env := NewProxyTestEnv(t)

	// Create 4 providers, one for each protocol
	claudeProviderID := createProvider(t, env, "mock-claude", mockClaude.URL, []string{"claude"})
	openaiProviderID := createProvider(t, env, "mock-openai", mockOpenAI.URL, []string{"openai"})
	codexProviderID := createProvider(t, env, "mock-codex", mockCodex.URL, []string{"codex"})
	geminiProviderID := createProvider(t, env, "mock-gemini", mockGemini.URL, []string{"gemini"})

	// Route each client type to a DIFFERENT upstream (cross-protocol)
	// OpenAI clients → Claude upstream
	createRoute(t, env, "openai", claudeProviderID)
	// Claude clients → Codex upstream
	createRoute(t, env, "claude", codexProviderID)
	// Codex clients → OpenAI upstream
	createRoute(t, env, "codex", openaiProviderID)
	// Gemini clients → Codex upstream
	createRoute(t, env, "gemini", codexProviderID)
	// (geminiProviderID is used below for a separate subtest)
	_ = geminiProviderID

	t.Run("OpenAI_to_Claude", func(t *testing.T) {
		resp := env.ProxyPost("/v1/chat/completions", openaiRequest("claude-sonnet-4-20250514"), nil)
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusOK)

		_, path, _, _ := capturedClaude.Get()
		if path != "/v1/messages" {
			t.Errorf("Expected upstream /v1/messages, got %s", path)
		}
	})

	t.Run("Claude_to_Codex", func(t *testing.T) {
		resp := env.ProxyPost("/v1/messages", claudeRequest("gpt-4o"), nil)
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusOK)

		_, path, _, _ := capturedCodex.Get()
		if path != "/responses" {
			t.Errorf("Expected upstream /responses, got %s", path)
		}
	})

	t.Run("Codex_to_OpenAI", func(t *testing.T) {
		resp := env.ProxyPost("/responses", codexRequest("gpt-4o"), nil)
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusOK)

		_, path, _, _ := capturedOpenAI.Get()
		if path != "/v1/chat/completions" {
			t.Errorf("Expected upstream /v1/chat/completions, got %s", path)
		}
	})

	t.Run("Gemini_to_Codex", func(t *testing.T) {
		resp := env.ProxyPost("/v1beta/models/gpt-4o:generateContent", geminiRequest(), nil)
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusOK)

		_, path, _, _ := capturedCodex.Get()
		if path != "/responses" {
			t.Errorf("Expected upstream /responses, got %s", path)
		}
	})
}

// ============================================================
// Cooldown Integration Tests (using mock server)
// ============================================================

func TestTokenConcurrencyLimitRecordsRejectedRequest(t *testing.T) {
	cases := []struct {
		name               string
		configureRetention func(t *testing.T, env *ProxyTestEnv)
	}{
		{
			name: "unified retention zero",
			configureRetention: func(t *testing.T, env *ProxyTestEnv) {
				setRequestDetailRetentionSetting(t, env, "request_detail_retention_seconds", "0")
			},
		},
		{
			name: "split failed retention zero",
			configureRetention: func(t *testing.T, env *ProxyTestEnv) {
				setRequestDetailRetentionSetting(t, env, "request_detail_retention_seconds", "86400")
				setRequestDetailRetentionSetting(t, env, "request_detail_retention_split_enabled", "true")
				setRequestDetailRetentionSetting(t, env, "request_detail_retention_seconds_success", "86400")
				setRequestDetailRetentionSetting(t, env, "request_detail_retention_seconds_failed", "0")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runTokenConcurrencyLimitRecordsRejectedRequest(t, tc.configureRetention)
		})
	}
}

func setRequestDetailRetentionSetting(t *testing.T, env *ProxyTestEnv, key, value string) {
	t.Helper()
	resp := env.AdminPut("/api/admin/settings/"+key, map[string]any{"value": value})
	AssertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func runTokenConcurrencyLimitRecordsRejectedRequest(t *testing.T, configureRetention func(t *testing.T, env *ProxyTestEnv)) {
	blocked := make(chan struct{})
	entered := make(chan struct{}, 1)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-blocked
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-mock-001",
			"object":  "chat.completion",
			"model":   "gpt-4o",
			"created": 1700000000,
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "ok",
				},
				"finish_reason": "stop",
			}},
		})
	}))
	defer mock.Close()

	env := NewProxyTestEnv(t)
	providerID := createProvider(t, env, "mock-openai", mock.URL, []string{"openai"})
	createRoute(t, env, "openai", providerID)

	configureRetention(t, env)

	resp := env.AdminPut("/api/admin/settings/api_token_auth_enabled", map[string]any{"value": "true"})
	AssertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = env.AdminPut("/api/admin/settings/api_token_concurrent_limit", map[string]any{"value": "1"})
	AssertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = env.AdminPost("/api/admin/api-tokens", map[string]any{
		"name":        "concurrency-token",
		"description": "Token for concurrency limit test",
	})
	AssertStatus(t, resp, http.StatusCreated)
	var created map[string]any
	DecodeJSON(t, resp, &created)
	tokenStr, ok := created["token"].(string)
	if !ok || tokenStr == "" {
		t.Fatalf("Expected token string, got %v", created["token"])
	}
	apiToken, ok := created["apiToken"].(map[string]any)
	if !ok {
		t.Fatalf("Expected apiToken object, got %v", created["apiToken"])
	}
	apiTokenID := int(apiToken["id"].(float64))

	firstDone := make(chan error, 1)
	go func() {
		resp := env.ProxyPost("/v1/chat/completions", openaiRequest("gpt-4o"), map[string]string{
			"Authorization": "Bearer " + tokenStr,
		})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			firstDone <- fmt.Errorf("first request status=%d body=%s", resp.StatusCode, body)
			return
		}
		firstDone <- nil
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first request to reach upstream")
	}

	secretPayload := map[string]any{
		"model": "gpt-4o",
		"messages": []map[string]any{{
			"role":    "user",
			"content": "do not persist rejected payload",
		}},
	}
	resp = env.ProxyPost("/v1/chat/completions", secretPayload, map[string]string{
		"Authorization": "Bearer " + tokenStr,
	})
	AssertStatus(t, resp, http.StatusTooManyRequests)
	var rateLimited map[string]map[string]any
	DecodeJSON(t, resp, &rateLimited)
	if got := rateLimited["error"]["type"]; got != "rate_limit_error" {
		t.Fatalf("error.type = %v, want rate_limit_error", got)
	}
	if got := resp.Header.Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}

	deadline := time.Now().Add(2 * time.Second)
	foundRejected := false
	foundActive := false
	rejectedID := 0
	for {
		resp = env.AdminGet(fmt.Sprintf("/api/admin/requests?limit=20&apiTokenId=%d", apiTokenID))
		AssertStatus(t, resp, http.StatusOK)
		var result map[string]any
		DecodeJSON(t, resp, &result)
		items, ok := result["items"].([]any)
		if !ok {
			t.Fatalf("Expected items array, got %T", result["items"])
		}

		foundRejected = false
		foundActive = false
		for _, item := range items {
			request, ok := item.(map[string]any)
			if !ok {
				continue
			}
			status, _ := request["status"].(string)
			statusCode, _ := request["statusCode"].(float64)
			errorMsg, _ := request["error"].(string)
			if status == "REJECTED" && int(statusCode) == http.StatusTooManyRequests && strings.Contains(errorMsg, "concurrent request limit") {
				foundRejected = true
				rejectedID = int(request["id"].(float64))
			}
			if status == "PENDING" || status == "IN_PROGRESS" || status == "COMPLETED" {
				foundActive = true
			}
		}
		if foundRejected && foundActive {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected rejected+active requests, got items=%v", items)
		}
		time.Sleep(50 * time.Millisecond)
	}

	resp = env.AdminGet(fmt.Sprintf("/api/admin/requests/%d", rejectedID))
	AssertStatus(t, resp, http.StatusOK)
	var rejected map[string]any
	DecodeJSON(t, resp, &rejected)
	if rejected["requestInfo"] != nil {
		t.Fatalf("requestInfo must be nil when retention clears failed requests, got %#v", rejected["requestInfo"])
	}
	if rejected["responseInfo"] != nil {
		t.Fatalf("responseInfo must be nil when retention clears failed requests, got %#v", rejected["responseInfo"])
	}

	close(blocked)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
}

func TestCooldown_429TriggersKeyLevelCooldown(t *testing.T) {
	env := NewProxyTestEnv(t)
	srv := mockserver.New()
	defer srv.Close()

	providerID := createProvider(t, env, "mock-openai", srv.URL, []string{"openai"})
	createRoute(t, env, "openai", providerID)

	// Send request that triggers 429
	resp := env.ProxyPost("/v1/chat/completions",
		map[string]any{
			"model":    "gpt-4o",
			"messages": []map[string]any{{"role": "user", "content": "hello"}},
		},
		map[string]string{
			mockserver.MockHeader: `{"status":429,"headers":{"Retry-After":"5"},"body":{"error":{"type":"rate_limit_exceeded","message":"Rate limit exceeded"}}}`,
		},
	)
	defer resp.Body.Close()
	assertStatus(t, resp, 429)

	// Verify cooldown was set
	cdResp := env.AdminGet("/api/admin/cooldowns")
	defer cdResp.Body.Close()
	cdBody, _ := io.ReadAll(cdResp.Body)
	cooldowns := gjson.ParseBytes(cdBody).Array()
	if len(cooldowns) == 0 {
		t.Fatal("expected cooldown to be set after 429")
	}

	found := false
	for _, cd := range cooldowns {
		if cd.Get("providerID").Uint() == providerID {
			found = true
			t.Logf("Cooldown set: reason=%s, until=%s", cd.Get("reason").String(), cd.Get("until").String())
		}
	}
	if !found {
		t.Errorf("no cooldown found for provider %d in: %s", providerID, cdBody)
	}
}

func TestCooldown_503ModelOverloadedTriggersModelCooldown(t *testing.T) {
	env := NewProxyTestEnv(t)
	srv := mockserver.New()
	defer srv.Close()

	providerID := createProvider(t, env, "mock-openai-2", srv.URL, []string{"openai"})
	createRoute(t, env, "openai", providerID)

	// Send request that triggers 503 with model overloaded message
	resp := env.ProxyPost("/v1/chat/completions",
		map[string]any{
			"model":    "gpt-4o",
			"messages": []map[string]any{{"role": "user", "content": "hello"}},
		},
		map[string]string{
			mockserver.MockHeader: `{"status":503,"body":{"error":{"type":"server_error","message":"Model gpt-4o is overloaded"}}}`,
		},
	)
	defer resp.Body.Close()
	assertStatus(t, resp, 503)

	// Verify cooldown was set
	cdResp := env.AdminGet("/api/admin/cooldowns")
	defer cdResp.Body.Close()
	cdBody, _ := io.ReadAll(cdResp.Body)
	cooldowns := gjson.ParseBytes(cdBody).Array()

	found := false
	for _, cd := range cooldowns {
		if cd.Get("providerID").Uint() == providerID {
			found = true
			// Should be a model-level cooldown (model field should be non-empty)
			model := cd.Get("model").String()
			t.Logf("Cooldown: reason=%s, model=%s", cd.Get("reason").String(), model)
		}
	}
	if !found {
		t.Errorf("no cooldown found for provider %d in: %s", providerID, cdBody)
	}
}

func TestCooldown_200ClearsCooldown(t *testing.T) {
	env := NewProxyTestEnv(t)
	srv := mockserver.New()
	defer srv.Close()

	providerID := createProvider(t, env, "mock-openai-3", srv.URL, []string{"openai"})
	createRoute(t, env, "openai", providerID)

	// First request: trigger 429 to set cooldown
	resp1 := env.ProxyPost("/v1/chat/completions",
		map[string]any{
			"model":    "gpt-4o",
			"messages": []map[string]any{{"role": "user", "content": "hello"}},
		},
		map[string]string{
			mockserver.MockHeader: `{"status":429,"headers":{"Retry-After":"1"}}`,
		},
	)
	resp1.Body.Close()

	// Wait for cooldown to expire (1 second + buffer)
	time.Sleep(2 * time.Second)

	// Second request: success (should clear cooldown)
	resp2 := env.ProxyPost("/v1/chat/completions",
		map[string]any{
			"model":    "gpt-4o",
			"messages": []map[string]any{{"role": "user", "content": "hello again"}},
		},
		nil,
	)
	defer resp2.Body.Close()
	assertStatus(t, resp2, 200)

	// Verify cooldown is cleared
	cdResp := env.AdminGet("/api/admin/cooldowns")
	defer cdResp.Body.Close()
	cdBody, _ := io.ReadAll(cdResp.Body)
	cooldowns := gjson.ParseBytes(cdBody).Array()

	for _, cd := range cooldowns {
		if cd.Get("providerID").Uint() == providerID {
			t.Errorf("expected cooldown to be cleared after success, but found: %s", cd.Raw)
		}
	}
}

func TestCooldown_GeminiProtocol(t *testing.T) {
	env := NewProxyTestEnv(t)
	srv := mockserver.New()
	defer srv.Close()

	providerID := createProvider(t, env, "mock-gemini", srv.URL, []string{"gemini"})
	createRoute(t, env, "gemini", providerID)

	// Send Gemini request that triggers 429
	resp := env.ProxyPost("/v1beta/models/gemini-2.5-flash:generateContent",
		map[string]any{
			"contents": []map[string]any{
				{"role": "user", "parts": []map[string]any{{"text": "hello"}}},
			},
		},
		map[string]string{
			mockserver.MockHeader: `{"status":429,"body":{"error":{"code":429,"message":"Quota exceeded","status":"RESOURCE_EXHAUSTED"}}}`,
		},
	)
	defer resp.Body.Close()
	assertStatus(t, resp, 429)

	// Verify cooldown
	cdResp := env.AdminGet("/api/admin/cooldowns")
	defer cdResp.Body.Close()
	cdBody, _ := io.ReadAll(cdResp.Body)
	cooldowns := gjson.ParseBytes(cdBody).Array()

	found := false
	for _, cd := range cooldowns {
		if cd.Get("providerID").Uint() == providerID {
			found = true
			t.Logf("Gemini cooldown: reason=%s", cd.Get("reason").String())
		}
	}
	if !found {
		t.Errorf("no cooldown for Gemini provider %d: %s", providerID, cdBody)
	}
}
