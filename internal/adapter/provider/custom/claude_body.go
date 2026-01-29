package custom

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Claude Code system prompt for cloaking
const claudeCodeSystemPrompt = `You are Claude Code, Anthropic's official CLI for Claude.`

// userIDPattern matches Claude Code format: user_[64-hex]_account__session_[uuid-v4]
var userIDPattern = regexp.MustCompile(`^user_[a-fA-F0-9]{64}_account__session_[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// processClaudeRequestBody processes Claude request body before sending to upstream.
// Following CLIProxyAPI order:
// 1. applyCloaking (system prompt injection, fake user_id)
// 2. disableThinkingIfToolChoiceForced
// 3. extractAndRemoveBetas
// Returns processed body and extra betas for header.
func processClaudeRequestBody(body []byte, clientUserAgent string) ([]byte, []string) {
	// 1. Apply cloaking (system prompt injection, fake user_id)
	body = applyCloaking(body, clientUserAgent)

	// 2. Disable thinking if tool_choice forces tool use
	body = disableThinkingIfToolChoiceForced(body)

	// 3. Extract betas from body (to be added to header)
	var extraBetas []string
	extraBetas, body = extractAndRemoveBetas(body)

	return body, extraBetas
}

// applyCloaking applies cloaking transformations for non-Claude Code clients.
// Cloaking includes: system prompt injection, fake user_id injection.
func applyCloaking(body []byte, clientUserAgent string) []byte {
	// If client is already Claude Code, no cloaking needed
	if isClaudeCodeClient(clientUserAgent) {
		return body
	}

	// Inject Claude Code system prompt
	body = injectClaudeCodeSystemPrompt(body)

	// Inject fake user_id
	body = injectFakeUserID(body)

	return body
}

// isClaudeCodeClient checks if the User-Agent indicates a Claude Code client.
func isClaudeCodeClient(userAgent string) bool {
	return strings.HasPrefix(userAgent, "claude-cli")
}

// injectClaudeCodeSystemPrompt injects Claude Code system prompt into the request.
// Prepends to existing system messages.
func injectClaudeCodeSystemPrompt(body []byte) []byte {
	system := gjson.GetBytes(body, "system")

	// Create Claude Code system instruction entry
	claudeCodeEntry := map[string]string{
		"type": "text",
		"text": claudeCodeSystemPrompt,
	}

	if !system.Exists() {
		// No existing system, create new array with Claude Code instruction
		body, _ = sjson.SetBytes(body, "system", []interface{}{claudeCodeEntry})
		return body
	}

	if system.IsArray() {
		// Prepend Claude Code instruction to existing array
		existingSystem := system.Array()
		newSystem := make([]interface{}, 0, len(existingSystem)+1)
		newSystem = append(newSystem, claudeCodeEntry)
		for _, entry := range existingSystem {
			newSystem = append(newSystem, entry.Value())
		}
		body, _ = sjson.SetBytes(body, "system", newSystem)
		return body
	}

	// system is a string, convert to array format
	existingText := system.String()
	if existingText != "" {
		newSystem := []interface{}{
			claudeCodeEntry,
			map[string]string{"type": "text", "text": existingText},
		}
		body, _ = sjson.SetBytes(body, "system", newSystem)
	} else {
		body, _ = sjson.SetBytes(body, "system", []interface{}{claudeCodeEntry})
	}

	return body
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
