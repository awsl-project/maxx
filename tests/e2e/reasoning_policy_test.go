package e2e_test

import (
	"io"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/tidwall/gjson"
)

// A provider-scoped reasoning ceiling (MaxEffort=medium) clamps the outbound
// request the OpenRouter upstream actually receives, even though the openrouter/
// custom adapter never touches effort — proving the executor's authoritative
// param stage enforces the policy end-to-end for the passthrough path.
func TestOpenRouterReasoningEffortCeiling(t *testing.T) {
	captured := &capturedRequest{}
	var calls int64
	mock := newOpenRouterMock(t, captured, &calls)
	defer mock.Close()
	t.Setenv("MAXX_OPENROUTER_BASE_URL", mock.URL)

	env := NewProxyTestEnv(t)
	resp := env.AdminPost("/api/admin/providers", map[string]any{
		"name": "or-effort-cap",
		"type": "openrouter",
		"config": map[string]any{
			"openrouter": map[string]any{"apiKey": "sk-test-key"},
			"reasoning":  map[string]any{"maxEffort": "medium"},
		},
		"supportedClientTypes": []string{"openai"},
	})
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("create provider: status=%d body=%s", resp.StatusCode, b)
	}
	var created struct {
		ID uint64 `json:"id"`
	}
	DecodeJSON(t, resp, &created)
	createRouteAt(t, env, "openai", created.ID, 1)

	reqBody := map[string]any{
		"model":            "openai/gpt-5",
		"reasoning_effort": "high",
		"messages":         []map[string]any{{"role": "user", "content": "hi"}},
	}
	resp = env.ProxyPost("/v1/chat/completions", reqBody,
		map[string]string{"Authorization": "Bearer client-placeholder"})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	if atomic.LoadInt64(&calls) == 0 {
		t.Fatal("upstream was never called")
	}
	_, _, _, up := captured.Get()
	if got := gjson.GetBytes(up, "reasoning_effort").String(); got != "medium" {
		t.Fatalf("upstream reasoning_effort = %q, want clamped to medium; body=%s", got, up)
	}
}
