package custom

import (
	"net/http"
	"strings"
)

const (
	// Codex API version
	codexVersion = "0.21.0"

	// User-Agent mimics Codex CLI
	codexUserAgent = "codex_cli_rs/0.50.0 (Mac OS 26.0.1; arm64)"

	// Originator header
	codexOriginator = "codex_cli_rs"

	// OpenAI Beta header
	openAIBetaHeader = "responses=experimental"
)

// applyCodexHeaders sets Codex API request headers, mimicking the official Codex CLI
// It follows the pattern: passthrough client headers, use defaults only when missing
func applyCodexHeaders(upstreamReq, clientReq *http.Request, apiKey string) {
	// 1. Copy passthrough headers from client request (excluding hop-by-hop and auth)
	if clientReq != nil {
		copyCodexPassthroughHeaders(upstreamReq.Header, clientReq.Header)
	}

	// 2. Set required headers (these always override)
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Accept", "text/event-stream")
	upstreamReq.Header.Set("Connection", "Keep-Alive")

	// 3. Set authentication (only if apiKey is provided)
	if apiKey != "" {
		upstreamReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	// 4. Set Codex-specific headers only if client didn't provide them
	ensureCodexHeader(upstreamReq.Header, clientReq, "Version", codexVersion)
	ensureCodexHeader(upstreamReq.Header, clientReq, "Openai-Beta", openAIBetaHeader)
	ensureCodexHeader(upstreamReq.Header, clientReq, "User-Agent", codexUserAgent)
	ensureCodexHeader(upstreamReq.Header, clientReq, "Originator", codexOriginator)
}

// copyCodexPassthroughHeaders copies headers from client request, excluding hop-by-hop, auth, and proxy headers
func copyCodexPassthroughHeaders(dst, src http.Header) {
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

		// Auth headers
		"authorization": true,

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

// ensureCodexHeader sets a header only if the client request doesn't already have it
func ensureCodexHeader(dst http.Header, clientReq *http.Request, key, defaultValue string) {
	if clientReq != nil && clientReq.Header.Get(key) != "" {
		// Client provided this header, it's already copied, don't override
		return
	}
	dst.Set(key, defaultValue)
}
