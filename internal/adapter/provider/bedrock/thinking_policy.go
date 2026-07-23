package bedrock

import (
	"regexp"

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
// enabled). What's new is the adaptive-only tier. AdaptThinkingForModel
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

// AdaptThinkingForModel rewrites a classic extended-thinking config into
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
func AdaptThinkingForModel(body []byte, shortName string) []byte {
	if !requiresAdaptiveThinking(shortName) {
		return body
	}

	// Adaptive-thinking-only models (e.g. Opus 4.7) treat *every* request
	// as a thinking request, even when the caller didn't set thinking.type.
	// Bedrock therefore rejects temperature / top_p / top_k unconditionally
	// on these SKUs. SanitizeForBedrockCompat already strips sampling params
	// when thinking is enabled in the body, but it has no way to know about
	// always-on adaptive — so we re-strip here, with model context.
	body = StripSamplingParams(body)

	return RewriteClassicThinkingToAdaptive(body)
}

// RewriteClassicThinkingToAdaptive converts Claude's classic extended-thinking
// shape into the adaptive shape used by newer Bedrock Claude SKUs. It does not
// require model context, so the error-driven retry path can use it when AWS
// tells us at runtime that a model is adaptive-only.
func RewriteClassicThinkingToAdaptive(body []byte) []byte {
	thinkingType := gjson.GetBytes(body, "thinking.type").String()
	if thinkingType == "" || thinkingType == "adaptive" || thinkingType == "disabled" {
		return body
	}

	effort := effortForThinkingBudget(gjson.GetBytes(body, "thinking.budget_tokens").Int())

	// Replace the thinking block with exactly {type: adaptive}. budget_tokens
	// and any other classic-only thinking fields are not valid under adaptive
	// and would be rejected; the effort signal moves to output_config.effort.
	body, _ = sjson.SetRawBytes(body, "thinking", []byte(`{"type":"adaptive"}`))

	// Preserve any effort the client already set on output_config; only
	// fill it in when absent, so a caller who knows what they want wins.
	if !gjson.GetBytes(body, "output_config.effort").Exists() {
		body, _ = sjson.SetBytes(body, "output_config.effort", effort)
	}
	return body
}

func effortForThinkingBudget(budget int64) string {
	switch {
	case budget >= 32000:
		return "high"
	case budget >= 8000:
		return "medium"
	default:
		return "low"
	}
}

var classicThinkingRejectedPattern = regexp.MustCompile(
	`(?i)(?:"?thinking\.type\.enabled"?|thinking\.type\s*=\s*"?enabled"?)` +
		`[^\n]{0,200}\b(?:not\s+supported|unsupported|is\s+not\s+allowed|requires?\s+adaptive)\b` +
		`|` +
		`\buse\b[^\n]{0,120}"?thinking\.type\.adaptive"?[^\n]{0,120}\boutput_config\.effort\b`,
)

// IsClassicThinkingRejectedError reports whether Bedrock rejected the classic
// thinking.type="enabled" shape and asked for adaptive thinking instead.
func IsClassicThinkingRejectedError(body []byte) bool {
	return classicThinkingRejectedPattern.Match(body)
}
