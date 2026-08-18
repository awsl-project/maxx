package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// These tests exercise the first-class openrouter provider end-to-end against a
// mock upstream (no live key, CI-safe). MAXX_OPENROUTER_BASE_URL redirects the
// adapter's fixed base URL at the mock server.

// newOpenRouterMock returns a mock OpenRouter upstream: it records the last
// request, counts calls, and shapes the response by path (/v1/messages →
// Anthropic, else OpenAI). An Authorization containing "fail-500"/"fail-401"
// forces that status so provider-error and failover paths can be driven.
func newOpenRouterMock(t *testing.T, captured *capturedRequest, calls *int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(calls, 1)
		// Read the body once and restore it so captured.Set records it too; the
		// streaming branch needs to know whether the client asked for stream:true.
		reqBody, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(reqBody))
		captured.Set(r)
		auth := r.Header.Get("Authorization")
		switch {
		case strings.Contains(auth, "fail-500"):
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"boom","code":500}}`))
			return
		case strings.Contains(auth, "fail-401"):
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"no auth","code":401}}`))
			return
		}
		isMessages := strings.Contains(r.URL.Path, "/messages")
		if bytes.Contains(reqBody, []byte(`"stream":true`)) {
			writeOpenRouterMockStream(t, w, isMessages)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if isMessages {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "msg_or", "type": "message", "role": "assistant",
				"model":       "anthropic/claude-3-haiku",
				"content":     []map[string]any{{"type": "text", "text": "hi"}},
				"stop_reason": "end_turn",
				"usage":       map[string]any{"input_tokens": 3, "output_tokens": 1},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl_or", "object": "chat.completion",
			"model": "openai/gpt-4o-mini", "created": 1700000000,
			"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": "hi"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 1, "total_tokens": 4},
		})
	}))
}

