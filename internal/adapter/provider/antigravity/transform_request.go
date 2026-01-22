package antigravity

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// TransformClaudeToGemini converts a Claude API request to Gemini v1internal format
// Reference: Antigravity-Manager's transform_claude_request_in
func TransformClaudeToGemini(
	claudeReqBody []byte,
	mappedModel string,
	stream bool,
	sessionID string,
	signatureCache *SignatureCache,
) (geminiReqBody []byte, effectiveMappedModel string, hasThinking bool, err error) {
	effectiveMappedModel = mappedModel

	// 1. Parse Claude request
	var claudeReq ClaudeRequest
	if err := json.Unmarshal(claudeReqBody, &claudeReq); err != nil {
		return nil, effectiveMappedModel, false, fmt.Errorf("failed to parse Claude request: %w", err)
	}

	// 2. Cache Control cleanup (before conversion)
	cleanCacheControlFromRequest(&claudeReq)

	// 3. Detect Web Search tool and apply model fallback
	// Reference: Antigravity-Manager's web search detection
	hasWebSearch := detectWebSearchTool(&claudeReq)
	if hasWebSearch {
		// Web Search only works reliably with gemini-2.5-flash
		log.Printf("[Antigravity] Detected Web Search tool, forcing model to gemini-2.5-flash (was: %s)", mappedModel)
		mappedModel = "gemini-2.5-flash"
		effectiveMappedModel = mappedModel
	}

	// 4. Thinking block pre-filtering
	filterInvalidThinkingBlocks(&claudeReq.Messages)

	// 5. Tool loop recovery
	closeToolLoopForThinking(&claudeReq.Messages)

	// 6. Remove trailing unsigned thinking blocks (like Antigravity-Manager)
	removeTrailingUnsignedThinking(&claudeReq.Messages)

	// 7. Calculate final thinking mode state (before building request)
	// Reference: Antigravity-Manager's thinking mode resolution (line 170-251)
	hasThinking = calculateFinalThinkingState(&claudeReq, mappedModel, signatureCache)

	// 8. Build Gemini request
	geminiReq := make(map[string]interface{})

	// 7.1 System instruction
	if systemInstruction := buildSystemInstruction(&claudeReq, mappedModel); systemInstruction != nil {
		geminiReq["systemInstruction"] = systemInstruction
	}

	// 7.2 Message contents
	contents, err := buildContents(claudeReq.Messages, mappedModel, sessionID, signatureCache)
	if err != nil {
		return nil, effectiveMappedModel, hasThinking, fmt.Errorf("failed to build contents: %w", err)
	}
	geminiReq["contents"] = contents

	// 7.3 Tools
	if tools := buildTools(&claudeReq); tools != nil {
		geminiReq["tools"] = tools
	}

	// 7.4 Generation Config (use pre-calculated hasThinking)
	genConfig := buildGenerationConfig(&claudeReq, mappedModel, stream, hasThinking)
	geminiReq["generationConfig"] = genConfig

	// 5.5 Safety Settings (configurable via environment)
	// Reference: Antigravity-Manager's build_safety_settings
	safetyThreshold := GetSafetyThresholdFromEnv()
	safetySettings := BuildSafetySettingsMap(safetyThreshold)
	geminiReq["safetySettings"] = safetySettings

	// 5.6 Deep clean [undefined] strings (Cherry Studio injection fix)
	// Reference: Antigravity-Manager line 278
	deepCleanUndefined(geminiReq)

	// 6. Serialize
	geminiReqBody, err = json.Marshal(geminiReq)
	if err != nil {
		return nil, effectiveMappedModel, hasThinking, err
	}
	return geminiReqBody, effectiveMappedModel, hasThinking, nil
}

