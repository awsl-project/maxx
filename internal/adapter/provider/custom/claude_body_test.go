package custom

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSystemPromptInjection(t *testing.T) {
	// Test case: empty body
	body := []byte(`{"model":"claude-3-5-sonnet","messages":[]}`)
	result := injectClaudeCodeSystemPrompt(body)

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}

	// Check system field exists and is array
	system, ok := parsed["system"].([]interface{})
	if !ok {
		t.Fatalf("system field is not an array: %T", parsed["system"])
	}

	// Should have 1 entry: Claude Code prompt
	if len(system) != 1 {
		t.Fatalf("Expected 1 system entry, got %d", len(system))
	}

	// Check first entry is Claude Code prompt
	entry0, ok := system[0].(map[string]interface{})
	if !ok {
		t.Fatalf("system entry 0 is not a map: %T", system[0])
	}
	if entry0["type"] != "text" {
		t.Errorf("Expected entry 0 type='text', got %v", entry0["type"])
	}
	if entry0["text"] != claudeCodeSystemPrompt {
		t.Errorf("Expected entry 0 text='%s', got %v", claudeCodeSystemPrompt, entry0["text"])
	}

	t.Logf("Injected system prompt: %s", string(result))
}

func TestUserIDGeneration(t *testing.T) {
	userID := generateFakeUserID()

	// Check format matches sub2api's regex: ^user_[a-fA-F0-9]{64}_account__session_[\w-]+$
	if !isValidUserID(userID) {
		t.Errorf("Generated user_id doesn't match expected format: %s", userID)
	}

	t.Logf("Generated user_id: %s", userID)
}

func TestCloakingForNonClaudeClient(t *testing.T) {
	body := []byte(`{"model":"claude-3-5-sonnet","messages":[{"role":"user","content":"hello"}]}`)

	// Non-Claude Code client (e.g., curl)
	result := applyCloaking(body, "curl/7.68.0")

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}

	// Should have system prompt injected
	system, ok := parsed["system"].([]interface{})
	if !ok || len(system) == 0 {
		t.Error("System prompt was not injected for non-Claude client")
	}

	// Should have metadata.user_id injected
	metadata, ok := parsed["metadata"].(map[string]interface{})
	if !ok {
		t.Error("metadata was not created")
	}

	userID, ok := metadata["user_id"].(string)
	if !ok || userID == "" {
		t.Error("user_id was not injected")
	}

	if !isValidUserID(userID) {
		t.Errorf("Injected user_id doesn't match expected format: %s", userID)
	}

	t.Logf("Cloaked body: %s", string(result))
}

func TestNoCloakingForClaudeClient(t *testing.T) {
	body := []byte(`{"model":"claude-3-5-sonnet","messages":[{"role":"user","content":"hello"}]}`)

	// Claude Code client
	result := applyCloaking(body, "claude-cli/2.1.23 (external, cli)")

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}

	// Should NOT have system prompt injected
	if _, ok := parsed["system"]; ok {
		t.Error("System prompt was injected for Claude Code client (should not)")
	}

	// Should NOT have metadata injected
	if _, ok := parsed["metadata"]; ok {
		t.Error("metadata was injected for Claude Code client (should not)")
	}
}

func TestUserAgentCloakingForNonClaudeClient(t *testing.T) {
	// Non-Claude client (e.g., curl)
	clientUA := "curl/7.68.0"

	// isClaudeCodeClient should return false
	if isClaudeCodeClient(clientUA) {
		t.Error("curl should not be detected as Claude Code client")
	}

	// For non-Claude clients, User-Agent should be forced to default
	// (tested via the header function behavior)
	t.Logf("Non-Claude client '%s' will have User-Agent forced to '%s'", clientUA, defaultClaudeUserAgent)
}

func TestUserAgentPassthroughForClaudeClient(t *testing.T) {
	// Claude Code client
	clientUA := "claude-cli/2.1.23 (external, cli)"

	// isClaudeCodeClient should return true
	if !isClaudeCodeClient(clientUA) {
		t.Error("claude-cli should be detected as Claude Code client")
	}

	// For Claude clients, User-Agent should be passed through
	t.Logf("Claude client '%s' will have User-Agent passed through", clientUA)
}

func TestFullBodyProcessing(t *testing.T) {
	body := []byte(`{"model":"claude-3-5-sonnet","max_tokens":1024,"messages":[{"role":"user","content":"hello"}]}`)

	// Process body with non-Claude client
	result, _ := processClaudeRequestBody(body, "curl/7.68.0")

	// Parse result
	resultStr := string(result)
	t.Logf("Processed body: %s", resultStr)

	// Check field order by finding positions
	modelPos := strings.Index(resultStr, `"model"`)
	messagesPos := strings.Index(resultStr, `"messages"`)
	systemPos := strings.Index(resultStr, `"system"`)
	toolsPos := strings.Index(resultStr, `"tools"`)
	metadataPos := strings.Index(resultStr, `"metadata"`)
	maxTokensPos := strings.Index(resultStr, `"max_tokens"`)
	streamPos := strings.Index(resultStr, `"stream"`)

	// Verify order: model < messages < system < tools < metadata < max_tokens < stream
	if modelPos > messagesPos {
		t.Error("model should come before messages")
	}
	if messagesPos > systemPos {
		t.Error("messages should come before system")
	}
	if systemPos > toolsPos {
		t.Error("system should come before tools")
	}
	if toolsPos > metadataPos {
		t.Error("tools should come before metadata")
	}
	if metadataPos > maxTokensPos {
		t.Error("metadata should come before max_tokens")
	}
	if maxTokensPos > streamPos {
		t.Error("max_tokens should come before stream")
	}

	// Verify stream is true
	if !strings.Contains(resultStr, `"stream":true`) {
		t.Error("stream should be true")
	}
}

