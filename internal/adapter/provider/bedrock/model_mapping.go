package bedrock

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
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

// minorVersionPattern captures the new-style "claude-<family>-<major>-<minor>"
// short names (e.g. "claude-sonnet-4-6", "claude-opus-4-1"). Used to drive
// the degrade loop — we don't want it firing on old-style
// "claude-3-5-sonnet" (version before family) or on dated names.
var minorVersionPattern = regexp.MustCompile(`^(claude-[a-z]+)-(\d+)-(\d+)$`)

// degradeCandidates yields progressively-older fallback short names for a
// client-requested short name whose AWS inference profile has not yet
// shipped. Given "claude-sonnet-4-6" it returns
// ["claude-sonnet-4-5", "claude-sonnet-4-4", ..., "claude-sonnet-4-0",
// "claude-sonnet-4"]. Inputs that don't match minorVersionPattern yield
// nothing — so dated names, old-style names, and bare majors don't degrade.
func degradeCandidates(short string) []string {
	m := minorVersionPattern.FindStringSubmatch(short)
	if len(m) != 4 {
		return nil
	}
	family, major := m[1], m[2]
	minor, err := strconv.Atoi(m[3])
	if err != nil || minor <= 0 {
		return []string{family + "-" + major}
	}
	out := make([]string, 0, minor+1)
	for i := minor - 1; i >= 0; i-- {
		out = append(out, fmt.Sprintf("%s-%s-%d", family, major, i))
	}
	out = append(out, family+"-"+major)
	return out
}

// degradeLog deduplicates degrade warnings so a high-QPS client
// requesting an unshipped model doesn't flood the log with one line per
// request. Logging once per (requested, fallback) pair is enough to
// surface the substitution to an operator; the pair changes only when
// AWS ships or retires a profile, at which point we want the next
// occurrence logged again.
var degradeLog struct {
	sync.Mutex
	seen map[string]struct{}
}

// lookupWithFallback queries discovery for the exact short name and, on a
// miss, walks degradeCandidates until one hits. Logs a WARN on the first
// occurrence of each degrade pair so operators notice that a request for
// 4.6/4.7 was silently served by 4.5 — without flooding the log on a
// busy endpoint.
func lookupWithFallback(discovered discoveredLookup, short string) (string, bool) {
	if discovered == nil {
		return "", false
	}
	if id, ok := discovered(short); ok {
		return id, true
	}
	for _, cand := range degradeCandidates(short) {
		if id, ok := discovered(cand); ok {
			key := short + "->" + cand
			degradeLog.Lock()
			if degradeLog.seen == nil {
				degradeLog.seen = map[string]struct{}{}
			}
			_, already := degradeLog.seen[key]
			if !already {
				degradeLog.seen[key] = struct{}{}
			}
			degradeLog.Unlock()
			if !already {
				log.Printf("bedrock: no inference profile for %q, falling back to %q (%s)", short, cand, id)
			}
			return id, true
		}
	}
	return "", false
}

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
	if id, ok := lookupWithFallback(discovered, model); ok {
		// Discovery returns the exact invoke-ready ID: inference
		// profiles already carry their region prefix, foundation
		// models must not receive one (a region-prefixed foundation
		// model ID like "us.anthropic.claude-sonnet-4-6" is not a
		// valid Bedrock target).
		return id, true
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
				if id, hit := lookupWithFallback(discovered, short); hit {
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