// ClaudeRequest represents a Claude API request
type ClaudeRequest struct {
	Model        string          `json:"model"`
	MaxTokens    int             `json:"max_tokens,omitempty"`
	Messages     []ClaudeMessage `json:"messages"`
	System       interface{}     `json:"system,omitempty"` // string or []SystemBlock
	Tools        []ClaudeTool    `json:"tools,omitempty"`
	Temperature  *float64        `json:"temperature,omitempty"`
	TopP         *float64        `json:"top_p,omitempty"`
	TopK         *int            `json:"top_k,omitempty"`
	Stream       bool            `json:"stream,omitempty"`
	Thinking     *ThinkingConfig `json:"thinking,omitempty"`
	OutputConfig *OutputConfig   `json:"output_config,omitempty"`
	Metadata     *Metadata       `json:"metadata,omitempty"`
}

// ClaudeMessage represents a message in Claude format
type ClaudeMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string or []ContentBlock
}

// ContentBlock represents a content block in Claude format
type ContentBlock struct {
	Type         string                 `json:"type"` // "text", "thinking", "redacted_thinking", "tool_use", "tool_result", "image", "document"
	Text         string                 `json:"text,omitempty"`
	Thinking     string                 `json:"thinking,omitempty"`
	Data         string                 `json:"data,omitempty"` // for redacted_thinking
	Signature    string                 `json:"signature,omitempty"`
	ID           string                 `json:"id,omitempty"`
	Name         string                 `json:"name,omitempty"`
	Input        map[string]interface{} `json:"input,omitempty"`
	ToolUseID    string                 `json:"tool_use_id,omitempty"`
	Content      interface{}            `json:"content,omitempty"` // tool_result content
	IsError      *bool                  `json:"is_error,omitempty"`
	Source       *ImageSource           `json:"source,omitempty"`
	CacheControl *CacheControl          `json:"cache_control,omitempty"`
}

// ClaudeTool represents a tool definition in Claude format
type ClaudeTool struct {
	Type         string                 `json:"type,omitempty"` // server tools like "web_search_20250305"
	Name         string                 `json:"name"`
	Description  string                 `json:"description,omitempty"`
	InputSchema  map[string]interface{} `json:"input_schema"`
	CacheControl *CacheControl          `json:"cache_control,omitempty"`
}

// ThinkingConfig represents thinking configuration
type ThinkingConfig struct {
	Type         string `json:"type"` // "enabled"
	BudgetTokens *int   `json:"budget_tokens,omitempty"`
}

// OutputConfig represents output configuration
type OutputConfig struct {
	Effort string `json:"effort,omitempty"` // "high", "medium", "low"
}

// Metadata represents request metadata
type Metadata struct {
	UserID string `json:"user_id,omitempty"`
}

// ImageSource represents an image source
type ImageSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // "image/png", etc.
	Data      string `json:"data"`       // base64 encoded
}

// CacheControl represents cache control settings
type CacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// cleanCacheControlFromRequest removes cache_control from all blocks
// Reference: Antigravity-Manager's clean_cache_control_from_messages
func cleanCacheControlFromRequest(claudeReq *ClaudeRequest) {
	// 1. Clean messages
	for i := range claudeReq.Messages {
		blocks := parseContentBlocks(claudeReq.Messages[i].Content)
		if blocks == nil {
			continue
		}

		for j := range blocks {
			blocks[j].CacheControl = nil
		}

		claudeReq.Messages[i].Content = blocks
	}

	// 2. Clean system (if it's an array)
	if systemBlocks, ok := claudeReq.System.([]interface{}); ok {
		for _, block := range systemBlocks {
			if blockMap, ok := block.(map[string]interface{}); ok {
				delete(blockMap, "cache_control")
			}
		}
	}

	// 3. Clean tools
	for i := range claudeReq.Tools {
		claudeReq.Tools[i].CacheControl = nil
	}
}

func hasValidThinkingSignature(thinkingText, signature string) bool {
	// Empty thinking + any signature = valid (trailing signature case)
	if thinkingText == "" && signature != "" {
		return true
	}
	// Non-empty thinking must have a "long enough" signature
	return signature != "" && len(signature) >= MinThinkingSignatureLength
}

