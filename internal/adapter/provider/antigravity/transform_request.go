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
) (geminiReqBody []byte, err error) {
	// 1. Parse Claude request
	var claudeReq ClaudeRequest
	if err := json.Unmarshal(claudeReqBody, &claudeReq); err != nil {
		return nil, fmt.Errorf("failed to parse Claude request: %w", err)
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
	}

	// 4. Thinking block pre-filtering
	filterInvalidThinkingBlocks(&claudeReq.Messages)

	// 5. Tool loop recovery
	closeToolLoopForThinking(&claudeReq.Messages)

	// 6. Build Gemini request
	geminiReq := make(map[string]interface{})

	// 5.1 System instruction
	if systemInstruction := buildSystemInstruction(&claudeReq, mappedModel); systemInstruction != nil {
		geminiReq["systemInstruction"] = systemInstruction
	}

	// 5.2 Message contents
	contents, err := buildContents(claudeReq.Messages, mappedModel, sessionID, signatureCache)
	if err != nil {
		return nil, fmt.Errorf("failed to build contents: %w", err)
	}
	geminiReq["contents"] = contents

	// 5.3 Tools
	if tools := buildTools(&claudeReq); tools != nil {
		geminiReq["tools"] = tools
	}

	// 5.4 Generation Config
	genConfig := buildGenerationConfig(&claudeReq, mappedModel, stream)
	geminiReq["generationConfig"] = genConfig

	// 5.5 Safety Settings (configurable via environment)
	// Reference: Antigravity-Manager's build_safety_settings
	safetyThreshold := GetSafetyThresholdFromEnv()
	safetySettings := BuildSafetySettingsMap(safetyThreshold)
	geminiReq["safetySettings"] = safetySettings

	// 6. Serialize
	return json.Marshal(geminiReq)
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
	Type         string                 `json:"type"` // "text", "thinking", "redacted_thinking", "tool_use", "tool_result", "image"
	Text         string                 `json:"text,omitempty"`
	Thinking     string                 `json:"thinking,omitempty"`
	Data         string                 `json:"data,omitempty"`         // for redacted_thinking
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

// filterInvalidThinkingBlocks removes thinking blocks without signature or content
// Reference: Antigravity-Manager's filter_invalid_thinking_blocks
func filterInvalidThinkingBlocks(messages *[]ClaudeMessage) {
	for i := range *messages {
		blocks := parseContentBlocks((*messages)[i].Content)
		if blocks == nil {
			continue
		}

		filtered := []ContentBlock{}
		for _, block := range blocks {
			if block.Type == "thinking" {
				// Keep if: has signature OR has content
				if block.Signature != "" || block.Thinking != "" {
					filtered = append(filtered, block)
				}
			} else {
				filtered = append(filtered, block)
			}
		}

		(*messages)[i].Content = filtered
	}
}

// closeToolLoopForThinking injects synthetic messages to break tool loops
// Reference: Antigravity-Manager's close_tool_loop_for_thinking
func closeToolLoopForThinking(messages *[]ClaudeMessage) {
	if len(*messages) == 0 {
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

	// Check if it has ToolUse but no Thinking
	blocks := parseContentBlocks((*messages)[lastAssistantIdx].Content)
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

	if hasToolUse && !hasThinking {
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
		// Check by name
		nameLower := strings.ToLower(tool.Name)
		if nameLower == "web_search" ||
			nameLower == "websearch" ||
			nameLower == "google_search" ||
			nameLower == "googlesearch" ||
			nameLower == "googlesearchretrieval" ||
			nameLower == "web_search_20250305" {
			return true
		}

		// Check description for web search keywords
		descLower := strings.ToLower(tool.Description)
		if strings.Contains(descLower, "web search") ||
			strings.Contains(descLower, "google search") ||
			strings.Contains(descLower, "internet search") {
			return true
		}
	}

	return false
}
