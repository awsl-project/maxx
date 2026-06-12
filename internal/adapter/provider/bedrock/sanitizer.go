package bedrock

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// sanitizeRequestBody prepares the request body for direct Bedrock invocation:
// - Removes `model` (Bedrock takes model in URL, not body)
// - Removes `stream` (Bedrock streams via the *Stream API verb)
// - Sets `anthropic_version`
// - Then runs the relay-safe transformations via SanitizeForBedrockCompat.
//
// Bedrock feature support (verified via real API tests):
//
//	ACCEPTED: cache_control (system/tools/messages), thinking(enabled), tools, tool_use
//	REJECTED: stream, output_config, context_management, reasoning, betas,
//	          thinking(adaptive), tools[].custom, cache_control.scope
func sanitizeRequestBody(body []byte) []byte {
	// Remove `model` field (Bedrock uses URL path)
	body, _ = sjson.DeleteBytes(body, "model")

	// Set anthropic_version
	body, _ = sjson.SetBytes(body, "anthropic_version", BedrockAPIVersion)

	// Remove `stream` (only valid against direct Bedrock; relay still wants it)
	if gjson.GetBytes(body, "stream").Exists() {
		body, _ = sjson.DeleteBytes(body, "stream")
	}

	body = SanitizeForBedrockCompat(body)
	body = RemoveOrphanedToolResults(body)
	return body
}

// SanitizeForBedrockCompat applies the subset of Bedrock-compatibility
// transformations that are safe to run before forwarding through a relay
// station whose backend is AWS Bedrock. It deliberately does NOT remove
// `model` or `stream`, since the relay still needs those for routing.
//
// Used both by the direct Bedrock adapter and by the custom adapter's
// "bedrock" disguise mode.
func SanitizeForBedrockCompat(body []byte) []byte {
	// Remove unsupported top-level fields
	for _, field := range []string{"output_config", "context_management", "reasoning", "betas"} {
		if gjson.GetBytes(body, field).Exists() {
			body, _ = sjson.DeleteBytes(body, field)
		}
	}

	// Fix thinking config: Bedrock only supports "enabled"/"disabled", not "adaptive"
	thinkingType := gjson.GetBytes(body, "thinking.type").String()
	if thinkingType == "adaptive" {
		body, _ = sjson.SetBytes(body, "thinking.type", "enabled")
		// Ensure budget_tokens is set
		if !gjson.GetBytes(body, "thinking.budget_tokens").Exists() {
			maxTokens := gjson.GetBytes(body, "max_tokens").Int()
			if maxTokens > 1024 {
				body, _ = sjson.SetBytes(body, "thinking.budget_tokens", maxTokens-1)
			} else {
				body, _ = sjson.SetBytes(body, "thinking.budget_tokens", 1024)
			}
		}
	}

	body = EnsureMaxTokensAboveThinkingBudget(body)

	// Anthropic rule (enforced by Bedrock): when extended thinking is on,
	// `temperature` must be 1 and `top_p` / `top_k` are not allowed. Rather
	// than try to clamp temperature, strip all three — the model picks
	// sensible defaults and the caller's intent of "no sampling override"
	// is preserved. The model-specific always-on adaptive case (Opus 4.7
	// without an explicit thinking block) is handled later by
	// AdaptThinkingForModel, which calls StripSamplingParams again.
	switch gjson.GetBytes(body, "thinking.type").String() {
	case "enabled", "adaptive":
		body = StripSamplingParams(body)
	}

	// cache_control is SUPPORTED by Bedrock, but the "scope" sub-field is NOT.
	// Claude Code CLI sends cache_control like {"type":"ephemeral","scope":"turn"}
	// Bedrock only accepts {"type":"ephemeral"}, so strip just the scope field.
	body = stripCacheControlScope(body)

	// Strip tools[].custom (Bedrock rejects custom tool fields)
	tools := gjson.GetBytes(body, "tools")
	if tools.IsArray() {
		for i := int(tools.Get("#").Int()) - 1; i >= 0; i-- {
			path := fmt.Sprintf("tools.%d.custom", i)
			if gjson.GetBytes(body, path).Exists() {
				body, _ = sjson.DeleteBytes(body, path)
			}
		}
	}

	body = NormalizeToolIdentifiers(body)

	// Remove empty top-level arrays that Bedrock rejects with ValidationException.
	// Clients sometimes send `"system":[]` or `"tools":[]` which Anthropic API
	// tolerates but Bedrock does not.
	for _, field := range []string{"system", "tools"} {
		v := gjson.GetBytes(body, field)
		if v.IsArray() && v.Get("#").Int() == 0 {
			body, _ = sjson.DeleteBytes(body, field)
		}
	}

	return body
}

