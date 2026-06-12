package custom

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
)

func TestCustomAdapterExecutePreservesProvidedClaudeUserAgent(t *testing.T) {
	assertCustomAdapterExecuteUserAgent(t, domain.ClientTypeClaude, "/v1/messages", "Mozilla/5.0 maxx-ua-regression", "Mozilla/5.0 maxx-ua-regression")
}

func TestCustomAdapterExecuteFallsBackClaudeUserAgentWhenMissing(t *testing.T) {
	assertCustomAdapterExecuteUserAgent(t, domain.ClientTypeClaude, "/v1/messages", "", defaultClaudeUserAgent)
}

func TestCustomAdapterExecutePreservesProvidedCodexUserAgent(t *testing.T) {
	assertCustomAdapterExecuteUserAgent(t, domain.ClientTypeCodex, "/v1/responses", "custom-codex-client/9.9", "custom-codex-client/9.9")
}

func TestCustomAdapterExecuteFallsBackCodexUserAgentWhenMissing(t *testing.T) {
	assertCustomAdapterExecuteUserAgent(t, domain.ClientTypeCodex, "/v1/responses", "", codexUserAgent)
}

func assertCustomAdapterExecuteUserAgent(t *testing.T, clientType domain.ClientType, requestURI string, clientUA string, expectedUA string) {
	t.Helper()

	var capturedUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, mockUpstreamResponseForClientType(clientType))
	}))
	defer server.Close()

	adapter, err := NewAdapter(&domain.Provider{
		Name:                 "test-custom",
		Type:                 "custom",
		SupportedClientTypes: []domain.ClientType{clientType},
		Config: &domain.ProviderConfig{
			Custom: &domain.ProviderConfigCustom{
				BaseURL: server.URL,
				APIKey:  "sk-test",
			},
		},
	})
	if err != nil {
		t.Fatalf("NewAdapter error: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost, "http://localhost"+requestURI, nil)
	if clientUA != "" {
		req.Header.Set("User-Agent", clientUA)
	}

	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, req)
	ctx.Set(flow.KeyClientType, clientType)
	ctx.Set(flow.KeyRequestURI, requestURI)
	ctx.Set(flow.KeyRequestBody, mockRequestBodyForClientType(clientType))

	if err := adapter.Execute(ctx, &domain.Provider{}); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if capturedUA != expectedUA {
		t.Fatalf("expected upstream User-Agent %q, got %q", expectedUA, capturedUA)
	}
}

func mockRequestBodyForClientType(clientType domain.ClientType) []byte {
	switch clientType {
	case domain.ClientTypeClaude:
		return []byte(`{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	case domain.ClientTypeCodex:
		return []byte(`{"model":"gpt-5","input":"hello","stream":false}`)
	default:
		return []byte(`{}`)
	}
}

func mockUpstreamResponseForClientType(clientType domain.ClientType) string {
	switch clientType {
	case domain.ClientTypeClaude:
		return `{"id":"msg_123","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"ok"}]}`
	case domain.ClientTypeCodex:
		return `{"id":"resp_123","model":"gpt-5","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`
	default:
		return `{}`
	}
}
