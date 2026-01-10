package antigravity

import (
	"encoding/json"
	"log"
	"strings"
)

// buildContents converts Claude messages to Gemini contents
// Reference: Antigravity-Manager's build_contents
func buildContents(
	messages []ClaudeMessage,
	mappedModel string,
	sessionID string,
	signatureCache *SignatureCache,
) ([]map[string]interface{}, error) {
	contents := []map[string]interface{}{}

	for _, msg := range messages {
		parts := []map[string]interface{}{}

		// Parse content blocks
		contentBlocks := parseContentBlocks(msg.Content)

		// Context variables for signature passing
		var lastThoughtSignature *string
		toolIDToName := make(map[string]string)

		for _, block := range contentBlocks {
			switch block.Type {
			case "thinking":
				part := processThinkingBlock(block, &parts, mappedModel, lastThoughtSignature, signatureCache)
				if part != nil {
					parts = append(parts, part)
					if sig, ok := part["thoughtSignature"].(string); ok {
						lastThoughtSignature = &sig
					}
				}

			case "redacted_thinking":
				// Downgrade to text with redacted marker
				// Reference: Antigravity-Manager's RedactedThinking handling
				parts = append(parts, map[string]interface{}{
					"text": "[Redacted Thinking: " + block.Data + "]",
				})

			case "text":
				parts = append(parts, map[string]interface{}{
					"text": block.Text,
				})

			case "tool_use":
				part := processToolUseBlock(block, lastThoughtSignature, signatureCache)
				parts = append(parts, part)
				toolIDToName[block.ID] = block.Name

				// Cache tool signature
				if sig, ok := part["thoughtSignature"].(string); ok && signatureCache != nil {
					signatureCache.CacheToolSignature(block.ID, sig)
				}

			case "tool_result":
				part := processToolResultBlock(block, toolIDToName, lastThoughtSignature)
				parts = append(parts, part)

			case "image":
				part := processImageBlock(block)
				parts = append(parts, part)
			}
		}

		// Build content
		role := mapRole(msg.Role)
		contents = append(contents, map[string]interface{}{
			"role":  role,
			"parts": parts,
		})
	}

	// Merge adjacent same roles
	contents = mergeAdjacentRoles(contents)

	return contents, nil
}

// processThinkingBlock handles Thinking blocks with position and compatibility checks
// Reference: Antigravity-Manager's Thinking block processing
func processThinkingBlock(
	block ContentBlock,
	parts *[]map[string]interface{},
	mappedModel string,
	lastThoughtSignature *string,
	signatureCache *SignatureCache,
) map[string]interface{} {
	// 1. Position check: must be first block
	if len(*parts) > 0 {
		log.Println("[Antigravity] Thinking block not first, downgrade to text")
		return map[string]interface{}{
			"text": block.Thinking,
		}
	}

	// 2. Empty block check
	if block.Thinking == "" {
		return map[string]interface{}{
			"text": "...",
		}
	}

	// 3. Build Thinking Part
	part := map[string]interface{}{
		"text":    block.Thinking,
		"thought": true,
	}

	// 4. Signature handling
	signature := block.Signature
	if signature == "" && lastThoughtSignature != nil {
		signature = *lastThoughtSignature
	}

	// 5. Signature compatibility check
	if signature != "" && signatureCache != nil {
		if cachedFamily := signatureCache.GetSignatureFamily(signature); cachedFamily != "" {
			if !IsModelCompatible(cachedFamily, mappedModel) {
				log.Printf("[Antigravity] Signature family %s incompatible with %s, downgrade", cachedFamily, mappedModel)
				return map[string]interface{}{
					"text": block.Thinking,
				}
			}
		}

		// Valid signature
		if HasValidSignature(signature) {
			part["thoughtSignature"] = signature
		}
	}

	return part
}