// filterInvalidThinkingBlocks filters invalid thinking blocks from message history.
// Mirrors Antigravity-Manager's `filter_invalid_thinking_blocks` behavior:
// - Only touches assistant/model roles
// - Invalid thinking blocks are converted to text (preserve content) or dropped if empty
// - Ensures message content is not empty (injects an empty text block)
func filterInvalidThinkingBlocks(messages *[]ClaudeMessage) {
	for i := range *messages {
		role := (*messages)[i].Role
		if role != "assistant" && role != "model" {
			continue
		}

		blocks := parseContentBlocks((*messages)[i].Content)
		if blocks == nil {
			continue
		}

		filtered := make([]ContentBlock, 0, len(blocks))
		for _, block := range blocks {
			if block.Type != "thinking" {
				filtered = append(filtered, block)
				continue
			}

			if hasValidThinkingSignature(block.Thinking, block.Signature) {
				// Sanitize: cache_control should not be forwarded
				block.CacheControl = nil
				filtered = append(filtered, block)
				continue
			}

			// Invalid signature: preserve content by downgrading to text (Manager behavior)
			if strings.TrimSpace(block.Thinking) != "" {
				filtered = append(filtered, ContentBlock{
					Type: "text",
					Text: block.Thinking,
				})
			}
		}

		if len(filtered) == 0 {
			filtered = append(filtered, ContentBlock{
				Type: "text",
				Text: "",
			})
		}

		(*messages)[i].Content = filtered
	}
}

// removeTrailingUnsignedThinking removes trailing thinking blocks without valid signatures from assistant/model messages.
// Mirrors Antigravity-Manager's `remove_trailing_unsigned_thinking`.
func removeTrailingUnsignedThinking(messages *[]ClaudeMessage) {
	for i := range *messages {
		role := (*messages)[i].Role
		if role != "assistant" && role != "model" {
			continue
		}

		blocks := parseContentBlocks((*messages)[i].Content)
		if len(blocks) == 0 {
			continue
		}

		endIndex := len(blocks)
		for j := len(blocks) - 1; j >= 0; j-- {
			if blocks[j].Type != "thinking" {
				break
			}
			if !hasValidThinkingSignature(blocks[j].Thinking, blocks[j].Signature) {
				endIndex = j
				continue
			}
			break
		}

		if endIndex < len(blocks) {
			blocks = blocks[:endIndex]
			(*messages)[i].Content = blocks
		}
	}
}

// closeToolLoopForThinking injects synthetic messages to break tool loops
// Reference: Antigravity-Manager's close_tool_loop_for_thinking
func closeToolLoopForThinking(messages *[]ClaudeMessage) {
	if len(*messages) == 0 {
		return
	}

	// Only recover when we are in a tool loop:
	// the last message is a user ToolResult, but the preceding assistant message has no Thinking block.
	lastMsg := (*messages)[len(*messages)-1]
	if lastMsg.Role != "user" {
		return
	}

	lastBlocks := parseContentBlocks(lastMsg.Content)
	inToolLoop := false
	for _, block := range lastBlocks {
		if block.Type == "tool_result" {
			inToolLoop = true
			break
		}
	}
	if !inToolLoop {
		return
	}

	// Find last assistant message
	lastAssistantIdx := -1
	for i := len(*messages) - 1; i >= 0; i-- {
		if (*messages)[i].Role == "assistant" {
			lastAssistantIdx = i
			break
		}
	}

	if lastAssistantIdx == -1 {
		return
	}

	// Check if it has a Thinking block
	blocks := parseContentBlocks((*messages)[lastAssistantIdx].Content)
	hasThinking := false

	for _, block := range blocks {
		if block.Type == "thinking" {
			hasThinking = true
			break
		}
	}

	if !hasThinking {
		log.Println("[Antigravity] Detected broken tool loop, injecting synthetic messages")

		// Inject synthetic assistant message
		*messages = append(*messages, ClaudeMessage{
			Role: "assistant",
			Content: []ContentBlock{
				{
					Type: "text",
					Text: "[Tool execution completed. Please proceed.]",
				},
			},
		})

		// Inject synthetic user message
		*messages = append(*messages, ClaudeMessage{
			Role: "user",
			Content: []ContentBlock{
				{
					Type: "text",
					Text: "Proceed.",
				},
			},
		})
	}
}

