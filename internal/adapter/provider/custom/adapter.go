package custom

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/usage"
)

func init() {
	provider.RegisterAdapterFactory("custom", NewAdapter)
}

type CustomAdapter struct {
	provider *domain.Provider
}

func NewAdapter(p *domain.Provider) (provider.ProviderAdapter, error) {
	if p.Config == nil || p.Config.Custom == nil {
		return nil, fmt.Errorf("provider %s missing custom config", p.Name)
	}
	return &CustomAdapter{
		provider: p,
	}, nil
}

func (a *CustomAdapter) SupportedClientTypes() []domain.ClientType {
	return a.provider.SupportedClientTypes
}

func (a *CustomAdapter) Execute(c *flow.Ctx, provider *domain.Provider) error {
	clientType := flow.GetClientType(c)
	mappedModel := flow.GetMappedModel(c)
	requestBody := flow.GetRequestBody(c)
	request := c.Request
	ctx := context.Background()
	if request != nil {
		ctx = request.Context()
	}

	// Determine if streaming
	stream := isStreamRequest(requestBody)

	// Note: Format conversion is now handled by Executor layer
	// The clientType in context is already the correct type that this provider supports
	// We use clientType directly for URL building and auth header selection

	// Build upstream URL
	baseURL := a.getBaseURL(clientType)
	requestURI := flow.GetRequestURI(c)

	// Apply model mapping if configured
	var err error
	if mappedModel != "" {
		// For Gemini, update model in URL path
		if clientType == domain.ClientTypeGemini {
			requestURI = updateGeminiModelInPath(requestURI, mappedModel)
		}
		// For other types, update model in request body
		requestBody, err = updateModelInBody(requestBody, mappedModel, clientType)
		if err != nil {
			return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to update model in body")
		}
	}

	upstreamURL := buildUpstreamURL(baseURL, requestURI)

	// For Claude, add query parameters (following CLIProxyAPI)
	if clientType == domain.ClientTypeClaude {
		upstreamURL = addClaudeQueryParams(upstreamURL)
	}

	// Create upstream request
	upstreamReq, err := http.NewRequestWithContext(ctx, "POST", upstreamURL, bytes.NewReader(requestBody))
	if err != nil {
		return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to create upstream request")
	}

	// Set headers based on client type
	isOAuthToken := false
	switch clientType {
	case domain.ClientTypeClaude:
		// Claude: Following CLIProxyAPI pattern
		// 1. Process body first (get extraBetas, inject cloaking/cache_control)
		apiKey := a.provider.Config.Custom.APIKey
		clientUA := ""
		if request != nil {
			clientUA = request.Header.Get("User-Agent")
		}
		var extraBetas []string
		requestBody, extraBetas = processClaudeRequestBody(requestBody, clientUA, a.provider.Config.Custom.Cloak)
		useAPIKey := shouldUseClaudeAPIKey(apiKey, request)
		isOAuthToken = isClaudeOAuthToken(apiKey)
		if isOAuthToken {
			requestBody = applyClaudeToolPrefix(requestBody, claudeToolPrefix)
		}

		// 2. Set headers (streaming only if requested)
		applyClaudeHeaders(upstreamReq, request, apiKey, useAPIKey, extraBetas, stream)

		// 3. Update request body and ContentLength (IMPORTANT: body was modified)
		upstreamReq.Body = io.NopCloser(bytes.NewReader(requestBody))
		upstreamReq.ContentLength = int64(len(requestBody))
	case domain.ClientTypeCodex:
		// Codex: Use Codex CLI-style headers with passthrough support
		applyCodexHeaders(upstreamReq, request, a.provider.Config.Custom.APIKey)
	case domain.ClientTypeGemini:
		// Gemini: Use Gemini-style headers with passthrough support
		applyGeminiHeaders(upstreamReq, request, a.provider.Config.Custom.APIKey)
	default:
		// Other types: Preserve original header forwarding logic
		originalHeaders := flow.GetRequestHeaders(c)
		upstreamReq.Header = make(http.Header)
		copyHeadersFiltered(upstreamReq.Header, originalHeaders)

		// Override auth headers with provider's credentials
		if a.provider.Config.Custom.APIKey != "" {
			originalClientType := flow.GetOriginalClientType(c)
			isConversion := originalClientType != "" && originalClientType != clientType
			setAuthHeader(upstreamReq, clientType, a.provider.Config.Custom.APIKey, isConversion)
		}
	}

	// Send request info via EventChannel
	if eventChan := flow.GetEventChan(c); eventChan != nil {
		eventChan.SendRequestInfo(&domain.RequestInfo{
			Method:  upstreamReq.Method,
			URL:     upstreamURL,
			Headers: flattenHeaders(upstreamReq.Header),
			Body:    string(requestBody),
		})
	}

	// Execute request with reasonable timeout
	client := &http.Client{
		Timeout: 10 * time.Minute, // Long timeout for LLM requests
	}
	resp, err := client.Do(upstreamReq)
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to connect to upstream")
		proxyErr.IsNetworkError = true
		return proxyErr
	}
	resp.Body = flow.WrapResponseBody(c, resp.Body)
	defer resp.Body.Close()

	// Check for error response
	if resp.StatusCode >= 400 {
		// Decompress error response if needed (Claude requests use Accept-Encoding)
		reader, decompErr := decompressResponse(resp)
		if decompErr != nil {
			return domain.NewProxyErrorWithMessage(decompErr, false, "failed to decompress error response")
		}
		defer reader.Close()

		body, _ := io.ReadAll(reader)
		// Send error response info via EventChannel
		if eventChan := flow.GetEventChan(c); eventChan != nil {
			eventChan.SendResponseInfo(&domain.ResponseInfo{
				Status:  resp.StatusCode,
				Headers: flattenHeaders(resp.Header),
				Body:    string(body),
			})
		}

		proxyErr := domain.NewProxyErrorWithMessage(
			fmt.Errorf("upstream error: %s", string(body)),
			isRetryableStatusCode(resp.StatusCode),
			fmt.Sprintf("upstream returned status %d", resp.StatusCode),
		)

		// Set status code and check if it's a server error (5xx)
		proxyErr.HTTPStatusCode = resp.StatusCode
		proxyErr.IsServerError = resp.StatusCode >= 500 && resp.StatusCode < 600

		// Parse rate limit info for 429 errors
		if resp.StatusCode == http.StatusTooManyRequests {
			rateLimitInfo := parseRateLimitInfo(resp, body, clientType)
			if rateLimitInfo != nil {
				proxyErr.RateLimitInfo = rateLimitInfo
				applyRateLimitRetryHints(proxyErr, resp, rateLimitInfo)
			}
		}

		return proxyErr
	}

	// Handle response
	// Note: Response format conversion is handled by Executor's ConvertingResponseWriter
	// Adapters simply pass through the upstream response
	if stream {
		return a.handleStreamResponse(c, resp, clientType, isOAuthToken)
	}
	return a.handleNonStreamResponse(c, resp, clientType, isOAuthToken)
}

