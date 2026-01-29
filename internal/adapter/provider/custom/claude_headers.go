package custom

import (
	"net/http"
	"strings"
)

const (
	defaultAnthropicVersion = "2023-06-01"
	defaultClaudeUserAgent  = "claude-cli/2.1.17 (external, cli)"
)

// applyClaudeHeaders sets Claude API request headers
// Following CLIProxyAPI pattern: build headers from scratch, use EnsureHeader for selective client passthrough
func applyClaudeHeaders(req *http.Request, clientReq *http.Request, apiKey string, stream bool, extraBetas []string) {
	// Get client headers for EnsureHeader
	var clientHeaders http.Header
	if clientReq != nil {
		clientHeaders = clientReq.Header
	}

	// 1. Set authentication (only if apiKey is provided)
	if apiKey != "" {
		// Determine which auth header the client used
		if clientReq != nil && clientReq.Header.Get("x-api-key") != "" {
			req.Header.Del("Authorization")
			req.Header.Set("x-api-key", apiKey)
		} else if clientReq != nil && clientReq.Header.Get("Authorization") != "" {
			req.Header.Del("x-api-key")
			req.Header.Set("Authorization", "Bearer "+apiKey)
		} else {
			// Default to x-api-key for Claude API
			req.Header.Set("x-api-key", apiKey)
		}
	}

	// 2. Set Content-Type (always)
	req.Header.Set("Content-Type", "application/json")

	// 3. Build Anthropic-Beta header
	baseBetas := "claude-code-20250219,interleaved-thinking-2025-05-14"
	if clientHeaders != nil {
		if val := strings.TrimSpace(clientHeaders.Get("Anthropic-Beta")); val != "" {
			baseBetas = val
		}
	}

	// Merge extra betas from request body
	if len(extraBetas) > 0 {
		existingSet := make(map[string]bool)
		for _, b := range strings.Split(baseBetas, ",") {
			existingSet[strings.TrimSpace(b)] = true
		}
		for _, beta := range extraBetas {
			beta = strings.TrimSpace(beta)
			if beta != "" && !existingSet[beta] {
				baseBetas += "," + beta
				existingSet[beta] = true
			}
		}
	}
	req.Header.Set("Anthropic-Beta", baseBetas)

	// 4. Set headers with EnsureHeader pattern (client value takes priority, then default)
	ensureHeader(req.Header, clientHeaders, "Anthropic-Version", defaultAnthropicVersion)
	ensureHeader(req.Header, clientHeaders, "Anthropic-Dangerous-Direct-Browser-Access", "true")
	ensureHeader(req.Header, clientHeaders, "X-App", "cli")
	ensureHeader(req.Header, clientHeaders, "X-Stainless-Retry-Count", "0")
	ensureHeader(req.Header, clientHeaders, "X-Stainless-Runtime-Version", "v24.3.0")
	ensureHeader(req.Header, clientHeaders, "X-Stainless-Package-Version", "0.70.0")
	ensureHeader(req.Header, clientHeaders, "X-Stainless-Runtime", "node")
	ensureHeader(req.Header, clientHeaders, "X-Stainless-Lang", "js")
	ensureHeader(req.Header, clientHeaders, "X-Stainless-Arch", "arm64")
	ensureHeader(req.Header, clientHeaders, "X-Stainless-Os", "MacOS")
	ensureHeader(req.Header, clientHeaders, "X-Stainless-Timeout", "600")
	ensureHeader(req.Header, clientHeaders, "User-Agent", defaultClaudeUserAgent)

	// 5. Set connection and encoding headers (always override)
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")

	// 6. Set Accept based on stream mode
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
}

// ensureHeader sets a header value with priority: source > target existing > default
// This matches CLIProxyAPI's misc.EnsureHeader behavior
func ensureHeader(target http.Header, source http.Header, key, defaultValue string) {
	if target == nil {
		return
	}
	// Priority 1: Use source value if available
	if source != nil {
		if val := strings.TrimSpace(source.Get(key)); val != "" {
			target.Set(key, val)
			return
		}
	}
	// Priority 2: Keep existing target value
	if strings.TrimSpace(target.Get(key)) != "" {
		return
	}
	// Priority 3: Use default value
	if val := strings.TrimSpace(defaultValue); val != "" {
		target.Set(key, val)
	}
}
