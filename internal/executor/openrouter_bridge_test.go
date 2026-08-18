package executor

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestShouldBridgeCustomCodexViaOpenAIForOpenRouter(t *testing.T) {
	provider := &domain.Provider{
		Type: "custom",
		Name: "openrouter",
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL: "https://openrouter.ai/api/v1",
		}},
	}

	if !shouldBridgeCustomCodexViaOpenAI(provider, domain.ClientTypeCodex, []domain.ClientType{domain.ClientTypeCodex, domain.ClientTypeOpenAI}) {
		t.Fatal("expected OpenRouter custom Codex route to bridge through OpenAI")
	}
}

func TestShouldBridgeNativeOpenRouterCodex(t *testing.T) {
	// The first-class openrouter provider type carries its config in
	// Config.OpenRouter (Config.Custom is nil); it must still bridge Codex through
	// OpenAI Chat Completions.
	provider := &domain.Provider{
		Type: "openrouter",
		Name: "my-openrouter",
		Config: &domain.ProviderConfig{OpenRouter: &domain.ProviderConfigOpenRouter{
			APIKey: "sk-or-test",
		}},
	}

	if !shouldBridgeCustomCodexViaOpenAI(provider, domain.ClientTypeCodex, []domain.ClientType{domain.ClientTypeClaude, domain.ClientTypeOpenAI}) {
		t.Fatal("native openrouter provider must bridge Codex through OpenAI Chat Completions")
	}
}

func TestShouldNotBridgeNativeOpenRouterWithoutOpenAISupport(t *testing.T) {
	// Without OpenAI in the supported set there is no Chat Completions target to
	// bridge to, so the native provider must not claim the Codex bridge.
	provider := &domain.Provider{
		Type: "openrouter",
		Name: "my-openrouter",
		Config: &domain.ProviderConfig{OpenRouter: &domain.ProviderConfigOpenRouter{
			APIKey: "sk-or-test",
		}},
	}

	if shouldBridgeCustomCodexViaOpenAI(provider, domain.ClientTypeCodex, []domain.ClientType{domain.ClientTypeClaude}) {
		t.Fatal("native openrouter provider without OpenAI support must not bridge Codex")
	}
}

func TestShouldNotBridgeOpenRouterWithoutOpenAISupport(t *testing.T) {
	provider := &domain.Provider{
		Type: "custom",
		Name: "openrouter",
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL: "https://openrouter.ai/api/v1",
		}},
	}

	if shouldBridgeCustomCodexViaOpenAI(provider, domain.ClientTypeCodex, []domain.ClientType{domain.ClientTypeCodex}) {
		t.Fatal("must not bridge when provider cannot accept OpenAI Chat Completions")
	}
}

func TestShouldNotBridgeNonOpenRouterCustomCodex(t *testing.T) {
	provider := &domain.Provider{
		Type: "custom",
		Name: "generic-relay",
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL: "https://relay.example.com/v1",
		}},
	}

	if shouldBridgeCustomCodexViaOpenAI(provider, domain.ClientTypeCodex, []domain.ClientType{domain.ClientTypeCodex, domain.ClientTypeOpenAI}) {
		t.Fatal("generic custom Codex routes should keep their declared Codex Responses path")
	}
}

func TestShouldBridgeOpenRouterClientBaseURL(t *testing.T) {
	tests := []struct {
		name          string
		clientBaseURL map[domain.ClientType]string
	}{
		{
			name: "codex client base URL",
			clientBaseURL: map[domain.ClientType]string{
				domain.ClientTypeCodex: "https://openrouter.ai/api/v1",
			},
		},
		{
			name: "openai client base URL",
			clientBaseURL: map[domain.ClientType]string{
				domain.ClientTypeOpenAI: "https://openrouter.ai/api/v1",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &domain.Provider{
				Type: "custom",
				Name: "relay",
				Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
					BaseURL:       "https://relay.example.com/v1",
					ClientBaseURL: tt.clientBaseURL,
				}},
			}

			if !shouldBridgeCustomCodexViaOpenAI(provider, domain.ClientTypeCodex, []domain.ClientType{domain.ClientTypeCodex, domain.ClientTypeOpenAI}) {
				t.Fatal("expected OpenRouter client base URL to bridge through OpenAI")
			}
		})
	}
}