func (a *CustomAdapter) supportsClientType(ct domain.ClientType) bool {
	for _, supported := range a.provider.SupportedClientTypes {
		if supported == ct {
			return true
		}
	}
	return false
}

func (a *CustomAdapter) getBaseURL(clientType domain.ClientType) string {
	config := a.provider.Config.Custom
	if url, ok := config.ClientBaseURL[clientType]; ok && url != "" {
		return url
	}
	return config.BaseURL
}

func (a *CustomAdapter) handleNonStreamResponse(c *flow.Ctx, resp *http.Response, clientType domain.ClientType, isOAuthToken bool) error {
	// Decompress response body if needed
	reader, err := decompressResponse(resp)
	if err != nil {
		return domain.NewProxyErrorWithMessage(err, false, "failed to decompress response")
	}
	defer reader.Close()

	body, err := io.ReadAll(reader)
	if err != nil {
		return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to read upstream response")
	}
	// Claude API sometimes returns gzip without Content-Encoding header
	if len(body) >= 2 && body[0] == 0x1f && body[1] == 0x8b {
		if gzReader, gzErr := gzip.NewReader(bytes.NewReader(body)); gzErr == nil {
			if decompressed, readErr := io.ReadAll(gzReader); readErr == nil {
				body = decompressed
			}
			_ = gzReader.Close()
		}
	}
	if isOAuthToken {
		body = stripClaudeToolPrefixFromResponse(body, claudeToolPrefix)
	}

	eventChan := flow.GetEventChan(c)

	if eventChan != nil {
		eventChan.SendResponseInfo(&domain.ResponseInfo{
			Status:  resp.StatusCode,
			Headers: flattenHeaders(resp.Header),
			Body:    string(body),
		})
	}

	// Extract and send token usage metrics
	if metrics := usage.ExtractFromResponse(string(body)); metrics != nil {
		// Adjust for client-specific quirks (e.g., Codex input_tokens includes cached tokens)
		metrics = usage.AdjustForClientType(metrics, clientType)
		if eventChan != nil {
			eventChan.SendMetrics(&domain.AdapterMetrics{
				InputTokens:          metrics.InputTokens,
				OutputTokens:         metrics.OutputTokens,
				CacheReadCount:       metrics.CacheReadCount,
				CacheCreationCount:   metrics.CacheCreationCount,
				Cache5mCreationCount: metrics.Cache5mCreationCount,
				Cache1hCreationCount: metrics.Cache1hCreationCount,
			})
		}
	}

	// Extract and send responseModel
	if responseModel := extractResponseModel(body, clientType); responseModel != "" {
		if eventChan != nil {
			eventChan.SendResponseModel(responseModel)
		}
	}

	// Note: Response format conversion is handled by Executor's ConvertingResponseWriter
	// Adapter simply passes through the upstream response body

	// Copy upstream headers (except those we override)
	copyResponseHeaders(c.Writer.Header(), resp.Header)
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = c.Writer.Write(body)
	return nil
}

