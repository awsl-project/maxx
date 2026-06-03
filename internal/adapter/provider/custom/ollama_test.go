package custom

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
)

func TestBuildOllamaChatRequestFromClaude(t *testing.T) {
	body := []byte(`{
		"model":"qwen2.5-coder:14b",
		"system":[{"type":"text","text":"be concise"}],
		"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}],
		"tools":[{"name":"search","description":"Search docs","input_schema":{"type":"object","properties":{"q":{"type":"string"}}}}],
		"max_tokens":128,
		"temperature":0.2,
		"stop_sequences":["STOP"]
	}`)

	got, _, err := buildOllamaChatRequest(body, "")
	if err != nil {
		t.Fatalf("buildOllamaChatRequest: %v", err)
	}
	if got.Model != "qwen2.5-coder:14b" {
		t.Fatalf("model = %q", got.Model)
	}
	if len(got.Messages) != 2 || got.Messages[0].Role != "system" || got.Messages[0].Content != "be concise" || got.Messages[1].Content != "hello" {
		t.Fatalf("unexpected messages: %#v", got.Messages)
	}
	if len(got.Tools) != 1 || got.Tools[0].Function.Name != "search" {
		t.Fatalf("unexpected tools: %#v", got.Tools)
	}
	if got.Options["num_predict"] != 128 {
		t.Fatalf("num_predict = %#v", got.Options["num_predict"])
	}
}

func TestOllamaBackendNonStreamWrapsClaudeResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		var req ollamaChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if req.Model != "qwen2.5-coder:14b" {
			t.Fatalf("upstream model = %q", req.Model)
		}
		if len(req.Messages) != 1 || req.Messages[0].Role != "user" || req.Messages[0].Content != "hello" {
			t.Fatalf("upstream messages = %#v", req.Messages)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"qwen2.5-coder:14b","message":{"role":"assistant","content":"world"},"done":true,"prompt_eval_count":3,"eval_count":5}`))
	}))
	defer server.Close()

	provider := &domain.Provider{
		Name: "local ollama",
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL: server.URL,
			Backend: customBackendOllama,
		}},
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeClaude},
	}
	adapter := &CustomAdapter{provider: provider}

	body := []byte(`{"model":"qwen2.5-coder:14b","messages":[{"role":"user","content":"hello"}]}`)
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(string(body))))
	ctx.Set(flow.KeyClientType, domain.ClientTypeClaude)
	ctx.Set(flow.KeyRequestBody, body)

	if err := adapter.Execute(ctx, provider); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Model   string `json:"model"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Type != "message" || resp.Role != "assistant" || resp.Model != "qwen2.5-coder:14b" {
		t.Fatalf("unexpected claude envelope: %#v", resp)
	}
	if len(resp.Content) != 1 || resp.Content[0].Type != "text" || resp.Content[0].Text != "world" {
		t.Fatalf("unexpected content: %#v", resp.Content)
	}
	if resp.Usage.InputTokens != 3 || resp.Usage.OutputTokens != 5 {
		t.Fatalf("unexpected usage: %#v", resp.Usage)
	}
}

func TestCustomBackendEmptyKeepsHTTPRelayPassthrough(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer custom-key" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if req["model"] != "gpt-test" {
			t.Fatalf("upstream model = %#v", req["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","object":"chat.completion","model":"gpt-test","choices":[{"message":{"role":"assistant","content":"legacy ok"}}]}`))
	}))
	defer server.Close()

	provider := &domain.Provider{
		Name: "legacy custom",
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL: server.URL,
			APIKey:  "custom-key",
		}},
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeOpenAI},
	}
	adapter := &CustomAdapter{provider: provider}

	body := []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"hello"}]}`)
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body))))
	ctx.Set(flow.KeyClientType, domain.ClientTypeOpenAI)
	ctx.Set(flow.KeyOriginalClientType, domain.ClientTypeOpenAI)
	ctx.Set(flow.KeyRequestBody, body)
	ctx.Set(flow.KeyRequestHeaders, http.Header{
		"Content-Type":  []string{"application/json"},
		"Authorization": []string{"Bearer inbound-key"},
	})
	ctx.Set(flow.KeyRequestURI, "/v1/chat/completions")

	if err := adapter.Execute(ctx, provider); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "legacy ok") {
		t.Fatalf("response body = %s", rec.Body.String())
	}
}

func TestOllamaBackendStreamEmitsSSEErrorBeforeReturning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(`{"model":"qwen","message":{"role":"assistant","content":"hello"}}` + "\n"))
		_, _ = w.Write([]byte(`{"error":"boom"}` + "\n"))
	}))
	defer server.Close()

	provider := &domain.Provider{
		Name: "local ollama",
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL: server.URL,
			Backend: customBackendOllama,
		}},
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeClaude},
	}
	adapter := &CustomAdapter{provider: provider}

	body := []byte(`{"model":"qwen","stream":true,"messages":[{"role":"user","content":"hello"}]}`)
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(string(body))))
	ctx.Set(flow.KeyClientType, domain.ClientTypeClaude)
	ctx.Set(flow.KeyRequestBody, body)

	err := adapter.Execute(ctx, provider)
	if err == nil {
		t.Fatal("expected stream error")
	}
	bodyText := rec.Body.String()
	if !strings.Contains(bodyText, "event: error") || !strings.Contains(bodyText, "boom") {
		t.Fatalf("stream body missing SSE error event: %s", bodyText)
	}
	if !strings.Contains(bodyText, "content_block_delta") || !strings.Contains(bodyText, "hello") {
		t.Fatalf("stream body missing prior content delta: %s", bodyText)
	}
}

func TestClassifyOllamaHTTPErrorRateLimitIsRetryableProviderError(t *testing.T) {
	err := classifyOllamaHTTPError(http.StatusTooManyRequests, []byte(`{"error":"rate limited"}`), "qwen")
	proxyErr, ok := err.(*domain.ProxyError)
	if !ok {
		t.Fatalf("expected ProxyError, got %T", err)
	}
	if proxyErr.Scope != domain.ScopeProvider {
		t.Fatalf("scope = %s", proxyErr.Scope)
	}
	if proxyErr.Reason != domain.CooldownReasonRateLimitExceeded {
		t.Fatalf("reason = %s", proxyErr.Reason)
	}
	if !proxyErr.Retryable {
		t.Fatal("expected retryable 429")
	}
	if proxyErr.HTTPStatusCode != http.StatusTooManyRequests {
		t.Fatalf("HTTPStatusCode = %d", proxyErr.HTTPStatusCode)
	}
}

func TestOllamaBackendRejectsNonClaudeClient(t *testing.T) {
	provider := &domain.Provider{
		Name: "local ollama",
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL: "http://localhost:11434",
			Backend: customBackendOllama,
		}},
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeClaude},
	}
	adapter := &CustomAdapter{provider: provider}
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	ctx.Set(flow.KeyClientType, domain.ClientTypeOpenAI)
	ctx.Set(flow.KeyRequestBody, []byte(`{"model":"qwen","messages":[]}`))

	err := adapter.Execute(ctx, provider)
	if err == nil {
		t.Fatal("expected error")
	}
	proxyErr, ok := err.(*domain.ProxyError)
	if !ok || proxyErr.Scope != domain.ScopeRequest {
		t.Fatalf("expected request-scoped proxy error, got %#v", err)
	}
}
