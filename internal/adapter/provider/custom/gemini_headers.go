package custom

import (
	"net/http"
	"strings"
)

const (
	// User-Agent for Gemini API requests
	// Mimics Google AI SDK style
	geminiUserAgent = "google-ai-sdk/0.1.0"
)

// applyGeminiHeaders sets Gemini API request headers
// Unlike Claude/Codex, Gemini uses a simpler header set
func applyGeminiHeaders(upstreamReq, clientReq *http.Request, apiKey string) {
	// 1. Copy passthrough headers from client request (excluding hop-by-hop and auth)
	if clientReq != nil {
		copyGeminiPassthroughHeaders(upstreamReq.Header, clientReq.Header)
	}

	// 2. Set required headers
	upstreamReq.Header.Set("Content-Type", "application/json")

	// 3. Set authentication (only if apiKey is provided)
	// Gemini uses x-goog-api-key for API key auth
	if apiKey != "" {
		upstreamReq.Header.Set("x-goog-api-key", apiKey)
		// Remove Authorization header if we're using x-goog-api-key
		upstreamReq.Header.Del("Authorization")
	}

	// 4. Set User-Agent if client didn't provide one
	if clientReq == nil || clientReq.Header.Get("User-Agent") == "" {
		upstreamReq.Header.Set("User-Agent", geminiUserAgent)
	}

	// 5. Set Accept header based on URL (streaming or not)
	if strings.Contains(upstreamReq.URL.String(), "streamGenerateContent") {
		upstreamReq.Header.Set("Accept", "text/event-stream")
	} else {
		upstreamReq.Header.Set("Accept", "application/json")
	}
}

// copyGeminiPassthroughHeaders copies headers from client request, excluding hop-by-hop, auth, and proxy headers
func copyGeminiPassthroughHeaders(dst, src http.Header) {
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
		"authorization":  true,
		"x-goog-api-key": true,

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
