package domain

// CanonicalSupportedClientTypes returns the authoritative native ClientType
// capabilities for a provider type. configured is only consulted for
// providers that allow user-defined protocol support (custom/newapi/openrouter).
func CanonicalSupportedClientTypes(providerType string, configured []ClientType) []ClientType {
	switch providerType {
	case "antigravity":
		return []ClientType{ClientTypeGemini, ClientTypeClaude}

	case "kiro":
		return []ClientType{ClientTypeClaude}

	case "codex":
		return []ClientType{ClientTypeCodex}

	case "claude":
		return []ClientType{ClientTypeClaude}

	case "ollama":
		return []ClientType{ClientTypeClaude}

	case "zai":
		// z.ai (智谱 GLM) natively speaks THREE protocols: Anthropic Messages
		// (/api/anthropic, Claude clients), OpenAI Chat Completions
		// (/api/[coding/]paas/v4, OpenAI clients) AND OpenAI Responses
		// (/api/v1/responses, Codex clients). A Codex route must pass through
		// natively (Responses→Responses) rather than fall into a lossy Chat
		// Completions conversion, so default to all three when unset; users can
		// narrow via the configured set. GLM ids are reached via ModelMapping just
		// like every other relay. Falls through to return the configured set.
		if len(configured) == 0 {
			return []ClientType{ClientTypeClaude, ClientTypeOpenAI, ClientTypeCodex}
		}

	case "grok":
		return []ClientType{ClientTypeOpenAI}

	case "fal":
		// fal (fal.ai) is a queue-based inference platform, NOT OpenAI-compatible.
		// maxx exposes it through two existing surfaces via a translation layer:
		// synchronous image generation (ClientTypeOpenAI, /v1/images/generations)
		// and async video generation (ClientTypeVideo, /v1/video/generations). Both
		// are native fal capabilities, so advertise both unconditionally — a video
		// route and an image route can each reach a fal provider without a
		// hand-authored capability set. fal model ids are reached via ModelMapping
		// (executor.mapModel) and become the URL path segment.
		return []ClientType{ClientTypeOpenAI, ClientTypeVideo}

	case "custom", "newapi":
		if len(configured) == 0 {
			return []ClientType{ClientTypeOpenAI}
		}

	case "openrouter":
		// OpenRouter natively speaks Claude (/v1/messages), OpenAI
		// (/v1/chat/completions) AND Codex (/v1/responses) — Responses/Codex is a
		// first-class capability, not an add-on. Always include Codex so a
		// pre-Codex [claude, openai] config (or an empty default) can't suppress
		// native /responses passthrough, while still honoring any other configured
		// protocols. A Codex request still only reaches this provider when an
		// explicit Codex route points at it.
		base := configured
		if len(base) == 0 {
			base = []ClientType{ClientTypeClaude, ClientTypeOpenAI}
		}
		return ensureClientType(base, ClientTypeCodex)
	}

	return append([]ClientType(nil), configured...)
}

// ensureClientType returns a copy of types with ct appended if not already present.
func ensureClientType(types []ClientType, ct ClientType) []ClientType {
	out := make([]ClientType, 0, len(types)+1)
	found := false
	for _, t := range types {
		out = append(out, t)
		if t == ct {
			found = true
		}
	}
	if !found {
		out = append(out, ct)
	}
	return out
}

// NormalizeProviderSupportedClientTypes rewrites provider.SupportedClientTypes
// to the canonical native capability set for its type.
func NormalizeProviderSupportedClientTypes(provider *Provider) {
	if provider == nil {
		return
	}

	provider.SupportedClientTypes = CanonicalSupportedClientTypes(
		provider.Type,
		provider.SupportedClientTypes,
	)
}

// ProviderNativelySupports reports whether the provider natively understands
// clientType. Stale SupportedClientTypes snapshots are ignored via the
// canonical helper so historical codex providers still count as native Codex.
func ProviderNativelySupports(provider *Provider, clientType ClientType) bool {
	if provider == nil {
		return false
	}

	supportedTypes := CanonicalSupportedClientTypes(
		provider.Type,
		provider.SupportedClientTypes,
	)

	for _, supported := range supportedTypes {
		if supported == clientType {
			return true
		}
	}

	return false
}

// RouteIsNative is the single source of truth for Route.IsNative:
// true when the target provider natively supports the route client type.
func RouteIsNative(provider *Provider, route *Route) bool {
	return route != nil &&
		ProviderNativelySupports(provider, route.ClientType)
}
