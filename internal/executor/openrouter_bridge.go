package executor

import (
	"net/url"
	"strings"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/systemsettingcache"
)

// codexOpenRouterBridgeEnabled reports whether the legacy Codex→OpenRouter chat
// bridge kill switch is on. Default false → native /responses passthrough.
func (e *Executor) codexOpenRouterBridgeEnabled() bool {
	return systemsettingcache.GetBooleanDefault(e.settingsRepo, domain.SettingKeyCodexOpenRouterBridgeEnabled, false)
}

// bridgeCodexForOpenRouter reports whether a Codex request bound for an
// OpenRouter-style provider should be bridged to OpenAI Chat Completions instead
// of passing through natively to /responses.
//
// Default is passthrough (bridgeEnabled=false). OpenRouter's /responses endpoint
// natively accepts Codex requests — function tools, the multi_agent namespace,
// web_search, AND custom/freeform tools (apply_patch, "code mode" exec) — and
// returns proper custom_tool_call items (empirically verified against
// openai/gpt-5.5 and openai/gpt-5.6-sol). The old chat bridge, by contrast, is
// lossy: it rewrites a Codex `custom` tool into an OpenAI `function(input)`,
// which the model then refuses to drive ("no terminal tool"), silently breaking
// code-mode agents while returning HTTP 200. Passthrough preserves fidelity and
// is the only path on which code mode can work at all.
//
// The chat bridge is retained purely as an opt-in kill switch
// (SettingKeyCodexOpenRouterBridgeEnabled) in case a future OpenRouter/model
// regression makes /responses passthrough unviable.
func bridgeCodexForOpenRouter(bridgeEnabled bool, provider *domain.Provider, clientType domain.ClientType, supportedTypes []domain.ClientType) bool {
	if !bridgeEnabled {
		return false
	}
	return shouldBridgeCustomCodexViaOpenAI(provider, clientType, supportedTypes)
}

// shouldBridgeCustomCodexViaOpenAI reports whether a provider is an
// OpenRouter-style endpoint reachable through OpenAI Chat Completions — the
// predicate the legacy chat bridge acted on. It covers both the native
// `openrouter` provider type and a `custom` provider pointed at OpenRouter
// (baseURL, per-client baseURL, or a name containing "openrouter"). It is now
// only consulted when the bridge kill switch is enabled; see
// bridgeCodexForOpenRouter for the default passthrough rationale.
func shouldBridgeCustomCodexViaOpenAI(provider *domain.Provider, clientType domain.ClientType, supportedTypes []domain.ClientType) bool {
	if provider == nil || clientType != domain.ClientTypeCodex {
		return false
	}
	if !supportsClientType(supportedTypes, domain.ClientTypeOpenAI) {
		return false
	}

	// A native OpenRouter provider IS OpenRouter, so always bridge its Codex
	// requests through OpenAI Chat Completions. Its config lives in
	// Config.OpenRouter (Config.Custom is nil), so the custom-URL probes below
	// never apply — this branch is the only thing that makes Codex usable on the
	// first-class openrouter type.
	if provider.Type == "openrouter" {
		return true
	}
	if provider.Type != "custom" {
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
