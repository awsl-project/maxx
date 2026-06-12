package custom

import (
	"net/http"
	"strings"
)

const (
	defaultAnthropicVersion = "2023-06-01"
	defaultClaudeUserAgent  = "claude-cli/2.1.17 (external, cli)"

	// defaultBedrockCompatUserAgent mimics the User-Agent string emitted by a real
	// AWS SDK for Go v2 Bedrock Runtime client. Used by the "bedrock" disguise mode
	// so that upstream relay stations forwarding to AWS Bedrock won't reject the
	// request based on a Claude Code fingerprint.
	defaultBedrockCompatUserAgent = "aws-sdk-go-v2/1.32.6 os/linux lang/go#1.23.0 md/GOOS#linux md/GOARCH#amd64 api/bedrockruntime#1.20.0"
)

// claudeIdentityHeaders is the set of request headers that fingerprint a request
// as coming from Claude Code CLI. The "bedrock" disguise mode strips these so the
// upstream Bedrock backend won't see Claude Code identifiers.
var claudeIdentityHeaders = []string{
	"Anthropic-Beta",
	"Anthropic-Version",
	"Anthropic-Dangerous-Direct-Browser-Access",
	"X-App",
	"X-Stainless-Helper-Method",
	"X-Stainless-Retry-Count",
	"X-Stainless-Runtime-Version",
	"X-Stainless-Package-Version",
	"X-Stainless-Runtime",
	"X-Stainless-Lang",
	"X-Stainless-Arch",
	"X-Stainless-Os",
	"X-Stainless-Timeout",
}

