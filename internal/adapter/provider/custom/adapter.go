package custom

import (
	"bytes"
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
	ctxutil "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
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

func (a *CustomAdapter) Execute(ctx context.Context, w http.ResponseWriter, req *http.Request, provider *domain.Provider) error {
	clientType := ctxutil.GetClientType(ctx)
	mappedModel := ctxutil.GetMappedModel(ctx)
	requestBody := ctxutil.GetRequestBody(ctx)

	// Determine if streaming
	stream := isStreamRequest(requestBody)

	// Note: Format conversion is now handled by Executor layer
	// The clientType in context is already the correct type that this provider supports
	// We use clientType directly for URL building and auth header selection

	// Build upstream URL
	baseURL := a.getBaseURL(clientType)
	requestURI := ctxutil.GetRequestURI(ctx)

	// For Gemini, update model in URL path if mapping is configured
	if clientType == domain.ClientTypeGemini && mappedModel != "" {
		requestURI = updateGeminiModelInPath(requestURI, mappedModel)
	}

	upstreamURL := buildUpstreamURL(baseURL, requestURI)

	// Create upstream request
	upstreamReq, err := http.NewRequestWithContext(ctx, "POST", upstreamURL, bytes.NewReader(requestBody))
	if err != nil {
		return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to create upstream request")
	}

	// Forward original headers (filtered) - preserves anthropic-version, anthropic-beta, user-agent, etc.
	originalHeaders := ctxutil.GetRequestHeaders(ctx)
	upstreamReq.Header = originalHeaders

	// Override auth headers with provider's credentials
	if a.provider.Config.Custom.APIKey != "" {
		setAuthHeader(upstreamReq, clientType, a.provider.Config.Custom.APIKey)
	}

	// Send request info via EventChannel
	if eventChan := ctxutil.GetEventChan(ctx); eventChan != nil {
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
	defer resp.Body.Close()

	// Check for error response
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		// Send error response info via EventChannel
		if eventChan := ctxutil.GetEventChan(ctx); eventChan != nil {
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
			}
		}

		return proxyErr
	}

	// Handle response
	// Note: Response format conversion is handled by Executor's ConvertingResponseWriter
	// Adapters simply pass through the upstream response
	if stream {
		return a.handleStreamResponse(ctx, w, resp, clientType)
	}
	return a.handleNonStreamResponse(ctx, w, resp, clientType)
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

func (a *CustomAdapter) handleNonStreamResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response, clientType domain.ClientType) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to read upstream response")
	}

	eventChan := ctxutil.GetEventChan(ctx)

	// Send response info via EventChannel
	eventChan.SendResponseInfo(&domain.ResponseInfo{
		Status:  resp.StatusCode,
		Headers: flattenHeaders(resp.Header),
		Body:    string(body),
	})

	// Extract and send token usage metrics
	if metrics := usage.ExtractFromResponse(string(body)); metrics != nil {
		// Adjust for client-specific quirks (e.g., Codex input_tokens includes cached tokens)
		metrics = usage.AdjustForClientType(metrics, clientType)
		eventChan.SendMetrics(&domain.AdapterMetrics{
			InputTokens:          metrics.InputTokens,
			OutputTokens:         metrics.OutputTokens,
			CacheReadCount:       metrics.CacheReadCount,
			CacheCreationCount:   metrics.CacheCreationCount,
			Cache5mCreationCount: metrics.Cache5mCreationCount,
			Cache1hCreationCount: metrics.Cache1hCreationCount,
		})
	}

	// Extract and send responseModel
	if responseModel := extractResponseModel(body, clientType); responseModel != "" {
		eventChan.SendResponseModel(responseModel)
	}

	// Note: Response format conversion is handled by Executor's ConvertingResponseWriter
	// Adapter simply passes through the upstream response body

	// Copy upstream headers (except those we override)
	copyResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
	return nil
}

