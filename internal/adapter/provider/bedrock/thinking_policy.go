package bedrock

import (
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Claude's extended-thinking config has evolved through two incompatible
// shapes:
//
//   classic  — thinking.type="enabled" + thinking.budget_tokens=N
//   adaptive — thinking.type="adaptive" + output_config.effort="low|medium|high"
//
// Models split into three camps:
//
//   adaptive-only   — Opus 4.7: rejects thinking.type="enabled" with
//                     "\"thinking.type.enabled\" is not supported"
//   classic-only    — Sonnet/Opus 4.x pre-4.6, 3.7 and older: reject adaptive
//   either         — Opus 4.6, Sonnet 4.6: accept both (adaptive recommended)
//
// The sanitizer already handles classic-only (it rewrites adaptive →
// enabled). What's new is the adaptive-only tier. adaptThinkingForModel
// performs the inverse rewrite when the resolved short name requires it,
// so clients that only speak the classic shape (Claude Code CLI on
// Bedrock) can still hit Opus 4.7.

// adaptiveOnlyModels is the set of short names that reject classic
// extended-thinking. Kept as a small hand-maintained list because
// neither Bedrock's ListFoundationModels nor Anthropic's /v1/models
// exposes the capability flag — there's no authoritative source to
// derive it from.
var adaptiveOnlyModels = map[string]bool{
	"claude-opus-4-7": true,
}

// requiresAdaptiveThinking reports whether the model identified by
// shortName refuses the classic thinking.type="enabled" shape.
func requiresAdaptiveThinking(shortName string) bool {
	return adaptiveOnlyModels[shortName]
}

// adaptThinkingForModel rewrites a classic extended-thinking config into
// adaptive form when the target model demands it. Idempotent: if the
// payload already uses adaptive, or the model doesn't require it, or
// there's no thinking config at all, the body is returned unchanged.
//
// Budget → effort translation is conservative:
//   - budget_tokens >= 32k → "high"
//   - budget_tokens >= 8k  → "medium"
//   - otherwise            → "low"
//
// We don't bother trying to back out of the payload exactly how much
// thinking the client asked for. Adaptive's whole point is that Claude
// decides dynamically; we just need to land in the same ballpark so
// clients that crank budget_tokens up don't silently get capped to a
// tiny thinking allotment.
func adaptThinkingForModel(body []byte, shortName string) []byte {
	if !requiresAdaptiveThinking(shortName) {
		return body
	}
	thinkingType := gjson.GetBytes(body, "thinking.type").String()
	if thinkingType == "" || thinkingType == "adaptive" || thinkingType == "disabled" {
		return body
	}

	budget := gjson.GetBytes(body, "thinking.budget_tokens").Int()
	effort := "low"
	switch {
	case budget >= 32000:
		effort = "high"
	case budget >= 8000:
		effort = "medium"
	}

	// Replace the thinking block with just {type: adaptive}. budget_tokens
	// is not valid under adaptive and would be rejected; the effort
	// signal moves to output_config.effort.
	body, _ = sjson.SetBytes(body, "thinking.type", "adaptive")
	body, _ = sjson.DeleteBytes(body, "thinking.budget_tokens")

	// Preserve any effort the client already set on output_config; only
	// fill it in when absent, so a caller who knows what they want wins.
	if !gjson.GetBytes(body, "output_config.effort").Exists() {
		body, _ = sjson.SetBytes(body, "output_config.effort", effort)
	}
	return body
}