// EnsureMaxTokensAboveThinkingBudget enforces Bedrock's invariant that
// `max_tokens > thinking.budget_tokens` whenever extended thinking is enabled.
// If the caller's max_tokens is unset or zero we leave it alone (treating that
// as "caller didn't pin a ceiling"); otherwise we raise it to budget+1 when
// it's too low.
//
// Exposed publicly so the custom adapter's bedrock disguise mode can re-run
// this check after later body-processing steps (e.g. ensureMinThinkingBudget)
// raise the budget above an originally-acceptable max_tokens.
func EnsureMaxTokensAboveThinkingBudget(body []byte) []byte {
	if gjson.GetBytes(body, "thinking.type").String() != "enabled" {
		return body
	}
	budgetTokens := gjson.GetBytes(body, "thinking.budget_tokens").Int()
	maxTokens := gjson.GetBytes(body, "max_tokens").Int()
	if maxTokens > 0 && budgetTokens >= maxTokens {
		// Guard against the pathological case where budget_tokens is so large
		// that budget+1 would overflow to a negative number. In that case the
		// request is going to be rejected by Bedrock anyway; just clamp to
		// MaxInt64 instead of corrupting the field with a negative value.
		newMax := budgetTokens + 1
		if budgetTokens == math.MaxInt64 {
			newMax = math.MaxInt64
		}
		body, _ = sjson.SetBytes(body, "max_tokens", newMax)
	}
	return body
}

// samplingParamFields lists the sampling-control fields that Anthropic's
// extended-thinking mode rejects. Kept as a package-level slice so the
// error-driven retry path can iterate the same set without drift.
var samplingParamFields = []string{"temperature", "top_p", "top_k"}

// StripSamplingParams removes `temperature`, `top_p`, and `top_k` from the
// request body. Exported for reuse by AdaptThinkingForModel (model-specific
// always-on adaptive) and the adapter's error-driven retry path.
func StripSamplingParams(body []byte) []byte {
	for _, field := range samplingParamFields {
		if gjson.GetBytes(body, field).Exists() {
			body, _ = sjson.DeleteBytes(body, field)
		}
	}
	return body
}

// samplingParamRejectedPattern matches Bedrock 400 validation messages that
// reject `temperature` / `top_p` / `top_k` because the target model is in
// (possibly always-on) extended-thinking mode. Wording shifts across
// releases:
//
//  1. Thinking-anchored: "temperature may only be set to 1 when thinking
//     is enabled" / "with extended thinking" / etc. The Anthropic-style
//     format wraps the field in backticks while the Bedrock-native format
//     drops them; the field may appear before or after the thinking clause.
//  2. Deprecation-style (observed in production starting 2026-05-12 for
//     `claude-opus-4-7`): "`temperature` is deprecated for this model."
//     No mention of "thinking" at all. The same body mutation (strip
//     sampling params and replay) is the right recovery.
//
// The thinking-anchored alternation deliberately rejects bare
// co-occurrence — a message like "temperature must be between 0 and 1;
// thinking budget exceeds max_tokens" mentions both words but is not a
// thinking-mode rejection. The deprecation alternation has no such
// ambiguity: a 400 saying a sampling param "is deprecated for this model"
// is exactly the case we want to recover from.
var samplingParamRejectedPattern = regexp.MustCompile(
	// (1a) field-then-thinking
	`\b(?:temperature|top_p|top_k)\b[^\n]{0,200}` +
		`(?:\b(?:when|with|during|while|in)\s+(?:extended\s+|adaptive\s+)?thinking\b|\bthinking\s+(?:is\s+(?:enabled|active|on)|mode)\b)` +
		`|` +
		// (1b) thinking-then-field
		`(?:\b(?:when|with|during|while|in)\s+(?:extended\s+|adaptive\s+)?thinking\b|\bthinking\s+(?:is\s+(?:enabled|active|on)|mode)\b)` +
		`[^\n]{0,200}\b(?:temperature|top_p|top_k)\b` +
		`|` +
		// (2) deprecation wording (no thinking phrase). The 200-char window
		// matches the thinking-anchored branch — "deprecated" near a
		// sampling field name is itself a strong anchor, so we tolerate
		// longer prose like "`temperature` is deprecated for this model
		// when X; use default behavior instead." A spurious match would
		// just waste a single replay (the stripped body either succeeds
		// or fails the same way), so the failure mode is self-correcting.
		`\b(?:temperature|top_p|top_k)\b[^\n]{0,200}\b(?:is\s+)?deprecated\b` +
		`|` +
		`\bdeprecated\b[^\n]{0,200}\b(?:temperature|top_p|top_k)\b`,
)

