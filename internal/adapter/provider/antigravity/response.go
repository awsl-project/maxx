package antigravity

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Response headers to exclude when copying
var excludedResponseHeaders = map[string]bool{
	"content-length":    true,
	"transfer-encoding": true,
	"connection":        true,
	"keep-alive":        true,
}

// unwrapV1InternalResponse extracts the response from v1internal wrapper
func unwrapV1InternalResponse(body []byte) []byte {
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return body
	}

	if response, ok := data["response"]; ok {
		if unwrapped, err := json.Marshal(response); err == nil {
			return unwrapped
		}
	}

	return body
}

// unwrapV1InternalSSEChunk unwraps a single SSE chunk from v1internal format
// Input: "data: {"response": {...}}\n"
// Output: "data: {...}\n\n" (with double newline for proper SSE format)
// Returns nil for empty lines (they are already handled by \n\n terminator)
func unwrapV1InternalSSEChunk(line []byte) []byte {
	lineStr := strings.TrimSpace(string(line))

	// Skip empty lines - we already add \n\n after each data line
	if lineStr == "" {
		return nil
	}

	// Non-data lines pass through with proper SSE terminator
	if !strings.HasPrefix(lineStr, "data: ") {
		return []byte(lineStr + "\n\n")
	}

	jsonPart := strings.TrimPrefix(lineStr, "data: ")

	// Non-JSON data passes through with proper SSE terminator
	if !strings.HasPrefix(jsonPart, "{") {
		return []byte(lineStr + "\n\n")
	}

	// Try to parse and extract response field
	var wrapper map[string]interface{}
	if err := json.Unmarshal([]byte(jsonPart), &wrapper); err != nil {
		return []byte(lineStr + "\n\n")
	}

	// Extract "response" field if present (v1internal wraps response)
	if response, ok := wrapper["response"]; ok {
		if unwrapped, err := json.Marshal(response); err == nil {
			return []byte("data: " + string(unwrapped) + "\n\n")
		}
	}

	// No response field - pass through with proper SSE terminator
	return []byte(lineStr + "\n\n")
}

// copyResponseHeaders copies response headers from upstream, excluding certain headers
func copyResponseHeaders(dst, src http.Header) {
	if src == nil {
		return
	}
	for key, values := range src {
		lowerKey := strings.ToLower(key)
		if excludedResponseHeaders[lowerKey] {
			continue
		}
		for _, v := range values {
			dst.Add(key, v)
		}
	}
}

// flattenHeaders converts http.Header to map[string]string (first value only)
func flattenHeaders(h http.Header) map[string]string {
	result := make(map[string]string)
	for key, values := range h {
		if len(values) > 0 {
			result[key] = values[0]
		}
	}
	return result
}

// isRetryableStatusCode returns true if the status code indicates a retryable error
func isRetryableStatusCode(code int) bool {
	switch code {
	case http.StatusTooManyRequests, // 429
		http.StatusInternalServerError,    // 500
		http.StatusBadGateway,             // 502
		http.StatusServiceUnavailable,     // 503
		http.StatusGatewayTimeout:         // 504
		return true
	default:
		return false
	}
}

