package e2e_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/executor"
)

func TestFailoverPrefersHealthyProviderAfterTimeoutSeries(t *testing.T) {
	env := NewProxyTestEnv(t)

	var slowFailures atomic.Int32
	var healthyHits atomic.Int32

	slowProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slowFailures.Add(1)
		time.Sleep(60 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "temporary upstream failure",
			},
		})
	}))
	defer closeTestServer(slowProvider)

	healthyProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		healthyHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-healthy",
			"object":  "chat.completion",
			"model":   "gpt-4o",
			"created": 1700000001,
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "healthy provider",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		})
	}))
	defer closeTestServer(healthyProvider)

	slowProviderID := createProviderWithErrorCooldownDisabled(t, env, "slow-openai", slowProvider.URL, []string{"openai"})
	healthyProviderID := createProvider(t, env, "healthy-openai", healthyProvider.URL, []string{"openai"})
	createRouteWithPosition(t, env, "openai", slowProviderID, 1)
	createRouteWithPosition(t, env, "openai", healthyProviderID, 2)

	firstResp := env.ProxyPost("/v1/chat/completions", openaiRequest("gpt-4o"), nil)
	defer firstResp.Body.Close()
	assertStatus(t, firstResp, http.StatusOK)
	if body, _ := io.ReadAll(firstResp.Body); !strings.Contains(string(body), "healthy provider") {
		t.Fatalf("first response body = %s, want healthy provider response", string(body))
	}

	firstSlowHits := slowFailures.Load()
	firstHealthyHits := healthyHits.Load()
	if firstSlowHits != 1 || firstHealthyHits != 1 {
		t.Fatalf("first request hits = slow:%d healthy:%d, want both providers hit once", firstSlowHits, firstHealthyHits)
	}

	secondResp := env.ProxyPost("/v1/chat/completions", openaiRequest("gpt-4o"), nil)
	defer secondResp.Body.Close()
	assertStatus(t, secondResp, http.StatusOK)

	if slowFailures.Load() != firstSlowHits {
		t.Fatalf("slow provider hit count after second request = %d, want unchanged at %d", slowFailures.Load(), firstSlowHits)
	}
	if healthyHits.Load() != firstHealthyHits+1 {
		t.Fatalf("healthy provider hit count after second request = %d, want %d", healthyHits.Load(), firstHealthyHits+1)
	}
}

func TestCircuitBreakerSkipsProviderDuringOpenWindow(t *testing.T) {
	env := NewProxyTestEnv(t)

	var badProviderHits atomic.Int32
	var healthyProviderHits atomic.Int32

	badProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		badProviderHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "still broken",
			},
		})
	}))
	defer closeTestServer(badProvider)

	healthyProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		healthyProviderHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-healthy",
			"object":  "chat.completion",
			"model":   "gpt-4o",
			"created": 1700000002,
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "healthy fallback",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		})
	}))
	defer closeTestServer(healthyProvider)

	badProviderID := createProviderWithErrorCooldownDisabled(t, env, "broken-openai", badProvider.URL, []string{"openai"})
	createRouteWithPosition(t, env, "openai", badProviderID, 1)

	for attempt := 0; attempt < 3; attempt++ {
		resp := env.ProxyPost("/v1/chat/completions", openaiRequest("gpt-4o"), nil)
		if resp.StatusCode == http.StatusOK {
			t.Fatalf("warmup request %d unexpectedly succeeded; want bad provider only failure", attempt+1)
		}
		resp.Body.Close()
	}

	if badProviderHits.Load() != 3 {
		t.Fatalf("bad provider hit count after breaker warmup = %d, want 3", badProviderHits.Load())
	}

	healthyProviderID := createProvider(t, env, "healthy-openai", healthyProvider.URL, []string{"openai"})
	createRouteWithPosition(t, env, "openai", healthyProviderID, 2)

	resp := env.ProxyPost("/v1/chat/completions", openaiRequest("gpt-4o"), nil)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	if badProviderHits.Load() != 3 {
		t.Fatalf("bad provider hit count during open window = %d, want breaker to keep it at 3", badProviderHits.Load())
	}
	if healthyProviderHits.Load() != 1 {
		t.Fatalf("healthy provider hit count during open window = %d, want 1", healthyProviderHits.Load())
	}
}