// IsSamplingParamRejectedError reports whether the upstream error body is
// a Bedrock rejection of temperature / top_p / top_k that can be recovered
// from by stripping those fields and replaying once.
func IsSamplingParamRejectedError(body []byte) bool {
	return samplingParamRejectedPattern.Match(body)
}

// RemoveOrphanedToolResults removes tool_result blocks from user messages
// whose tool_use_id does not match any tool_use block in the immediately
// preceding assistant message. The Anthropic API (and Bedrock) rejects such
// requests with: "unexpected `tool_use_id` found in `tool_result` blocks".
//
// Exported so both the direct Bedrock adapter and the custom adapter can
// call it before sending requests upstream.
func RemoveOrphanedToolResults(body []byte) []byte {
	messages := gjson.GetBytes(body, "messages")
	if !messages.Exists() || !messages.IsArray() {
		return body
	}

	msgArr := messages.Array()
	if len(msgArr) < 2 {
		return body
	}

	rebuilt := make([]string, 0, len(msgArr))
	modified := false

	for i := 0; i < len(msgArr); i++ {
		msg := msgArr[i]

		// Only process user messages that follow an assistant message.
		if msg.Get("role").String() != "user" || i == 0 {
			rebuilt = append(rebuilt, msg.Raw)
			continue
		}

		prevMsg := msgArr[i-1]
		if prevMsg.Get("role").String() != "assistant" {
			rebuilt = append(rebuilt, msg.Raw)
			continue
		}

		content := msg.Get("content")
		if !content.IsArray() {
			rebuilt = append(rebuilt, msg.Raw)
			continue
		}

		// Collect tool_use IDs from the preceding assistant message.
		validIDs := make(map[string]bool)
		prevContent := prevMsg.Get("content")
		if prevContent.IsArray() {
			prevContent.ForEach(func(_, block gjson.Result) bool {
				if block.Get("type").String() == "tool_use" {
					if id := block.Get("id").String(); id != "" {
						validIDs[id] = true
					}
				}
				return true
			})
		}

		// Filter content blocks: keep non-tool_result blocks and
		// tool_result blocks whose tool_use_id is valid.
		// When validIDs is empty (assistant had no tool_use), ALL tool_results
		// in this user message are orphaned and will be removed.
		var kept []string
		removed := false
		content.ForEach(func(_, block gjson.Result) bool {
			if block.Get("type").String() == "tool_result" {
				id := block.Get("tool_use_id").String()
				if !validIDs[id] {
					removed = true
					return true // skip this block
				}
			}
			kept = append(kept, block.Raw)
			return true
		})

		if !removed {
			rebuilt = append(rebuilt, msg.Raw)
			continue
		}

		modified = true
		if len(kept) == 0 {
			// All content was orphaned tool_results; replace with a minimal text block
			// so the message isn't empty (Anthropic rejects empty content arrays).
			kept = append(kept, `{"type":"text","text":"[empty]"}`)
		}
		if updatedMsg, err := sjson.SetRaw(msg.Raw, "content", "["+strings.Join(kept, ",")+"]"); err == nil {
			rebuilt = append(rebuilt, updatedMsg)
		} else {
			rebuilt = append(rebuilt, msg.Raw)
		}
	}

	if !modified {
		return body
	}

	updatedBody, err := sjson.SetRawBytes(body, "messages", []byte("["+strings.Join(rebuilt, ",")+"]"))
	if err != nil {
		return body
	}
	return updatedBody
}

