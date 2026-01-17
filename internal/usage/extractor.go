// Package usage provides utilities for extracting token usage metrics from API responses.
// Supports Claude, OpenAI, Gemini, and Codex response formats (both JSON and SSE).
package usage

import (
	"encoding/json"
	"strings"

	"github.com/awsl-project/maxx/internal/domain"
)

// Metrics represents extracted usage information from an API response.
type Metrics struct {
	InputTokens  uint64 `json:"inputTokens"`
	OutputTokens uint64 `json:"outputTokens"`

	// Cache metrics
	CacheCreationCount   uint64 `json:"cacheCreationCount"`   // Total cache write tokens (= Cache5mCreation + Cache1hCreation)
	CacheReadCount       uint64 `json:"cacheReadCount"`       // Cache read/hit tokens
	Cache5mCreationCount uint64 `json:"cache5mCreationCount"` // 5-minute TTL cache creation tokens (price: input × 1.25)
	Cache1hCreationCount uint64 `json:"cache1hCreationCount"` // 1-hour TTL cache creation tokens (price: input × 2.0)
}

// IsEmpty returns true if no tokens were extracted.
func (m *Metrics) IsEmpty() bool {
	return m.InputTokens == 0 && m.OutputTokens == 0 && m.CacheCreationCount == 0 && m.CacheReadCount == 0
}

// ExtractFromResponse extracts usage metrics from a response body.
// Supports JSON and SSE formats from Claude, OpenAI, Gemini, and Codex APIs.
func ExtractFromResponse(body string) *Metrics {
	if body == "" {
		return nil
	}

	// Try parsing as JSON first
	metrics := extractFromJSON(body)
	if metrics != nil && !metrics.IsEmpty() {
		return metrics
	}

	// Try parsing as SSE (for streaming responses)
	metrics = extractFromSSE(body)
	if metrics != nil && !metrics.IsEmpty() {
		return metrics
	}

	return nil
}

// extractFromJSON tries to parse usage from a JSON response body.
func extractFromJSON(body string) *Metrics {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		return nil
	}

	return extractUsageFromMap(data)
}

// extractFromSSE extracts usage from SSE (Server-Sent Events) format.
// Looks for the final event containing usage information.
func extractFromSSE(body string) *Metrics {
	lines := strings.Split(body, "\n")
	var lastMetrics *Metrics

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip non-data lines
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		// Extract JSON from data: prefix
		jsonStr := strings.TrimPrefix(line, "data:")
		jsonStr = strings.TrimSpace(jsonStr)

		// Skip [DONE] marker
		if jsonStr == "[DONE]" {
			continue
		}

		var data map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			continue
		}

		// Try to extract metrics from this event
		metrics := extractUsageFromMap(data)
		if metrics != nil && !metrics.IsEmpty() {
			lastMetrics = metrics
		}

		// Claude SSE: Check for message_delta type which contains final usage
		if eventType, ok := data["type"].(string); ok {
			if eventType == "message_delta" {
				if usage, ok := data["usage"].(map[string]interface{}); ok {
					m := extractClaudeUsage(usage)
					if m != nil && !m.IsEmpty() {
						lastMetrics = m
					}
				}
			}
			// Codex SSE: Check for response.completed type which contains final usage
			if eventType == "response.completed" {
				if response, ok := data["response"].(map[string]interface{}); ok {
					if usage, ok := response["usage"].(map[string]interface{}); ok {
						m := extractOpenAIUsage(usage)
						if m != nil && !m.IsEmpty() {
							lastMetrics = m
						}
					}
				}
			}
		}
	}

	return lastMetrics
}

// extractUsageFromMap extracts usage metrics from a parsed JSON map.
// Handles multiple API formats.
func extractUsageFromMap(data map[string]interface{}) *Metrics {
	// Try Claude/Anthropic format: { "usage": { ... } }
	if usage, ok := data["usage"].(map[string]interface{}); ok {
		return extractClaudeUsage(usage)
	}

	// Try Gemini format: { "usageMetadata": { ... } }
	if usage, ok := data["usageMetadata"].(map[string]interface{}); ok {
		return extractGeminiUsage(usage)
	}

	// Try Claude message format: { "message": { "usage": { ... } } }
	if message, ok := data["message"].(map[string]interface{}); ok {
		if usage, ok := message["usage"].(map[string]interface{}); ok {
			return extractClaudeUsage(usage)
		}
	}

	// Try Codex/OpenAI Response API format: { "response": { "usage": { ... } } }
	// Also handles Gemini v1internal wrapper: { "response": { "usageMetadata": { ... } } }
	if response, ok := data["response"].(map[string]interface{}); ok {
		// Try OpenAI usage format first
		if usage, ok := response["usage"].(map[string]interface{}); ok {
			return extractOpenAIUsage(usage)
		}
		// Try Gemini usageMetadata format (v1internal wrapper)
		if usage, ok := response["usageMetadata"].(map[string]interface{}); ok {
			return extractGeminiUsage(usage)
		}
	}

	// Try OpenAI choices format for some responses
	if choices, ok := data["choices"].([]interface{}); ok && len(choices) > 0 {
		// Usage might be at root level alongside choices
		if usage, ok := data["usage"].(map[string]interface{}); ok {
			return extractOpenAIUsage(usage)
		}
	}

	return nil
}