// writeOpenRouterMockStream emits a minimal SSE response in the format matching
// the client: Anthropic message events for /messages, OpenAI chunk deltas ending
// in [DONE] otherwise. It flushes so the proxy sees a real streaming upstream.
func writeOpenRouterMockStream(t *testing.T, w http.ResponseWriter, isMessages bool) {
	t.Helper()
	flusher, ok := w.(http.Flusher)
	if !ok {
		t.Error("mock upstream: streaming not supported by ResponseWriter")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	var frames []string
	if isMessages {
		frames = []string{
			`event: message_start` + "\n" + `data: {"type":"message_start","message":{"id":"msg_or","role":"assistant","model":"anthropic/claude-3-haiku","content":[],"usage":{"input_tokens":3,"output_tokens":0}}}` + "\n\n",
			`event: content_block_delta` + "\n" + `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}` + "\n\n",
			`event: message_stop` + "\n" + `data: {"type":"message_stop"}` + "\n\n",
		}
	} else {
		frames = []string{
			`data: {"id":"chatcmpl_or","object":"chat.completion.chunk","model":"openai/gpt-4o-mini","choices":[{"index":0,"delta":{"role":"assistant","content":"hi"}}]}` + "\n\n",
			`data: {"id":"chatcmpl_or","object":"chat.completion.chunk","model":"openai/gpt-4o-mini","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\n",
			"data: [DONE]\n\n",
		}
	}
	for _, f := range frames {
		if _, err := io.WriteString(w, f); err != nil {
			t.Errorf("mock upstream: write SSE frame: %v", err)
			return
		}
		flusher.Flush()
	}
}

func createORProvider(t *testing.T, env *ProxyTestEnv, name, apiKey string, clientTypes []string) uint64 {
	t.Helper()
	resp := env.AdminPost("/api/admin/providers", map[string]any{
		"name":                 name,
		"type":                 "openrouter",
		"config":               map[string]any{"openrouter": map[string]any{"apiKey": apiKey}},
		"supportedClientTypes": clientTypes,
	})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create openrouter provider %s: status=%d body=%s", name, resp.StatusCode, b)
	}
	var out struct {
		ID uint64 `json:"id"`
	}
	DecodeJSON(t, resp, &out)
	return out.ID
}

func createRouteAt(t *testing.T, env *ProxyTestEnv, clientType string, providerID uint64, position int) {
	t.Helper()
	resp := env.AdminPost("/api/admin/routes", map[string]any{
		"isEnabled":  true,
		"clientType": clientType,
		"providerID": providerID,
		"position":   position,
	})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create route: status=%d body=%s", resp.StatusCode, b)
	}
	resp.Body.Close()
}

// Delegation: both formats reach the mock at the right path, the provider key
// overwrites the client's Authorization, and disguise is off (no cache_control).
func TestOpenRouterMockDelegation(t *testing.T) {
	captured := &capturedRequest{}
	var calls int64
	mock := newOpenRouterMock(t, captured, &calls)
	defer mock.Close()
	t.Setenv("MAXX_OPENROUTER_BASE_URL", mock.URL)

	env := NewProxyTestEnv(t)
	pid := createORProvider(t, env, "or-mock", "sk-test-key", []string{"openai", "claude"})
	createRouteAt(t, env, "openai", pid, 1)
	createRouteAt(t, env, "claude", pid, 1)

	// OpenAI passthrough
	resp := env.ProxyPost("/v1/chat/completions", openaiRequest("openai/gpt-4o-mini"),
		map[string]string{"Authorization": "Bearer client-placeholder"})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	_, path, hdr, _ := captured.Get()
	if path != "/v1/chat/completions" {
		t.Errorf("openai upstream path = %s, want /v1/chat/completions", path)
	}
	if got := hdr.Get("Authorization"); got != "Bearer sk-test-key" {
		t.Errorf("openai upstream auth = %q, want Bearer sk-test-key (provider key overwrites client)", got)
	}

	// Claude passthrough — disguise must be off (no injected prompt-caching)
	resp = env.ProxyPost("/v1/messages", claudeRequest("anthropic/claude-3-haiku"),
		map[string]string{"anthropic-version": "2023-06-01"})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	_, path, _, upBody := captured.Get()
	if path != "/v1/messages" {
		t.Errorf("claude upstream path = %s, want /v1/messages", path)
	}
	if strings.Contains(string(upBody), "cache_control") {
		t.Errorf("disguise should be off: upstream body must not contain cache_control: %s", upBody)
	}
}

// A failing upstream is surfaced (not masked as 200).
func TestOpenRouterMockUpstreamError(t *testing.T) {
	captured := &capturedRequest{}
	var calls int64
	mock := newOpenRouterMock(t, captured, &calls)
	defer mock.Close()
	t.Setenv("MAXX_OPENROUTER_BASE_URL", mock.URL)

	env := NewProxyTestEnv(t)
	pid := createORProvider(t, env, "or-badkey", "fail-401-key", []string{"openai"})
	createRouteAt(t, env, "openai", pid, 1)

	resp := env.ProxyPost("/v1/chat/completions", openaiRequest("openai/gpt-4o-mini"),
		map[string]string{"Authorization": "Bearer client-placeholder"})
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("expected non-200 when upstream returns 401")
	}
}

// A 5xx on the first provider fails over to the next route's provider.
func TestOpenRouterMockFailover(t *testing.T) {
	captured := &capturedRequest{}
	var calls int64
	mock := newOpenRouterMock(t, captured, &calls)
	defer mock.Close()
	t.Setenv("MAXX_OPENROUTER_BASE_URL", mock.URL)

	env := NewProxyTestEnv(t)
	bad := createORProvider(t, env, "or-bad", "fail-500-key", []string{"openai"})
	good := createORProvider(t, env, "or-good", "sk-good-key", []string{"openai"})
	createRouteAt(t, env, "openai", bad, 1)
	createRouteAt(t, env, "openai", good, 2)

	resp := env.ProxyPost("/v1/chat/completions", openaiRequest("openai/gpt-4o-mini"),
		map[string]string{"Authorization": "Bearer client-placeholder"})
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
	if n := atomic.LoadInt64(&calls); n < 2 {
		t.Errorf("expected >=2 upstream calls (failover), got %d", n)
	}
}

// A provider that only enables openai has no route for claude traffic.
func TestOpenRouterMockSingleClientType(t *testing.T) {
	captured := &capturedRequest{}
	var calls int64
	mock := newOpenRouterMock(t, captured, &calls)
	defer mock.Close()
	t.Setenv("MAXX_OPENROUTER_BASE_URL", mock.URL)

	env := NewProxyTestEnv(t)
	pid := createORProvider(t, env, "or-openai-only", "sk-test-key", []string{"openai"})
	createRouteAt(t, env, "openai", pid, 1)

	// openai works
	resp := env.ProxyPost("/v1/chat/completions", openaiRequest("openai/gpt-4o-mini"),
		map[string]string{"Authorization": "Bearer client-placeholder"})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// claude has no route → not a 200
	resp = env.ProxyPost("/v1/messages", claudeRequest("anthropic/claude-3-haiku"),
		map[string]string{"anthropic-version": "2023-06-01"})
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("claude request should have no route on an openai-only provider")
	}
}

// Secrets: the API key is redacted on read, and a blank apiKey on edit preserves
// the stored key (verified by the key the upstream actually receives).
func TestOpenRouterMockSecretHandling(t *testing.T) {
	captured := &capturedRequest{}
	var calls int64
	mock := newOpenRouterMock(t, captured, &calls)
	defer mock.Close()
	t.Setenv("MAXX_OPENROUTER_BASE_URL", mock.URL)

	env := NewProxyTestEnv(t)

	// Redaction: for an export-excluded provider, secrets are write-only even for
	// admins, so the raw key must not appear in an admin read. (Non-excluded
	// providers intentionally return the key to admins so they can edit/export.)
	rresp := env.AdminPost("/api/admin/providers", map[string]any{
		"name":                 "or-redact",
		"type":                 "openrouter",
		"config":               map[string]any{"openrouter": map[string]any{"apiKey": "sk-secret-xyz"}},
		"supportedClientTypes": []string{"openai"},
		"excludeFromExport":    true,
	})
	if rresp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(rresp.Body)
		t.Fatalf("create export-excluded provider: status=%d body=%s", rresp.StatusCode, b)
	}
	rresp.Body.Close()
	lresp := env.AdminGet("/api/admin/providers")
	lb, _ := io.ReadAll(lresp.Body)
	lresp.Body.Close()
	if strings.Contains(string(lb), "sk-secret-xyz") {
		t.Fatalf("export-excluded provider leaked apiKey: %s", lb)
	}

	pid := createORProvider(t, env, "or-secret", "sk-keepme", []string{"openai"})
	createRouteAt(t, env, "openai", pid, 1)

	// Edit with a blank apiKey → stored key is preserved.
	uresp := env.AdminPut(fmt.Sprintf("/api/admin/providers/%d", pid), map[string]any{
		"name":                 "or-secret-renamed",
		"type":                 "openrouter",
		"config":               map[string]any{"openrouter": map[string]any{"apiKey": ""}},
		"supportedClientTypes": []string{"openai"},
	})
	if uresp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(uresp.Body)
		t.Fatalf("update provider: status=%d body=%s", uresp.StatusCode, b)
	}
	uresp.Body.Close()

	resp := env.ProxyPost("/v1/chat/completions", openaiRequest("openai/gpt-4o-mini"),
		map[string]string{"Authorization": "Bearer client-placeholder"})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	_, _, hdr, _ := captured.Get()
	if got := hdr.Get("Authorization"); got != "Bearer sk-keepme" {
		t.Errorf("blank-apiKey edit should preserve stored key; upstream auth = %q, want Bearer sk-keepme", got)
	}
}

// Streaming: an SSE upstream is streamed back untouched for both formats, and
// the proxy still asks the upstream for a stream (stream:true in the body).
func TestOpenRouterMockStreaming(t *testing.T) {
	captured := &capturedRequest{}
	var calls int64
	mock := newOpenRouterMock(t, captured, &calls)
	defer mock.Close()
	t.Setenv("MAXX_OPENROUTER_BASE_URL", mock.URL)

	env := NewProxyTestEnv(t)
	pid := createORProvider(t, env, "or-stream", "sk-test-key", []string{"openai", "claude"})
	createRouteAt(t, env, "openai", pid, 1)
	createRouteAt(t, env, "claude", pid, 1)

	t.Run("openai_stream", func(t *testing.T) {
		req := openaiRequest("openai/gpt-4o-mini")
		req["stream"] = true
		resp := env.ProxyPost("/v1/chat/completions", req,
			map[string]string{"Authorization": "Bearer client-placeholder"})
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusOK)
		if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
			t.Fatalf("openai stream Content-Type = %q, want text/event-stream", ct)
		}
		body, _ := io.ReadAll(resp.Body)
		s := string(body)
		if !strings.Contains(s, "data:") || !strings.Contains(s, "[DONE]") {
			t.Fatalf("expected SSE ending in [DONE], got: %s", s)
		}
		if _, _, _, up := captured.Get(); !bytes.Contains(up, []byte(`"stream":true`)) {
			t.Errorf("upstream request should carry stream:true, body=%s", up)
		}
	})

	t.Run("claude_stream", func(t *testing.T) {
		req := claudeRequest("anthropic/claude-3-haiku")
		req["stream"] = true
		resp := env.ProxyPost("/v1/messages", req,
			map[string]string{"anthropic-version": "2023-06-01"})
		defer resp.Body.Close()
		assertStatus(t, resp, http.StatusOK)
		if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
			t.Fatalf("claude stream Content-Type = %q, want text/event-stream", ct)
		}
		body, _ := io.ReadAll(resp.Body)
		if s := string(body); !strings.Contains(s, "message_start") && !strings.Contains(s, "content_block_delta") {
			t.Fatalf("expected Anthropic SSE events, got: %s", s)
		}
	})
}

// Model mapping: a provider-scoped ModelMapping rewrites the client's alias to a
// real id in the request that actually reaches the upstream. (The live test only
// asserts the response model; here we assert the outgoing upstream request body,
// which is what executor.mapModel rewrites before delegation.)
func TestOpenRouterMockModelMapping(t *testing.T) {
	captured := &capturedRequest{}
	var calls int64
	mock := newOpenRouterMock(t, captured, &calls)
	defer mock.Close()
	t.Setenv("MAXX_OPENROUTER_BASE_URL", mock.URL)

	env := NewProxyTestEnv(t)
	pid := createORProvider(t, env, "or-map", "sk-test-key", []string{"openai"})
	createRouteAt(t, env, "openai", pid, 1)

	mresp := env.AdminPost("/api/admin/model-mappings", map[string]any{
		"scope":        "provider",
		"providerID":   pid,
		"providerType": "openrouter",
		"pattern":      "or-alias-mini",
		"target":       "openai/gpt-4o-mini",
		"priority":     10,
		"isEnabled":    true,
	})
	if mresp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(mresp.Body)
		t.Fatalf("create model mapping: status=%d body=%s", mresp.StatusCode, b)
	}
	mresp.Body.Close()

	resp := env.ProxyPost("/v1/chat/completions", openaiRequest("or-alias-mini"),
		map[string]string{"Authorization": "Bearer client-placeholder"})
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	_, _, _, up := captured.Get()
	model := gjsonModel(up)
	if model != "openai/gpt-4o-mini" {
		t.Errorf("upstream request model = %q, want openai/gpt-4o-mini (mapping applied); body=%s", model, up)
	}
	if strings.Contains(string(up), "or-alias-mini") {
		t.Errorf("upstream body should not still contain the alias; body=%s", up)
	}
}

// gjsonModel extracts the top-level "model" field from a JSON request body.
func gjsonModel(body []byte) string {
	var m struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(body, &m)
	return m.Model
}

// Base URL composition: production base is "https://openrouter.ai/api", so the
// client's versioned path is appended onto the /api prefix (→ /api/v1/...). Every
// other test points the base at a bare mock.URL, so this is the ONLY test that
// exercises the /api join — a guard on defaultBaseURL + buildUpstreamURL against
// a regression that would send traffic to the wrong upstream path.
func TestOpenRouterMockBaseURLComposition(t *testing.T) {
	captured := &capturedRequest{}
	var calls int64
	mock := newOpenRouterMock(t, captured, &calls)
	defer mock.Close()
	t.Setenv("MAXX_OPENROUTER_BASE_URL", mock.URL+"/api")

	env := NewProxyTestEnv(t)
	pid := createORProvider(t, env, "or-basepath", "sk-test-key", []string{"openai", "claude"})
	createRouteAt(t, env, "openai", pid, 1)
	createRouteAt(t, env, "claude", pid, 1)

	resp := env.ProxyPost("/v1/chat/completions", openaiRequest("openai/gpt-4o-mini"),
		map[string]string{"Authorization": "Bearer client-placeholder"})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	if _, path, _, _ := captured.Get(); path != "/api/v1/chat/completions" {
		t.Errorf("openai upstream path = %s, want /api/v1/chat/completions (/api prefix preserved)", path)
	}

	resp = env.ProxyPost("/v1/messages", claudeRequest("anthropic/claude-3-haiku"),
		map[string]string{"anthropic-version": "2023-06-01"})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	if _, path, _, _ := captured.Get(); path != "/api/v1/messages" {
		t.Errorf("claude upstream path = %s, want /api/v1/messages (/api prefix preserved)", path)
	}
}

// Header fidelity: OpenRouter's app-attribution headers (HTTP-Referer, X-Title)
// forward to upstream, privacy/hop headers (X-Forwarded-For) are stripped, and
// the provider key still overrides the client's Authorization.
func TestOpenRouterMockHeaderPassthrough(t *testing.T) {
	captured := &capturedRequest{}
	var calls int64
	mock := newOpenRouterMock(t, captured, &calls)
	defer mock.Close()
	t.Setenv("MAXX_OPENROUTER_BASE_URL", mock.URL)

	env := NewProxyTestEnv(t)
	pid := createORProvider(t, env, "or-hdr", "sk-test-key", []string{"openai"})
	createRouteAt(t, env, "openai", pid, 1)

	resp := env.ProxyPost("/v1/chat/completions", openaiRequest("openai/gpt-4o-mini"),
		map[string]string{
			"Authorization":   "Bearer client-placeholder",
			"HTTP-Referer":    "https://myapp.example",
			"X-Title":         "MyApp",
			"X-Forwarded-For": "203.0.113.7",
		})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	_, _, hdr, _ := captured.Get()
	if got := hdr.Get("HTTP-Referer"); got != "https://myapp.example" {
		t.Errorf("HTTP-Referer not forwarded: got %q", got)
	}
	if got := hdr.Get("X-Title"); got != "MyApp" {
		t.Errorf("X-Title not forwarded: got %q", got)
	}
	if got := hdr.Get("X-Forwarded-For"); got != "" {
		t.Errorf("X-Forwarded-For should be stripped for privacy, got %q", got)
	}
	if got := hdr.Get("Authorization"); got != "Bearer sk-test-key" {
		t.Errorf("provider key should override client auth, got %q", got)
	}
}

// Without a matching ModelMapping the model is forwarded verbatim — a guard that
// delegation never rewrites models on its own.
func TestOpenRouterMockNoMapping(t *testing.T) {
	captured := &capturedRequest{}
	var calls int64
	mock := newOpenRouterMock(t, captured, &calls)
	defer mock.Close()
	t.Setenv("MAXX_OPENROUTER_BASE_URL", mock.URL)

	env := NewProxyTestEnv(t)
	pid := createORProvider(t, env, "or-nomap", "sk-test-key", []string{"openai"})
	createRouteAt(t, env, "openai", pid, 1)

	resp := env.ProxyPost("/v1/chat/completions", openaiRequest("openai/gpt-4o"),
		map[string]string{"Authorization": "Bearer client-placeholder"})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	_, _, _, up := captured.Get()
	if m := gjsonModel(up); m != "openai/gpt-4o" {
		t.Errorf("model should pass through unchanged with no mapping, upstream got %q", m)
	}
}

// Config boundary: disableErrorCooldown coexists with the openrouter block and
// round-trips through create → admin read (a single JSON config can carry both
// the union member and the shared cooldown flag).
func TestOpenRouterConfigRoundTrip(t *testing.T) {
	env := NewProxyTestEnv(t)

	cresp := env.AdminPost("/api/admin/providers", map[string]any{
		"name": "or-cfg",
		"type": "openrouter",
		"config": map[string]any{
			"openrouter":           map[string]any{"apiKey": "sk-cfg"},
			"disableErrorCooldown": true,
		},
		"supportedClientTypes": []string{"openai"},
	})
	if cresp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(cresp.Body)
		t.Fatalf("create: status=%d body=%s", cresp.StatusCode, b)
	}
	var created struct {
		ID uint64 `json:"id"`
	}
	DecodeJSON(t, cresp, &created)

	gresp := env.AdminGet(fmt.Sprintf("/api/admin/providers/%d", created.ID))
	var got struct {
		Type   string `json:"type"`
		Config struct {
			DisableErrorCooldown bool `json:"disableErrorCooldown"`
			OpenRouter           *struct {
				APIKey string `json:"apiKey"`
			} `json:"openrouter"`
		} `json:"config"`
	}
	DecodeJSON(t, gresp, &got)

	if got.Type != "openrouter" {
		t.Errorf("type = %q, want openrouter", got.Type)
	}
	if !got.Config.DisableErrorCooldown {
		t.Error("disableErrorCooldown did not round-trip alongside the openrouter block")
	}
	if got.Config.OpenRouter == nil {
		t.Error("openrouter config block missing after round-trip")
	}
}

// Creating an openrouter provider with no supportedClientTypes defaults to all
// three native protocols (claude + openai + codex) instead of leaving it
// unserviceable. Codex is always advertised because OpenRouter speaks it
// natively (/v1/responses); see domain.CanonicalSupportedClientTypes.
func TestOpenRouterDefaultsClientTypes(t *testing.T) {
	env := NewProxyTestEnv(t)
	cresp := env.AdminPost("/api/admin/providers", map[string]any{
		"name":   "or-defaults",
		"type":   "openrouter",
		"config": map[string]any{"openrouter": map[string]any{"apiKey": "sk-cfg"}},
		// supportedClientTypes intentionally omitted
	})
	if cresp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(cresp.Body)
		t.Fatalf("create: status=%d body=%s", cresp.StatusCode, b)
	}
	var created struct {
		SupportedClientTypes []string `json:"supportedClientTypes"`
	}
	DecodeJSON(t, cresp, &created)

	got := map[string]bool{}
	for _, ct := range created.SupportedClientTypes {
		got[ct] = true
	}
	if len(created.SupportedClientTypes) != 3 || !got["claude"] || !got["openai"] || !got["codex"] {
		t.Errorf("empty supportedClientTypes should default to [claude openai codex], got %v", created.SupportedClientTypes)
	}
}

// providerIDByName finds a provider's id from the admin list by name.
func providerIDByName(t *testing.T, env *ProxyTestEnv, name string) uint64 {
	t.Helper()
	resp := env.AdminGet("/api/admin/providers")
	var list []map[string]any
	DecodeJSON(t, resp, &list)
	for _, p := range list {
		if p["name"] == name {
			return uint64(p["id"].(float64))
		}
	}
	t.Fatalf("provider %q not found in admin list", name)
	return 0
}

// Export/import round-trip: an exportable openrouter provider's plaintext key is
// carried in the export (so re-import yields a working provider), an
// excludeFromExport provider is omitted, and the re-imported provider actually
// proxies with the preserved key.
func TestOpenRouterExportImportRoundTrip(t *testing.T) {
	captured := &capturedRequest{}
	var calls int64
	mock := newOpenRouterMock(t, captured, &calls)
	defer mock.Close()
	t.Setenv("MAXX_OPENROUTER_BASE_URL", mock.URL)

	env := NewProxyTestEnv(t)
	createORProvider(t, env, "or-export", "sk-export-me", []string{"openai"})

	// An excludeFromExport provider must never appear in the export dump.
	xresp := env.AdminPost("/api/admin/providers", map[string]any{
		"name":                 "or-secret-hidden",
		"type":                 "openrouter",
		"config":               map[string]any{"openrouter": map[string]any{"apiKey": "sk-hidden"}},
		"supportedClientTypes": []string{"openai"},
		"excludeFromExport":    true,
	})
	if xresp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(xresp.Body)
		t.Fatalf("create excluded provider: status=%d body=%s", xresp.StatusCode, b)
	}
	xresp.Body.Close()

	// Export and locate the exportable provider; assert the excluded one is gone.
	eresp := env.AdminGet("/api/admin/providers/export")
	var exported []map[string]any
	DecodeJSON(t, eresp, &exported)
	var exportedA map[string]any
	for _, p := range exported {
		if p["name"] == "or-secret-hidden" {
			t.Fatal("excludeFromExport provider leaked into the export dump")
		}
		if p["name"] == "or-export" {
			exportedA = p
		}
	}
	if exportedA == nil {
		t.Fatal("exportable provider missing from export dump")
	}
	// Export carries the plaintext key (unlike admin-list redaction) so that a
	// re-import produces a provider that can actually authenticate upstream.
	cfg, _ := exportedA["config"].(map[string]any)
	or, _ := cfg["openrouter"].(map[string]any)
	if or == nil || or["apiKey"] != "sk-export-me" {
		t.Fatalf("export should carry the plaintext key for re-import, got config=%v", cfg)
	}

	// Re-import under a new name (import skips duplicate names) and verify it lands.
	exportedA["name"] = "or-imported"
	delete(exportedA, "id")
	iresp := env.AdminPost("/api/admin/providers/import", []map[string]any{exportedA})
	if iresp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(iresp.Body)
		t.Fatalf("import: status=%d body=%s", iresp.StatusCode, b)
	}
	var imp struct {
		Imported int `json:"imported"`
	}
	DecodeJSON(t, iresp, &imp)
	if imp.Imported != 1 {
		t.Fatalf("expected imported=1, got %d", imp.Imported)
	}

	// The re-imported provider proxies with the key that survived the round-trip.
	importedID := providerIDByName(t, env, "or-imported")
	createRouteAt(t, env, "openai", importedID, 1)
	resp := env.ProxyPost("/v1/chat/completions", openaiRequest("openai/gpt-4o-mini"),
		map[string]string{"Authorization": "Bearer client-placeholder"})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	if _, _, hdr, _ := captured.Get(); hdr.Get("Authorization") != "Bearer sk-export-me" {
		t.Errorf("re-imported provider should use the round-tripped key; upstream auth = %q, want Bearer sk-export-me",
			hdr.Get("Authorization"))
	}
}

// Dynamic client-type change: OpenRouter honors the client-supplied client types
// verbatim (no autoSetSupportedClientTypes case). While the provider is
// openai-only, a claude request is converted to OpenAI and hits
// /v1/chat/completions upstream. After the provider is edited to also support
// claude natively, the same request passes through to /v1/messages — the upstream
// path flips, proving the executor re-reads the updated supportedClientTypes.
func TestOpenRouterMockUpdateClientTypes(t *testing.T) {
	captured := &capturedRequest{}
	var calls int64
	mock := newOpenRouterMock(t, captured, &calls)
	defer mock.Close()
	t.Setenv("MAXX_OPENROUTER_BASE_URL", mock.URL)

	env := NewProxyTestEnv(t)
	pid := createORProvider(t, env, "or-dyn", "sk-test-key", []string{"openai"})
	createRouteAt(t, env, "openai", pid, 1)
	createRouteAt(t, env, "claude", pid, 1)

	// openai-only: claude traffic is converted → upstream OpenAI path.
	resp := env.ProxyPost("/v1/messages", claudeRequest("anthropic/claude-3-haiku"),
		map[string]string{"anthropic-version": "2023-06-01"})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	if _, path, _, _ := captured.Get(); path != "/v1/chat/completions" {
		t.Fatalf("openai-only provider should convert claude→openai; upstream path = %s, want /v1/chat/completions", path)
	}

	// Enable claude natively via PUT (blank apiKey preserves the stored key).
	uresp := env.AdminPut(fmt.Sprintf("/api/admin/providers/%d", pid), map[string]any{
		"name":                 "or-dyn",
		"type":                 "openrouter",
		"config":               map[string]any{"openrouter": map[string]any{"apiKey": ""}},
		"supportedClientTypes": []string{"openai", "claude"},
	})
	if uresp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(uresp.Body)
		t.Fatalf("update client types: status=%d body=%s", uresp.StatusCode, b)
	}
	uresp.Body.Close()

	// Now claude is native → passthrough to the Anthropic path.
	resp = env.ProxyPost("/v1/messages", claudeRequest("anthropic/claude-3-haiku"),
		map[string]string{"anthropic-version": "2023-06-01"})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	if _, path, _, _ := captured.Get(); path != "/v1/messages" {
		t.Fatalf("after enabling claude, request should pass through; upstream path = %s, want /v1/messages", path)
	}
}
