package executor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/systemsettingcache"
)

func TestApplyGlobalUserAgentOverrideUpdatesFlowHeadersAndRequest(t *testing.T) {
	systemsettingcache.Invalidate(domain.SettingKeyGlobalUserAgentOverrideEnabled)
	exec := &Executor{settingsRepo: &stubExecutorSettingsRepo{values: map[string]string{
		domain.SettingKeyGlobalUserAgentOverrideEnabled: "true",
		domain.SettingKeyGlobalUserAgent:                " Mozilla/5.0 MaxxOverride ",
	}}}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("User-Agent", "source-client/1.0")
	ctx := flow.NewCtx(httptest.NewRecorder(), req)
	state := &execState{requestHeaders: req.Header}

	exec.applyGlobalUserAgentOverride(ctx, state)

	if got := req.Header.Get("User-Agent"); got != "Mozilla/5.0 MaxxOverride" {
		t.Fatalf("request User-Agent = %q", got)
	}
	if got := state.requestHeaders.Get("User-Agent"); got != "Mozilla/5.0 MaxxOverride" {
		t.Fatalf("state User-Agent = %q", got)
	}
	if got := flow.GetGlobalUserAgentOverride(ctx); got != "Mozilla/5.0 MaxxOverride" {
		t.Fatalf("flow override = %q", got)
	}
}

func TestApplyGlobalUserAgentOverrideDefaultOff(t *testing.T) {
	systemsettingcache.Invalidate(domain.SettingKeyGlobalUserAgentOverrideEnabled)
	exec := &Executor{settingsRepo: &stubExecutorSettingsRepo{values: map[string]string{
		domain.SettingKeyGlobalUserAgent: "Mozilla/5.0 MaxxOverride",
	}}}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("User-Agent", "source-client/1.0")
	ctx := flow.NewCtx(httptest.NewRecorder(), req)
	state := &execState{requestHeaders: req.Header}

	exec.applyGlobalUserAgentOverride(ctx, state)

	if got := req.Header.Get("User-Agent"); got != "source-client/1.0" {
		t.Fatalf("request User-Agent = %q", got)
	}
	if got := flow.GetGlobalUserAgentOverride(ctx); got != "" {
		t.Fatalf("flow override = %q", got)
	}
}