// extractClaudeUsage extracts metrics from Claude/Anthropic usage format.
// Example: { "input_tokens": 100, "output_tokens": 50, "cache_read_input_tokens": 20,
//            "cache_creation_input_tokens": 30, "cache_creation_5m_input_tokens": 10,
//            "cache_creation_1h_input_tokens": 20 }
func extractClaudeUsage(usage map[string]interface{}) *Metrics {
	metrics := &Metrics{}

	// Input tokens
	if v, ok := usage["input_tokens"].(float64); ok {
		metrics.InputTokens = uint64(v)
	}

	// Output tokens
	if v, ok := usage["output_tokens"].(float64); ok {
		metrics.OutputTokens = uint64(v)
	}

	// Cache creation tokens (total write)
	if v, ok := usage["cache_creation_input_tokens"].(float64); ok {
		metrics.CacheCreationCount = uint64(v)
	}

	// Cache creation 5-minute TTL tokens (price: input × 1.25)
	if v, ok := usage["cache_creation_5m_input_tokens"].(float64); ok {
		metrics.Cache5mCreationCount = uint64(v)
	}

	// Cache creation 1-hour TTL tokens (price: input × 2.0)
	if v, ok := usage["cache_creation_1h_input_tokens"].(float64); ok {
		metrics.Cache1hCreationCount = uint64(v)
	}

	// Cache read tokens
	if v, ok := usage["cache_read_input_tokens"].(float64); ok {
		metrics.CacheReadCount = uint64(v)
	}

	return metrics
}

// extractOpenAIUsage extracts metrics from OpenAI usage format.
// Supports both standard OpenAI format and Codex/Response API format:
// - Standard: { "prompt_tokens": 100, "completion_tokens": 50, "total_tokens": 150 }
// - Response API: { "input_tokens": 100, "output_tokens": 50, "input_tokens_details": {...} }
func extractOpenAIUsage(usage map[string]interface{}) *Metrics {
	metrics := &Metrics{}

	// Standard OpenAI format: prompt_tokens → Input tokens
	if v, ok := usage["prompt_tokens"].(float64); ok {
		metrics.InputTokens = uint64(v)
	}

	// Response API / Codex format: input_tokens (may include cached tokens)
	if v, ok := usage["input_tokens"].(float64); ok {
		metrics.InputTokens = uint64(v)
	}

	// Standard OpenAI format: completion_tokens → Output tokens
	if v, ok := usage["completion_tokens"].(float64); ok {
		metrics.OutputTokens = uint64(v)
	}

	// Response API / Codex format: output_tokens
	if v, ok := usage["output_tokens"].(float64); ok {
		metrics.OutputTokens = uint64(v)
	}

	// OpenAI Response API format: prompt_tokens_details.cached_tokens
	if details, ok := usage["prompt_tokens_details"].(map[string]interface{}); ok {
		if v, ok := details["cached_tokens"].(float64); ok {
			metrics.CacheReadCount = uint64(v)
		}
	}

	// Alternative: input_tokens_details (Codex format)
	if details, ok := usage["input_tokens_details"].(map[string]interface{}); ok {
		if v, ok := details["cached_tokens"].(float64); ok {
			metrics.CacheReadCount = uint64(v)
		}
	}

	// Also check top-level cache_read_input_tokens (some relays use this)
	if v, ok := usage["cache_read_input_tokens"].(float64); ok {
		metrics.CacheReadCount = uint64(v)
	}

	return metrics
}

// extractGeminiUsage extracts metrics from Gemini usage format.
// Example: { "promptTokenCount": 100, "candidatesTokenCount": 50, "cachedContentTokenCount": 20 }
func extractGeminiUsage(usage map[string]interface{}) *Metrics {
	metrics := &Metrics{}

	// Gemini: promptTokenCount includes cachedContentTokenCount
	// To get actual input tokens, we need to subtract cached tokens
	var promptTokens uint64
	var cachedTokens uint64

	if v, ok := usage["promptTokenCount"].(float64); ok {
		promptTokens = uint64(v)
	}

	if v, ok := usage["cachedContentTokenCount"].(float64); ok {
		cachedTokens = uint64(v)
		metrics.CacheReadCount = cachedTokens
	}

	// Input = prompt - cached (to avoid double counting)
	if promptTokens >= cachedTokens {
		metrics.InputTokens = promptTokens - cachedTokens
	}

	// Candidates (output) tokens
	if v, ok := usage["candidatesTokenCount"].(float64); ok {
		metrics.OutputTokens = uint64(v)
	}

	// Gemini thinking tokens (add to output)
	if v, ok := usage["thoughtsTokenCount"].(float64); ok {
		metrics.OutputTokens += uint64(v)
	}

	return metrics
}

// ExtractFromStreamContent extracts usage from accumulated streaming content.
// This is useful when you've collected all SSE chunks into a single string.
func ExtractFromStreamContent(content string) *Metrics {
	return extractFromSSE(content)
}

// AdjustForClientType adjusts metrics based on client type specific quirks.
// For Codex: input_tokens includes cached_tokens, so we subtract to avoid double counting.
// For other clients: returns metrics unchanged.
func AdjustForClientType(metrics *Metrics, clientType domain.ClientType) *Metrics {
	if metrics == nil {
		return nil
	}

	// Codex/OpenAI Response API: input_tokens includes cached_tokens
	// We need to subtract to get actual input tokens (avoiding double billing)
	if clientType == domain.ClientTypeCodex {
		if metrics.CacheReadCount > 0 && metrics.InputTokens >= metrics.CacheReadCount {
			metrics.InputTokens = metrics.InputTokens - metrics.CacheReadCount
		}
	}

	return metrics
}
