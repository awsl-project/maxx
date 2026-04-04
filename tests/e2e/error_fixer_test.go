package e2e_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tidwall/gjson"
)

// newErrorThenSuccessUpstream creates a mock upstream that returns an error on
// the first request and a Claude success response on subsequent requests.
// The validateRetry callback can inspect the retried request body.
func newErrorThenSuccessUpstream(
	t *testing.T,
	errorStatus int,
	errorBody map[string]any,
	validateRetry func(t *testing.T, body []byte),
) *httptest.Server {
	t.Helper()
	var callCount atomic.Int32

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		n := callCount.Add(1)

		if n == 1 {
			// First call: return error
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(errorStatus)
			json.NewEncoder(w).Encode(errorBody)
			return
		}

		// Retry: validate the fixed request
		if validateRetry != nil {
			validateRetry(t, body)
		}

		// Return success
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":    "msg_mock_retry",
			"type":  "message",
			"role":  "assistant",
			"model": "claude-sonnet-4-20250514",
			"content": []map[string]any{
				{"type": "text", "text": "Hello from retry!"},
			},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 10, "output_tokens": 5},
		})
	}))
}

// --- Error Fixer Integration Tests ---

func TestErrorFixer_CacheControl(t *testing.T) {
	mock := newErrorThenSuccessUpstream(t, 400,
		map[string]any{
			"type": "error",
			"error": map[string]any{
				"type":    "invalid_request_error",
				"message": "system.2.cache_control.ephemeral.scope: Extra inputs are not permitted",
			},
		},
		func(t *testing.T, body []byte) {
			t.Helper()
			// Verify all cache_control fields are stripped
			system := gjson.GetBytes(body, "system")
			if system.IsArray() {
				system.ForEach(func(_, item gjson.Result) bool {
					if item.Get("cache_control").Exists() {
						t.Error("cache_control not stripped from system on retry")
					}
					return true
				})
			}
			messages := gjson.GetBytes(body, "messages")
			if messages.IsArray() {
				messages.ForEach(func(_, msg gjson.Result) bool {
					content := msg.Get("content")
					if content.IsArray() {
						content.ForEach(func(_, block gjson.Result) bool {
							if block.Get("cache_control").Exists() {
								t.Error("cache_control not stripped from messages on retry")
							}
							return true
						})
					}
					return true
				})
			}
		},
	)
	defer mock.Close()

	env := NewProxyTestEnv(t)
	providerID := createProvider(t, env, "bedrock-mock", mock.URL, []string{"claude"})
	createRoute(t, env, "claude", providerID)

	// Send request with cache_control in system and messages
	resp := env.ProxyPost("/v1/messages", map[string]any{
		"model":      "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"system": []map[string]any{
			{"type": "text", "text": "You are helpful.", "cache_control": map[string]any{"type": "ephemeral"}},
		},
		"messages": []map[string]any{
			{"role": "user", "content": []map[string]any{
				{"type": "text", "text": "Hello", "cache_control": map[string]any{"type": "ephemeral"}},
			}},
		},
	}, nil)
	defer resp.Body.Close()

	// Should succeed via retry
	assertStatus(t, resp, http.StatusOK)
	respBody, _ := io.ReadAll(resp.Body)
	if gjson.GetBytes(respBody, "content.0.text").String() != "Hello from retry!" {
		t.Errorf("unexpected response: %s", string(respBody))
	}
}

func TestErrorFixer_ExtraBodyFields(t *testing.T) {
	mock := newErrorThenSuccessUpstream(t, 400,
		map[string]any{
			"type": "error",
			"error": map[string]any{
				"type":    "invalid_request_error",
				"message": "output_config.effort: Extra inputs are not permitted",
			},
		},
		func(t *testing.T, body []byte) {
			t.Helper()
			if gjson.GetBytes(body, "output_config").Exists() {
				t.Error("output_config not stripped on retry")
			}
			if gjson.GetBytes(body, "context_management").Exists() {
				t.Error("context_management not stripped on retry")
			}
			if gjson.GetBytes(body, "reasoning").Exists() {
				t.Error("reasoning not stripped on retry")
			}
			// Core fields must survive
			if gjson.GetBytes(body, "model").String() == "" {
				t.Error("model was stripped")
			}
			if !gjson.GetBytes(body, "messages").Exists() {
				t.Error("messages was stripped")
			}
		},
	)
	defer mock.Close()

	env := NewProxyTestEnv(t)
	providerID := createProvider(t, env, "bedrock-mock-2", mock.URL, []string{"claude"})
	createRoute(t, env, "claude", providerID)

	resp := env.ProxyPost("/v1/messages", map[string]any{
		"model":              "claude-sonnet-4-20250514",
		"max_tokens":         1024,
		"output_config":      map[string]any{"effort": "high"},
		"context_management": map[string]any{"truncation": "auto"},
		"reasoning":          map[string]any{"budget_tokens": 5000},
		"messages": []map[string]any{
			{"role": "user", "content": "Hello"},
		},
	}, nil)
	defer resp.Body.Close()

	assertStatus(t, resp, http.StatusOK)
}

