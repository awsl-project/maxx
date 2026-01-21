package antigravity

import (
	"log"
	"strings"
)

// buildTools converts Claude tools to Gemini tools format
// Reference: Antigravity-Manager's build_tools
func buildTools(claudeReq *ClaudeRequest) interface{} {
	if len(claudeReq.Tools) == 0 {
		return nil
	}

	functionDeclarations := []map[string]interface{}{}
	hasWebSearch := false

	for _, tool := range claudeReq.Tools {
		// 1. Detect server-side tools (Web Search)
		if isWebSearchTool(tool) {
			hasWebSearch = true
			continue
		}

		// Server tools may omit name; only client tools are converted to functionDeclarations.
		if strings.TrimSpace(tool.Name) == "" {
			continue
		}

		// 2. Client-side tools: clean input_schema
		inputSchema := tool.InputSchema
		if inputSchema == nil {
			inputSchema = map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			}
		}

		// Deep copy to avoid modifying original
		cleanedSchema := deepCopyMap(inputSchema)
		CleanJSONSchema(cleanedSchema)

		functionDeclarations = append(functionDeclarations, map[string]interface{}{
			"name":        tool.Name,
			"description": tool.Description,
			"parameters":  cleanedSchema,
		})
	}

	// 3. Build tools object
	// [CRITICAL FIX] Gemini v1internal does NOT allow mixing functionDeclarations and googleSearch
	// in the same tool object. Must choose one or the other.
	// Reference: Antigravity-Manager lines 906-921
	if len(functionDeclarations) == 0 && !hasWebSearch {
		return nil
	}

	toolObj := make(map[string]interface{})

	if len(functionDeclarations) > 0 {
		// If we have client-side tools, ONLY use functionDeclarations
		// Skip googleSearch injection to avoid 400 error
		toolObj["functionDeclarations"] = functionDeclarations

		if hasWebSearch {
			// Log that we're skipping googleSearch due to existing function declarations
			// Gemini v1internal does not support mixed tool types
			log.Printf("[Antigravity] Skipping googleSearch injection due to %d existing function declarations. "+
				"Gemini v1internal does not support mixed tool types.", len(functionDeclarations))
		}
	} else if hasWebSearch {
		// Only inject googleSearch when there are NO client-side tools
		toolObj["googleSearch"] = map[string]interface{}{}
	}

	return []map[string]interface{}{toolObj}
}

// isWebSearchTool checks if a tool is a Web Search tool
// These are server-side tools that should be converted to googleSearch
func isWebSearchTool(tool ClaudeTool) bool {
	// Server tools: type starts with "web_search" (preferred)
	if strings.HasPrefix(strings.ToLower(tool.Type), "web_search") {
		return true
	}

	// Fallback: name-based detection (includes legacy "google_search")
	switch strings.ToLower(tool.Name) {
	case "web_search", "google_search":
		return true
	default:
		return false
	}
}

// hasWebSearchTool checks if the request contains any Web Search tools
func hasWebSearchTool(claudeReq *ClaudeRequest) bool {
	if claudeReq.Tools == nil {
		return false
	}

	for _, tool := range claudeReq.Tools {
		if isWebSearchTool(tool) {
			return true
		}
	}

	return false
}

// deepCopyMap creates a deep copy of a map to avoid modifying original data
func deepCopyMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}

	dst := make(map[string]interface{}, len(src))

	for key, value := range src {
		switch v := value.(type) {
		case map[string]interface{}:
			dst[key] = deepCopyMap(v)
		case []interface{}:
			dst[key] = deepCopySlice(v)
		default:
			dst[key] = v
		}
	}

	return dst
}

// deepCopySlice creates a deep copy of a slice
func deepCopySlice(src []interface{}) []interface{} {
	if src == nil {
		return nil
	}

	dst := make([]interface{}, len(src))

	for i, value := range src {
		switch v := value.(type) {
		case map[string]interface{}:
			dst[i] = deepCopyMap(v)
		case []interface{}:
			dst[i] = deepCopySlice(v)
		default:
			dst[i] = v
		}
	}

	return dst
}