// processToolUseBlock handles ToolUse blocks with signature recovery
// Reference: Antigravity-Manager's ToolUse processing
func processToolUseBlock(
	block ContentBlock,
	lastThoughtSignature *string,
	signatureCache *SignatureCache,
) map[string]interface{} {
	part := map[string]interface{}{
		"functionCall": map[string]interface{}{
			"name": block.Name,
			"args": block.Input,
			"id":   block.ID,
		},
	}

	// Signature recovery priority (4 layers):
	// 1. Client-provided signature
	// 2. Context signature (last_thought_signature)
	// 3. Cached signature (from previous tool calls)
	// 4. Global fallback signature (from cache)
	// Reference: Antigravity-Manager's multi-layer signature recovery
	signature := block.Signature
	if signature == "" && lastThoughtSignature != nil {
		signature = *lastThoughtSignature
	}
	if signature == "" && signatureCache != nil {
		signature = signatureCache.GetToolSignature(block.ID)
	}
	if signature == "" && signatureCache != nil {
		// Final fallback: global signature
		signature = signatureCache.GetGlobalSignature()
	}

	// Strict signature validation
	// Reference: Antigravity-Manager's has_valid_signature_for_function_calls
	if signature != "" && HasValidSignature(signature) {
		part["thoughtSignature"] = signature
	} else if signature != "" && len(signature) < MinSignatureLength {
		// Log when signature is too short (防止 Gemini 拒绝)
		log.Printf("[Antigravity] Tool call %s has invalid signature (len=%d < %d), not including",
			block.ID, len(signature), MinSignatureLength)
	}

	return part
}

// processToolResultBlock handles ToolResult blocks with empty result injection
// Reference: Antigravity-Manager's ToolResult processing
func processToolResultBlock(
	block ContentBlock,
	toolIDToName map[string]string,
	lastThoughtSignature *string,
) map[string]interface{} {
	// 1. Merge content
	mergedContent := extractToolResultContent(block.Content)

	// 2. Empty result injection
	if strings.TrimSpace(mergedContent) == "" {
		if block.IsError != nil && *block.IsError {
			mergedContent = "Tool execution failed with no output."
		} else {
			mergedContent = "Command executed successfully."
		}
	}

	// 3. Get tool name
	toolName := toolIDToName[block.ToolUseID]
	if toolName == "" {
		toolName = block.ToolUseID
	}

	part := map[string]interface{}{
		"functionResponse": map[string]interface{}{
			"name": toolName,
			"response": map[string]interface{}{
				"result": mergedContent,
			},
			"id": block.ToolUseID,
		},
	}

	// Backfill signature from context
	// Reference: Antigravity-Manager's tool result signature backfill
	if lastThoughtSignature != nil && *lastThoughtSignature != "" && HasValidSignature(*lastThoughtSignature) {
		part["thoughtSignature"] = *lastThoughtSignature
	}

	return part
}

// processImageBlock handles image blocks
func processImageBlock(block ContentBlock) map[string]interface{} {
	if block.Source != nil {
		return map[string]interface{}{
			"inlineData": map[string]interface{}{
				"mimeType": block.Source.MediaType,
				"data":     block.Source.Data,
			},
		}
	}
	return map[string]interface{}{
		"text": "[Image]",
	}
}

// extractToolResultContent extracts text content from tool_result
func extractToolResultContent(content interface{}) string {
	switch c := content.(type) {
	case string:
		return c
	case []interface{}:
		var texts []string
		for _, item := range c {
			if blockMap, ok := item.(map[string]interface{}); ok {
				if text, ok := blockMap["text"].(string); ok {
					texts = append(texts, text)
				}
			}
		}
		return strings.Join(texts, "\n")
	default:
		// Try to serialize as JSON
		if data, err := json.Marshal(content); err == nil {
			return string(data)
		}
		return ""
	}
}

// mapRole converts Claude role to Gemini role
func mapRole(claudeRole string) string {
	switch claudeRole {
	case "user":
		return "user"
	case "assistant":
		return "model"
	default:
		return claudeRole
	}
}

// mergeAdjacentRoles merges adjacent contents with same role
// Gemini API strictly requires alternating user/model roles
func mergeAdjacentRoles(contents []map[string]interface{}) []map[string]interface{} {
	if len(contents) <= 1 {
		return contents
	}

	merged := []map[string]interface{}{contents[0]}

	for i := 1; i < len(contents); i++ {
		lastRole := merged[len(merged)-1]["role"].(string)
		currRole := contents[i]["role"].(string)

		if lastRole == currRole {
			// Merge parts
			lastParts, _ := merged[len(merged)-1]["parts"].([]map[string]interface{})
			currParts, _ := contents[i]["parts"].([]map[string]interface{})
			merged[len(merged)-1]["parts"] = append(lastParts, currParts...)
		} else {
			merged = append(merged, contents[i])
		}
	}

	return merged
}