func TestRequestBudgetStopsLongSerialFailover(t *testing.T) {
	env := NewProxyTestEnvWithAttemptBudget(t, executor.AttemptBudget{
		RequestTimeout:    120 * time.Millisecond,
		TotalTimeout:      60 * time.Millisecond,
		FirstByteTimeout:  60 * time.Millisecond,
		StreamIdleTimeout: 60 * time.Millisecond,
		MaxRetryAfter:     10 * time.Millisecond,
		MaxRetryWait:      10 * time.Millisecond,
	})

	var slowHits [3]atomic.Int32
	var healthyHits atomic.Int32

	newSlowProvider := func(idx int) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			slowHits[idx].Add(1)
			defer r.Body.Close()
			_, _ = io.Copy(io.Discard, r.Body)
			select {
			case <-r.Context().Done():
			case <-time.After(100 * time.Millisecond):
				// HTTP/1.1 test servers do not always surface client cancellation promptly
				// when the handler neither reads nor writes further data.
			}
		}))
	}

	slowProvider1 := newSlowProvider(0)
	defer closeTestServer(slowProvider1)
	slowProvider2 := newSlowProvider(1)
	defer closeTestServer(slowProvider2)
	slowProvider3 := newSlowProvider(2)
	defer closeTestServer(slowProvider3)

	healthyProvider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		healthyHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-budget-healthy",
			"object":  "chat.completion",
			"model":   "gpt-4o",
			"created": 1700000003,
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "should not be reached",
					},
					"finish_reason": "stop",
				},
			},
		})
	}))
	defer closeTestServer(healthyProvider)

	slowProviderID1 := createProviderWithErrorCooldownDisabled(t, env, "budget-slow-1", slowProvider1.URL, []string{"openai"})
	slowProviderID2 := createProviderWithErrorCooldownDisabled(t, env, "budget-slow-2", slowProvider2.URL, []string{"openai"})
	slowProviderID3 := createProviderWithErrorCooldownDisabled(t, env, "budget-slow-3", slowProvider3.URL, []string{"openai"})
	healthyProviderID := createProvider(t, env, "budget-healthy", healthyProvider.URL, []string{"openai"})

	createRouteWithPosition(t, env, "openai", slowProviderID1, 1)
	createRouteWithPosition(t, env, "openai", slowProviderID2, 2)
	createRouteWithPosition(t, env, "openai", slowProviderID3, 3)
	createRouteWithPosition(t, env, "openai", healthyProviderID, 4)

	resp := env.ProxyPost("/v1/chat/completions", openaiRequest("gpt-4o"), nil)
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("response unexpectedly succeeded: status=%d body=%s", resp.StatusCode, body)
	}
	if resp.StatusCode != http.StatusGatewayTimeout {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("response status = %d body=%s, want request budget exhaustion to surface as 504", resp.StatusCode, body)
	}
	if healthyHits.Load() != 0 {
		t.Fatalf("healthy provider hit count = %d, want request budget exhaustion before healthy fallback", healthyHits.Load())
	}
	firstTwoSlowHits := slowHits[0].Load() + slowHits[1].Load()
	if firstTwoSlowHits < 1 || firstTwoSlowHits > 2 {
		t.Fatalf("slow provider hits = [%d %d %d], want only the first one or two slow routes attempted before budget exhaustion", slowHits[0].Load(), slowHits[1].Load(), slowHits[2].Load())
	}
	if slowHits[2].Load() != 0 {
		t.Fatalf("third slow provider hit count = %d, want shared request budget to stop before attempting route 3", slowHits[2].Load())
	}
}

func createRouteWithPosition(t *testing.T, env *ProxyTestEnv, clientType string, providerID uint64, position int) uint64 {
	t.Helper()
	resp := env.AdminPost("/api/admin/routes", map[string]any{
		"isEnabled":  true,
		"clientType": clientType,
		"providerID": providerID,
		"position":   position,
	})
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Failed to create route: status=%d body=%s", resp.StatusCode, body)
	}

	var result struct {
		ID uint64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode route create response: %v", err)
	}
	resp.Body.Close()
	return result.ID
}

func createProviderWithErrorCooldownDisabled(t *testing.T, env *ProxyTestEnv, name, baseURL string, supportedTypes []string) uint64 {
	t.Helper()
	resp := env.AdminPost("/api/admin/providers", map[string]any{
		"name": name,
		"type": "custom",
		"config": map[string]any{
			"disableErrorCooldown": true,
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
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode provider create response: %v", err)
	}
	resp.Body.Close()
	return result.ID
}

func closeTestServer(server *httptest.Server) {
	if server == nil {
		return
	}
	server.CloseClientConnections()
	server.Close()
}