// parseContentBlocks converts interface{} content to []ContentBlock
func parseContentBlocks(content interface{}) []ContentBlock {
	switch c := content.(type) {
	case string:
		// Simple text message
		return []ContentBlock{
			{
				Type: "text",
				Text: c,
			},
		}
	case []interface{}:
		// Array of blocks
		blocks := make([]ContentBlock, 0, len(c))
		for _, item := range c {
			if blockMap, ok := item.(map[string]interface{}); ok {
				block := ContentBlock{}
				if data, err := json.Marshal(blockMap); err == nil {
					if err := json.Unmarshal(data, &block); err == nil {
						blocks = append(blocks, block)
					}
				}
			}
		}
		return blocks
	case []ContentBlock:
		// Already ContentBlock array
		return c
	default:
		return nil
	}
}

// extractSystemText extracts text from system prompt (string or array)
func extractSystemText(system interface{}) string {
	switch sys := system.(type) {
	case string:
		return sys
	case []interface{}:
		var texts []string
		for _, block := range sys {
			if blockMap, ok := block.(map[string]interface{}); ok {
				if text, ok := blockMap["text"].(string); ok {
					texts = append(texts, text)
				}
			}
		}
		return strings.Join(texts, "\n")
	default:
		return ""
	}
}

// detectWebSearchTool detects if the request contains Web Search tools
// Reference: Antigravity-Manager's web search detection
func detectWebSearchTool(claudeReq *ClaudeRequest) bool {
	if claudeReq.Tools == nil {
		return false
	}

	for _, tool := range claudeReq.Tools {
		// Server tools: type starts with "web_search" (preferred)
		if strings.HasPrefix(strings.ToLower(tool.Type), "web_search") {
			return true
		}

		// Fallback: name-based detection (includes legacy "google_search")
		switch strings.ToLower(tool.Name) {
		case "web_search", "google_search":
			return true
		}
	}

	return false
}

// calculateFinalThinkingState determines the final thinking mode state
// after all checks (model defaults, target support, history compatibility)
// Reference: Antigravity-Manager's thinking mode resolution (line 170-251)
func calculateFinalThinkingState(claudeReq *ClaudeRequest, mappedModel string, signatureCache *SignatureCache) bool {
	// 1. Check explicit thinking config first
	thinkingRequested := claudeReq.Thinking != nil && claudeReq.Thinking.Type == "enabled"

	// 2. If no explicit config, check if model should enable thinking by default (Opus 4.5)
	if !thinkingRequested && shouldEnableThinkingByDefault(claudeReq.Model) {
		thinkingRequested = true
	}

	// 3. Check if target model supports thinking
	if thinkingRequested && !TargetModelSupportsThinking(mappedModel) {
		log.Printf("[Antigravity] Target model '%s' does not support thinking. Force disabling.", mappedModel)
		return false
	}

	// 4. Check history compatibility
	// Reference: Antigravity-Manager's should_disable_thinking_due_to_history (line 196-202)
	if thinkingRequested {
		// Need to convert messages to Gemini format first to check compatibility
		// For now, we'll do a simplified check on Claude messages
		if shouldDisableThinkingDueToClaudeHistory(claudeReq.Messages) {
			log.Printf("[Antigravity] Disabling thinking due to incompatible tool-use history (mixed application)")
			return false
		}
	}

	// 5. [FIX #295 & #298] Check signature validity for function calls
	// Reference: Antigravity-Manager's signature validation (line 204-251)
	// This prevents Gemini 3 Pro from rejecting requests due to missing thought_signature
	if thinkingRequested {
		globalSig := GetThoughtSignature()

		// Check if there are thinking blocks in history
		hasThinkingHistory := hasThinkingInMessages(claudeReq.Messages)

		// Check if there are function calls
		hasFunctionCalls := hasFunctionCallsInMessages(claudeReq.Messages)

		// [FIX #298] For first-time thinking requests (no thinking history),
		// we use permissive mode and let upstream handle validation.
		// We only enforce strict signature checks when function calls are involved.
		needsSignatureCheck := hasFunctionCalls

		if !hasThinkingHistory && thinkingRequested {
			log.Printf("[Antigravity] First thinking request detected. Using permissive mode - " +
				"signature validation will be handled by upstream API.")
		}

		if needsSignatureCheck && !hasValidSignatureForFunctionCalls(claudeReq.Messages, globalSig) {
			log.Printf("[Antigravity] [FIX #295] No valid signature found for function calls. " +
				"Disabling thinking to prevent Gemini 3 Pro rejection.")
			return false
		}
	}

	return thinkingRequested
}

