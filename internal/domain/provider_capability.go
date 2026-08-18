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

	case "grok":
		return []ClientType{ClientTypeOpenAI}

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
