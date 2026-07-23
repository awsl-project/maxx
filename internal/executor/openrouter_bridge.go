package executor

import (
	"net/url"
	"strings"

	"github.com/awsl-project/maxx/internal/domain"
)

// shouldBridgeCustomCodexViaOpenAI returns true for custom OpenRouter-style
// providers that are reachable through OpenAI Chat Completions but reject Codex
// Responses API tool schemas. Codex CLI sends Responses-shaped tool definitions
// such as web_search/image_generation, while OpenRouter accepts only its own
// openrouter:* built-in tool types on /responses. Routing through OpenAI keeps
// user-defined function tools compatible and avoids breaking normal Codex
// providers.
func shouldBridgeCustomCodexViaOpenAI(provider *domain.Provider, clientType domain.ClientType, supportedTypes []domain.ClientType) bool {
	if provider == nil || clientType != domain.ClientTypeCodex || provider.Type != "custom" {
		return false
	}
	if !supportsClientType(supportedTypes, domain.ClientTypeOpenAI) {
		return false
	}
	if provider.Config == nil || provider.Config.Custom == nil {
		return false
	}

	custom := provider.Config.Custom
	if isOpenRouterCompatibleURL(custom.BaseURL) {
		return true
	}
	if custom.ClientBaseURL != nil {
		if isOpenRouterCompatibleURL(custom.ClientBaseURL[domain.ClientTypeCodex]) {
			return true
		}
		if isOpenRouterCompatibleURL(custom.ClientBaseURL[domain.ClientTypeOpenAI]) {
			return true
		}
	}
	return isOpenRouterCompatibleProviderName(provider.Name)
}

func supportsClientType(types []domain.ClientType, target domain.ClientType) bool {
	for _, candidate := range types {
		if candidate == target {
			return true
		}
	}
	return false
}

func isOpenRouterCompatibleURL(rawURL string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(rawURL))
	if trimmed == "" {
		return false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return false
	}
	host := strings.TrimPrefix(parsed.Hostname(), "www.")
	return host == "openrouter.ai" || strings.HasSuffix(host, ".openrouter.ai")
}

func isOpenRouterCompatibleProviderName(name string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(name)), "openrouter")
}
