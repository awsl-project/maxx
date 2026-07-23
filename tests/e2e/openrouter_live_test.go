package e2e_test

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/tidwall/gjson"

	// Register the openrouter adapter factory via init().
	_ "github.com/awsl-project/maxx/internal/adapter/provider/openrouter"
)

// TestOpenRouterLiveProxy drives a real request through the full maxx proxy
// pipeline into the live OpenRouter API, exercising the first-class openrouter
// provider (which delegates to the custom core with a synthesized config).
//
// It is skipped unless MAXX_OPENROUTER_E2E_KEY is set, so the secret never
// lives in the repo and CI stays offline. Run with:
//
//	MAXX_OPENROUTER_E2E_KEY=sk-or-... go test ./tests/e2e/ -run TestOpenRouterLiveProxy -v
func TestOpenRouterLiveProxy(t *testing.T) {
	key := os.Getenv("MAXX_OPENROUTER_E2E_KEY")
	if key == "" {
		t.Skip("MAXX_OPENROUTER_E2E_KEY not set; skipping live OpenRouter test")
	}

	env := NewProxyTestEnv(t)

	resp := env.AdminPost("/api/admin/providers", map[string]any{
		"name": "openrouter-live",
		"type": "openrouter",
		"config": map[string]any{
			"openrouter": map[string]any{"apiKey": key},
		},
		"supportedClientTypes": []string{"openai", "claude"},
	})
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create openrouter provider: status=%d body=%s", resp.StatusCode, body)
	}
	var created struct {
		ID uint64 `json:"id"`
	}
	DecodeJSON(t, resp, &created)
	createRoute(t, env, "openai", created.ID)
	createRoute(t, env, "claude", created.ID)

	// OpenAI-format client → OpenRouter /v1/chat/completions
	t.Run("openai", func(t *testing.T) {
		// Real OpenAI clients send an Authorization header; maxx overwrites it
		// with the provider key. Mirror that so the passthrough auth path fires.
		resp := env.ProxyPost("/v1/chat/completions", map[string]any{
			"model":      "openai/gpt-4o-mini",
			"max_tokens": 16,
			"messages":   []map[string]any{{"role": "user", "content": "Reply with just: OK"}},
		}, map[string]string{"Authorization": "Bearer client-placeholder"})
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("openai proxy status=%d body=%s", resp.StatusCode, body)
		}
		if gjson.GetBytes(body, "object").String() != "chat.completion" {
			t.Fatalf("expected object=chat.completion, got: %s", body)
		}
		t.Logf("openai OK: model=%s content=%q",
			gjson.GetBytes(body, "model").String(),
			gjson.GetBytes(body, "choices.0.message.content").String())
	})

	// Claude-format client → OpenRouter /v1/messages (Anthropic Skin)
	t.Run("claude", func(t *testing.T) {
		resp := env.ProxyPost("/v1/messages", map[string]any{
			"model":      "anthropic/claude-3-haiku",
			"max_tokens": 16,
			"messages":   []map[string]any{{"role": "user", "content": "Reply with just: OK"}},
		}, map[string]string{"anthropic-version": "2023-06-01"})
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("claude proxy status=%d body=%s", resp.StatusCode, body)
		}
		if gjson.GetBytes(body, "type").String() != "message" {
			t.Fatalf("expected type=message, got: %s", body)
		}
		t.Logf("claude OK: model=%s text=%q",
			gjson.GetBytes(body, "model").String(),
			gjson.GetBytes(body, "content.0.text").String())
	})

	// OpenAI streaming (SSE) → OpenRouter
	t.Run("openai_stream", func(t *testing.T) {
		resp := env.ProxyPost("/v1/chat/completions", map[string]any{
			"model":      "openai/gpt-4o-mini",
			"max_tokens": 16,
			"stream":     true,
			"messages":   []map[string]any{{"role": "user", "content": "count 1 to 3"}},
		}, map[string]string{"Authorization": "Bearer client-placeholder"})
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("openai stream status=%d body=%s", resp.StatusCode, body)
		}
		s := string(body)
		if !strings.Contains(s, "data:") || !strings.Contains(s, "[DONE]") {
			t.Fatalf("expected SSE stream ending in [DONE], got: %s", s)
		}
		t.Logf("openai stream OK: %d bytes, %d data lines", len(body), strings.Count(s, "data:"))
	})

	// Claude streaming (SSE, Anthropic event framing) → OpenRouter
	t.Run("claude_stream", func(t *testing.T) {
		resp := env.ProxyPost("/v1/messages", map[string]any{
			"model":      "anthropic/claude-3-haiku",
			"max_tokens": 16,
			"stream":     true,
			"messages":   []map[string]any{{"role": "user", "content": "count 1 to 3"}},
		}, map[string]string{"anthropic-version": "2023-06-01"})
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("claude stream status=%d body=%s", resp.StatusCode, body)
		}
		s := string(body)
		if !strings.Contains(s, "message_start") && !strings.Contains(s, "content_block_delta") {
			t.Fatalf("expected Anthropic SSE events, got: %s", s)
		}
		t.Logf("claude stream OK: %d bytes", len(body))
	})

	// Model mapping via provider-scoped ModelMapping entity: the client sends an
	// alias that OpenRouter does not know; the mapping rewrites it to a real id.
	// A 200 (not 404 "unknown model") plus the mapped model in the response
	// proves executor.mapModel applied the entity for this openrouter provider.
	t.Run("model_mapping", func(t *testing.T) {
		mresp := env.AdminPost("/api/admin/model-mappings", map[string]any{
			"scope":        "provider",
			"providerID":   created.ID,
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

		resp := env.ProxyPost("/v1/chat/completions", map[string]any{
			"model":      "or-alias-mini",
			"max_tokens": 16,
			"messages":   []map[string]any{{"role": "user", "content": "Reply with just: OK"}},
		}, map[string]string{"Authorization": "Bearer client-placeholder"})
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("mapped request status=%d body=%s", resp.StatusCode, body)
		}
		if got := gjson.GetBytes(body, "model").String(); got != "openai/gpt-4o-mini" {
			t.Fatalf("expected upstream model openai/gpt-4o-mini (mapping applied), got %q; body=%s", got, body)
		}
		t.Logf("model mapping OK: or-alias-mini -> %s", gjson.GetBytes(body, "model").String())
	})
}
