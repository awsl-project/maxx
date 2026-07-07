package openrouter

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

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

	// SupportedClientTypes passes through from the real provider.
	got := a.SupportedClientTypes()
	if len(got) != 2 || got[0] != domain.ClientTypeClaude || got[1] != domain.ClientTypeOpenAI {
		t.Errorf("SupportedClientTypes = %v, want [claude openai]", got)
	}
}