func (a *CustomAdapter) handleStreamResponse(c *flow.Ctx, resp *http.Response, clientType domain.ClientType, isOAuthToken bool) error {
	// Decompress response body if needed
	reader, err := decompressResponse(resp)
	if err != nil {
		return domain.NewProxyErrorWithMessage(err, false, "failed to decompress response")
	}
	defer reader.Close()

	eventChan := flow.GetEventChan(c)

	// Send initial response info (for streaming, we only capture status and headers)
	eventChan.SendResponseInfo(&domain.ResponseInfo{
		Status:  resp.StatusCode,
		Headers: flattenHeaders(resp.Header),
		Body:    "[streaming]",
	})

	// Copy upstream headers (except those we override)
	copyResponseHeaders(c.Writer.Header(), resp.Header)

	// Set streaming headers only if not already set by upstream
	// These are required for SSE (Server-Sent Events) to work correctly
	if c.Writer.Header().Get("Content-Type") == "" {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
	}
	if c.Writer.Header().Get("Cache-Control") == "" {
		c.Writer.Header().Set("Cache-Control", "no-cache")
	}
	if c.Writer.Header().Get("Connection") == "" {
		c.Writer.Header().Set("Connection", "keep-alive")
	}
	if c.Writer.Header().Get("X-Accel-Buffering") == "" {
		c.Writer.Header().Set("X-Accel-Buffering", "no")
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, false, "streaming not supported")
	}

	// Note: Response format conversion is handled by Executor's ConvertingResponseWriter
	// Adapter simply passes through the upstream SSE data

	// Incrementally extract metrics and model from SSE lines (no full-stream buffering)
	var collector usage.StreamCollector
	var responseModel string
	var sseError error // Track any SSE error event
	ctx := context.Background()
	if c.Request != nil {
		ctx = c.Request.Context()
	}

	// Helper to send final events via EventChannel
	sendFinalEvents := func() {
		// Send response info (body not accumulated to avoid unbounded memory growth)
		eventChan.SendResponseInfo(&domain.ResponseInfo{
			Status:  resp.StatusCode,
			Headers: flattenHeaders(resp.Header),
			Body:    "[streaming]",
		})

		// Send token usage collected incrementally
		if collector.Metrics != nil && !collector.Metrics.IsEmpty() {
			// Adjust for client-specific quirks (e.g., Codex input_tokens includes cached tokens)
			metrics := usage.AdjustForClientType(collector.Metrics, clientType)
			eventChan.SendMetrics(&domain.AdapterMetrics{
				InputTokens:          metrics.InputTokens,
				OutputTokens:         metrics.OutputTokens,
				CacheReadCount:       metrics.CacheReadCount,
				CacheCreationCount:   metrics.CacheCreationCount,
				Cache5mCreationCount: metrics.Cache5mCreationCount,
				Cache1hCreationCount: metrics.Cache1hCreationCount,
			})
		}

		// Send model collected incrementally
		if responseModel != "" {
			eventChan.SendResponseModel(responseModel)
		}
	}

	// Helper to parse SSE error event from data line
	parseSSEError := func(dataLine string) error {
		// Remove "data:" prefix and trim whitespace
		data := strings.TrimSpace(strings.TrimPrefix(dataLine, "data:"))
		if data == "" || data == "[DONE]" {
			return nil
		}

		// Try to parse as JSON
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			return nil
		}

		// Check for Claude-style error: {"type": "error", "error": {...}}
		if payloadType, ok := payload["type"].(string); ok && payloadType == "error" {
			if errObj, ok := payload["error"].(map[string]interface{}); ok {
				msg := "SSE error"
				if m, ok := errObj["message"].(string); ok {
					msg = m
				}
				code := 0
				if c, ok := errObj["code"].(float64); ok {
					code = int(c)
				}
				errType := ""
				if t, ok := errObj["type"].(string); ok {
					errType = t
				}
				return domain.NewProxyErrorWithMessage(
					fmt.Errorf("SSE error (code=%d): %s", code, msg),
					isRetryableSSEError(code, errType, msg),
					msg,
				)
			}
		}

		// Check for OpenAI-style error: {"error": {"message": "...", "type": "..."}}
		// This format is used by some upstream providers (e.g., Poe, OpenAI-compatible APIs)
		if errObj, ok := payload["error"].(map[string]interface{}); ok {
			// Ensure this is an error object, not a normal response that happens to have an "error" field
			if _, hasMsg := errObj["message"]; hasMsg {
				msg := "SSE error"
				if m, ok := errObj["message"].(string); ok {
					msg = m
				}
				code := 0
				if c, ok := errObj["code"].(float64); ok {
					code = int(c)
				}
				errType := ""
				if t, ok := errObj["type"].(string); ok {
					errType = t
				}
				proxyErr := domain.NewProxyErrorWithMessage(
					fmt.Errorf("SSE error (code=%d): %s", code, msg),
					isRetryableSSEError(code, errType, msg),
					msg,
				)
				proxyErr.IsServerError = errType == "internal_error" || errType == "server_error"
				return proxyErr
			}
		}

		return nil
	}

	// Use buffer-based approach to handle incomplete lines properly
	var lineBuffer bytes.Buffer
	buf := make([]byte, 4096)
	firstChunkSent := false // Track TTFT

	for {
		// Check context before reading
		select {
		case <-ctx.Done():
			sendFinalEvents() // Try to extract tokens before returning
			return domain.NewProxyErrorWithMessage(ctx.Err(), false, "client disconnected")
		default:
		}

		n, err := reader.Read(buf)
		if n > 0 {
			lineBuffer.Write(buf[:n])

			// Process complete lines (lines ending with \n)
			for {
				line, readErr := lineBuffer.ReadString('\n')
				if readErr != nil {
					// No complete line yet, put partial data back
					lineBuffer.WriteString(line)
					break
				}

				processedLine := line
				if isOAuthToken {
					trimmedLine := strings.TrimSuffix(processedLine, "\n")
					stripped := stripClaudeToolPrefixFromStreamLine([]byte(trimmedLine), claudeToolPrefix)
					processedLine = string(stripped)
					if strings.HasSuffix(line, "\n") && !strings.HasSuffix(processedLine, "\n") {
						processedLine += "\n"
					}
				}

				// Extract metrics and model incrementally per line
				collector.ProcessSSELine(processedLine)
				extractResponseModelFromSSELine(processedLine, clientType, &responseModel)

				// Check for SSE error events in data lines BEFORE writing to client
				lineStr := processedLine
				if strings.HasPrefix(strings.TrimSpace(lineStr), "data:") {
					if parseErr := parseSSEError(lineStr); parseErr != nil {
						sseError = parseErr
						// If no real data has been written to the client yet,
						// return the error immediately so retry logic can try another route
						if !firstChunkSent {
							sendFinalEvents()
							return sseError
						}
						// Otherwise continue to forward the error to client
					}
				}

				// Note: Response format conversion is handled by Executor's ConvertingResponseWriter
				// Adapter simply passes through the upstream SSE data
				if len(processedLine) > 0 {
					_, writeErr := c.Writer.Write([]byte(processedLine))
					if writeErr != nil {
						// Client disconnected
						sendFinalEvents()
						return domain.NewProxyErrorWithMessage(writeErr, false, "client disconnected")
					}
					flusher.Flush()

					// Track TTFT: send first token time on first successful write
					if !firstChunkSent && eventChan != nil {
						firstChunkSent = true
						eventChan.SendFirstToken(time.Now().UnixMilli())
					}
				}
			}
		}

		if err != nil {
			if err == io.EOF {
				sendFinalEvents() // Extract tokens at normal completion
				// Return SSE error if one was detected during streaming
				if sseError != nil {
					return sseError
				}
				return nil
			}
			// Upstream connection closed - check if client is still connected
			if ctx.Err() != nil {
				sendFinalEvents()
				return domain.NewProxyErrorWithMessage(ctx.Err(), false, "client disconnected")
			}
			sendFinalEvents()
			// Return SSE error if one was detected during streaming
			if sseError != nil {
				return sseError
			}
			return nil // Upstream closed normally
		}
	}
}

