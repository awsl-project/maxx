package e2e_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// TestDisableErrorCooldownRetriesBeyondRetryPolicy verifies the updated
// contract: when disableErrorCooldown is enabled, retry-attempt limits are
// still ignored, but only genuinely retryable classes keep retrying on the
// same provider. Most 4xx errors must fail over immediately; 429 and 5xx may
// continue retrying until success or context cancellation.
func TestDisableErrorCooldownRetriesBeyondRetryPolicy(t *testing.T) {
	cases := []struct {
		status               int
		name                 string
		expectStatus         int
		expectFailingHits    int64
		expectFallbackHits   int64
		expectProviderBody   string
		expectCooldownAbsent bool
	}{
		{status: http.StatusBadRequest, name: "400_request_error", expectStatus: http.StatusBadRequest, expectFailingHits: 1, expectFallbackHits: 0, expectProviderBody: "", expectCooldownAbsent: true},
		{status: http.StatusUnauthorized, name: "401_auth_error", expectStatus: http.StatusOK, expectFailingHits: 1, expectFallbackHits: 1, expectProviderBody: "fallback-ok", expectCooldownAbsent: true},
		{status: http.StatusForbidden, name: "403_auth_error", expectStatus: http.StatusOK, expectFailingHits: 1, expectFallbackHits: 1, expectProviderBody: "fallback-ok", expectCooldownAbsent: true},
		{status: http.StatusNotFound, name: "404_not_found", expectStatus: http.StatusOK, expectFailingHits: 1, expectFallbackHits: 1, expectProviderBody: "fallback-ok", expectCooldownAbsent: true},
		{status: http.StatusPaymentRequired, name: "402_quota_error", expectStatus: http.StatusOK, expectFailingHits: 1, expectFallbackHits: 1, expectProviderBody: "fallback-ok", expectCooldownAbsent: true},
		{status: http.StatusTeapot, name: "418_other_client_error", expectStatus: http.StatusTeapot, expectFailingHits: 1, expectFallbackHits: 0, expectProviderBody: "", expectCooldownAbsent: true},
		{status: http.StatusUnprocessableEntity, name: "422_request_error", expectStatus: http.StatusUnprocessableEntity, expectFailingHits: 1, expectFallbackHits: 0, expectProviderBody: "", expectCooldownAbsent: true},
		{status: http.StatusTooManyRequests, name: "429_rate_limit", expectStatus: http.StatusOK, expectFailingHits: 5, expectFallbackHits: 0, expectProviderBody: "eventual-success", expectCooldownAbsent: true},
		{status: http.StatusInternalServerError, name: "500_server_error", expectStatus: http.StatusOK, expectFailingHits: 5, expectFallbackHits: 0, expectProviderBody: "eventual-success", expectCooldownAbsent: true},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			env := NewProxyTestEnv(t)

			var failingHits atomic.Int64
			resilient := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hit := failingHits.Add(1)
				if hit > 4 {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id":      "chatcmpl-eventual-success",
						"object":  "chat.completion",
						"model":   "gpt-4o",
						"created": 1700000000,
						"choices": []map[string]any{{
							"index":         0,
							"message":       map[string]any{"role": "assistant", "content": "eventual-success"},
							"finish_reason": "stop",
						}},
						"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
					})
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"error":{"message":"Insufficient balance","type":"bad_response_status_code","code":"bad_response_status_code"}}`))
			}))
			defer resilient.Close()

			var fallbackHits atomic.Int64
			fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fallbackHits.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":      "chatcmpl-fallback",
					"object":  "chat.completion",
					"model":   "gpt-4o",
					"created": 1700000000,
					"choices": []map[string]any{{
						"index":         0,
						"message":       map[string]any{"role": "assistant", "content": "fallback-ok"},
						"finish_reason": "stop",
					}},
					"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
				})
			}))
			defer fallback.Close()

			resilientID := createDisableErrorCooldownProvider(t, env, "resilient-disable-cooldown", resilient.URL, true)
			fallbackID := createDisableErrorCooldownProvider(t, env, "fallback", fallback.URL, false)
			retryID := createFastRetryConfig(t, env, "retry-limit-zero")

			createRouteWithOptionalRetryConfig(t, env, "openai", resilientID, 1, retryID)
			createRouteWithOptionalRetryConfig(t, env, "openai", fallbackID, 2, 0)

			resp := env.ProxyPost("/v1/chat/completions", openaiRequest("gpt-4o"), nil)
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			if resp.StatusCode != tt.expectStatus {
				t.Fatalf("final status=%d body=%s, want %d", resp.StatusCode, body, tt.expectStatus)
			}
			if failingHits.Load() != tt.expectFailingHits {
				t.Fatalf("status %d provider hits=%d, want %d", tt.status, failingHits.Load(), tt.expectFailingHits)
			}
			if fallbackHits.Load() != tt.expectFallbackHits {
				t.Fatalf("status %d fallback hits=%d, want %d", tt.status, fallbackHits.Load(), tt.expectFallbackHits)
			}
			if tt.expectProviderBody != "" && !jsonBodyContainsContent(body, tt.expectProviderBody) {
				t.Fatalf("status %d body=%s, want content %q", tt.status, body, tt.expectProviderBody)
			}
			if tt.expectCooldownAbsent {
				assertNoCooldownForProvider(t, env, resilientID)
			}
		})
	}
}

func jsonBodyContainsContent(body []byte, want string) bool {
	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	return len(payload.Choices) > 0 && payload.Choices[0].Message.Content == want
}

func createDisableErrorCooldownProvider(t *testing.T, env *ProxyTestEnv, name, baseURL string, disable bool) uint64 {
	t.Helper()
	resp := env.AdminPost("/api/admin/providers", map[string]any{
		"name": name,
		"type": "custom",
		"config": map[string]any{
			"disableErrorCooldown": disable,
			"custom": map[string]any{
				"baseURL": baseURL,
				"apiKey":  "sk-test-key",
			},
		},
		"supportedClientTypes": []string{"openai"},
	})
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create provider %s: status=%d body=%s", name, resp.StatusCode, body)
	}
	var result struct {
		ID uint64 `json:"id"`
	}
	DecodeJSON(t, resp, &result)
	return result.ID
}

func createFastRetryConfig(t *testing.T, env *ProxyTestEnv, name string) uint64 {
	t.Helper()
	resp := env.AdminPost("/api/admin/retry-configs", map[string]any{
		"name":            name,
		"maxRetries":      0,
		"initialInterval": 1,
		"backoffRate":     1.0,
		"maxInterval":     1,
	})
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create retry config: status=%d body=%s", resp.StatusCode, body)
	}
	var result struct {
		ID uint64 `json:"id"`
	}
	DecodeJSON(t, resp, &result)
	return result.ID
}

func createRouteWithOptionalRetryConfig(t *testing.T, env *ProxyTestEnv, clientType string, providerID uint64, position int, retryConfigID uint64) {
	t.Helper()
	body := map[string]any{
		"isEnabled":  true,
		"clientType": clientType,
		"providerID": providerID,
		"position":   position,
		"weight":     1,
	}
	if retryConfigID != 0 {
		body["retryConfigID"] = retryConfigID
	}
	resp := env.AdminPost("/api/admin/routes", body)
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create route provider=%d: status=%d body=%s", providerID, resp.StatusCode, b)
	}
	resp.Body.Close()
}

func assertNoCooldownForProvider(t *testing.T, env *ProxyTestEnv, providerID uint64) {
	t.Helper()
	resp := env.AdminGet("/api/admin/cooldowns")
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("list cooldowns: status=%d body=%s", resp.StatusCode, body)
	}
	var cooldowns []map[string]any
	DecodeJSON(t, resp, &cooldowns)
	for _, cd := range cooldowns {
		got, ok := cd["providerID"].(float64)
		if ok && uint64(got) == providerID {
			t.Fatalf("expected no cooldown for provider %d, found %#v", providerID, cd)
		}
		got, ok = cd["providerId"].(float64)
		if ok && uint64(got) == providerID {
			t.Fatalf("expected no cooldown for provider %d, found %#v", providerID, cd)
		}
		got, ok = cd["provider_id"].(float64)
		if ok && uint64(got) == providerID {
			t.Fatalf("expected no cooldown for provider %d, found %#v", providerID, cd)
		}
	}
}