func TestShouldNotBridgeLookalikeOpenRouterHost(t *testing.T) {
	provider := &domain.Provider{
		Type: "custom",
		Name: "generic-relay",
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL: "https://openrouter.ai.evil.example/api/v1",
		}},
	}

	if shouldBridgeCustomCodexViaOpenAI(provider, domain.ClientTypeCodex, []domain.ClientType{domain.ClientTypeCodex, domain.ClientTypeOpenAI}) {
		t.Fatal("must not bridge OpenRouter lookalike hosts")
	}
}

func TestBridgeCodexForOpenRouter_DefaultPassthrough(t *testing.T) {
	// The kill switch is off by default: even a native OpenRouter provider that
	// the legacy predicate would bridge must pass through natively to /responses.
	provider := &domain.Provider{
		Type: "openrouter",
		Name: "my-openrouter",
		Config: &domain.ProviderConfig{OpenRouter: &domain.ProviderConfigOpenRouter{
			APIKey: "sk-or-test",
		}},
	}
	supported := []domain.ClientType{domain.ClientTypeClaude, domain.ClientTypeOpenAI, domain.ClientTypeCodex}

	// Sanity: the underlying predicate still wants to bridge this route.
	if !shouldBridgeCustomCodexViaOpenAI(provider, domain.ClientTypeCodex, supported) {
		t.Fatal("precondition: predicate should recognize native OpenRouter Codex route")
	}
	// But with the bridge disabled (default), we must NOT bridge — passthrough wins.
	if bridgeCodexForOpenRouter(false, provider, domain.ClientTypeCodex, supported) {
		t.Fatal("bridge disabled: Codex→OpenRouter must pass through to /responses, not bridge to chat")
	}
}

func TestBridgeCodexForOpenRouter_KillSwitchRestoresBridge(t *testing.T) {
	provider := &domain.Provider{
		Type: "openrouter",
		Name: "my-openrouter",
		Config: &domain.ProviderConfig{OpenRouter: &domain.ProviderConfigOpenRouter{
			APIKey: "sk-or-test",
		}},
	}
	supported := []domain.ClientType{domain.ClientTypeClaude, domain.ClientTypeOpenAI, domain.ClientTypeCodex}

	// With the kill switch on, behavior falls back to the legacy predicate.
	if !bridgeCodexForOpenRouter(true, provider, domain.ClientTypeCodex, supported) {
		t.Fatal("bridge enabled: native OpenRouter Codex route should bridge through OpenAI")
	}
	// Enabling the switch never bridges a route the predicate rejects (no OpenAI target).
	if bridgeCodexForOpenRouter(true, provider, domain.ClientTypeCodex, []domain.ClientType{domain.ClientTypeClaude}) {
		t.Fatal("bridge enabled but no OpenAI support: must not bridge")
	}
}

func TestOpenRouterCodexBridgeUsesChatCompletionsPath(t *testing.T) {
	if got := ConvertRequestURI("/responses", domain.ClientTypeCodex, domain.ClientTypeOpenAI, "", true); got != "/v1/chat/completions" {
		t.Fatalf("ConvertRequestURI(/responses) = %q, want /v1/chat/completions", got)
	}
	if got := ConvertRequestURI("/v1/responses", domain.ClientTypeCodex, domain.ClientTypeOpenAI, "", true); got != "/v1/chat/completions" {
		t.Fatalf("ConvertRequestURI(/v1/responses) = %q, want /v1/chat/completions", got)
	}
}