func TestCleanCacheControl(t *testing.T) {
	// Body with cache_control in messages and system
	body := []byte(`{
		"model":"claude-3-5-sonnet",
		"messages":[
			{"role":"user","content":[{"type":"text","text":"hello","cache_control":{"type":"ephemeral"}}]}
		],
		"system":[{"type":"text","text":"You are helpful","cache_control":{"type":"ephemeral"}}]
	}`)

	result := cleanCacheControl(body)

	// Verify cache_control is removed
	if strings.Contains(string(result), "cache_control") {
		t.Error("cache_control should be removed from body")
	}

	// Verify other content is preserved
	if !strings.Contains(string(result), `"text":"hello"`) {
		t.Error("message text should be preserved")
	}
	if !strings.Contains(string(result), `"text":"You are helpful"`) {
		t.Error("system text should be preserved")
	}

	t.Logf("Cleaned body: %s", string(result))
}

func TestInterleavedThinkingHint(t *testing.T) {
	// Body with thinking enabled and tools present
	body := []byte(`{
		"model":"claude-3-5-sonnet",
		"messages":[{"role":"user","content":"hello"}],
		"system":[{"type":"text","text":"You are helpful"}],
		"tools":[{"name":"test_tool","description":"A test tool"}],
		"thinking":{"type":"enabled","budget_tokens":10000}
	}`)

	result := injectInterleavedThinkingHint(body)

	// Verify hint is appended to system
	if !strings.Contains(string(result), interleavedThinkingHint) {
		t.Error("interleaved thinking hint should be injected")
	}

	// Verify original system content is preserved
	if !strings.Contains(string(result), "You are helpful") {
		t.Error("original system text should be preserved")
	}

	t.Logf("Body with hint: %s", string(result))
}

func TestNoThinkingHintWithoutThinking(t *testing.T) {
	// Body without thinking configuration
	body := []byte(`{
		"model":"claude-3-5-sonnet",
		"messages":[{"role":"user","content":"hello"}],
		"tools":[{"name":"test_tool"}]
	}`)

	result := injectInterleavedThinkingHint(body)

	// Verify hint is NOT injected
	if strings.Contains(string(result), interleavedThinkingHint) {
		t.Error("thinking hint should NOT be injected without thinking enabled")
	}
}

func TestNoThinkingHintWithoutTools(t *testing.T) {
	// Body with thinking but no tools
	body := []byte(`{
		"model":"claude-3-5-sonnet",
		"messages":[{"role":"user","content":"hello"}],
		"thinking":{"type":"enabled","budget_tokens":10000}
	}`)

	result := injectInterleavedThinkingHint(body)

	// Verify hint is NOT injected
	if strings.Contains(string(result), interleavedThinkingHint) {
		t.Error("thinking hint should NOT be injected without tools")
	}
}

func TestFieldOrderWithThinking(t *testing.T) {
	// Body with thinking and tool_choice
	body := []byte(`{
		"stream":true,
		"thinking":{"type":"enabled"},
		"tool_choice":{"type":"auto"},
		"model":"claude-3-5-sonnet",
		"messages":[{"role":"user","content":"hello"}],
		"max_tokens":1024
	}`)

	result := reorderBodyFields(body)
	resultStr := string(result)

	// Check field order
	modelPos := strings.Index(resultStr, `"model"`)
	messagesPos := strings.Index(resultStr, `"messages"`)
	toolChoicePos := strings.Index(resultStr, `"tool_choice"`)
	thinkingPos := strings.Index(resultStr, `"thinking"`)
	maxTokensPos := strings.Index(resultStr, `"max_tokens"`)
	streamPos := strings.Index(resultStr, `"stream"`)

	if modelPos > messagesPos {
		t.Error("model should come before messages")
	}
	if toolChoicePos > thinkingPos {
		t.Error("tool_choice should come before thinking")
	}
	if thinkingPos > maxTokensPos {
		t.Error("thinking should come before max_tokens")
	}
	if maxTokensPos > streamPos {
		t.Error("max_tokens should come before stream")
	}

	t.Logf("Reordered body: %s", resultStr)
}

func TestNoDuplicateSystemPromptInjection(t *testing.T) {
	// Body that already has Claude Code system prompt
	body := []byte(`{
		"model":"claude-3-5-sonnet",
		"messages":[{"role":"user","content":"hello"}],
		"system":[{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude."},{"type":"text","text":"Additional instructions"}]
	}`)

	result := injectClaudeCodeSystemPrompt(body)

	// Count occurrences of "Claude Code"
	count := strings.Count(string(result), "Claude Code")
	if count != 1 {
		t.Errorf("Expected 1 occurrence of 'Claude Code', got %d", count)
	}

	t.Logf("Result (no duplicate): %s", string(result))
}
