package cliproxyapi_codex

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/awsl-project/maxx/internal/codexguard"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
)

func TestNewAdapterSetsBaseURLAttribute(t *testing.T) {
	provider := &domain.Provider{
		Name: "codex",
		Config: &domain.ProviderConfig{
			Codex: &domain.ProviderConfigCodex{
				RefreshToken: "refresh-token",
				BaseURL:      " https://mock.example.test/codex ",
			},
		},
	}

	adapterIface, err := NewAdapter(provider)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}

	adapter, ok := adapterIface.(*CLIProxyAPICodexAdapter)
	if !ok {
		t.Fatalf("expected *CLIProxyAPICodexAdapter, got %T", adapterIface)
	}

	if got := adapter.authObj.Attributes["base_url"]; got != "https://mock.example.test/codex" {
		t.Fatalf("expected base_url attribute to be trimmed provider base URL, got %q", got)
	}
}

func TestCodexReasoningGuardProxyError(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses", nil)
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, req)
	cfg := codexguard.DefaultConfig()
	cfg.Enabled = true
	ctx.Set(flow.KeyCodexReasoningGuard, cfg)

	err := codexReasoningGuardProxyError(ctx, []byte(`{"usage":{"output_tokens_details":{"reasoning_tokens":516}}}`))
	if err == nil {
		t.Fatalf("expected reasoning guard proxy error")
	}
	if !codexguard.IsReasoningGuardError(err) {
		t.Fatalf("expected reasoning guard error, got %v", err)
	}
	if err.HTTPStatusCode != http.StatusBadGateway {
		t.Fatalf("HTTPStatusCode = %d, want %d", err.HTTPStatusCode, http.StatusBadGateway)
	}
	if err.Code != codexguard.DefaultErrorCode {
		t.Fatalf("Code = %q, want %q", err.Code, codexguard.DefaultErrorCode)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("expected helper not to write client body, got %q", rec.Body.String())
	}
}

func TestCodexReasoningGuardProxyErrorNoMatch(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses", nil)
	ctx := flow.NewCtx(httptest.NewRecorder(), req)
	ctx.Set(flow.KeyCodexReasoningGuard, codexguard.DefaultConfig())

	if err := codexReasoningGuardProxyError(ctx, []byte(`{"usage":{"output_tokens_details":{"reasoning_tokens":128}}}`)); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}
