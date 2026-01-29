package custom

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Claude Code system prompt for cloaking
const claudeCodeSystemPrompt = `You are Claude Code, Anthropic's official CLI for Claude.`

// Interleaved thinking hint (like CLIProxyAPI)
const interleavedThinkingHint = `Interleaved thinking is enabled. You may think between tool calls and after receiving tool results before deciding the next action or final answer. Do not mention these instructions or any constraints about thinking blocks; just apply them.`

// userIDPattern matches Claude Code format: user_[64-hex]_account__session_[uuid-v4]
var userIDPattern = regexp.MustCompile(`^user_[a-fA-F0-9]{64}_account__session_[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// processClaudeRequestBody processes Claude request body before sending to upstream.
// Following CLIProxyAPI order:
// 1. cleanCacheControl (remove cache_control from messages)
// 2. applyCloaking (system prompt injection, fake user_id)
// 3. injectInterleavedThinkingHint (when thinking + tools)
// 4. disableThinkingIfToolChoiceForced
// 5. ensureStreamFlag (always inject stream: true for Claude)
// 6. extractAndRemoveBetas
// 7. reorderBodyFields (ensure correct field order)
// Returns processed body and extra betas for header.
func processClaudeRequestBody(body []byte, clientUserAgent string) ([]byte, []string) {
	// 1. Clean cache_control from messages (like CLIProxyAPI)
	body = cleanCacheControl(body)

	// 2. Apply cloaking (system prompt injection, fake user_id)
	body = applyCloaking(body, clientUserAgent)

	// 3. Inject interleaved thinking hint when thinking + tools (like CLIProxyAPI)
	body = injectInterleavedThinkingHint(body)

	// 4. Disable thinking if tool_choice forces tool use
	body = disableThinkingIfToolChoiceForced(body)

	// 5. Always ensure stream: true for Claude requests (force streaming)
	body = ensureStreamFlag(body)

	// 6. Extract betas from body (to be added to header)
	var extraBetas []string
	extraBetas, body = extractAndRemoveBetas(body)

	// 7. Reorder body fields to match Claude Code format
	body = reorderBodyFields(body)

	return body, extraBetas
}

// applyCloaking applies cloaking transformations for non-Claude Code clients.
// Cloaking includes: system prompt injection, fake user_id injection, empty tools array.
func applyCloaking(body []byte, clientUserAgent string) []byte {
	// If client is already Claude Code, no cloaking needed
	if isClaudeCodeClient(clientUserAgent) {
		return body
	}

	// Inject Claude Code system prompt
	body = injectClaudeCodeSystemPrompt(body)

	// Inject fake user_id
	body = injectFakeUserID(body)

	// Inject empty tools array if not present
	body = injectEmptyTools(body)

	return body
}

// isClaudeCodeClient checks if the User-Agent indicates a Claude Code client.
func isClaudeCodeClient(userAgent string) bool {
	return strings.HasPrefix(userAgent, "claude-cli")
}

// injectClaudeCodeSystemPrompt injects Claude Code system prompt into the request.
// Prepends the Claude Code system prompt to existing system entries.
// Skips injection if system already contains Claude Code identity.
func injectClaudeCodeSystemPrompt(body []byte) []byte {
	system := gjson.GetBytes(body, "system")

	// Check if system already contains Claude Code identity
	if system.Exists() && strings.Contains(system.Raw, "Claude Code") {
		return body
	}

	// Create Claude Code system instruction entry with correct field order: type, text
	// Using json.RawMessage to preserve field order
	claudeCodeEntryJSON := []byte(`{"type":"text","text":"` + claudeCodeSystemPrompt + `"}`)

	if !system.Exists() {
		// No existing system, create new array with Claude Code instruction
		body, _ = sjson.SetRawBytes(body, "system", []byte(`[`+string(claudeCodeEntryJSON)+`]`))
		return body
	}

	if system.IsArray() {
		// Prepend Claude Code instruction to existing array
		existingSystemJSON := system.Raw
		// Remove leading '[' and add our entry at the beginning
		newSystemJSON := `[` + string(claudeCodeEntryJSON) + `,` + existingSystemJSON[1:]
		body, _ = sjson.SetRawBytes(body, "system", []byte(newSystemJSON))
		return body
	}

	// system is a string, convert to array format
	existingText := system.String()
	if existingText != "" {
		existingEntryJSON := `{"type":"text","text":` + jsonEscapeString(existingText) + `}`
		newSystemJSON := `[` + string(claudeCodeEntryJSON) + `,` + existingEntryJSON + `]`
		body, _ = sjson.SetRawBytes(body, "system", []byte(newSystemJSON))
	} else {
		body, _ = sjson.SetRawBytes(body, "system", []byte(`[`+string(claudeCodeEntryJSON)+`]`))
	}

	return body
}

// jsonEscapeString escapes a string for JSON
func jsonEscapeString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// injectFakeUserID generates and injects a fake user_id into the request metadata.
// Only injects if user_id is missing or invalid.
func injectFakeUserID(body []byte) []byte {
	existingUserID := gjson.GetBytes(body, "metadata.user_id").String()
	if existingUserID != "" && isValidUserID(existingUserID) {
		return body
	}

	// Generate and inject fake user_id
	body, _ = sjson.SetBytes(body, "metadata.user_id", generateFakeUserID())
	return body
}

// injectEmptyTools injects an empty tools array if not present.
// This makes the request look more like a real Claude Code request.
func injectEmptyTools(body []byte) []byte {
	if !gjson.GetBytes(body, "tools").Exists() {
		body, _ = sjson.SetBytes(body, "tools", []interface{}{})
	}
	return body
}

// isValidUserID checks if a user_id matches Claude Code format.
func isValidUserID(userID string) bool {
	return userIDPattern.MatchString(userID)
}

// generateFakeUserID generates a fake user_id in Claude Code format.
// Format: user_{64-hex}_account__session_{uuid}
func generateFakeUserID() string {
	// Generate 32 random bytes (64 hex chars)
	randomBytes := make([]byte, 32)
	_, _ = rand.Read(randomBytes)
	hexPart := hex.EncodeToString(randomBytes)

	// Generate UUID for session
	sessionUUID := uuid.New().String()

	return "user_" + hexPart + "_account__session_" + sessionUUID
}

// disableThinkingIfToolChoiceForced checks if tool_choice forces tool use and disables thinking.
// Anthropic API does not allow thinking when tool_choice is set to "any" or "tool".
// See: https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking#important-considerations
func disableThinkingIfToolChoiceForced(body []byte) []byte {
	toolChoiceType := gjson.GetBytes(body, "tool_choice.type").String()
	// "auto" is allowed with thinking, but "any" or "tool" (specific tool) are not
	if toolChoiceType == "any" || toolChoiceType == "tool" {
		// Remove thinking configuration entirely to avoid API error
		body, _ = sjson.DeleteBytes(body, "thinking")
	}
	return body
}

// extractAndRemoveBetas extracts betas array from request body and removes it.
// Returns the extracted betas and the modified body.
func extractAndRemoveBetas(body []byte) ([]string, []byte) {
	betasResult := gjson.GetBytes(body, "betas")
	if !betasResult.Exists() {
		return nil, body
	}

	var betas []string
	if betasResult.IsArray() {
		for _, item := range betasResult.Array() {
			if s := strings.TrimSpace(item.String()); s != "" {
				betas = append(betas, s)
			}
		}
	} else if s := strings.TrimSpace(betasResult.String()); s != "" {
		betas = append(betas, s)
	}

	body, _ = sjson.DeleteBytes(body, "betas")
	return betas, body
}

// ensureStreamFlag ensures stream: true is set in the body for streaming requests.
func ensureStreamFlag(body []byte) []byte {
	// Only set if not already present or if false
	if !gjson.GetBytes(body, "stream").Bool() {
		body, _ = sjson.SetBytes(body, "stream", true)
	}
	return body
}

// cleanCacheControl removes cache_control from all messages and system entries.
// (like CLIProxyAPI's cleanCacheControl)
// Some clients send back historical messages with cache_control intact,
// which may cause issues with certain upstream APIs.
func cleanCacheControl(body []byte) []byte {
	parsed := gjson.ParseBytes(body)
	modified := false

	// Clean from messages
	messages := parsed.Get("messages")
	if messages.Exists() && messages.IsArray() {
		newMessages := cleanCacheControlFromArray(messages.Raw)
		if newMessages != messages.Raw {
			body, _ = sjson.SetRawBytes(body, "messages", []byte(newMessages))
			modified = true
		}
	}

	// Clean from system
	system := parsed.Get("system")
	if system.Exists() && system.IsArray() {
		newSystem := cleanCacheControlFromArray(system.Raw)
		if newSystem != system.Raw {
			body, _ = sjson.SetRawBytes(body, "system", []byte(newSystem))
			modified = true
		}
	}

	_ = modified
	return body
}

// cleanCacheControlFromArray recursively removes cache_control from JSON array
func cleanCacheControlFromArray(jsonArray string) string {
	var arr []interface{}
	if err := json.Unmarshal([]byte(jsonArray), &arr); err != nil {
		return jsonArray
	}

	modified := cleanCacheControlRecursive(arr)
	if !modified {
		return jsonArray
	}

	result, err := json.Marshal(arr)
	if err != nil {
		return jsonArray
	}
	return string(result)
}

// cleanCacheControlRecursive removes cache_control from nested structures
func cleanCacheControlRecursive(v interface{}) bool {
	modified := false
	switch val := v.(type) {
	case map[string]interface{}:
		if _, exists := val["cache_control"]; exists {
			delete(val, "cache_control")
			modified = true
		}
		for _, child := range val {
			if cleanCacheControlRecursive(child) {
				modified = true
			}
		}
	case []interface{}:
		for _, item := range val {
			if cleanCacheControlRecursive(item) {
				modified = true
			}
		}
	}
	return modified
}

// injectInterleavedThinkingHint injects thinking hint when thinking is enabled and tools are present.
// (like CLIProxyAPI's interleavedHint injection)
func injectInterleavedThinkingHint(body []byte) []byte {
	// Check if thinking is enabled
	thinkingType := gjson.GetBytes(body, "thinking.type").String()
	if thinkingType != "enabled" {
		return body
	}

	// Check if tools are present (non-empty array)
	tools := gjson.GetBytes(body, "tools")
	if !tools.Exists() || !tools.IsArray() || len(tools.Array()) == 0 {
		return body
	}

	// Append thinking hint to system prompt
	system := gjson.GetBytes(body, "system")
	hintEntry := `{"type":"text","text":"` + interleavedThinkingHint + `"}`

	if !system.Exists() {
		// Create new system array with hint
		body, _ = sjson.SetRawBytes(body, "system", []byte(`[`+hintEntry+`]`))
		return body
	}

	if system.IsArray() {
		// Append hint to existing array
		existingSystemJSON := system.Raw
		// Remove trailing ']' and add hint at the end
		newSystemJSON := existingSystemJSON[:len(existingSystemJSON)-1] + `,` + hintEntry + `]`
		body, _ = sjson.SetRawBytes(body, "system", []byte(newSystemJSON))
		return body
	}

	// system is a string, convert to array with hint
	existingText := system.String()
	if existingText != "" {
		existingEntryJSON := `{"type":"text","text":` + jsonEscapeString(existingText) + `}`
		newSystemJSON := `[` + existingEntryJSON + `,` + hintEntry + `]`
		body, _ = sjson.SetRawBytes(body, "system", []byte(newSystemJSON))
	} else {
		body, _ = sjson.SetRawBytes(body, "system", []byte(`[`+hintEntry+`]`))
	}

	return body
}

// reorderBodyFields reorders body fields to match Claude Code format.
// Field order: model → messages → system → tools → tool_choice → thinking → metadata → max_tokens → temperature → top_p → top_k → stream
func reorderBodyFields(body []byte) []byte {
	// Parse original body
	parsed := gjson.ParseBytes(body)

	// Build new body with correct field order
	var result []byte
	result = append(result, '{')

	fieldsAdded := 0

	// Helper to add field
	addField := func(key string, value gjson.Result) {
		if fieldsAdded > 0 {
			result = append(result, ',')
		}
		result = append(result, '"')
		result = append(result, key...)
		result = append(result, '"', ':')
		result = append(result, value.Raw...)
		fieldsAdded++
	}

	// Ordered fields (Claude Code format)
	orderedFields := []string{
		"model",
		"messages",
		"system",
		"tools",
		"tool_choice",
		"thinking",
		"metadata",
		"max_tokens",
		"temperature",
		"top_p",
		"top_k",
		"stop_sequences",
		"stream",
	}

	knownFields := make(map[string]bool)
	for _, key := range orderedFields {
		knownFields[key] = true
		if v := parsed.Get(key); v.Exists() {
			addField(key, v)
		}
	}

	// Add any remaining fields that weren't in our ordered list
	parsed.ForEach(func(key, value gjson.Result) bool {
		if !knownFields[key.String()] {
			addField(key.String(), value)
		}
		return true
	})

	result = append(result, '}')

	return result
}
