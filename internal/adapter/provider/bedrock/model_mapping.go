package bedrock

import (
	"regexp"
	"strings"
)

// modelDatePattern matches an Anthropic short name that already carries an
// explicit release date, e.g. "claude-sonnet-4-5-20250929". When the client
// supplies one of these, the date is authoritative and we wrap it into a
// Bedrock ID without inventing any information.
var modelDatePattern = regexp.MustCompile(`^claude-[\w-]+-\d{8}$`)

// bedrockIDPattern matches an already-qualified Bedrock model / inference
// profile ID — optionally with a region prefix. Passed through untouched.
var bedrockIDPattern = regexp.MustCompile(`^(?:[a-z]{2,}\.)?anthropic\.`)

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

// applyPrefix adds the region prefix (e.g. "us.") when the model ID doesn't
// already carry one. If the ID starts with a non-"anthropic" dotted segment
// (like "us.anthropic...") it's assumed to already be prefixed.
func applyPrefix(modelID, prefix string) string {
	if prefix == "" {
		return modelID
	}
	if strings.Contains(modelID, ".") && !strings.HasPrefix(modelID, "anthropic.") {
		return modelID
	}
	return prefix + "." + modelID
}
