package e2e_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

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
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()
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
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()
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
		"model":            model,
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
	clientType  string // route clientType
	clientPath  string // URL path to send request to
	clientReq   map[string]any
	// Upstream side
	upstreamType      string // provider supportedClientTypes
	upstreamMock      func(t *testing.T, captured *capturedRequest) *httptest.Server
	expectedUpPath    string // expected upstream path prefix
	expectedUpField   string // field that should exist in upstream request body
	// Response verification
	respAssert func(t *testing.T, respBody []byte)
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
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.Error("Expected non-200 status for upstream error")
	}
}

func TestProxyNoMatchingRoute(t *testing.T) {
	env := NewProxyTestEnv(t)

	resp := env.ProxyPost("/v1/chat/completions", openaiRequest("gpt-4o"), nil)
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.Error("Expected non-200 status when no routes match")
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
