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

// regionPrefixedPattern matches a fully-qualified profile ID that already
// starts with a region prefix like "us.", "eu.", "apac.", "global." —
// applyPrefix uses it to avoid re-adding the configured prefix. Keeping
// this strict (instead of "contains a dot") prevents silently dropping
// the configured prefix on unusual user-mapping values.
var regionPrefixedPattern = regexp.MustCompile(`^[a-z]{2,}\.anthropic\.`)

// inferenceProfilePattern matches the canonical dated + versioned
// Anthropic ID that becomes a valid Bedrock inference profile once a
// region prefix is attached. Any other shape — bare foundation models
// ("anthropic.claude-sonnet-4-6"), non-Anthropic providers, or
// user-defined aliases — is left unprefixed by applyPrefix because
// Bedrock does not accept region prefixes on those targets.
var inferenceProfilePattern = regexp.MustCompile(`^anthropic\.claude-[a-z0-9-]+-\d{8}-v\d+:\d+$`)

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
//                          and ListFoundationModels; returns ready-to-use
//                          IDs that must not be further prefixed
//  3. client-supplied dated name (claude-*-YYYYMMDD) — wrap as anthropic.X-v1:0
//  4. client-supplied bare "anthropic.<short>" — retry discovery on the
//                          stripped short name so releases that Bedrock
//                          only serves via inference profile can resolve
//                          to an invoke-ready ID; miss falls through to 5
//  5. client-supplied fully-qualified Bedrock ID — passthrough
func resolveModelID(model string, configMapping map[string]string, modelPrefix string, discovered discoveredLookup) (string, bool) {
	if configMapping != nil {
		if mapped, ok := configMapping[model]; ok {
			return applyPrefix(mapped, modelPrefix), true
		}
	}
	if discovered != nil {
		if id, ok := discovered(model); ok {
			// Discovery returns the exact invoke-ready ID: inference
			// profiles already carry their region prefix, foundation
			// models must not receive one (a region-prefixed foundation
			// model ID like "us.anthropic.claude-sonnet-4-6" is not a
			// valid Bedrock target).
			return id, true
		}
	}
	if modelDatePattern.MatchString(model) {
		return applyPrefix("anthropic."+model+"-v1:0", modelPrefix), true
	}
	if bedrockIDPattern.MatchString(model) {
		// Bare "anthropic.<short>" (no region prefix, no date+version) is a
		// foundation-model shape. Many current Claude releases have no
		// on-demand foundation SKU and only invoke through an inference
		// profile, so give discovery a chance at the short name before
		// falling back to passthrough. If discovery is absent or misses,
		// the original passthrough stands (operators/tests relying on it
		// stay unaffected).
		if discovered != nil &&
			!regionPrefixedPattern.MatchString(model) &&
			!inferenceProfilePattern.MatchString(model) {
			if short, ok := strings.CutPrefix(model, "anthropic."); ok && short != "" {
				if id, hit := discovered(short); hit {
					return id, true
				}
			}
		}
		return applyPrefix(model, modelPrefix), true
	}
	return "", false
}

// applyPrefix prepends the region prefix (e.g. "us.") only to IDs whose
// shape matches a Bedrock inference profile missing its region prefix —
// i.e. "anthropic.claude-X-YYYYMMDD-vN:M". Every other shape passes
// through untouched:
//
//   - Already region-prefixed ("us.anthropic.X…") — would double-prefix.
//   - Bare foundation models ("anthropic.claude-sonnet-4-6") — Bedrock
//     rejects region-prefixed foundation-model IDs as invalid targets.
//   - Non-Anthropic providers, user-defined aliases, typos — we have no
//     basis for claiming a region prefix would make them valid.
//
// Keeping the prefix behaviour narrowly scoped to canonical inference
// profile shapes prevents silently turning valid IDs into invalid ones
// (e.g. a user-configured modelMapping value of
// "anthropic.claude-sonnet-4-6" must not become
// "us.anthropic.claude-sonnet-4-6").
func applyPrefix(modelID, prefix string) string {
	if prefix == "" {
		return modelID
	}
	if regionPrefixedPattern.MatchString(modelID) {
		return modelID
	}
	if inferenceProfilePattern.MatchString(modelID) {
		return prefix + "." + modelID
	}
	return modelID
}
