package domain

import "strings"

// RuntimeContextLimitForModel returns the configured runtime context window for
// a model on this provider. Zero means unset and preserves legacy behavior.
func RuntimeContextLimitForModel(provider *Provider, model string) uint64 {
	if provider == nil {
		return 0
	}
	model = strings.TrimSpace(model)
	if model != "" && len(provider.RuntimeContextLimits) > 0 {
		if limit := provider.RuntimeContextLimits[model]; limit > 0 {
			return limit
		}
		var wildcardLimit uint64
		var wildcardPattern string
		for pattern, limit := range provider.RuntimeContextLimits {
			pattern = strings.TrimSpace(pattern)
			if pattern == "" || limit == 0 || !strings.Contains(pattern, "*") || !MatchWildcard(pattern, model) {
				continue
			}
			if wildcardPattern == "" || len(pattern) > len(wildcardPattern) {
				wildcardPattern = pattern
				wildcardLimit = limit
			}
		}
		if wildcardLimit > 0 {
			return wildcardLimit
		}
	}
	return provider.RuntimeContextLimit
}
