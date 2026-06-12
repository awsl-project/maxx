package payloadoverride

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// ModelSelector describes which protocol/model combinations a rule applies to.
type ModelSelector struct {
	Name     string `json:"name"`
	Protocol string `json:"protocol,omitempty"`
}

// Rule describes a single payload override rule.
type Rule struct {
	Models []ModelSelector            `json:"models"`
	Params map[string]json.RawMessage `json:"params"`
}

// GlobalSettings holds runtime payload override configuration.
type GlobalSettings struct {
	Rules []Rule
}

var (
	cachedSettings     *GlobalSettings
	cacheLoaded        bool
	cacheGeneration    uint64
	globalSettingsMu   sync.RWMutex
	settingsGetterFunc func() (*GlobalSettings, error)
)

// SetGlobalSettingsGetter sets the function used to retrieve runtime settings.
func SetGlobalSettingsGetter(getter func() (*GlobalSettings, error)) {
	globalSettingsMu.Lock()
	defer globalSettingsMu.Unlock()
	settingsGetterFunc = getter
	cachedSettings = nil
	cacheLoaded = false
	cacheGeneration++
}

// GetGlobalSettings retrieves the current runtime settings.
func GetGlobalSettings() *GlobalSettings {
	globalSettingsMu.RLock()
	if cacheLoaded {
		settings := cachedSettings
		globalSettingsMu.RUnlock()
		return settings
	}
	globalSettingsMu.RUnlock()

	settings, err := ReloadGlobalSettings()
	if err != nil {
		log.Printf("[PayloadOverride] Failed to reload global settings: %v; payload override rules will be skipped until reload succeeds", err)
		return nil
	}
	return settings
}

// InvalidateGlobalSettingsCache clears the runtime settings cache.
func InvalidateGlobalSettingsCache() {
	globalSettingsMu.Lock()
	defer globalSettingsMu.Unlock()
	cachedSettings = nil
	cacheLoaded = false
	cacheGeneration++
}

// ReloadGlobalSettings reloads settings through the configured getter and refreshes the cache.
func ReloadGlobalSettings() (*GlobalSettings, error) {
	for {
		globalSettingsMu.RLock()
		getter := settingsGetterFunc
		generation := cacheGeneration
		globalSettingsMu.RUnlock()
		if getter == nil {
			return nil, nil
		}

		settings, err := getter()
		if err != nil {
			return nil, err
		}
		if settings == nil {
			settings = &GlobalSettings{}
		}

		globalSettingsMu.Lock()
		if cacheGeneration == generation {
			cachedSettings = settings
			cacheLoaded = true
			ret := cachedSettings
			globalSettingsMu.Unlock()
			return ret, nil
		}
		currentSettings := cachedSettings
		currentLoaded := cacheLoaded
		globalSettingsMu.Unlock()

		if currentLoaded {
			return currentSettings, nil
		}
	}
}

// ValidateRulesJSON validates a payload override setting before it is persisted.
func ValidateRulesJSON(jsonStr string) error {
	trimmed := strings.TrimSpace(jsonStr)
	if trimmed == "" {
		return validationError("payload_override_rules cannot be empty")
	}

	var topLevel any
	if err := json.Unmarshal([]byte(trimmed), &topLevel); err != nil {
		return validationError("payload_override_rules must be a valid JSON array: %v", err)
	}
	if _, ok := topLevel.([]any); !ok {
		return validationError("payload_override_rules top level must be an array")
	}

	var rawRules []json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &rawRules); err != nil {
		return validationError("payload_override_rules must be a valid JSON array: %v", err)
	}

	seenSelectors := make(map[string]int, len(rawRules))
	for idx, rawRule := range rawRules {
		var rule Rule
		if err := json.Unmarshal(rawRule, &rule); err != nil {
			return validationError("payload_override_rules rule #%d is invalid: %v", idx+1, err)
		}
		if err := validateRule(rule, idx+1, seenSelectors); err != nil {
			return err
		}
	}

	return nil
}

// ParseRules parses payload override rules from JSON.
func ParseRules(jsonStr string) ([]Rule, error) {
	trimmed := strings.TrimSpace(jsonStr)
	if trimmed == "" {
		return nil, nil
	}

	var rules []Rule
	if err := json.Unmarshal([]byte(trimmed), &rules); err != nil {
		return nil, err
	}

	normalized := make([]Rule, 0, len(rules))
	for _, rule := range rules {
		next := Rule{
			Models: make([]ModelSelector, 0, len(rule.Models)),
			Params: make(map[string]json.RawMessage, len(rule.Params)),
		}
		for _, selector := range rule.Models {
			name := strings.TrimSpace(selector.Name)
			if name == "" {
				continue
			}
			next.Models = append(next.Models, ModelSelector{
				Name:     name,
				Protocol: effectiveSelectorProtocol(selector),
			})
		}
		for path, value := range rule.Params {
			trimmedPath := strings.TrimSpace(path)
			if trimmedPath == "" {
				continue
			}
			if IsReservedPath(trimmedPath) {
				log.Printf("[PayloadOverride] ignoring reserved path=%q while parsing rules", trimmedPath)
				continue
			}
			next.Params[trimmedPath] = value
		}
		if len(next.Models) == 0 {
			continue
		}
		if len(next.Params) == 0 {
			continue
		}
		normalized = append(normalized, next)
	}

	return normalized, nil
}