// Helper functions

func isStreamRequest(body []byte) bool {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return false
	}
	stream, _ := req["stream"].(bool)
	return stream
}

func updateModelInBody(body []byte, model string, clientType domain.ClientType) ([]byte, error) {
	// For Gemini, model is in URL path, not in body - pass through unchanged
	if clientType == domain.ClientTypeGemini {
		return body, nil
	}

	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	req["model"] = model
	return json.Marshal(req)
}

func buildUpstreamURL(baseURL string, requestPath string) string {
	return strings.TrimSuffix(baseURL, "/") + requestPath
}

func shouldUseClaudeAPIKey(apiKey string, clientReq *http.Request) bool {
	if clientReq != nil {
		if strings.TrimSpace(clientReq.Header.Get("x-api-key")) != "" {
			return true
		}
		if strings.TrimSpace(clientReq.Header.Get("Authorization")) != "" {
			return false
		}
	}

	return !isClaudeOAuthToken(apiKey)
}

// addClaudeQueryParams adds query parameters to URL for Claude API (following CLIProxyAPI)
// Adds: beta=true
// Skips adding if parameter already exists
func addClaudeQueryParams(urlStr string) string {
	// Add beta=true if not already present
	if !strings.Contains(urlStr, "beta=true") {
		if strings.Contains(urlStr, "?") {
			urlStr = urlStr + "&beta=true"
		} else {
			urlStr = urlStr + "?beta=true"
		}
	}

	return urlStr
}

