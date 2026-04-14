package bedrock

import (
	"fmt"
	"math"
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
//   ACCEPTED: cache_control (system/tools/messages), thinking(enabled), tools, tool_use
//   REJECTED: stream, output_config, context_management, reasoning, betas,
//             thinking(adaptive), tools[].custom, cache_control.scope
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