// ApplyGlobal applies runtime rules to the request body.
func ApplyGlobal(raw []byte, protocol, model string) []byte {
	settings := GetGlobalSettings()
	if settings == nil || len(settings.Rules) == 0 {
		return raw
	}
	return ApplyRules(raw, settings.Rules, protocol, model)
}

// ApplyRules applies the provided rules to the request body.
func ApplyRules(raw []byte, rules []Rule, protocol, model string) []byte {
	if len(raw) == 0 || len(rules) == 0 {
		return raw
	}

	currentProtocol := strings.ToLower(strings.TrimSpace(protocol))
	currentModel := strings.TrimSpace(model)
	if currentModel == "" {
		currentModel = strings.TrimSpace(gjson.GetBytes(raw, "model").String())
	}
	if currentModel == "" {
		return raw
	}

	body := raw
	for _, rule := range rules {
		if !ruleMatches(rule, currentProtocol, currentModel) {
			continue
		}
		body = applyRuleParams(body, currentProtocol, currentModel, rule.Params)
	}
	return body
}

func ruleMatches(rule Rule, protocol, model string) bool {
	for _, selector := range rule.Models {
		if effectiveSelectorProtocol(selector) != protocol {
			continue
		}
		if domain.MatchWildcard(selector.Name, model) {
			return true
		}
	}
	return false
}

func applyRuleParams(body []byte, protocol, model string, params map[string]json.RawMessage) []byte {
	if len(params) == 0 {
		return body
	}

	paths := make([]string, 0, len(params))
	for path := range params {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}
		paths = append(paths, trimmed)
	}

	// Apply shallow paths first so more specific nested paths can override them deterministically.
	sort.Slice(paths, func(i, j int) bool {
		depthI := strings.Count(paths[i], ".")
		depthJ := strings.Count(paths[j], ".")
		if depthI != depthJ {
			return depthI < depthJ
		}
		return paths[i] < paths[j]
	})

	out := body
	for _, path := range paths {
		if IsReservedPath(path) {
			log.Printf("[PayloadOverride] ignoring reserved path=%q protocol=%s model=%s", path, protocol, model)
			continue
		}
		updated, err := sjson.SetRawBytes(out, path, params[path])
		if err != nil {
			log.Printf("[PayloadOverride] failed to apply path=%q protocol=%s model=%s: %v", path, protocol, model, err)
			continue
		}
		out = updated
	}
	return out
}

// IsReservedPath reports whether a JSON path targets adapter-controlled fields
// instead of pure payload content.
func IsReservedPath(path string) bool {
	first := firstPathSegment(path)
	return strings.EqualFold(first, "model") || strings.EqualFold(first, "stream")
}

func firstPathSegment(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	for i, r := range trimmed {
		if r == '.' || r == '[' {
			return trimmed[:i]
		}
	}
	return trimmed
}

func validateRule(rule Rule, index int, seenSelectors map[string]int) error {
	if len(rule.Models) == 0 {
		return validationError("payload_override_rules rule #%d must define at least one model selector", index)
	}
	for _, selector := range rule.Models {
		name := strings.TrimSpace(selector.Name)
		if name == "" {
			return validationError("payload_override_rules rule #%d contains an empty model selector name", index)
		}

		protocol := strings.ToLower(strings.TrimSpace(selector.Protocol))
		if protocol != "" && protocol != "codex" {
			return validationError(
				"payload_override_rules rule #%d uses unsupported protocol %q",
				index,
				selector.Protocol,
			)
		}

		if seenSelectors != nil {
			effectiveProtocol := effectiveSelectorProtocol(selector)
			selectorKey := effectiveProtocol + "\x00" + strings.ToLower(name)
			if firstIndex, exists := seenSelectors[selectorKey]; exists {
				return validationError(
					"payload_override_rules rule #%d duplicates model selector %q for protocol %q from rule #%d",
					index,
					name,
					effectiveProtocol,
					firstIndex,
				)
			}
			seenSelectors[selectorKey] = index
		}
	}

	if len(rule.Params) == 0 {
		return validationError("payload_override_rules rule #%d params must contain at least one path", index)
	}

	validPaths := 0
	for path := range rule.Params {
		trimmedPath := strings.TrimSpace(path)
		if trimmedPath == "" {
			return validationError("payload_override_rules rule #%d contains an empty params path", index)
		}
		if IsReservedPath(trimmedPath) {
			return validationError(
				"payload_override_rules rule #%d uses reserved path %q",
				index,
				trimmedPath,
			)
		}
		validPaths++
	}

	if validPaths == 0 {
		return validationError("payload_override_rules rule #%d params must contain at least one path", index)
	}

	return nil
}

func validationError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", domain.ErrInvalidInput, fmt.Sprintf(format, args...))
}

func effectiveSelectorProtocol(selector ModelSelector) string {
	protocol := strings.ToLower(strings.TrimSpace(selector.Protocol))
	if protocol == "" {
		return "codex"
	}
	return protocol
}