// Gemini URL patterns for model replacement
var geminiModelPathPattern = regexp.MustCompile(`(/v1(?:beta|internal)?/models/)([^/:]+)(:[^/]+)?`)

// updateGeminiModelInPath replaces the model in Gemini URL path
// e.g., /v1beta/models/gemini-2.5-flash:generateContent -> /v1beta/models/gemini-2.5-pro:generateContent
func updateGeminiModelInPath(path string, newModel string) string {
	return geminiModelPathPattern.ReplaceAllString(path, "${1}"+newModel+"${3}")
}

func setAuthHeader(req *http.Request, clientType domain.ClientType, apiKey string, forceCreate bool) {
	// For format conversion scenarios, we need to create the appropriate auth header
	// even if the original request didn't have it (e.g., Claude x-api-key -> OpenAI Authorization)
	if forceCreate {
		switch clientType {
		case domain.ClientTypeOpenAI, domain.ClientTypeCodex:
			// OpenAI/Codex-style: Authorization: Bearer <key>
			req.Header.Set("Authorization", "Bearer "+apiKey)
		case domain.ClientTypeClaude:
			// Claude-style: x-api-key
			req.Header.Set("x-api-key", apiKey)
		case domain.ClientTypeGemini:
			// Gemini-style: x-goog-api-key
			req.Header.Set("x-goog-api-key", apiKey)
		default:
			// Default to OpenAI style for unknown types
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		return
	}

	// Only update authentication headers that already exist in the request
	// Do not create new headers - preserve the original request format

	// Check which auth header the client used and update only that one

	if req.Header.Get("x-api-key") != "" {
		// Claude-style auth
		req.Header.Set("x-api-key", apiKey)
	}
	if req.Header.Get("Authorization") != "" {
		// OpenAI/Codex-style auth
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	if req.Header.Get("x-goog-api-key") != "" {
		// Gemini-style auth
		req.Header.Set("x-goog-api-key", apiKey)
	}
	// If no auth header exists, don't create one
	// The request will be sent as-is (useful for providers that use query params or other auth methods)
}

func isRetryableStatusCode(code int) bool {
	switch code {
	case 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}

// isRetryableSSEError checks if an SSE error should trigger a retry
func isRetryableSSEError(code int, errType, msg string) bool {
	// HTTP-like status codes that are retryable
	if isRetryableStatusCode(code) {
		return true
	}

	// Server errors are generally retryable
	if errType == "server_error" || errType == "internal_error" {
		return true
	}

	// Specific messages that indicate transient failures
	lowerMsg := strings.ToLower(msg)
	if strings.Contains(lowerMsg, "upstream") ||
		strings.Contains(lowerMsg, "timeout") ||
		strings.Contains(lowerMsg, "overloaded") ||
		strings.Contains(lowerMsg, "temporarily") ||
		strings.Contains(lowerMsg, "rate limit") ||
		strings.Contains(lowerMsg, "internal server error") {
		return true
	}

	return false
}

func flattenHeaders(h http.Header) map[string]string {
	result := make(map[string]string)
	for k, v := range h {
		if len(v) > 0 {
			result[k] = v[0]
		}
	}
	return result
}

// Headers to filter out - only privacy/proxy related, NOT application headers like anthropic-version
var filteredHeaders = map[string]bool{
	// IP and client identification headers (privacy protection)
	"x-forwarded-for":   true,
	"x-forwarded-host":  true,
	"x-forwarded-proto": true,
	"x-forwarded-port":  true,
	"x-real-ip":         true,
	"x-client-ip":       true,
	"x-originating-ip":  true,
	"x-remote-ip":       true,
	"x-remote-addr":     true,
	"forwarded":         true,

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

	// Headers that will be overridden (not filtered, just replaced)
	"host":           true, // Will be set by http client
	"content-length": true, // Will be recalculated
}

// copyHeadersFiltered copies headers from src to dst, filtering out sensitive headers
func copyHeadersFiltered(dst, src http.Header) {
	if src == nil {
		return
	}
	for key, values := range src {
		lowerKey := strings.ToLower(key)
		if filteredHeaders[lowerKey] {
			continue
		}
		for _, v := range values {
			dst.Add(key, v)
		}
	}
}

// Response headers to exclude when copying
var excludedResponseHeaders = map[string]bool{
	"content-length":    true,
	"content-encoding":  true, // We decompress the response, so don't tell client it's compressed
	"transfer-encoding": true,
	"connection":        true,
	"keep-alive":        true,
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

// parseRateLimitInfo parses rate limit information from 429 responses
// Supports multiple API formats: OpenAI, Anthropic, Gemini, etc.
func parseRateLimitInfo(resp *http.Response, body []byte, clientType domain.ClientType) *domain.RateLimitInfo {
	var resetTime time.Time
	var rateLimitType = "rate_limit_exceeded"

	// Method 1: Parse Retry-After header
	if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
		// Try as seconds
		if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
			resetTime = time.Now().Add(time.Duration(seconds) * time.Second)
		} else if t, err := http.ParseTime(retryAfter); err == nil {
			resetTime = t
		}
	}

	// Method 2: Parse response body
	bodyStr := string(body)
	bodyLower := strings.ToLower(bodyStr)

	// Detect rate limit type from message
	if strings.Contains(bodyLower, "quota") || strings.Contains(bodyLower, "exceeded your") {
		rateLimitType = "quota_exhausted"
	} else if strings.Contains(bodyLower, "per minute") || strings.Contains(bodyLower, "rpm") || strings.Contains(bodyLower, "tpm") {
		rateLimitType = "rate_limit_exceeded"
	} else if strings.Contains(bodyLower, "concurrent") {
		rateLimitType = "concurrent_limit"
	}

	// Try to parse structured error response
	var errResp struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}

	if json.Unmarshal(body, &errResp) == nil {
		// OpenAI/Anthropic style
		if errResp.Error.Type == "rate_limit_error" || errResp.Error.Code == "rate_limit_exceeded" {
			// Try to extract time from message
			if t := extractTimeFromMessage(errResp.Error.Message); !t.IsZero() {
				resetTime = t
			}
		}
		if errResp.Error.Type == "insufficient_quota" || errResp.Error.Code == "insufficient_quota" {
			rateLimitType = "quota_exhausted"
		}
	}

	if resetTime.IsZero() {
		if t := extractStructuredResetTime(body); !t.IsZero() {
			resetTime = t
		}
	}

	// If no reset time found, use default based on type
	if resetTime.IsZero() {
		switch rateLimitType {
		case "quota_exhausted":
			// Default to 1 hour for quota exhaustion
			resetTime = time.Now().Add(1 * time.Hour)
		case "concurrent_limit":
			// Short cooldown for concurrent limits
			resetTime = time.Now().Add(10 * time.Second)
		default:
			// Default to 1 minute for rate limits
			resetTime = time.Now().Add(1 * time.Minute)
		}
	}

	return &domain.RateLimitInfo{
		Type:             rateLimitType,
		QuotaResetTime:   resetTime,
		RetryHintMessage: bodyStr,
		ClientType:       string(clientType), // Cooldown applies to specific client type
	}
}

func applyRateLimitRetryHints(proxyErr *domain.ProxyError, resp *http.Response, rateLimitInfo *domain.RateLimitInfo) {
	if proxyErr == nil || resp == nil {
		return
	}

	if retryAfter, until := parseRetryAfterHeader(resp.Header.Get("Retry-After")); retryAfter > 0 {
		proxyErr.RetryAfter = retryAfter
		if until != nil {
			proxyErr.CooldownUntil = until
		}
	}

	if rateLimitInfo != nil && !rateLimitInfo.QuotaResetTime.IsZero() {
		until := rateLimitInfo.QuotaResetTime
		if proxyErr.CooldownUntil == nil {
			proxyErr.CooldownUntil = &until
		}
		if proxyErr.RetryAfter <= 0 {
			if retryAfter := time.Until(until); retryAfter > 0 {
				proxyErr.RetryAfter = retryAfter
			}
		}
	}
}

func parseRetryAfterHeader(value string) (time.Duration, *time.Time) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		until := time.Now().Add(time.Duration(seconds) * time.Second)
		return time.Duration(seconds) * time.Second, &until
	}
	if t, err := http.ParseTime(value); err == nil {
		delay := time.Until(t)
		if delay > 0 {
			return delay, &t
		}
		return 0, nil
	}
	return 0, nil
}

