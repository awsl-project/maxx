package custom

import (
	"net/http"
	"strings"
)

const (
	// Default values used only when client doesn't provide them
	defaultAnthropicVersion   = "2023-06-01"
	defaultAnthropicBetaFlags = "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,fine-grained-tool-streaming-2025-05-14"
	defaultClaudeUserAgent    = "claude-cli/1.0.83 (external, cli)"
)

// applyClaudeHeaders sets Claude API request headers
// It follows the CLIProxyAPI pattern: passthrough client headers, use defaults only when missing
func applyClaudeHeaders(req *http.Request, clientReq *http.Request, apiKey string, stream bool) {
	// 1. Copy passthrough headers from client request (excluding hop-by-hop and auth)
	if clientReq != nil {
		copyClaudePassthroughHeaders(req.Header, clientReq.Header)
	}

	// 2. Set authentication based on client's auth header type (only if apiKey is provided)
	if apiKey != "" {
		// Determine which auth header the client used
		if clientReq != nil && clientReq.Header.Get("x-api-key") != "" {
			// Client used x-api-key style
			req.Header.Del("Authorization")
			req.Header.Set("x-api-key", apiKey)
		} else if clientReq != nil && clientReq.Header.Get("Authorization") != "" {
			// Client used Authorization style
			req.Header.Del("x-api-key")
			req.Header.Set("Authorization", "Bearer "+apiKey)
		} else {
			// No client auth header, default to x-api-key for Claude API
			req.Header.Set("x-api-key", apiKey)
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
	ensureClaudeHeader(req.Header, clientReq, "X-Stainless-Helper-Method", "stream")
	ensureClaudeHeader(req.Header, clientReq, "X-Stainless-Retry-Count", "0")
	ensureClaudeHeader(req.Header, clientReq, "X-Stainless-Runtime-Version", "v24.3.0")
	ensureClaudeHeader(req.Header, clientReq, "X-Stainless-Package-Version", "0.55.1")
	ensureClaudeHeader(req.Header, clientReq, "X-Stainless-Runtime", "node")
	ensureClaudeHeader(req.Header, clientReq, "X-Stainless-Lang", "js")
	ensureClaudeHeader(req.Header, clientReq, "X-Stainless-Arch", "arm64")
	ensureClaudeHeader(req.Header, clientReq, "X-Stainless-Os", "MacOS")
	ensureClaudeHeader(req.Header, clientReq, "X-Stainless-Timeout", "60")

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

// copyClaudePassthroughHeaders copies headers from client request, excluding hop-by-hop, auth, and proxy headers
func copyClaudePassthroughHeaders(dst, src http.Header) {
	if src == nil {
		return
	}

	// Headers to skip (hop-by-hop, auth, proxy/privacy, and headers we'll set explicitly)
	skipHeaders := map[string]bool{
		// Hop-by-hop headers
		"connection":        true,
		"keep-alive":        true,
		"transfer-encoding": true,
		"upgrade":           true,

		// Auth headers (we set these explicitly)
		"authorization": true,
		"x-api-key":     true,

		// Headers set by HTTP client
		"host":           true,
		"content-length": true,

		// Proxy/forwarding headers (privacy protection)
		"x-forwarded-for":    true,
		"x-forwarded-host":   true,
		"x-forwarded-proto":  true,
		"x-forwarded-port":   true,
		"x-forwarded-server": true,
		"x-real-ip":          true,
		"x-client-ip":        true,
		"x-originating-ip":   true,
		"x-remote-ip":        true,
		"x-remote-addr":      true,
		"forwarded":          true,

		// CDN/Cloud provider headers
		"cf-connecting-ip": true,
		"cf-ipcountry":     true,
		"cf-ray":           true,
		"cf-visitor":       true,
		"true-client-ip":   true,
		"fastly-client-ip": true,
		"x-azure-clientip": true,
		"x-azure-fdid":     true,
		"x-azure-ref":      true,

		// Tracing headers
		"x-request-id":      true,
		"x-correlation-id":  true,
		"x-trace-id":        true,
		"x-amzn-trace-id":   true,
		"x-b3-traceid":      true,
		"x-b3-spanid":       true,
		"x-b3-parentspanid": true,
		"x-b3-sampled":      true,
		"traceparent":       true,
		"tracestate":        true,
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
