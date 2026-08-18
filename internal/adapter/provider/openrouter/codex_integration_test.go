package openrouter

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/awsl-project/maxx/internal/converter"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/tidwall/gjson"
)

// runOpenRouterExecuteMapped drives a request through the real openrouter adapter
// (Execute → custom core → HTTP) against a mock upstream, with an explicit mapped
// model set in the flow context — exactly what the executor sets after model
// mapping. It returns the path and JSON body the upstream actually received, so a
// test can assert the model slug the adapter sent on the wire.
func runOpenRouterExecuteMapped(t *testing.T, clientType domain.ClientType, requestURI, mappedModel string, reqBody []byte) (string, []byte) {
	t.Helper()

	var capturedPath string
	var captured []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		captured, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"x","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer server.Close()
	t.Setenv("MAXX_OPENROUTER_BASE_URL", server.URL)

	adapter, err := NewAdapter(&domain.Provider{
		Name:                 "test-openrouter",
		Type:                 "openrouter",
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeClaude, domain.ClientTypeOpenAI},
		Config: &domain.ProviderConfig{
			OpenRouter: &domain.ProviderConfigOpenRouter{APIKey: "sk-or-test"},
		},
	})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost, "http://localhost"+requestURI, nil)
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, req)
	ctx.Set(flow.KeyClientType, clientType)
	ctx.Set(flow.KeyOriginalClientType, clientType)
	ctx.Set(flow.KeyRequestHeaders, req.Header.Clone())
	ctx.Set(flow.KeyRequestURI, requestURI)
	ctx.Set(flow.KeyRequestBody, reqBody)
	ctx.Set(flow.KeyMappedModel, mappedModel)

	if err := adapter.Execute(ctx, &domain.Provider{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if captured == nil {
		t.Fatal("upstream received no body")
	}
	return capturedPath, captured
}

// TestIntegration_OpenRouterSlugNormalizationOnWire proves the auto-slug fix
// reaches the upstream: a vendor-native mapped model (claude-sonnet-4-6) is sent
// to OpenRouter as its required "<vendor>/<model>" slug (anthropic/claude-sonnet-4.6),
// while an already-namespaced slug is left untouched.
func TestIntegration_OpenRouterSlugNormalizationOnWire(t *testing.T) {
	cases := []struct {
		mapped string
		want   string
	}{
		{"claude-sonnet-4-6", "anthropic/claude-sonnet-4.6"},
		{"gpt-5.5", "openai/gpt-5.5"},
		{"anthropic/claude-sonnet-4.6", "anthropic/claude-sonnet-4.6"}, // explicit slug wins
	}
	for _, tc := range cases {
		t.Run(tc.mapped, func(t *testing.T) {
			body := []byte(`{"model":"placeholder","messages":[{"role":"user","content":"hi"}]}`)
			_, got := runOpenRouterExecuteMapped(t, domain.ClientTypeOpenAI, "/v1/chat/completions", tc.mapped, body)
			if v := gjson.GetBytes(got, "model").String(); v != tc.want {
				t.Errorf("upstream model = %q, want %q\n%s", v, tc.want, got)
			}
		})
	}
}

// TestIntegration_OpenRouterCodexEndToEnd proves Codex is usable on OpenRouter: a
// Codex Responses request is bridged to OpenAI Chat Completions (dropping the
// Responses-only built-in tools OpenRouter rejects) and dispatched through the
// openrouter adapter, which lands the request on /v1/chat/completions carrying the
// normalized vendor slug. This is the whole path the executor + adapter run for a
// real Codex CLI request pointed at a native openrouter provider.
func TestIntegration_OpenRouterCodexEndToEnd(t *testing.T) {
	// A Codex CLI-shaped Responses request, including built-in tools OpenRouter's
	// /responses endpoint rejects (web_search, image_generation) alongside a normal
	// user-defined function tool.
	codexReq := converter.CodexRequest{
		Model:        "gpt-5.5",
		Instructions: "be careful",
		Input: []interface{}{
			map[string]interface{}{"type": "message", "role": "user", "content": "run pwd"},
		},
		Tools: []converter.CodexTool{
			{Type: "function", Name: "shell", Description: "run a shell command", Parameters: map[string]interface{}{"type": "object"}},
			{Type: "web_search"},
			{Type: "image_generation"},
		},
	}
	rawBody, err := json.Marshal(codexReq)
	if err != nil {
		t.Fatal(err)
	}

	// The executor converts Codex→OpenAI Chat before dispatch (bridge path); mirror
	// that here so the adapter receives exactly what it would in production.
	convertedBody, err := converter.GetGlobalRegistry().TransformRequest(
		domain.ClientTypeCodex, domain.ClientTypeOpenAI, rawBody, "gpt-5.5", true)
	if err != nil {
		t.Fatalf("codex->openai TransformRequest: %v", err)
	}

	path, got := runOpenRouterExecuteMapped(t, domain.ClientTypeOpenAI, "/v1/chat/completions", "gpt-5.5", convertedBody)

	if path != "/v1/chat/completions" {
		t.Fatalf("upstream path = %q, want /v1/chat/completions", path)
	}
	// The Codex model must reach OpenRouter as a valid vendor-namespaced slug.
	if v := gjson.GetBytes(got, "model").String(); v != "openai/gpt-5.5" {
		t.Errorf("upstream model = %q, want openai/gpt-5.5\n%s", v, got)
	}
	// The upstream must receive a well-formed OpenAI Chat request...
	var openaiReq converter.OpenAIRequest
	if err := json.Unmarshal(got, &openaiReq); err != nil {
		t.Fatalf("upstream body is not a valid OpenAI request: %v\n%s", err, got)
	}
	if len(openaiReq.Messages) == 0 {
		t.Fatalf("converted request has no messages\n%s", got)
	}
	// ...with the Responses-only built-in tools dropped, keeping only the function.
	if len(openaiReq.Tools) != 1 || openaiReq.Tools[0].Type != "function" || openaiReq.Tools[0].Function.Name != "shell" {
		t.Fatalf("tools = %#v, want only the function tool 'shell'", openaiReq.Tools)
	}
}
