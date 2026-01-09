package custom

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/Bowl42/maxx-next/internal/adapter/provider"
	"github.com/Bowl42/maxx-next/internal/converter"
	ctxutil "github.com/Bowl42/maxx-next/internal/context"
	"github.com/Bowl42/maxx-next/internal/domain"
	"github.com/Bowl42/maxx-next/internal/pricing"
	"github.com/Bowl42/maxx-next/internal/usage"
)

func init() {
	provider.RegisterAdapterFactory("custom", NewAdapter)
}

type CustomAdapter struct {
	provider  *domain.Provider
	converter *converter.Registry
}

func NewAdapter(p *domain.Provider) (provider.ProviderAdapter, error) {
	if p.Config == nil || p.Config.Custom == nil {
		return nil, fmt.Errorf("provider %s missing custom config", p.Name)
	}
	return &CustomAdapter{
		provider:  p,
		converter: converter.NewRegistry(),
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

	// Determine target client type for the provider
	// If provider supports the client's type natively, use it directly
	// Otherwise, find a supported type and convert
	targetType := clientType
	needsConversion := false
	if !a.supportsClientType(clientType) {
		// Find a supported type (prefer OpenAI as it's most common)
		for _, supported := range a.provider.SupportedClientTypes {
			targetType = supported
			break
		}
		needsConversion = true
	}

	// Transform request if needed
	var upstreamBody []byte
	var err error
	if needsConversion {
		upstreamBody, err = a.converter.TransformRequest(clientType, targetType, requestBody, mappedModel, stream)
		if err != nil {
			return domain.NewProxyErrorWithMessage(domain.ErrFormatConversion, true, "failed to transform request")
		}
	} else {
		// Just update the model in the request
		upstreamBody, err = updateModelInBody(requestBody, mappedModel, clientType)
		if err != nil {
			return domain.NewProxyErrorWithMessage(domain.ErrFormatConversion, true, "failed to update model")
		}
	}

	// Build upstream URL
	baseURL := a.getBaseURL(targetType)
	requestPath := ctxutil.GetRequestPath(ctx)

	// For Gemini, update model in URL path if mapping is configured
	if clientType == domain.ClientTypeGemini && mappedModel != "" {
		requestPath = updateGeminiModelInPath(requestPath, mappedModel)
	}

	upstreamURL := buildUpstreamURL(baseURL, requestPath)

	// Create upstream request
	upstreamReq, err := http.NewRequestWithContext(ctx, "POST", upstreamURL, bytes.NewReader(upstreamBody))
	if err != nil {
		return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to create upstream request")
	}

	// Forward original headers (filtered) - preserves anthropic-version, anthropic-beta, user-agent, etc.
	originalHeaders := ctxutil.GetRequestHeaders(ctx)
	copyHeadersFiltered(upstreamReq.Header, originalHeaders)

	// Set content-type if not already set
	if upstreamReq.Header.Get("Content-Type") == "" {
		upstreamReq.Header.Set("Content-Type", "application/json")
	}
	// Disable compression to avoid gzip decode issues
	upstreamReq.Header.Set("Accept-Encoding", "identity")

	// Override auth headers with provider's credentials
	if a.provider.Config.Custom.APIKey != "" {
		setAuthHeader(upstreamReq, targetType, a.provider.Config.Custom.APIKey)
	}

	// Capture request info for attempt record
	if attempt := ctxutil.GetUpstreamAttempt(ctx); attempt != nil {
		attempt.RequestInfo = &domain.RequestInfo{
			Method:  upstreamReq.Method,
			URL:     upstreamURL,
			Headers: flattenHeaders(upstreamReq.Header),
			Body:    string(upstreamBody),
		}
	}

	// Execute request
	client := &http.Client{}
	resp, err := client.Do(upstreamReq)
	if err != nil {
		return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to connect to upstream")
	}
	defer resp.Body.Close()

	// Check for error response
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		// Capture error response info
		if attempt := ctxutil.GetUpstreamAttempt(ctx); attempt != nil {
			attempt.ResponseInfo = &domain.ResponseInfo{
				Status:  resp.StatusCode,
				Headers: flattenHeaders(resp.Header),
				Body:    string(body),
			}
		}
		return domain.NewProxyErrorWithMessage(
			fmt.Errorf("upstream error: %s", string(body)),
			isRetryableStatusCode(resp.StatusCode),
			fmt.Sprintf("upstream returned status %d", resp.StatusCode),
		)
	}

	// Handle response
	if stream {
		return a.handleStreamResponse(ctx, w, resp, clientType, targetType, needsConversion)
	}
	return a.handleNonStreamResponse(ctx, w, resp, clientType, targetType, needsConversion)
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

func (a *CustomAdapter) handleNonStreamResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response, clientType, targetType domain.ClientType, needsConversion bool) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to read upstream response")
	}

	// Capture response info and extract token usage
	if attempt := ctxutil.GetUpstreamAttempt(ctx); attempt != nil {
		attempt.ResponseInfo = &domain.ResponseInfo{
			Status:  resp.StatusCode,
			Headers: flattenHeaders(resp.Header),
			Body:    string(body),
		}

		// Extract token usage from response
		if metrics := usage.ExtractFromResponse(string(body)); metrics != nil {
			// Adjust for client-specific quirks (e.g., Codex input_tokens includes cached tokens)
			metrics = usage.AdjustForClientType(metrics, clientType)
			attempt.InputTokenCount = metrics.InputTokens
			attempt.OutputTokenCount = metrics.OutputTokens
			attempt.CacheReadCount = metrics.CacheReadCount
			attempt.CacheWriteCount = metrics.CacheCreationCount
			attempt.Cache5mWriteCount = metrics.Cache5mCreationCount
			attempt.Cache1hWriteCount = metrics.Cache1hCreationCount

			// Calculate cost
			attempt.Cost = pricing.GlobalCalculator().Calculate(ctxutil.GetMappedModel(ctx), metrics)
		}

		// Broadcast attempt update with token info
		if bc := ctxutil.GetBroadcaster(ctx); bc != nil {
			bc.BroadcastProxyUpstreamAttempt(attempt)
		}
	}

	var responseBody []byte
	if needsConversion {
		responseBody, err = a.converter.TransformResponse(targetType, clientType, body)
		if err != nil {
			return domain.NewProxyErrorWithMessage(domain.ErrFormatConversion, false, "failed to transform response")
		}
	} else {
		responseBody = body
	}

	// Copy upstream headers (except those we override)
	copyResponseHeaders(w.Header(), resp.Header)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(responseBody)
	return nil
}