// setClaudeAuthForURL writes the Claude-style provider auth header for a request,
// choosing between `x-api-key` (direct api.anthropic.com) and `Authorization: Bearer`
// (every other host) the same way applyClaudeHeaders does. Used by both
// applyClaudeHeaders and the `none` disguise raw-forwarding path so a force-create
// auth injection on a non-Anthropic relay still produces the Bearer header that
// such relays expect.
//
// Before writing the new header, every credential header that the source client
// might have sent is unconditionally deleted (Authorization / x-api-key /
// x-goog-api-key). This matters for the raw-forwarding path: an OpenAI- or
// Gemini-origin request that gets converted to Claude must not leak its source
// credential alongside the provider key.
func setClaudeAuthForURL(req *http.Request, apiKey string, useAPIKey bool) {
	if apiKey == "" {
		return
	}
	// We own auth from here on. Clear every credential header that might
	// have survived header copying so the upstream sees only the provider key.
	req.Header.Del("Authorization")
	req.Header.Del("x-api-key")
	req.Header.Del("x-goog-api-key")

	isAnthropicBase := req.URL != nil &&
		strings.EqualFold(req.URL.Scheme, "https") &&
		strings.EqualFold(req.URL.Host, "api.anthropic.com")
	if isAnthropicBase && useAPIKey {
		req.Header.Set("x-api-key", apiKey)
	} else {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
}

// applyClaudeHeaders sets Claude API request headers.
// Following CLIProxyAPI pattern: build headers from scratch, use EnsureHeader for selective passthrough.
func applyClaudeHeaders(req *http.Request, clientReq *http.Request, apiKey string, useAPIKey bool, extraBetas []string, stream bool) {
	// Get client headers for EnsureHeader
	var clientHeaders http.Header
	if clientReq != nil {
		clientHeaders = clientReq.Header
	}

	// 1. Set authentication (only if apiKey is provided)
	setClaudeAuthForURL(req, apiKey, useAPIKey)

	// 2. Set Content-Type (always)
	req.Header.Set("Content-Type", "application/json")

	// 4. Build Anthropic-Beta header
	promptCachingBeta := "prompt-caching-2024-07-31"
	baseBetas := "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,fine-grained-tool-streaming-2025-05-14," + promptCachingBeta
	if clientHeaders != nil {
		if val := strings.TrimSpace(clientHeaders.Get("Anthropic-Beta")); val != "" {
			baseBetas = val
			if !strings.Contains(val, "oauth") {
				baseBetas += ",oauth-2025-04-20"
			}
		}
	}
	if !strings.Contains(baseBetas, promptCachingBeta) {
		baseBetas += "," + promptCachingBeta
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

	// 5. Set headers (allow client passthrough, fallback to defaults)
	ensureHeader(req.Header, clientHeaders, "Anthropic-Version", defaultAnthropicVersion)
	ensureHeader(req.Header, clientHeaders, "Anthropic-Dangerous-Direct-Browser-Access", "true")
	ensureHeader(req.Header, clientHeaders, "X-App", "cli")
	ensureHeader(req.Header, clientHeaders, "X-Stainless-Helper-Method", "stream")
	ensureHeader(req.Header, clientHeaders, "X-Stainless-Retry-Count", "0")
	ensureHeader(req.Header, clientHeaders, "X-Stainless-Runtime-Version", "v24.3.0")
	ensureHeader(req.Header, clientHeaders, "X-Stainless-Package-Version", "0.55.1")
	ensureHeader(req.Header, clientHeaders, "X-Stainless-Runtime", "node")
	ensureHeader(req.Header, clientHeaders, "X-Stainless-Lang", "js")
	ensureHeader(req.Header, clientHeaders, "X-Stainless-Arch", "arm64")
	ensureHeader(req.Header, clientHeaders, "X-Stainless-Os", "MacOS")
	ensureHeader(req.Header, clientHeaders, "X-Stainless-Timeout", "60")

	clientUA := ""
	if clientHeaders != nil {
		clientUA = clientHeaders.Get("User-Agent")
	}
	if strings.TrimSpace(clientUA) != "" {
		req.Header.Set("User-Agent", clientUA)
	} else {
		req.Header.Set("User-Agent", defaultClaudeUserAgent)
	}

	// 6. Set connection and encoding headers (always override)
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")

	// 7. Set Accept based on stream flag
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
}

// applyBedrockCompatHeaders sets request headers for the "bedrock" disguise mode.
// Drops every Claude Code fingerprint header and replaces the User-Agent with a
// neutral AWS SDK string. Used when forwarding to a relay station whose backend
// is AWS Bedrock — such relays reject requests carrying claude-code-* beta flags
// and x-stainless-* SDK markers.
//
// The clientReq parameter is unused (the dispatch builds upstreamReq from
// scratch in adapter.go) but is kept for signature symmetry with
// applyClaudeHeaders / applyCodexHeaders / applyGeminiHeaders.
func applyBedrockCompatHeaders(req *http.Request, _ *http.Request, apiKey string, stream bool) {
	// 1. Authentication — unconditionally clear any pre-populated auth headers
	// so caller-side state can never leak through. Then set the provider's key
	// as a Bearer token if one is configured. Relay stations behind Bedrock
	// typically still accept a Bearer token even though direct AWS Bedrock
	// would require SigV4 — the relay re-signs internally.
	req.Header.Del("x-api-key")
	req.Header.Del("Authorization")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	// 2. Content-Type
	req.Header.Set("Content-Type", "application/json")

	// 3. Strip every Claude Code fingerprint header so the upstream Bedrock
	// backend doesn't see Claude Code identifiers.
	for _, h := range claudeIdentityHeaders {
		req.Header.Del(h)
	}

	// 4. User-Agent: pretend to be aws-sdk-go-v2's Bedrock Runtime client.
	req.Header.Set("User-Agent", defaultBedrockCompatUserAgent)

	// 5. Connection / encoding (match the AWS SDK defaults loosely)
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Accept-Encoding", "identity")

	// 6. Accept based on stream flag.
	// Even in bedrock disguise mode the relay station's outward-facing API is
	// Anthropic-compatible (the relay translates AWS Event Stream binary frames
	// from the actual Bedrock backend back into SSE before responding to us),
	// and CustomAdapter.handleStreamResponse only knows how to parse SSE text
	// lines. Asking for application/vnd.amazon.eventstream here would either
	// confuse the relay or, if the relay honored it, return binary frames the
	// stream parser couldn't decode.
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
}

// mergeBetaList merges an existing comma-separated Anthropic-Beta header value
// with an additional list of beta strings, deduplicating in first-occurrence
// order. Empty entries are dropped. Returns the merged comma-joined string.
func mergeBetaList(existing string, extra []string) string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(extra)+4)
	add := func(b string) {
		b = strings.TrimSpace(b)
		if b == "" || seen[b] {
			return
		}
		seen[b] = true
		out = append(out, b)
	}
	for _, b := range strings.Split(existing, ",") {
		add(b)
	}
	for _, b := range extra {
		add(b)
	}
	return strings.Join(out, ",")
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