// toolIdentifierInvalidChar matches any character disallowed in Bedrock's
// tool identifier pattern. Bedrock validates tool_use.id, tool_result.tool_use_id,
// tool_use.name, and tools[].name against `^[a-zA-Z0-9_-]+$` (name additionally
// capped at 128 chars), and rejects requests with characters like `.` or `:`.
// Some clients/SDKs synthesize ids like "functions.foo:0" which fail this regex.
var toolIdentifierInvalidChar = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// newToolIdentifierResolver returns a resolver closure that maps an original
// identifier to a Bedrock-valid form, disambiguating collisions across a
// single request. maxLen > 0 enforces a length cap; suffix allocation
// re-truncates the base so the final candidate (base + "_<n>") stays
// within the cap.
func newToolIdentifierResolver(maxLen int) func(string) string {
	mapping := make(map[string]string)
	used := make(map[string]bool)
	return func(original string) string {
		if original == "" {
			return original
		}
		if mapped, ok := mapping[original]; ok {
			return mapped
		}
		base := toolIdentifierInvalidChar.ReplaceAllString(original, "_")
		if maxLen > 0 && len(base) > maxLen {
			base = base[:maxLen]
		}
		candidate := base
		for n := 1; used[candidate]; n++ {
			suffix := fmt.Sprintf("_%d", n)
			trunc := base
			if maxLen > 0 && len(trunc)+len(suffix) > maxLen {
				trunc = base[:maxLen-len(suffix)]
			}
			candidate = trunc + suffix
		}
		mapping[original] = candidate
		used[candidate] = true
		return candidate
	}
}

