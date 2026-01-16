package kiro

import (
	"sync"
)

// GlobalSettings holds global Kiro configuration
type GlobalSettings struct {
	// ModelMappingRules is an ordered list of model mapping rules
	// Rules are matched in order, first match wins
	ModelMappingRules []ModelMappingRule
}

// ModelMappingRule represents a single model mapping rule
// Rules are matched in order, first match wins
type ModelMappingRule struct {
	Pattern string `json:"pattern"` // Source pattern, supports * wildcard
	Target  string `json:"target"`  // Target CodeWhisperer model ID
}

var (
	globalSettings     *GlobalSettings
	globalSettingsMu   sync.RWMutex
	settingsGetterFunc func() (*GlobalSettings, error)
)

// SetGlobalSettingsGetter sets the function to retrieve global settings
// This should be called during application initialization
func SetGlobalSettingsGetter(getter func() (*GlobalSettings, error)) {
	globalSettingsMu.Lock()
	defer globalSettingsMu.Unlock()
	settingsGetterFunc = getter
}

// GetGlobalSettings retrieves the current global settings
func GetGlobalSettings() *GlobalSettings {
	globalSettingsMu.RLock()
	defer globalSettingsMu.RUnlock()

	if settingsGetterFunc == nil {
		return nil
	}

	settings, err := settingsGetterFunc()
	if err != nil {
		return nil
	}
	return settings
}

// ParseModelMappingRules parses a JSON string into model mapping rules
// Supports both new array format and legacy map format for backwards compatibility
func ParseModelMappingRules(jsonStr string) ([]ModelMappingRule, error) {
	if jsonStr == "" {
		return nil, nil
	}

	// Try new array format first
	var rules []ModelMappingRule
	if err := FastUnmarshal([]byte(jsonStr), &rules); err == nil {
		return rules, nil
	}

	// Fall back to legacy map format: {"pattern": "target", ...}
	var legacyMap map[string]string
	if err := FastUnmarshal([]byte(jsonStr), &legacyMap); err != nil {
		return nil, err
	}

	// Convert legacy map to rules array
	rules = make([]ModelMappingRule, 0, len(legacyMap))
	for pattern, target := range legacyMap {
		rules = append(rules, ModelMappingRule{
			Pattern: pattern,
			Target:  target,
		})
	}
	return rules, nil
}

// GetDefaultModelMappingRules returns a copy of the default mapping rules
func GetDefaultModelMappingRules() []ModelMappingRule {
	result := make([]ModelMappingRule, len(defaultModelMappingRules))
	copy(result, defaultModelMappingRules)
	return result
}