func extractStructuredResetTime(body []byte) time.Time {
	var payload interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return time.Time{}
	}
	return findResetTime(payload)
}

func findResetTime(v interface{}) time.Time {
	switch value := v.(type) {
	case map[string]interface{}:
		for key, raw := range value {
			switch key {
			case "QuotaResetTime", "quotaResetTime", "quota_reset_time", "quotaResetTimeStamp", "cooldownUntil", "CooldownUntil":
				if s, ok := raw.(string); ok {
					if parsed, err := time.Parse(time.RFC3339, s); err == nil {
						return parsed
					}
				}
			}
			if parsed := findResetTime(raw); !parsed.IsZero() {
				return parsed
			}
		}
	case []interface{}:
		for _, item := range value {
			if parsed := findResetTime(item); !parsed.IsZero() {
				return parsed
			}
		}
	}
	return time.Time{}
}

// extractTimeFromMessage tries to extract time duration from error message
// Handles formats like "Try again in 20s", "in 2 minutes", "in 1 hour"
func extractTimeFromMessage(msg string) time.Time {
	msgLower := strings.ToLower(msg)

	// Pattern: "in X seconds/minutes/hours"
	patterns := []struct {
		re         *regexp.Regexp
		multiplier time.Duration
	}{
		{regexp.MustCompile(`in (\d+)\s*s(?:ec(?:ond)?s?)?`), time.Second},
		{regexp.MustCompile(`in (\d+)\s*m(?:in(?:ute)?s?)?`), time.Minute},
		{regexp.MustCompile(`in (\d+)\s*h(?:our)?s?`), time.Hour},
	}

	for _, p := range patterns {
		if matches := p.re.FindStringSubmatch(msgLower); len(matches) > 1 {
			if n, err := strconv.Atoi(matches[1]); err == nil {
				return time.Now().Add(time.Duration(n) * p.multiplier)
			}
		}
	}

	return time.Time{}
}