// shouldDisableThinkingDueToClaudeHistory checks Claude messages for thinking/tool incompatibility
// Reference: Antigravity-Manager's should_disable_thinking_due_to_history
func shouldDisableThinkingDueToClaudeHistory(messages []ClaudeMessage) bool {
	// Find last assistant message
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "assistant" {
			continue
		}

		// Parse content blocks
		blocks := parseContentBlocks(messages[i].Content)
		if blocks == nil {
			return false
		}

		hasToolUse := false
		hasThinking := false

		for _, block := range blocks {
			if block.Type == "tool_use" {
				hasToolUse = true
			}
			if block.Type == "thinking" {
				hasThinking = true
			}
		}

		// If has tool_use but no thinking -> incompatible
		if hasToolUse && !hasThinking {
			return true
		}

		// Only check the last assistant message
		return false
	}

	return false
}

// hasValidSignatureForFunctionCalls checks if we have any valid signature for function calls
// Reference: Antigravity-Manager's has_valid_signature_for_function_calls (line 405-435)
// [FIX #295] Prevents Gemini 3 Pro from rejecting requests due to missing thought_signature
func hasValidSignatureForFunctionCalls(messages []ClaudeMessage, globalSig string) bool {
	// 1. Check global store
	if globalSig != "" && len(globalSig) >= MinSignatureLength {
		return true
	}

	// 2. Check if any message has a thinking block with valid signature
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "assistant" {
			continue
		}

		blocks := parseContentBlocks(messages[i].Content)
		if blocks == nil {
			continue
		}

		for _, block := range blocks {
			if block.Type == "thinking" && block.Signature != "" {
				if len(block.Signature) >= MinSignatureLength {
					return true
				}
			}
		}
	}

	return false
}

// hasThinkingInMessages checks if any message contains thinking blocks
func hasThinkingInMessages(messages []ClaudeMessage) bool {
	for _, msg := range messages {
		if msg.Role != "assistant" {
			continue
		}

		blocks := parseContentBlocks(msg.Content)
		if blocks == nil {
			continue
		}

		for _, block := range blocks {
			if block.Type == "thinking" {
				return true
			}
		}
	}

	return false
}

// hasFunctionCallsInMessages checks if any message contains tool_use blocks
func hasFunctionCallsInMessages(messages []ClaudeMessage) bool {
	for _, msg := range messages {
		blocks := parseContentBlocks(msg.Content)
		if blocks == nil {
			continue
		}

		for _, block := range blocks {
			if block.Type == "tool_use" {
				return true
			}
		}
	}

	return false
}
