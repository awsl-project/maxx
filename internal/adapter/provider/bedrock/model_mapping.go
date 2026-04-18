package bedrock

import (
	"regexp"
)

// modelDatePattern matches an Anthropic short name that already carries an
// explicit release date, e.g. "claude-sonnet-4-5-20250929". When the client
// supplies one of these, the date is authoritative and we wrap it into a
// Bedrock ID without inventing any information.
var modelDatePattern = regexp.MustCompile(`^claude-[\w-]+-\d{8}$`)

// bedrockIDPattern matches an already-qualified Bedrock model / inference
// profile ID — optionally with a region prefix. Passed through untouched.
var bedrockIDPattern = regexp.MustCompile(`^(?:[a-z]{2,}\.)?anthropic\.`)

// regionPrefixedPattern matches a fully-qualified profile ID that already
// starts with a region prefix like "us.", "eu.", "apac." — applyPrefix
// uses it to avoid re-adding the configured prefix. Keeping this strict
// (instead of "contains a dot") prevents us from silently dropping the
// configured prefix on unusual user-mapping values.
var regionPrefixedPattern = regexp.MustCompile(`^[a-z]{2,}\.anthropic\.`)

// discoveredLookup returns a Bedrock profile ID for an Anthropic short name,
// or ("", false) on miss. May be nil when no discoverer is wired up.
type discoveredLookup func(shortName string) (id string, hit bool)

// resolveModelID maps a request model to a fully-qualified Bedrock ID.
// Returns ok=false for bare short names that cannot be resolved — the caller
// must surface an error rather than guess a date, since Bedrock profile IDs
// are AWS-controlled and any local guess risks silently substituting one
// model version for another.
//
// Priority:
//  1. user configMapping — explicit override, trusted
//  2. runtime discovery  — authoritative list from ListInferenceProfiles
//  3. client-supplied dated name (claude-*-YYYYMMDD) — wrap as anthropic.X-v1:0
//  4. client-supplied fully-qualified Bedrock ID — passthrough
func resolveModelID(model string, configMapping map[string]string, modelPrefix string, discovered discoveredLookup) (string, bool) {
	if configMapping != nil {
		if mapped, ok := configMapping[model]; ok {
			return applyPrefix(mapped, modelPrefix), true
		}
	}
	if discovered != nil {
		if id, ok := discovered(model); ok {
			return applyPrefix(id, modelPrefix), true
		}
	}
	if modelDatePattern.MatchString(model) {
		return applyPrefix("anthropic."+model+"-v1:0", modelPrefix), true
	}
	if bedrockIDPattern.MatchString(model) {
		return applyPrefix(model, modelPrefix), true
	}
	return "", false
}

// applyPrefix prepends the region prefix (e.g. "us.") unless the model ID
// already starts with one (e.g. "us.anthropic...", "eu.anthropic..."). The
// check is a strict region-prefix match rather than "contains a dot", so
// user-supplied values like "amazon.nova-pro-v1:0" or typos don't silently
// lose the configured prefix.
func applyPrefix(modelID, prefix string) string {
	if prefix == "" {
		return modelID
	}
	if regionPrefixedPattern.MatchString(modelID) {
		return modelID
	}
	return prefix + "." + modelID
}