// NormalizeToolIdentifiers replaces characters that Bedrock rejects in tool
// identifier fields. Covers:
//   - messages[*].content[*].id (tool_use)
//   - messages[*].content[*].name (tool_use)
//   - messages[*].content[*].tool_use_id (tool_result)
//   - tools[*].name
//
// Reference pairs stay consistent because we build request-wide
// original→normalized maps and apply them on both ends:
//   - id map links tool_use.id ↔ tool_result.tool_use_id
//   - name map links tools[].name (definition) ↔ tool_use.name (reference)
//
// Collisions get a stable `_<n>` suffix so two distinct originals (e.g.
// `functions.foo` and `functions/foo`, both collapsing to `functions_foo`)
// stay distinguishable on the wire. Names are capped at Bedrock's 128-char
// limit, and suffix allocation truncates the base if needed to keep the
// total within that cap.
func NormalizeToolIdentifiers(body []byte) []byte {
	resolveID := newToolIdentifierResolver(0)
	resolveName := newToolIdentifierResolver(128)

	// First pass (forward order, definitions before references): build maps.
	// tools[] is walked before messages so tool definitions own canonical
	// names; tool_use.name references then resolve through the same map.
	tools := gjson.GetBytes(body, "tools")
	if tools.IsArray() {
		tn := int(tools.Get("#").Int())
		for i := 0; i < tn; i++ {
			path := fmt.Sprintf("tools.%d.name", i)
			if name := gjson.GetBytes(body, path).String(); name != "" {
				_ = resolveName(name)
			}
		}
	}
	messages := gjson.GetBytes(body, "messages")
	if messages.IsArray() {
		n := int(messages.Get("#").Int())
		for i := 0; i < n; i++ {
			content := gjson.GetBytes(body, fmt.Sprintf("messages.%d.content", i))
			if !content.IsArray() {
				continue
			}
			cn := int(content.Get("#").Int())
			for j := 0; j < cn; j++ {
				blockPath := fmt.Sprintf("messages.%d.content.%d", i, j)
				switch gjson.GetBytes(body, blockPath+".type").String() {
				case "tool_use":
					if id := gjson.GetBytes(body, blockPath+".id").String(); id != "" {
						_ = resolveID(id)
					}
					if name := gjson.GetBytes(body, blockPath+".name").String(); name != "" {
						_ = resolveName(name)
					}
				case "tool_result":
					if id := gjson.GetBytes(body, blockPath+".tool_use_id").String(); id != "" {
						_ = resolveID(id)
					}
				}
			}
		}
	}

	// Second pass: apply mappings. Order doesn't matter for correctness
	// (only scalar fields are rewritten), but reverse iteration keeps the
	// pattern consistent with other sanitizer helpers in this file.
	if messages.IsArray() {
		for i := int(messages.Get("#").Int()) - 1; i >= 0; i-- {
			content := gjson.GetBytes(body, fmt.Sprintf("messages.%d.content", i))
			if !content.IsArray() {
				continue
			}
			for j := int(content.Get("#").Int()) - 1; j >= 0; j-- {
				blockPath := fmt.Sprintf("messages.%d.content.%d", i, j)
				switch gjson.GetBytes(body, blockPath+".type").String() {
				case "tool_use":
					if id := gjson.GetBytes(body, blockPath+".id").String(); id != "" {
						if mapped := resolveID(id); mapped != id {
							body, _ = sjson.SetBytes(body, blockPath+".id", mapped)
						}
					}
					if name := gjson.GetBytes(body, blockPath+".name").String(); name != "" {
						if mapped := resolveName(name); mapped != name {
							body, _ = sjson.SetBytes(body, blockPath+".name", mapped)
						}
					}
				case "tool_result":
					if id := gjson.GetBytes(body, blockPath+".tool_use_id").String(); id != "" {
						if mapped := resolveID(id); mapped != id {
							body, _ = sjson.SetBytes(body, blockPath+".tool_use_id", mapped)
						}
					}
				}
			}
		}
	}

	if tools.IsArray() {
		for i := int(tools.Get("#").Int()) - 1; i >= 0; i-- {
			path := fmt.Sprintf("tools.%d.name", i)
			if name := gjson.GetBytes(body, path).String(); name != "" {
				if mapped := resolveName(name); mapped != name {
					body, _ = sjson.SetBytes(body, path, mapped)
				}
			}
		}
	}

	return body
}

// stripCacheControlScope removes the "scope" sub-field from all cache_control objects.
// Bedrock accepts cache_control.type but rejects cache_control.scope (and any other extra sub-fields).
func stripCacheControlScope(payload []byte) []byte {
	// Strip from system array
	system := gjson.GetBytes(payload, "system")
	if system.IsArray() {
		for i := int(system.Get("#").Int()) - 1; i >= 0; i-- {
			path := fmt.Sprintf("system.%d.cache_control.scope", i)
			if gjson.GetBytes(payload, path).Exists() {
				payload, _ = sjson.DeleteBytes(payload, path)
			}
		}
	}

	// Strip from tools array
	tools := gjson.GetBytes(payload, "tools")
	if tools.IsArray() {
		for i := int(tools.Get("#").Int()) - 1; i >= 0; i-- {
			path := fmt.Sprintf("tools.%d.cache_control.scope", i)
			if gjson.GetBytes(payload, path).Exists() {
				payload, _ = sjson.DeleteBytes(payload, path)
			}
		}
	}

	// Strip from messages content blocks
	messages := gjson.GetBytes(payload, "messages")
	if messages.IsArray() {
		for i := int(messages.Get("#").Int()) - 1; i >= 0; i-- {
			content := gjson.GetBytes(payload, fmt.Sprintf("messages.%d.content", i))
			if content.IsArray() {
				for j := int(content.Get("#").Int()) - 1; j >= 0; j-- {
					path := fmt.Sprintf("messages.%d.content.%d.cache_control.scope", i, j)
					if gjson.GetBytes(payload, path).Exists() {
						payload, _ = sjson.DeleteBytes(payload, path)
					}
				}
			}
		}
	}

	return payload
}