func TestErrorFixer_ToolCustomFields(t *testing.T) {
	mock := newErrorThenSuccessUpstream(t, 400,
		map[string]any{
			"type": "error",
			"error": map[string]any{
				"type":    "invalid_request_error",
				"message": "tools.0.custom.defer_loading: Extra inputs are not permitted",
			},
		},
		func(t *testing.T, body []byte) {
			t.Helper()
			tools := gjson.GetBytes(body, "tools")
			if tools.IsArray() {
				tools.ForEach(func(idx gjson.Result, tool gjson.Result) bool {
					if tool.Get("custom").Exists() {
						t.Errorf("tools[%d].custom not stripped on retry", idx.Int())
					}
					return true
				})
			}
			// Tool name and schema must survive
			if gjson.GetBytes(body, "tools.0.name").String() != "bash" {
				t.Error("tools[0].name was corrupted")
			}
		},
	)
	defer mock.Close()

	env := NewProxyTestEnv(t)
	providerID := createProvider(t, env, "bedrock-mock-3", mock.URL, []string{"claude"})
	createRoute(t, env, "claude", providerID)

	resp := env.ProxyPost("/v1/messages", map[string]any{
		"model":      "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"tools": []map[string]any{
			{
				"name":         "bash",
				"description":  "Run a command",
				"custom":       map[string]any{"defer_loading": true, "eager_input_streaming": true},
				"input_schema": map[string]any{"type": "object"},
			},
		},
		"messages": []map[string]any{
			{"role": "user", "content": "Hello"},
		},
	}, nil)
	defer resp.Body.Close()

	assertStatus(t, resp, http.StatusOK)
}

func TestErrorFixer_BetaHeader(t *testing.T) {
	var retryHeaders http.Header
	var callCount atomic.Int32

	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]any{
				"type": "error",
				"error": map[string]any{
					"type":    "invalid_request_error",
					"message": "Unexpected value(s) 'prompt-caching-scope-2026-01-05' for the 'anthropic-beta' header",
				},
			})
			return
		}
		retryHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":          "msg_mock_beta",
			"type":        "message",
			"role":        "assistant",
			"model":       "claude-sonnet-4-20250514",
			"content":     []map[string]any{{"type": "text", "text": "OK"}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 10, "output_tokens": 2},
		})
	}))
	defer mock.Close()

	env := NewProxyTestEnv(t)
	providerID := createProvider(t, env, "bedrock-mock-4", mock.URL, []string{"claude"})
	createRoute(t, env, "claude", providerID)

	resp := env.ProxyPost("/v1/messages", claudeRequest("claude-sonnet-4-20250514"), nil)
	defer resp.Body.Close()

	assertStatus(t, resp, http.StatusOK)

	// Verify rejected beta was removed from retry headers
	if retryHeaders != nil {
		beta := retryHeaders.Get("Anthropic-Beta")
		if beta != "" {
			// Ensure the rejected value is not present
			for _, rejected := range []string{"prompt-caching-scope-2026-01-05"} {
				if strings.Contains(beta, rejected) {
					t.Errorf("rejected beta %q still present in retry header: %s", rejected, beta)
				}
			}
		}
	}
}

func TestErrorFixer_MultipleFixers(t *testing.T) {
	// Error body mentions both cache_control and defer_loading — both fixers should run
	mock := newErrorThenSuccessUpstream(t, 400,
		map[string]any{
			"type": "error",
			"error": map[string]any{
				"type":    "invalid_request_error",
				"message": "cache_control: Extra inputs are not permitted; tools.0.custom.defer_loading: Extra inputs are not permitted",
			},
		},
		func(t *testing.T, body []byte) {
			t.Helper()
			// cache_control should be stripped
			if gjson.GetBytes(body, "system.0.cache_control").Exists() {
				t.Error("cache_control not stripped from system")
			}
			// tools custom should be stripped
			if gjson.GetBytes(body, "tools.0.custom").Exists() {
				t.Error("tools[0].custom not stripped")
			}
		},
	)
	defer mock.Close()

	env := NewProxyTestEnv(t)
	providerID := createProvider(t, env, "bedrock-mock-5", mock.URL, []string{"claude"})
	createRoute(t, env, "claude", providerID)

	resp := env.ProxyPost("/v1/messages", map[string]any{
		"model":      "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"system": []map[string]any{
			{"type": "text", "text": "Hello", "cache_control": map[string]any{"type": "ephemeral"}},
		},
		"tools": []map[string]any{
			{
				"name":         "bash",
				"description":  "Run a command",
				"custom":       map[string]any{"defer_loading": true},
				"input_schema": map[string]any{"type": "object"},
			},
		},
		"messages": []map[string]any{
			{"role": "user", "content": "Hello"},
		},
	}, nil)
	defer resp.Body.Close()

	assertStatus(t, resp, http.StatusOK)
}

