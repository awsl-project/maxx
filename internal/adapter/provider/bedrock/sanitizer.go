package bedrock

import (
	"fmt"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// sanitizeRequestBody prepares the request body for Bedrock:
// - Removes fields not supported by Bedrock
// - Removes `model` (Bedrock takes model in URL, not body)
// - Sets `anthropic_version`
// - Converts adaptive thinking to enabled
//
// Bedrock feature support (verified via real API tests):
//   ACCEPTED: cache_control (system/tools/messages), thinking(enabled), tools, tool_use
//   REJECTED: stream, output_config, context_management, reasoning, betas,
//             thinking(adaptive), tools[].custom
func sanitizeRequestBody(body []byte) []byte {
	// Remove `model` field (Bedrock uses URL path)
	body, _ = sjson.DeleteBytes(body, "model")

	// Set anthropic_version
	body, _ = sjson.SetBytes(body, "anthropic_version", BedrockAPIVersion)

	// Remove unsupported top-level fields
	for _, field := range []string{"stream", "output_config", "context_management", "reasoning", "betas"} {
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

	// Bedrock requires max_tokens > thinking.budget_tokens
	if gjson.GetBytes(body, "thinking.type").String() == "enabled" {
		budgetTokens := gjson.GetBytes(body, "thinking.budget_tokens").Int()
		maxTokens := gjson.GetBytes(body, "max_tokens").Int()
		if maxTokens > 0 && budgetTokens >= maxTokens {
			body, _ = sjson.SetBytes(body, "max_tokens", budgetTokens+1)
		}
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
