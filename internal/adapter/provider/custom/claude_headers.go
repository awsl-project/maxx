package custom

import (
	"net/http"
	"strings"
)

const (
	// Default values used only when client doesn't provide them
	defaultAnthropicVersion   = "2023-06-01"
	defaultAnthropicBetaFlags = "claude-code-20250219,interleaved-thinking-2025-05-14"
	defaultClaudeUserAgent    = "claude-cli/2.1.17 (external, cli)"
)

// applyClaudeHeaders sets Claude API request headers
// It follows the CLIProxyAPI pattern: passthrough client headers, use defaults only when missing
func applyClaudeHeaders(req *http.Request, clientReq *http.Request, apiKey string, stream bool) {
	// 1. Copy passthrough headers from client request (excluding hop-by-hop and auth)
	if clientReq != nil {
		copyClaudePassthroughHeaders(req.Header, clientReq.Header)
	}

	// 2. Set authentication (only if apiKey is provided, always override)
	if apiKey != "" {
		if isAnthropicAPI(req.URL.String()) {
			req.Header.Del("Authorization")
			req.Header.Set("x-api-key", apiKey)
		} else {
			req.Header.Del("x-api-key")
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
	}

	// 3. Set required headers (always override)
	req.Header.Set("Content-Type", "application/json")

	// 4. Set Claude-specific headers only if client didn't provide them
	ensureClaudeHeader(req.Header, clientReq, "Anthropic-Version", defaultAnthropicVersion)
	ensureClaudeHeader(req.Header, clientReq, "Anthropic-Beta", defaultAnthropicBetaFlags)
	ensureClaudeHeader(req.Header, clientReq, "Anthropic-Dangerous-Direct-Browser-Access", "true")
	ensureClaudeHeader(req.Header, clientReq, "User-Agent", defaultClaudeUserAgent)

	// 5. Set Stainless headers only if client didn't provide them
	ensureClaudeHeader(req.Header, clientReq, "X-App", "cli")
	ensureClaudeHeader(req.Header, clientReq, "X-Stainless-Retry-Count", "0")
	ensureClaudeHeader(req.Header, clientReq, "X-Stainless-Runtime-Version", "v24.3.0")
	ensureClaudeHeader(req.Header, clientReq, "X-Stainless-Package-Version", "0.70.0")
	ensureClaudeHeader(req.Header, clientReq, "X-Stainless-Runtime", "node")
	ensureClaudeHeader(req.Header, clientReq, "X-Stainless-Lang", "js")
	ensureClaudeHeader(req.Header, clientReq, "X-Stainless-Arch", "arm64")
	ensureClaudeHeader(req.Header, clientReq, "X-Stainless-Os", "MacOS")
	ensureClaudeHeader(req.Header, clientReq, "X-Stainless-Timeout", "600")

	// 6. Set Accept-Encoding if client didn't provide
	ensureClaudeHeader(req.Header, clientReq, "Accept-Encoding", "gzip, deflate, br, zstd")

	// 7. Set Accept based on stream mode (only if client didn't provide)
	if clientReq == nil || clientReq.Header.Get("Accept") == "" {
		if stream {
			req.Header.Set("Accept", "text/event-stream")
		} else {
			req.Header.Set("Accept", "application/json")
		}
	}
}

// copyClaudePassthroughHeaders copies headers from client request, excluding hop-by-hop and auth
func copyClaudePassthroughHeaders(dst, src http.Header) {
	if src == nil {
		return
	}

	// Headers to skip (hop-by-hop, auth, and headers we'll set explicitly)
	skipHeaders := map[string]bool{
		"connection":        true,
		"keep-alive":        true,
		"transfer-encoding": true,
		"upgrade":           true,
		"authorization":     true,
		"x-api-key":         true,
		"host":              true,
		"content-length":    true,
	}

	for k, vv := range src {
		if skipHeaders[strings.ToLower(k)] {
			continue
		}
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

// ensureClaudeHeader sets a header only if the client request doesn't already have it
func ensureClaudeHeader(dst http.Header, clientReq *http.Request, key, defaultValue string) {
	if clientReq != nil && clientReq.Header.Get(key) != "" {
		// Client provided this header, it's already copied, don't override
		return
	}
	dst.Set(key, defaultValue)
}

func isAnthropicAPI(url string) bool {
	return strings.Contains(url, "api.anthropic.com")
}