func (a *CustomAdapter) handleStreamResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response, clientType, targetType domain.ClientType, needsConversion bool) error {
	attempt := ctxutil.GetUpstreamAttempt(ctx)

	// Capture response info (for streaming, we only capture status and headers)
	if attempt != nil {
		attempt.ResponseInfo = &domain.ResponseInfo{
			Status:  resp.StatusCode,
			Headers: flattenHeaders(resp.Header),
			Body:    "[streaming]",
		}
	}

	// Copy upstream headers (except those we override)
	copyResponseHeaders(w.Header(), resp.Header)

	// Set/override streaming headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, false, "streaming not supported")
	}

	var state *converter.TransformState
	if needsConversion {
		state = converter.NewTransformState()
	}

	// Collect all SSE events for response body and token extraction
	var sseBuffer strings.Builder
	var sseError error // Track any SSE error event

	// Helper to extract tokens and update attempt with final response body
	extractTokens := func() {
		if attempt != nil && sseBuffer.Len() > 0 {
			// Update response body with collected SSE content
			if attempt.ResponseInfo != nil {
				attempt.ResponseInfo.Body = sseBuffer.String()
			}
			// Extract token usage
			if metrics := usage.ExtractFromStreamContent(sseBuffer.String()); metrics != nil {
				// Adjust for client-specific quirks (e.g., Codex input_tokens includes cached tokens)
				metrics = usage.AdjustForClientType(metrics, clientType)
				attempt.InputTokenCount = metrics.InputTokens
				attempt.OutputTokenCount = metrics.OutputTokens
				attempt.CacheReadCount = metrics.CacheReadCount
				attempt.CacheWriteCount = metrics.CacheCreationCount
				attempt.Cache5mWriteCount = metrics.Cache5mCreationCount
				attempt.Cache1hWriteCount = metrics.Cache1hCreationCount

				// Calculate cost
				attempt.Cost = pricing.GlobalCalculator().Calculate(ctxutil.GetMappedModel(ctx), metrics)
			}
			// Broadcast attempt update with token info
			if bc := ctxutil.GetBroadcaster(ctx); bc != nil {
				bc.BroadcastProxyUpstreamAttempt(attempt)
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

	// Create a channel for read results
	type readResult struct {
		line []byte
		err  error
	}
	readCh := make(chan readResult, 1)

	reader := bufio.NewReader(resp.Body)
	for {
		// Check context before reading
		select {
		case <-ctx.Done():
			extractTokens() // Try to extract tokens before returning
			return domain.NewProxyErrorWithMessage(ctx.Err(), false, "client disconnected")
		default:
		}

		// Read in goroutine to allow context checking
		go func() {
			line, err := reader.ReadBytes('\n')
			readCh <- readResult{line: line, err: err}
		}()

		// Wait for read or context cancellation
		select {
		case <-ctx.Done():
			extractTokens()
			return domain.NewProxyErrorWithMessage(ctx.Err(), false, "client disconnected")
		case result := <-readCh:
			if result.err != nil {
				if result.err == io.EOF {
					extractTokens() // Extract tokens at normal completion
					// Return SSE error if one was detected during streaming
					if sseError != nil {
						return sseError
					}
					return nil
				}
				// Upstream connection closed - check if client is still connected
				if ctx.Err() != nil {
					extractTokens()
					return domain.NewProxyErrorWithMessage(ctx.Err(), false, "client disconnected")
				}
				extractTokens()
				// Return SSE error if one was detected during streaming
				if sseError != nil {
					return sseError
				}
				return nil // Upstream closed normally
			}

			// Collect all SSE content (preserve complete format including newlines)
			sseBuffer.Write(result.line)

			// Check for SSE error events in data lines
			lineStr := string(result.line)
			if strings.HasPrefix(strings.TrimSpace(lineStr), "data:") {
				if err := parseSSEError(lineStr); err != nil {
					sseError = err
					// Continue to forward the error to client, but track it
				}
			}

			var output []byte
			if needsConversion {
				// Transform the chunk
				transformed, err := a.converter.TransformStreamChunk(targetType, clientType, result.line, state)
				if err != nil {
					continue // Skip malformed chunks
				}
				output = transformed
			} else {
				output = result.line
			}

			if len(output) > 0 {
				_, writeErr := w.Write(output)
				if writeErr != nil {
					// Client disconnected
					extractTokens()
					return domain.NewProxyErrorWithMessage(writeErr, false, "client disconnected")
				}
				flusher.Flush()
			}
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
	switch clientType {
	case domain.ClientTypeClaude:
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	case domain.ClientTypeGemini:
		// Gemini uses query param, but we can also set header
		req.Header.Set("x-goog-api-key", apiKey)
	default:
		// OpenAI and Codex use Bearer token
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
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
	"x-forwarded-for":  true,
	"x-forwarded-host": true,
	"x-forwarded-proto": true,
	"x-forwarded-port": true,
	"x-real-ip":        true,
	"x-client-ip":      true,
	"x-originating-ip": true,
	"x-remote-ip":      true,
	"x-remote-addr":    true,
	"forwarded":        true,

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
	"x-request-id":     true,
	"x-correlation-id": true,
	"x-trace-id":       true,
	"x-amzn-trace-id":  true,
	"x-b3-traceid":     true,
	"x-b3-spanid":      true,
	"x-b3-parentspanid": true,
	"x-b3-sampled":     true,
	"traceparent":      true,
	"tracestate":       true,

	// Headers that will be overridden (not filtered, just replaced)
	"host":          true, // Will be set by http client
	"content-length": true, // Will be recalculated
	"authorization": true, // Will be replaced with provider's key
	"x-api-key":     true, // Will be replaced with provider's key
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
