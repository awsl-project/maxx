package zai

import (
	"testing"

	"github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/domain"
)

func TestNewAdapterRequiresZaiConfig(t *testing.T) {
	cases := []struct {
		name string
		p    *domain.Provider
	}{
		{"nil config", &domain.Provider{Name: "zai", Config: nil}},
		{"missing zai", &domain.Provider{Name: "zai", Config: &domain.ProviderConfig{}}},
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
		Name:                 "my-zai",
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeClaude, domain.ClientTypeOpenAI, domain.ClientTypeCodex},
		Config: &domain.ProviderConfig{
			DisableErrorCooldown: true,
			Zai: &domain.ProviderConfigZai{
				APIKey: "zai-secret",
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
	if custom.APIKey != "zai-secret" {
		t.Errorf("APIKey = %q, want zai-secret", custom.APIKey)
	}
	if custom.Backend != "" {
		t.Errorf("Backend = %q, want empty (HTTP passthrough)", custom.Backend)
	}
	// Claude routes to the shared Anthropic root; OpenAI routes to the plan root
	// (default plan = coding → /api/coding/paas/v4).
	if got := custom.ClientBaseURL[domain.ClientTypeClaude]; got != defaultBaseURL {
		t.Errorf("ClientBaseURL[claude] = %q, want %q", got, defaultBaseURL)
	}
	if got := custom.ClientBaseURL[domain.ClientTypeOpenAI]; got != openAICodingBaseURL {
		t.Errorf("ClientBaseURL[openai] = %q, want %q (default coding plan)", got, openAICodingBaseURL)
	}
	// Codex routes to the plan-independent Responses root.
	if got := custom.ClientBaseURL[domain.ClientTypeCodex]; got != openAIResponsesBaseURL {
		t.Errorf("ClientBaseURL[codex] = %q, want %q", got, openAIResponsesBaseURL)
	}
	// Disguise must be forced off: z.ai's /api/anthropic is a real Anthropic
	// gateway, and the default claude-code disguise injects Claude Code
	// fingerprints/prompt-caching the client never asked for.
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

	// z.ai natively speaks Anthropic Messages, OpenAI Chat Completions and OpenAI
	// Responses; the configured [claude, openai, codex] set is preserved.
	got := a.SupportedClientTypes()
	want := []domain.ClientType{domain.ClientTypeClaude, domain.ClientTypeOpenAI, domain.ClientTypeCodex}
	if len(got) != len(want) {
		t.Fatalf("SupportedClientTypes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("SupportedClientTypes = %v, want %v", got, want)
			break
		}
	}
}

// TestNewAdapterPlanSelectsOpenAIEndpoint verifies the plan switch: the standard
// API plan points OpenAI clients at /api/paas/v4, while claude stays on the
// shared Anthropic root regardless of plan.
func TestNewAdapterPlanSelectsOpenAIEndpoint(t *testing.T) {
	cases := []struct {
		name       string
		plan       string
		wantOpenAI string
	}{
		{"empty defaults to coding", "", openAICodingBaseURL},
		{"coding plan", "coding", openAICodingBaseURL},
		{"standard api plan", "api", openAIAPIBaseURL},
		{"unknown folds to coding", "bogus", openAICodingBaseURL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &domain.Provider{
				Name: "my-zai",
				Config: &domain.ProviderConfig{
					Zai: &domain.ProviderConfigZai{APIKey: "k", Plan: tc.plan},
				},
			}
			a, err := NewAdapter(p)
			if err != nil {
				t.Fatalf("NewAdapter: %v", err)
			}
			custom := a.(*Adapter).synth.Config.Custom
			if got := custom.ClientBaseURL[domain.ClientTypeOpenAI]; got != tc.wantOpenAI {
				t.Errorf("ClientBaseURL[openai] = %q, want %q", got, tc.wantOpenAI)
			}
			if got := custom.ClientBaseURL[domain.ClientTypeClaude]; got != defaultBaseURL {
				t.Errorf("ClientBaseURL[claude] = %q, want %q", got, defaultBaseURL)
			}
		})
	}
}

func TestAdapterFactoryRegistered(t *testing.T) {
	if _, ok := provider.GetAdapterFactory("zai"); !ok {
		t.Fatal("zai adapter factory not registered")
	}
}
