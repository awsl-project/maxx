package zai

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
)

// TestLiveZaiEndToEnd drives the real zai adapter (synth custom config → custom
// core → buildUpstreamURL → auth) against the live z.ai API for every
// protocol/plan combination, proving each "port" is actually reachable end to
// end. It is skipped unless MAXX_ZAI_LIVE_KEY is set:
//
//	MAXX_ZAI_LIVE_KEY=<key> go test ./internal/adapter/provider/zai/ -run TestLiveZaiEndToEnd -v
//
// Each case sends a tiny "reply pong" prompt for glm-4.6 and asserts HTTP 200.
func TestLiveZaiEndToEnd(t *testing.T) {
	key := strings.TrimSpace(os.Getenv("MAXX_ZAI_LIVE_KEY"))
	if key == "" {
		t.Skip("set MAXX_ZAI_LIVE_KEY to run the live z.ai end-to-end test")
	}

	const (
		claudeBody = `{"model":"glm-4.6","max_tokens":32,"messages":[{"role":"user","content":"reply with just: pong"}]}`
		openAIBody = `{"model":"glm-4.6","max_tokens":32,"stream":false,"messages":[{"role":"user","content":"reply with just: pong"}]}`
	)

	cases := []struct {
		name       string
		plan       string
		clientType domain.ClientType
		requestURI string
		body       string
	}{
		{"coding plan / claude", planCoding, domain.ClientTypeClaude, "/v1/messages", claudeBody},
		{"coding plan / openai", planCoding, domain.ClientTypeOpenAI, "/v1/chat/completions", openAIBody},
		{"standard api / claude", planAPI, domain.ClientTypeClaude, "/v1/messages", claudeBody},
		{"standard api / openai", planAPI, domain.ClientTypeOpenAI, "/v1/chat/completions", openAIBody},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			adapter, err := NewAdapter(&domain.Provider{
				Name:                 "live-zai",
				Type:                 "zai",
				SupportedClientTypes: []domain.ClientType{tc.clientType},
				Config: &domain.ProviderConfig{
					Zai: &domain.ProviderConfigZai{APIKey: key, Plan: tc.plan},
				},
			})
			if err != nil {
				t.Fatalf("NewAdapter: %v", err)
			}

			req, _ := http.NewRequest(http.MethodPost, "http://localhost"+tc.requestURI, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			// Simulate what a real client sends to maxx: its own maxx-side
			// credential. The custom core overwrites it with the provider key
			// (Claude via setClaudeAuthForURL; OpenAI via setAuthHeader, which
			// rewrites the existing Authorization header).
			switch tc.clientType {
			case domain.ClientTypeClaude:
				req.Header.Set("anthropic-version", "2023-06-01")
				req.Header.Set("x-api-key", "maxx-client-token")
			case domain.ClientTypeOpenAI:
				req.Header.Set("Authorization", "Bearer maxx-client-token")
			}

			rec := httptest.NewRecorder()
			ctx := flow.NewCtx(rec, req)
			ctx.Set(flow.KeyClientType, tc.clientType)
			ctx.Set(flow.KeyOriginalClientType, tc.clientType)
			ctx.Set(flow.KeyRequestHeaders, req.Header.Clone())
			ctx.Set(flow.KeyRequestURI, tc.requestURI)
			ctx.Set(flow.KeyRequestBody, []byte(tc.body))

			if err := adapter.Execute(ctx, &domain.Provider{}); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			body := rec.Body.String()
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body: %s", rec.Code, truncate(body, 400))
			}
			t.Logf("OK %s → %d; body: %s", tc.name, rec.Code, truncate(body, 200))
		})
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
