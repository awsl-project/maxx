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
	assertCustomAdapterExecuteUserAgent(t, domain.ClientTypeClaude, domain.ClientTypeClaude, "/v1/messages", "Mozilla/5.0 maxx-ua-regression", "Mozilla/5.0 maxx-ua-regression")
}

func TestCustomAdapterExecutePreservesMissingClaudeUserAgent(t *testing.T) {
	assertCustomAdapterExecuteUserAgent(t, domain.ClientTypeClaude, domain.ClientTypeClaude, "/v1/messages", "", "")
}

func TestCustomAdapterExecutePreservesProvidedCodexUserAgent(t *testing.T) {
	assertCustomAdapterExecuteUserAgent(t, domain.ClientTypeCodex, domain.ClientTypeCodex, "/v1/responses", "custom-codex-client/9.9", "custom-codex-client/9.9")
}

func TestCustomAdapterExecutePreservesMissingCodexUserAgent(t *testing.T) {
	assertCustomAdapterExecuteUserAgent(t, domain.ClientTypeCodex, domain.ClientTypeCodex, "/v1/responses", "", "")
}

func TestCustomAdapterExecutePreservesProvidedGeminiUserAgent(t *testing.T) {
	assertCustomAdapterExecuteUserAgent(t, domain.ClientTypeGemini, domain.ClientTypeGemini, "/v1beta/models/gemini-2.5-pro:generateContent", "gemini-cli/custom", "gemini-cli/custom")
}

func TestCustomAdapterExecutePreservesMissingGeminiUserAgent(t *testing.T) {
	assertCustomAdapterExecuteUserAgent(t, domain.ClientTypeGemini, domain.ClientTypeGemini, "/v1beta/models/gemini-2.5-pro:generateContent", "", "")
}

func TestCustomAdapterExecuteUsesTargetUserAgentAfterConversion(t *testing.T) {
	tests := []struct {
		name               string
		originalClientType domain.ClientType
		targetClientType   domain.ClientType
		requestURI         string
		expectedUA         string
	}{
		{"Codex to Claude", domain.ClientTypeCodex, domain.ClientTypeClaude, "/v1/messages", defaultClaudeUserAgent},
		{"Claude to Codex", domain.ClientTypeClaude, domain.ClientTypeCodex, "/v1/responses", codexUserAgent},
		{"Claude to Gemini", domain.ClientTypeClaude, domain.ClientTypeGemini, "/v1beta/models/gemini-2.5-pro:generateContent", geminiUserAgent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertCustomAdapterExecuteUserAgent(t, tt.originalClientType, tt.targetClientType, tt.requestURI, "source-client/1.0", tt.expectedUA)
		})
	}
}

func TestApplyCustomProtocolIdentityDropsSourceVersionForCodexConversion(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses", nil)
	req.Header.Set("User-Agent", "source-client/1.0")
	ctx := flow.NewCtx(httptest.NewRecorder(), req)
	ctx.Set(flow.KeyOriginalClientType, domain.ClientTypeClaude)
	ctx.Set(flow.KeyClientType, domain.ClientTypeCodex)

	headers := make(http.Header)
	headers.Set("Version", "source-version")
	applyCustomProtocolIdentity(ctx, domain.ClientTypeCodex, codexUserAgent, headers)

	if got := headers.Get("User-Agent"); got != codexUserAgent {
		t.Fatalf("User-Agent = %q, want %q", got, codexUserAgent)
	}
	if got := headers.Get("Version"); got != "" {
		t.Fatalf("Version = %q, want empty after conversion", got)
	}
}

func assertCustomAdapterExecuteUserAgent(t *testing.T, originalClientType, targetClientType domain.ClientType, requestURI string, clientUA string, expectedUA string) {
	t.Helper()

	var capturedUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, mockUpstreamResponseForClientType(targetClientType))
	}))
	defer server.Close()

	adapter, err := NewAdapter(&domain.Provider{
		Name:                 "test-custom",
		Type:                 "custom",
		SupportedClientTypes: []domain.ClientType{targetClientType},
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
	ctx.Set(flow.KeyClientType, targetClientType)
	ctx.Set(flow.KeyOriginalClientType, originalClientType)
	ctx.Set(flow.KeyRequestHeaders, req.Header.Clone())
	ctx.Set(flow.KeyRequestURI, requestURI)
	ctx.Set(flow.KeyRequestBody, mockRequestBodyForClientType(targetClientType))

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
	case domain.ClientTypeGemini:
		return []byte(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`)
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
	case domain.ClientTypeGemini:
		return `{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]}}]}`
	default:
		return `{}`
	}
}
