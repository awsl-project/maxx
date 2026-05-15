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
	provider := &domain.Provider{
		Type: "custom",
		Name: "relay",
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL: "https://relay.example.com/v1",
			ClientBaseURL: map[domain.ClientType]string{
				domain.ClientTypeCodex: "https://openrouter.ai/api/v1",
			},
		}},
	}

	if !shouldBridgeCustomCodexViaOpenAI(provider, domain.ClientTypeCodex, []domain.ClientType{domain.ClientTypeCodex, domain.ClientTypeOpenAI}) {
		t.Fatal("expected OpenRouter Codex client base URL to bridge through OpenAI")
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
