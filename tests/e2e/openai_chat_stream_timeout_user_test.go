package e2e_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newOpenAIChatStalledAfterChunkUpstream(t *testing.T, stall time.Duration) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("upstream path = %s, want /v1/chat/completions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("streaming not supported")
		}
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-stalled\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n")
		flusher.Flush()
		time.Sleep(stall)
	}))
}

func newOpenAIChatStalledBeforeFirstEventUpstream(t *testing.T, stall time.Duration) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("upstream path = %s, want /v1/chat/completions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("streaming not supported")
		}
		flusher.Flush()
		time.Sleep(stall)
	}))
}

func newOpenAIChatSuccessStreamUpstream(t *testing.T, marker string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("upstream path = %s, want /v1/chat/completions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-ok\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\""+marker+"\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
}

func createOpenAIChatTimeoutProvider(t *testing.T, env *ProxyTestEnv, name, baseURL string, enabled bool) uint64 {
	t.Helper()
	resp := env.AdminPost("/api/admin/providers", map[string]any{
		"name": name,
		"type": "custom",
		"config": map[string]any{
			"custom": map[string]any{
				"baseURL":                  baseURL,
				"apiKey":                   "sk-mock-test-key",
				"openAIChatStreamTimeouts": enabled,
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

func createOpenAIRouteAtPosition(t *testing.T, env *ProxyTestEnv, providerID uint64, position int) uint64 {
	t.Helper()
	resp := env.AdminPost("/api/admin/routes", map[string]any{
		"isEnabled":  true,
		"clientType": "openai",
		"providerID": providerID,
		"position":   position,
	})
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create route position %d: status=%d body=%s", position, resp.StatusCode, body)
	}
	var result struct {
		ID uint64 `json:"id"`
	}
	DecodeJSON(t, resp, &result)
	return result.ID
}

func proxyStreamPostWithClient(t *testing.T, env *ProxyTestEnv, client *http.Client, path string, body any) (*http.Response, error) {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal stream body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, env.URL(path), bytes.NewReader(data))
	if err != nil {
		t.Fatalf("create stream request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	return client.Do(req)
}

func TestOpenAIChatStreamTimeoutSwitchDefaultOffFromUserRequest(t *testing.T) {
	env := NewProxyTestEnv(t)
	stalled := newOpenAIChatStalledAfterChunkUpstream(t, 2*time.Second)
	defer stalled.Close()

	providerID := createOpenAIChatTimeoutProvider(t, env, "openai-chat-timeout-default-off", stalled.URL, false)
	createOpenAIRouteAtPosition(t, env, providerID, 1)

	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := proxyStreamPostWithClient(t, env, client, "/v1/chat/completions", map[string]any{
		"model":    "gpt-4o",
		"stream":   true,
		"messages": []map[string]any{{"role": "user", "content": "暴力测试默认关闭"}},
	})
	if err != nil {
		t.Fatalf("request should receive the first streamed chunk before client timeout: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	body, err := io.ReadAll(resp.Body)
	if err == nil {
		t.Fatalf("default-off request unexpectedly completed; body=%s", body)
	}
	if !strings.Contains(string(body), "hello") {
		t.Fatalf("client should see the upstream chunk before its own timeout, body=%s err=%v", body, err)
	}
}

func TestOpenAIChatStreamTimeoutSwitchFailsOverBeforeClientStalls(t *testing.T) {
	env := NewProxyTestEnv(t)
	stalled := newOpenAIChatStalledBeforeFirstEventUpstream(t, 5*time.Second)
	defer stalled.Close()
	success := newOpenAIChatSuccessStreamUpstream(t, "fallback-ok")
	defer success.Close()

	resp := env.AdminPut("/api/admin/settings/stream_timeouts_enabled", map[string]any{"value": "true"})
	AssertStatus(t, resp, http.StatusOK)
	resp = env.AdminPut("/api/admin/settings/stream_first_event_timeout_ms", map[string]any{"value": "1000"})
	AssertStatus(t, resp, http.StatusOK)
	resp = env.AdminPut("/api/admin/settings/stream_idle_timeout_ms", map[string]any{"value": "1000"})
	AssertStatus(t, resp, http.StatusOK)

	stalledProviderID := createOpenAIChatTimeoutProvider(t, env, "openai-chat-timeout-on-stalled", stalled.URL, true)
	successProviderID := createOpenAIChatTimeoutProvider(t, env, "openai-chat-timeout-on-success", success.URL, false)
	createOpenAIRouteAtPosition(t, env, stalledProviderID, 1)
	createOpenAIRouteAtPosition(t, env, successProviderID, 2)

	started := time.Now()
	resp = proxyStreamPost(t, env, "/v1/chat/completions", map[string]any{
		"model":    "gpt-4o",
		"stream":   true,
		"messages": []map[string]any{{"role": "user", "content": "暴力测试开启切换"}},
	}, nil)
	defer resp.Body.Close()
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("request took %s, want failover before AI SDK stalled timeout", elapsed)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if !strings.Contains(string(body), "fallback-ok") {
		t.Fatalf("expected fallback provider stream, body=%s", body)
	}
}