func (a *CustomAdapter) handleStreamResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response, clientType domain.ClientType) error {
	eventChan := ctxutil.GetEventChan(ctx)

	// Send initial response info (for streaming, we only capture status and headers)
	eventChan.SendResponseInfo(&domain.ResponseInfo{
		Status:  resp.StatusCode,
		Headers: flattenHeaders(resp.Header),
		Body:    "[streaming]",
	})

	// Copy upstream headers (except those we override)
	copyResponseHeaders(w.Header(), resp.Header)

	// Set streaming headers only if not already set by upstream
	// These are required for SSE (Server-Sent Events) to work correctly
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "text/event-stream")
	}
	if w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", "no-cache")
	}
	if w.Header().Get("Connection") == "" {
		w.Header().Set("Connection", "keep-alive")
	}
	if w.Header().Get("X-Accel-Buffering") == "" {
		w.Header().Set("X-Accel-Buffering", "no")
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, false, "streaming not supported")
	}

	// Note: Response format conversion is handled by Executor's ConvertingResponseWriter
	// Adapter simply passes through the upstream SSE data

	// Collect all SSE events for response body and token extraction
	var sseBuffer strings.Builder
	var sseError error // Track any SSE error event

	// Helper to send final events via EventChannel
	sendFinalEvents := func() {
		if sseBuffer.Len() > 0 {
			// Send updated response body
			eventChan.SendResponseInfo(&domain.ResponseInfo{
				Status:  resp.StatusCode,
				Headers: flattenHeaders(resp.Header),
				Body:    sseBuffer.String(),
			})

			// Extract and send token usage
			if metrics := usage.ExtractFromStreamContent(sseBuffer.String()); metrics != nil {
				// Adjust for client-specific quirks (e.g., Codex input_tokens includes cached tokens)
				metrics = usage.AdjustForClientType(metrics, clientType)
				eventChan.SendMetrics(&domain.AdapterMetrics{
					InputTokens:          metrics.InputTokens,
					OutputTokens:         metrics.OutputTokens,
					CacheReadCount:       metrics.CacheReadCount,
					CacheCreationCount:   metrics.CacheCreationCount,
					Cache5mCreationCount: metrics.Cache5mCreationCount,
					Cache1hCreationCount: metrics.Cache1hCreationCount,
				})
			}

			// Extract and send responseModel
			if responseModel := extractResponseModelFromSSE(sseBuffer.String(), clientType); responseModel != "" {
				eventChan.SendResponseModel(responseModel)
			}
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

		// Check for error type
		if payloadType, ok := payload["type"].(string); ok && payloadType == "error" {
			// Extract error message
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

		n, err := resp.Body.Read(buf)
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

				// Collect all SSE content (preserve complete format including newlines)
				sseBuffer.WriteString(line)

				// Check for SSE error events in data lines
				lineStr := line
				if strings.HasPrefix(strings.TrimSpace(lineStr), "data:") {
					if parseErr := parseSSEError(lineStr); parseErr != nil {
						sseError = parseErr
						// Continue to forward the error to client, but track it
					}
				}

				// Note: Response format conversion is handled by Executor's ConvertingResponseWriter
				// Adapter simply passes through the upstream SSE data
				if len(line) > 0 {
					_, writeErr := w.Write([]byte(line))
					if writeErr != nil {
						// Client disconnected
						sendFinalEvents()
						return domain.NewProxyErrorWithMessage(writeErr, false, "client disconnected")
					}
					flusher.Flush()

					// Track TTFT: send first token time on first successful write
					if !firstChunkSent {
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

// Gemini URL patterns for model replacement
var geminiModelPathPattern = regexp.MustCompile(`(/v1(?:beta|internal)?/models/)([^/:]+)(:[^/]+)?`)

// updateGeminiModelInPath replaces the model in Gemini URL path
// e.g., /v1beta/models/gemini-2.5-flash:generateContent -> /v1beta/models/gemini-2.5-pro:generateContent
func updateGeminiModelInPath(path string, newModel string) string {
	return geminiModelPathPattern.ReplaceAllString(path, "${1}"+newModel+"${3}")
}

func setAuthHeader(req *http.Request, clientType domain.ClientType, apiKey string) {
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
	if errType == "server_error" {
		return true
	}

	// Specific messages that indicate transient failures
	lowerMsg := strings.ToLower(msg)
	if strings.Contains(lowerMsg, "upstream") ||
		strings.Contains(lowerMsg, "timeout") ||
		strings.Contains(lowerMsg, "overloaded") ||
		strings.Contains(lowerMsg, "temporarily") ||
		strings.Contains(lowerMsg, "rate limit") {
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

// extractResponseModelFromSSE extracts the model name from SSE content based on target type
func extractResponseModelFromSSE(sseContent string, targetType domain.ClientType) string {
	var lastModel string
	lines := strings.Split(sseContent, "\n")

	for _, line := range lines {
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		dataStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if dataStr == "" || dataStr == "[DONE]" {
			continue
		}

		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(dataStr), &payload); err != nil {
			continue
		}

		switch targetType {
		case domain.ClientTypeClaude, domain.ClientTypeOpenAI, domain.ClientTypeCodex:
			// Claude/OpenAI: check for "model" in various places
			if model, ok := payload["model"].(string); ok && model != "" {
				lastModel = model
			}
			// Claude SSE: check message_start event
			if msg, ok := payload["message"].(map[string]interface{}); ok {
				if model, ok := msg["model"].(string); ok && model != "" {
					lastModel = model
				}
			}
		case domain.ClientTypeGemini:
			// Gemini: check for "modelVersion"
			if model, ok := payload["modelVersion"].(string); ok && model != "" {
				lastModel = model
			}
		}
	}

	return lastModel
}
