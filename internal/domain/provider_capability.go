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
		if len(configured) == 0 {
			return []ClientType{
				ClientTypeClaude,
				ClientTypeOpenAI,
			}
		}
	}

	return append([]ClientType(nil), configured...)
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
