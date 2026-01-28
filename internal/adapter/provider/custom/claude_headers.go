package custom

import (
	"net/http"
	"strings"
)

const (
	anthropicVersion   = "2023-06-01"
	anthropicBetaFlags = "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,fine-grained-tool-streaming-2025-05-14"
	claudeUserAgent    = "claude-cli/1.0.83 (external, cli)"
)

// applyClaudeHeaders sets Claude API request headers, mimicking the official CLI
func applyClaudeHeaders(req *http.Request, apiKey string, stream bool) {
	// 1. Authentication header (only set if apiKey is provided)
	if apiKey != "" {
		if isAnthropicAPI(req.URL.String()) {
			req.Header.Del("Authorization")
			req.Header.Set("x-api-key", apiKey)
		} else {
			req.Header.Del("x-api-key")
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
	}

	// 2. Core headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Anthropic-Version", anthropicVersion)
	req.Header.Set("Anthropic-Beta", anthropicBetaFlags)
	req.Header.Set("Anthropic-Dangerous-Direct-Browser-Access", "true")

	// 3. Stainless headers (mimics official Node.js SDK)
	req.Header.Set("X-App", "cli")
	req.Header.Set("X-Stainless-Helper-Method", "stream")
	req.Header.Set("X-Stainless-Retry-Count", "0")
	req.Header.Set("X-Stainless-Runtime-Version", "v24.3.0")
	req.Header.Set("X-Stainless-Package-Version", "0.55.1")
	req.Header.Set("X-Stainless-Runtime", "node")
	req.Header.Set("X-Stainless-Lang", "js")
	req.Header.Set("X-Stainless-Arch", "arm64")
	req.Header.Set("X-Stainless-Os", "MacOS")
	req.Header.Set("X-Stainless-Timeout", "60")
	req.Header.Set("User-Agent", claudeUserAgent)

	// 4. Connection and encoding
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")

	if stream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
}

func isAnthropicAPI(url string) bool {
	return strings.Contains(url, "api.anthropic.com")
}
