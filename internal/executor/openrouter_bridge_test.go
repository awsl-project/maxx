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

func TestOpenRouterCodexBridgeUsesChatCompletionsPath(t *testing.T) {
	if got := ConvertRequestURI("/responses", domain.ClientTypeCodex, domain.ClientTypeOpenAI, "", true); got != "/v1/chat/completions" {
		t.Fatalf("ConvertRequestURI(/responses) = %q, want /v1/chat/completions", got)
	}
	if got := ConvertRequestURI("/v1/responses", domain.ClientTypeCodex, domain.ClientTypeOpenAI, "", true); got != "/v1/chat/completions" {
		t.Fatalf("ConvertRequestURI(/v1/responses) = %q, want /v1/chat/completions", got)
	}
}
