package cliproxyapi_codex

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/gin-gonic/gin"
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

func TestCLIProxyAPIRequestHeadersFollowProtocolBoundary(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses", nil)
	req.Header.Set("User-Agent", "codex_cli_rs/0.144.1")
	req.Header.Set("Version", "0.144.1")
	req.Header.Set("Authorization", "Bearer source-token")
	req.Header.Set("X-Codex-Responses-Lite", "true")
	ctx := flow.NewCtx(httptest.NewRecorder(), req)
	ctx.Set(flow.KeyOriginalClientType, domain.ClientTypeCodex)
	ctx.Set(flow.KeyClientType, domain.ClientTypeCodex)

	sameProtocolHeaders := cliProxyAPIRequestHeaders(ctx)
	if got := sameProtocolHeaders.Get("User-Agent"); got != "codex_cli_rs/0.144.1" {
		t.Fatalf("same-protocol User-Agent = %q", got)
	}
	if got := sameProtocolHeaders.Get("Version"); got != "0.144.1" {
		t.Fatalf("same-protocol Version = %q", got)
	}
	if got := sameProtocolHeaders.Get("Authorization"); got != "" {
		t.Fatalf("source Authorization leaked into executor headers: %q", got)
	}
	if got := sameProtocolHeaders.Get("X-Codex-Responses-Lite"); got != "" {
		t.Fatalf("unrelated source header leaked into executor headers: %q", got)
	}

	ctx.Set(flow.KeyOriginalClientType, domain.ClientTypeClaude)
	convertedHeaders := cliProxyAPIRequestHeaders(ctx)
	if got := convertedHeaders.Get("User-Agent"); got != "" {
		t.Fatalf("converted User-Agent = %q, want empty", got)
	}
	if got := convertedHeaders.Get("Version"); got != "" {
		t.Fatalf("converted Version = %q, want empty", got)
	}
	if got := req.Header.Get("User-Agent"); got != "codex_cli_rs/0.144.1" {
		t.Fatalf("source request was mutated: User-Agent = %q", got)
	}

	execCtx := cliProxyAPIRequestContext(ctx, convertedHeaders)
	ginCtx, ok := execCtx.Value("gin").(*gin.Context)
	if !ok || ginCtx.Request == nil {
		t.Fatal("expected SDK request context")
	}
	if got := ginCtx.Request.Header.Get("User-Agent"); got != "" {
		t.Fatalf("SDK context User-Agent = %q, want empty", got)
	}
}
