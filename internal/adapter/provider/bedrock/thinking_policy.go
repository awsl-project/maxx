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
//   adaptive-first  — newer SKUs reject thinking.type="enabled" and ask for
//                     adaptive + output_config.effort
//   classic-only    — older SKUs reject adaptive / output_config.effort
//   either          — accept both (adaptive recommended)
//
// Default normalization is adaptive-first. Runtime validation errors can
// still rewrite back to classic for older Bedrock schemas.

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

// AdaptThinkingForModel applies model-specific adjustments after the generic
// adaptive-first sanitizer. Kept for adaptive-only models that reject sampling
// params even when no explicit thinking block is present.
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

// RewriteAdaptiveThinkingToClassic converts adaptive thinking into the classic
// Bedrock shape for older models that reject output_config.effort/adaptive.
func RewriteAdaptiveThinkingToClassic(body []byte) []byte {
	if gjson.GetBytes(body, "thinking.type").String() != "adaptive" {
		return body
	}

	effort := gjson.GetBytes(body, "output_config.effort").String()
	budget := budgetForEffort(effort)
	body, _ = sjson.SetBytes(body, "thinking.type", "enabled")
	body, _ = sjson.SetBytes(body, "thinking.budget_tokens", budget)
	body, _ = sjson.DeleteBytes(body, "output_config")
	body = EnsureMaxTokensAboveThinkingBudget(body)
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

func budgetForEffort(effort string) int64 {
	switch effort {
	case "max":
		return 64000
	case "high":
		return 32000
	case "medium":
		return 8192
	default:
		return 1024
	}
}

var classicThinkingRejectedPattern = regexp.MustCompile(
	`(?i)(?:"?thinking\.type\.enabled"?|"?\.\.enabled"?|thinking\.type\s*=\s*"?enabled"?)` +
		`[^\n]{0,200}\b(?:not\s+supported|unsupported|is\s+not\s+allowed|requires?\s+adaptive)\b` +
		`|` +
		`\buse\b[^\n]{0,120}(?:"?thinking\.type\.adaptive"?|"?\.\.adaptive"?)[^\n]{0,120}\boutput_config\.effort\b`,
)

// IsClassicThinkingRejectedError reports whether Bedrock rejected the classic
// thinking.type="enabled" shape and asked for adaptive thinking instead.
func IsClassicThinkingRejectedError(body []byte) bool {
	return classicThinkingRejectedPattern.Match(body)
}

var adaptiveThinkingRejectedPattern = regexp.MustCompile(
	`(?i)(?:output_config\.effort|thinking\.type\.adaptive|"?\.\.adaptive"?)` +
		`[^\n]{0,200}\b(?:extra\s+inputs?\s+are\s+not\s+permitted|not\s+supported|unsupported|is\s+not\s+allowed)\b` +
		`|` +
		`\buse\b[^\n]{0,120}(?:"?thinking\.type\.enabled"?|"?\.\.enabled"?)[^\n]{0,120}\b(?:budget_tokens|thinking\.budget_tokens)\b`,
)

// IsAdaptiveThinkingRejectedError reports whether Bedrock rejected the
// adaptive thinking shape and needs a retry with classic enabled/budget_tokens.
func IsAdaptiveThinkingRejectedError(body []byte) bool {
	return adaptiveThinkingRejectedPattern.Match(body)
}