func TestErrorFixer_NoRetryOnUnknownError(t *testing.T) {
	var callCount atomic.Int32

	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]any{
			"type": "error",
			"error": map[string]any{
				"type":    "invalid_request_error",
				"message": "model not found",
			},
		})
	}))
	defer mock.Close()

	env := NewProxyTestEnv(t)
	providerID := createProvider(t, env, "bedrock-mock-6", mock.URL, []string{"claude"})
	createRoute(t, env, "claude", providerID)

	resp := env.ProxyPost("/v1/messages", claudeRequest("claude-nonexistent"), nil)
	defer resp.Body.Close()

	// Should return 400 without retry
	assertStatus(t, resp, 400)

	// Should only have been called once (no retry)
	if count := callCount.Load(); count != 1 {
		t.Errorf("expected 1 upstream call (no retry), got %d", count)
	}
}

func TestErrorFixer_MaxRetriesExhausted(t *testing.T) {
	// Upstream always returns the same cache_control error no matter what.
	// The fixer matches every time, but the error never goes away.
	// Must detect no progress and stop immediately, not loop forever.
	var callCount atomic.Int32

	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]any{
			"type": "error",
			"error": map[string]any{
				"type":    "invalid_request_error",
				"message": "cache_control: Extra inputs are not permitted",
			},
		})
	}))
	defer mock.Close()

	env := NewProxyTestEnv(t)
	providerID := createProvider(t, env, "bedrock-mock-loop", mock.URL, []string{"claude"})
	createRoute(t, env, "claude", providerID)

	resp := env.ProxyPost("/v1/messages", map[string]any{
		"model":      "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"system": []map[string]any{
			{"type": "text", "text": "Hello", "cache_control": map[string]any{"type": "ephemeral"}},
		},
		"messages": []map[string]any{
			{"role": "user", "content": "Hello"},
		},
	}, nil)
	defer resp.Body.Close()

	// Should fail — the error never resolves
	assertStatus(t, resp, 400)

	// 1 original + 1 retry (fixer applied once, detected no progress on 2nd match) = exactly 2
	count := callCount.Load()
	if count != 2 {
		t.Errorf("expected exactly 2 upstream calls (1 original + 1 retry before detecting no progress), got %d", count)
	}
}

func TestErrorFixer_BedrockStripAll(t *testing.T) {
	// Bedrock error only mentions one field, but the bedrock fixer
	// detects "Bedrock Runtime" and strips ALL known fields at once.
	var callCount atomic.Int32

	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		n := callCount.Add(1)

		w.Header().Set("Content-Type", "application/json")

		if n == 1 {
			// Real Bedrock error format — only mentions one field
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]any{
				"type": "error",
				"error": map[string]any{
					"type":    "<nil>",
					"message": "InvokeModel: operation error Bedrock Runtime: InvokeModel, https response error StatusCode: 400, RequestID: fake-id, ValidationException: ***.***.***.custom: Extra inputs are not permitted",
				},
			})
			return
		}

		// Retry: bedrock fixer should have stripped ALL fields in one shot
		if gjson.GetBytes(body, "system.0.cache_control").Exists() {
			t.Error("cache_control not stripped")
		}
		if gjson.GetBytes(body, "output_config").Exists() {
			t.Error("output_config not stripped by bedrock fixer")
		}
		if gjson.GetBytes(body, "tools.0.custom").Exists() {
			t.Error("tools[0].custom not stripped by bedrock fixer")
		}
		// Core fields must survive
		if gjson.GetBytes(body, "model").String() != "claude-sonnet-4-20250514" {
			t.Error("model was corrupted")
		}
		if gjson.GetBytes(body, "tools.0.name").String() != "bash" {
			t.Error("tools[0].name was corrupted")
		}

		json.NewEncoder(w).Encode(map[string]any{
			"id":          "msg_bedrock_fixed",
			"type":        "message",
			"role":        "assistant",
			"model":       "claude-sonnet-4-20250514",
			"content":     []map[string]any{{"type": "text", "text": "One shot!"}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 10, "output_tokens": 2},
		})
	}))
	defer mock.Close()

	env := NewProxyTestEnv(t)
	providerID := createProvider(t, env, "bedrock-mock-all", mock.URL, []string{"claude"})
	createRoute(t, env, "claude", providerID)

	resp := env.ProxyPost("/v1/messages", map[string]any{
		"model":      "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"system": []map[string]any{
			{"type": "text", "text": "Hello", "cache_control": map[string]any{"type": "ephemeral"}},
		},
		"output_config": map[string]any{"effort": "high"},
		"tools": []map[string]any{
			{
				"name":         "bash",
				"description":  "Run a command",
				"custom":       map[string]any{"defer_loading": true},
				"input_schema": map[string]any{"type": "object"},
			},
		},
		"messages": []map[string]any{
			{"role": "user", "content": "Hello"},
		},
	}, nil)
	defer resp.Body.Close()

	assertStatus(t, resp, http.StatusOK)

	// Only 2 calls: 1 original + 1 retry (bedrock fixer fixes everything)
	if count := callCount.Load(); count != 2 {
		t.Errorf("expected 2 upstream calls (bedrock fixer in 1 retry), got %d", count)
	}
}

