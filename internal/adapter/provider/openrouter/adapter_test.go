package openrouter

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
)

// TestOpenRouterResponsesPath guards the Codex Responses path pinning: OpenRouter
// serves the Responses API only under /v1, so the upstream path must always carry
// the /v1 prefix while preserving any sub-path (/responses/{id}) and query string.
func TestOpenRouterResponsesPath(t *testing.T) {
	cases := []struct{ name, clientPath, requestURI, want string }{
		{"bare responses gets v1", "/responses", "/responses", "/v1/responses"},
		{"already versioned kept", "/v1/responses", "/responses", "/v1/responses"},
		{"query preserved", "/responses?foo=bar", "/responses", "/v1/responses?foo=bar"},
		{"sub-path preserved", "/responses/abc", "/responses", "/v1/responses/abc"},
		{"versioned sub-path+query kept", "/v1/responses/abc?x=1", "/responses", "/v1/responses/abc?x=1"},
		{"empty client path falls back to requestURI", "", "/responses", "/v1/responses"},
		{"empty both defaults", "", "", "/v1/responses"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://x/responses", nil)
			c := flow.NewCtx(httptest.NewRecorder(), req)
			if tc.clientPath != "" {
				c.Set(flow.KeyResponsesClientPath, tc.clientPath)
			}
			if tc.requestURI != "" {
				c.Set(flow.KeyRequestURI, tc.requestURI)
			}
			if got := openRouterResponsesPath(c); got != tc.want {
				t.Errorf("openRouterResponsesPath = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNewAdapterRequiresOpenRouterConfig(t *testing.T) {
	cases := []struct {
		name string
		p    *domain.Provider
	}{
		{"nil config", &domain.Provider{Name: "or", Config: nil}},
		{"missing openrouter", &domain.Provider{Name: "or", Config: &domain.ProviderConfig{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewAdapter(tc.p); err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestNewAdapterSynthesizesCustomConfig(t *testing.T) {
	p := &domain.Provider{
		Name:                 "my-openrouter",
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeClaude, domain.ClientTypeOpenAI},
		Config: &domain.ProviderConfig{
			DisableErrorCooldown: true,
			OpenRouter: &domain.ProviderConfigOpenRouter{
				APIKey: "sk-or-secret",
			},
		},
	}

	a, err := NewAdapter(p)
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}

	adapter, ok := a.(*Adapter)
	if !ok {
		t.Fatalf("expected *Adapter, got %T", a)
	}

	custom := adapter.synth.Config.Custom
	if custom == nil {
		t.Fatal("synthesized Custom config is nil")
	}
	if custom.BaseURL != defaultBaseURL {
		t.Errorf("BaseURL = %q, want %q", custom.BaseURL, defaultBaseURL)
	}
	if custom.APIKey != "sk-or-secret" {
		t.Errorf("APIKey = %q, want sk-or-secret", custom.APIKey)
	}
	if custom.Backend != "" {
		t.Errorf("Backend = %q, want empty (HTTP passthrough)", custom.Backend)
	}
	// Disguise must be forced off: OpenRouter is a real Anthropic/OpenAI gateway,
	// and the default claude-code disguise injects prompt caching that 400s some
	// OpenRouter models.
	if custom.Disguise == nil || custom.Disguise.Type != domain.DisguiseTypeNone {
		t.Errorf("Disguise = %+v, want type=none", custom.Disguise)
	}
	if !adapter.synth.Config.DisableErrorCooldown {
		t.Error("DisableErrorCooldown not propagated")
	}

	// The real provider's config must not be mutated by synthesis.
	if p.Config.Custom != nil {
		t.Error("real provider Config.Custom should stay nil")
	}

	// SupportedClientTypes reflects OpenRouter's native capabilities: the
	// configured claude+openai PLUS codex, which OpenRouter always speaks
	// natively (/v1/responses) even when the stored config predates Codex support.
	got := a.SupportedClientTypes()
	if len(got) != 3 || got[0] != domain.ClientTypeClaude || got[1] != domain.ClientTypeOpenAI || got[2] != domain.ClientTypeCodex {
		t.Errorf("SupportedClientTypes = %v, want [claude openai codex]", got)
	}
}

// TestSupportedClientTypesAlwaysAdvertisesCodex guards the core of the passthrough
// fix: even a provider whose stored config omits codex (created before Codex
// support) must advertise it, so Codex requests pass through to /responses rather
// than being converted to Chat Completions.
func TestSupportedClientTypesAlwaysAdvertisesCodex(t *testing.T) {
	p := &domain.Provider{
		Name:                 "legacy-openrouter",
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeClaude, domain.ClientTypeOpenAI},
		Config: &domain.ProviderConfig{
			OpenRouter: &domain.ProviderConfigOpenRouter{APIKey: "sk-or-secret"},
		},
	}
	a, err := NewAdapter(p)
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	found := false
	for _, ct := range a.SupportedClientTypes() {
		if ct == domain.ClientTypeCodex {
			found = true
		}
	}
	if !found {
		t.Fatalf("SupportedClientTypes = %v, want it to include codex", a.SupportedClientTypes())
	}
}
