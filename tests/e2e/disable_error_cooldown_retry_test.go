package e2e_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// TestDisableErrorCooldownRetriesAllHTTPErrorStatuses verifies the provider
// switch contract: when disableErrorCooldown is enabled, upstream HTTP error
// responses must not become request-level early exits. Every HTTP 4xx/5xx error
// should follow the route retry policy first, then fail over to the next route.
func TestDisableErrorCooldownRetriesAllHTTPErrorStatuses(t *testing.T) {
	cases := []struct {
		status int
		name   string
	}{
		{status: http.StatusBadRequest, name: "400_request_error"},
		{status: http.StatusUnauthorized, name: "401_auth_error"},
		{status: http.StatusForbidden, name: "403_auth_error"},
		{status: http.StatusNotFound, name: "404_not_found"},
		{status: http.StatusPaymentRequired, name: "402_quota_error"},
		{status: http.StatusTeapot, name: "418_other_client_error"},
		{status: http.StatusUnprocessableEntity, name: "422_request_error"},
		{status: http.StatusTooManyRequests, name: "429_rate_limit"},
		{status: http.StatusInternalServerError, name: "500_server_error"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			env := NewProxyTestEnv(t)

			var failingHits atomic.Int64
			failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				failingHits.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"error":{"message":"Insufficient balance","type":"bad_response_status_code","code":"bad_response_status_code"}}`))
			}))
			defer failing.Close()

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

			failingID := createDisableErrorCooldownProvider(t, env, "failing-disable-cooldown", failing.URL, true)
			fallbackID := createDisableErrorCooldownProvider(t, env, "fallback", fallback.URL, false)
			retryID := createFastRetryConfig(t, env, "retry-all-http-errors")

			createRouteWithOptionalRetryConfig(t, env, "openai", failingID, 1, retryID)
			createRouteWithOptionalRetryConfig(t, env, "openai", fallbackID, 2, 0)

			resp := env.ProxyPost("/v1/chat/completions", openaiRequest("gpt-4o"), nil)
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("final status=%d body=%s, want 200 from fallback", resp.StatusCode, body)
			}
			if failingHits.Load() != 3 {
				t.Fatalf("status %d failing hits=%d, want 3 (initial + 2 retries)", tt.status, failingHits.Load())
			}
			if fallbackHits.Load() != 1 {
				t.Fatalf("status %d fallback hits=%d, want 1", tt.status, fallbackHits.Load())
			}
			assertNoCooldownForProvider(t, env, failingID)
		})
	}
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
		"maxRetries":      2,
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