// extractResponseModel extracts the model name from response body based on target type
func extractResponseModel(body []byte, targetType domain.ClientType) string {
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return ""
	}

	switch targetType {
	case domain.ClientTypeClaude, domain.ClientTypeOpenAI, domain.ClientTypeCodex:
		// Claude/OpenAI/Codex: "model" field at root level
		if model, ok := data["model"].(string); ok {
			return model
		}
	case domain.ClientTypeGemini:
		// Gemini: "modelVersion" field at root level
		if model, ok := data["modelVersion"].(string); ok {
			return model
		}
	}

	return ""
}

// extractResponseModelFromSSELine extracts the model name from a single SSE line based on target type.
func extractResponseModelFromSSELine(line string, targetType domain.ClientType, model *string) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "data:") {
		return
	}
	dataStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if dataStr == "" || dataStr == "[DONE]" {
		return
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(dataStr), &payload); err != nil {
		return
	}

	switch targetType {
	case domain.ClientTypeClaude, domain.ClientTypeOpenAI, domain.ClientTypeCodex:
		// Claude/OpenAI: check for "model" in various places
		if m, ok := payload["model"].(string); ok && m != "" {
			*model = m
		}
		// Claude SSE: check message_start event for model in message object
		if eventType, ok := payload["type"].(string); ok && eventType == "message_start" {
			if msg, ok := payload["message"].(map[string]interface{}); ok {
				if m, ok := msg["model"].(string); ok && m != "" {
					*model = m
				}
			}
		}
	case domain.ClientTypeGemini:
		// Gemini: check for "modelVersion"
		if m, ok := payload["modelVersion"].(string); ok && m != "" {
			*model = m
		}
	}
}