// convertGeminiToClaudeResponse converts a non-streaming Gemini response to Claude format
// (like Antigravity-Manager's response conversion)
func convertGeminiToClaudeResponse(geminiBody []byte, requestModel string) ([]byte, error) {
	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text             string              `json:"text,omitempty"`
					Thought          bool                `json:"thought,omitempty"`
					ThoughtSignature string              `json:"thoughtSignature,omitempty"`
					FunctionCall     *GeminiFunctionCall `json:"functionCall,omitempty"`
					InlineData       *GeminiInlineData   `json:"inlineData,omitempty"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason      string                   `json:"finishReason,omitempty"`
			GroundingMetadata *GeminiGroundingMetadata `json:"groundingMetadata,omitempty"`
		} `json:"candidates"`
		UsageMetadata *GeminiUsageMetadata `json:"usageMetadata,omitempty"`
		ModelVersion  string               `json:"modelVersion,omitempty"`
		ResponseID    string               `json:"responseId,omitempty"`
	}

	if err := json.Unmarshal(geminiBody, &geminiResp); err != nil {
		return nil, err
	}

	// [Aligned with Antigravity-Manager] Use upstream modelVersion for transparency
	modelName := geminiResp.ModelVersion
	if modelName == "" {
		modelName = requestModel // Fallback to request model if upstream doesn't provide version
	}

	// Build Claude response
	claudeResp := map[string]interface{}{
		"id":            geminiResp.ResponseID,
		"type":          "message",
		"role":          "assistant",
		"model":         modelName, // Use upstream model version (like Antigravity-Manager)
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
	}

	if claudeResp["id"] == "" {
		claudeResp["id"] = fmt.Sprintf("msg_%d", generateRandomID())
	}

	// Build usage (like Antigravity-Manager's to_claude_usage)
	usage := map[string]interface{}{
		"input_tokens":                0,
		"output_tokens":               0,
		"cache_creation_input_tokens": 0,
	}
	if geminiResp.UsageMetadata != nil {
		cachedTokens := geminiResp.UsageMetadata.CachedContentTokenCount
		inputTokens := geminiResp.UsageMetadata.PromptTokenCount - cachedTokens
		if inputTokens < 0 {
			inputTokens = 0
		}
		usage["input_tokens"] = inputTokens
		usage["output_tokens"] = geminiResp.UsageMetadata.CandidatesTokenCount
		if cachedTokens > 0 {
			usage["cache_read_input_tokens"] = cachedTokens
		}
	}
	claudeResp["usage"] = usage

	// Build content blocks
	var content []map[string]interface{}
	hasToolUse := false
	toolCallCounter := 0
	var trailingSignature string

	if len(geminiResp.Candidates) > 0 {
		candidate := geminiResp.Candidates[0]
		for _, part := range candidate.Content.Parts {
			// Handle thinking blocks (including trailing signature case)
			if part.Thought || part.ThoughtSignature != "" {
				thinkingText := part.Text
				block := map[string]interface{}{
					"type":     "thinking",
					"thinking": thinkingText,
				}
				if part.ThoughtSignature != "" {
					block["signature"] = part.ThoughtSignature
				}
				content = append(content, block)
				trailingSignature = "" // reset trailing signature once consumed
				continue
			}

			// Handle function calls
			if part.FunctionCall != nil {
				hasToolUse = true
				toolCallCounter++
				toolID := part.FunctionCall.ID
				if toolID == "" {
					toolID = fmt.Sprintf("%s-%d", part.FunctionCall.Name, toolCallCounter)
				}
				args := part.FunctionCall.Args
				remapFunctionCallArgs(part.FunctionCall.Name, args)
				toolUse := map[string]interface{}{
					"type":  "tool_use",
					"id":    toolID,
					"name":  part.FunctionCall.Name,
					"input": args,
				}
				if part.ThoughtSignature != "" {
					toolUse["signature"] = part.ThoughtSignature
				} else if trailingSignature != "" {
					toolUse["signature"] = trailingSignature
				}
				content = append(content, toolUse)
				trailingSignature = ""
				continue
			}

			// Handle inline data (images)
			if part.InlineData != nil && part.InlineData.Data != "" {
				markdownImg := fmt.Sprintf("![image](data:%s;base64,%s)", part.InlineData.MimeType, part.InlineData.Data)
				content = append(content, map[string]interface{}{
					"type": "text",
					"text": markdownImg,
				})
				continue
			}

			// Handle text blocks (capture trailing signature if empty)
			if part.Text != "" {
				content = append(content, map[string]interface{}{
					"type": "text",
					"text": part.Text,
				})
				continue
			}

			// Empty text with signature (store for next block)
			if part.Text == "" && part.ThoughtSignature != "" {
				trailingSignature = part.ThoughtSignature
			}
		}

		// Append grounding metadata as a text block (like Antigravity-Manager)
		if candidate.GroundingMetadata != nil {
			if groundingText := buildGroundingText(candidate.GroundingMetadata); groundingText != "" {
				content = append(content, map[string]interface{}{
					"type": "text",
					"text": groundingText,
				})
			}
		}

		// Set stop reason
		switch candidate.FinishReason {
		case "STOP":
			if hasToolUse {
				claudeResp["stop_reason"] = "tool_use"
			} else {
				claudeResp["stop_reason"] = "end_turn"
			}
		case "MAX_TOKENS":
			claudeResp["stop_reason"] = "max_tokens"
		}
	}

	claudeResp["content"] = content

	return json.Marshal(claudeResp)
}

// buildGroundingText converts grounding metadata into a markdown text snippet
func buildGroundingText(grounding *GeminiGroundingMetadata) string {
	if grounding == nil {
		return ""
	}

	var b strings.Builder

	if len(grounding.WebSearchQueries) > 0 {
		b.WriteString("\n\n---\n**🔍 已为您搜索：** ")
		b.WriteString(strings.Join(grounding.WebSearchQueries, ", "))
	}

	if len(grounding.GroundingChunks) > 0 {
		var links []string
		for i, chunk := range grounding.GroundingChunks {
			if chunk.Web != nil {
				title := chunk.Web.Title
				if title == "" {
					title = "网页来源"
				}
				uri := chunk.Web.URI
				if uri == "" {
					uri = "#"
				}
				links = append(links, fmt.Sprintf("[%d] [%s](%s)", i+1, title, uri))
			}
		}
		if len(links) > 0 {
			b.WriteString("\n\n**🌐 来源引文：**\n")
			b.WriteString(strings.Join(links, "\n"))
		}
	}

	return b.String()
}
