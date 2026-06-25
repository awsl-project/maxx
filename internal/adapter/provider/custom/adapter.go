package custom

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/adapter/provider/custom/error_fixer"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/usage"
)

// mockMode enables forwarding X-Mock-* headers to upstream (for testing).
// Activated by setting MAXX_MOCK_MODE=1 environment variable.
var mockMode = os.Getenv("MAXX_MOCK_MODE") == "1"

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
	if strings.EqualFold(strings.TrimSpace(a.provider.Config.Custom.Backend), customBackendOllama) {
		return a.executeOllama(c, provider)
	}

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

	// Codex Responses passthrough: the proxy layer normalizes /v1/responses down
	// to /responses. When forwarding to a codex downstream as-is (no format
	// conversion), honor the client's original path so OpenAI-compatible gateways
	// that serve /v1/responses (e.g. New API) are hit at the right path instead of
	// a 404-ing /responses. Gated by the per-provider switch (default on).
	if clientType == domain.ClientTypeCodex && domain.ResponsesPassthroughEnabled(a.customPassthroughFlag()) {
		if p := flow.GetResponsesClientPath(c); p != "" {
			requestURI = p
		}
	}

	// Apply model mapping if configured
	var err error
	if mappedModel != "" {
		switch {
		case clientType == domain.ClientTypeGemini:
			// Gemini carries the model in the URL path, not the body.
			requestURI = updateGeminiModelInPath(requestURI, mappedModel)
		case isMultipartForm(request):
			// OpenAI images/edits sends multipart/form-data, so the JSON rewrite
			// in updateModelInBody can't be used (it would corrupt the upload).
			// Rewrite only the "model" form field and copy every other part
			// (including the image) through unchanged, so configured model
			// mapping still takes effect instead of being silently dropped.
			requestBody, err = updateModelInMultipartForm(requestBody, request, mappedModel)
			if err != nil {
				proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, false, "failed to update model in multipart body")
				proxyErr.Scope = domain.ScopeRequest
				return proxyErr
			}
		default:
			requestBody, err = updateModelInBody(requestBody, mappedModel, clientType)
			if err != nil {
				proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, false, "failed to update model in body")
				proxyErr.Scope = domain.ScopeRequest
				return proxyErr
			}
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
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, false, "failed to create upstream request")
		proxyErr.Scope = domain.ScopeEndpoint
		proxyErr.Reason = domain.CooldownReasonServerError
		return proxyErr
	}

	// Set headers based on client type
	isOAuthToken := false
	switch clientType {
	case domain.ClientTypeClaude:
		// Claude: Following CLIProxyAPI pattern
		// 1. Process body first (apply disguise + structural fixes, return extra betas)
		customCfg := a.provider.Config.Custom
		apiKey := customCfg.APIKey
		clientUA := ""
		if request != nil {
			clientUA = request.Header.Get("User-Agent")
		}
		var extraBetas []string
		requestBody, extraBetas = processClaudeRequestBody(requestBody, clientUA, customCfg)
		useAPIKey := shouldUseClaudeAPIKey(apiKey, request)
		isOAuthToken = isClaudeOAuthToken(apiKey)
		if isOAuthToken {
			requestBody = applyClaudeToolPrefix(requestBody, claudeToolPrefix)
		}

		// 2. Set headers — pick variant based on the effective disguise type
		// (ResolveDisguise migrates the legacy `cloak` JSON field on the way in).
		//   bedrock      — strip Claude Code identity entirely (applyBedrockCompatHeaders)
		//   none         — raw forwarding: copy client headers, override auth only
		//   claude-code  — inject Claude Code identity headers (legacy default)
		//   "" / nil     — same as claude-code (backward compatibility)
		effectiveDisguise := customCfg.ResolveDisguise()
		disguiseType := ""
		if effectiveDisguise != nil {
			disguiseType = strings.ToLower(strings.TrimSpace(effectiveDisguise.Type))
		}
		switch disguiseType {
		case domain.DisguiseTypeBedrock:
			applyBedrockCompatHeaders(upstreamReq, request, apiKey, stream)
		case domain.DisguiseTypeNone:
			// Raw forwarding: copy client headers, then override auth with the
			// provider's key. This preserves whatever the inbound client sent
			// without injecting any Claude Code fingerprints.
			originalHeaders := flow.GetRequestHeaders(c)
			upstreamReq.Header = make(http.Header)
			copyHeadersFiltered(upstreamReq.Header, originalHeaders)

			// processClaudeRequestBody always strips body-side `betas` into
			// extraBetas. The legacy claude-code header path re-merges them
			// into Anthropic-Beta; raw forwarding mode has to do it here too,
			// otherwise body beta flags silently disappear before reaching
			// upstream.
			if len(extraBetas) > 0 {
				upstreamReq.Header.Set(
					"Anthropic-Beta",
					mergeBetaList(upstreamReq.Header.Get("Anthropic-Beta"), extraBetas),
				)
			}

			// We're inside `case domain.ClientTypeClaude:`, so the upstream is
			// always a Claude-format endpoint regardless of what the inbound
			// client looked like. setClaudeAuthForURL handles all three concerns
			// in one call:
			//
			//   - clears every stale source credential header
			//     (Authorization / x-api-key / x-goog-api-key) so a converted
			//     OpenAI-, Codex- or Gemini-origin request can't leak its source
			//     auth alongside the provider key
			//   - writes the URL-appropriate Claude auth (x-api-key for direct
			//     api.anthropic.com, Authorization: Bearer for every other host)
			//   - is a no-op when apiKey is empty
			setClaudeAuthForURL(upstreamReq, apiKey, useAPIKey)
		default:
			applyClaudeHeaders(upstreamReq, request, apiKey, useAPIKey, extraBetas, stream)
		}

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

	// Forward X-Mock-* headers from client request to upstream (test mode only)
	if mockMode && request != nil {
		for key, values := range request.Header {
			if strings.HasPrefix(strings.ToLower(key), "x-mock-") {
				for _, v := range values {
					upstreamReq.Header.Set(key, v)
				}
			}
		}
	}

	// Send request info via EventChannel
	if eventChan := flow.GetEventChan(c); eventChan != nil {
		eventChan.SendRequestInfo(&domain.RequestInfo{
			Method:  upstreamReq.Method,
			URL:     upstreamURL,
			Headers: sanitizeHeadersForEvent(upstreamReq.Header),
			Body:    string(requestBody),
		})
	}

	// Execute request with reasonable timeout
	client := &http.Client{
		Timeout: 10 * time.Minute, // Long timeout for LLM requests
	}
	resp, err := client.Do(upstreamReq)
	if err != nil {
		proxyErr := domain.NewScopedProxyError(domain.ErrUpstreamError, domain.ScopeProvider, domain.CooldownReasonNetworkError)
		proxyErr.Message = "failed to connect to upstream"
		return proxyErr
	}
	defer resp.Body.Close()

	// Check for error response
	if resp.StatusCode >= 400 {
		// Decompress error response if needed (Claude requests use Accept-Encoding)
		reader, decompErr := decompressResponse(resp)
		if decompErr != nil {
			proxyErr := domain.NewProxyErrorWithMessage(decompErr, false, "failed to decompress error response")
			proxyErr.Scope = domain.ScopeRequest
			return proxyErr
		}
		defer reader.Close()

		body, _ := io.ReadAll(reader)

		// Try error fixers: if a fixer matches, fix the request and retry once
		if retryErr := a.retryWithFixer(c, ctx, resp, body, clientType, upstreamReq, requestBody, isOAuthToken, client); retryErr == nil {
			return nil
		}

		// Send error response info via EventChannel
		if eventChan := flow.GetEventChan(c); eventChan != nil {
			eventChan.SendResponseInfo(&domain.ResponseInfo{
				Status:  resp.StatusCode,
				Headers: flattenHeaders(resp.Header),
				Body:    string(body),
			})
		}

		proxyErr := classifyHTTPError(resp.StatusCode, body, resp.Header, clientType, flow.GetMappedModel(c))
		return proxyErr
	}

	// Handle response
	// Note: Response format conversion is handled by Executor's ConvertingResponseWriter
	// Adapters simply pass through the upstream response
	var handleErr error
	if stream {
		handleErr = a.handleStreamResponse(c, resp, clientType, isOAuthToken)
	} else {
		handleErr = a.handleNonStreamResponse(c, resp, clientType, isOAuthToken)
	}
	if handleErr == nil {
		return nil
	}

	// For SSE errors detected before any data was sent to client,
	// try error fixers (e.g. upstream rejects cache_control via SSE error event)
	if proxyErr, ok := handleErr.(*domain.ProxyError); ok && proxyErr.Message != "" {
		if retryErr := a.retryWithFixer(c, ctx, nil, []byte(proxyErr.Message), clientType, upstreamReq, requestBody, isOAuthToken, client); retryErr == nil {
			return nil
		}
	}

	return handleErr
}

func (a *CustomAdapter) supportsClientType(ct domain.ClientType) bool {
	for _, supported := range a.provider.SupportedClientTypes {
		if supported == ct {
			return true
		}
	}
	return false
}

// maxFixerRounds is an absolute safety limit for fixer retry rounds.
const maxFixerRounds = 5

// retryWithFixer tries to find matching error fixers and retries the request.
// If the retry produces a NEW error (different fixers match), it continues retrying.
// If the same set of fixers matches again (no progress), it stops immediately.
// Returns nil if retry succeeded, non-nil error otherwise (including no fixer found).
// resp may be nil for SSE errors.
func (a *CustomAdapter) retryWithFixer(
	c *flow.Ctx,
	ctx context.Context,
	resp *http.Response,
	errBody []byte,
	clientType domain.ClientType,
	origReq *http.Request,
	origBody []byte,
	isOAuthToken bool,
	client *http.Client,
) error {
	currentResp := resp
	currentErrBody := errBody
	currentReq := origReq
	currentBody := origBody
	appliedFixers := make(map[string]bool)

	for round := 0; round < maxFixerRounds; round++ {
		fixers := error_fixer.FindFixers(currentResp, currentErrBody, clientType)
		if len(fixers) == 0 {
			if round == 0 {
				return fmt.Errorf("no fixer matched")
			}
			return fmt.Errorf("retry failed with status %d (no fixer for new error)", currentResp.StatusCode)
		}

		// Check if any fixer is new (not yet applied)
		hasNew := false
		for _, fixer := range fixers {
			if !appliedFixers[fixer.Name()] {
				hasNew = true
				break
			}
		}
		if !hasNew {
			// All matched fixers have already been applied — no progress, stop
			return fmt.Errorf("retry failed: fixers %v already applied but error persists", fixerNames(fixers))
		}

		// Apply all matching fixers and record them
		retryReq := currentReq.Clone(ctx)
		fixedBody := currentBody
		for _, fixer := range fixers {
			log.Printf("[custom] error fixer %q matched (round %d)", fixer.Name(), round+1)
			retryReq, fixedBody = fixer.FixRequest(retryReq, fixedBody)
			appliedFixers[fixer.Name()] = true
		}

		// Set the fixed body on the request
		retryReq.Body = io.NopCloser(bytes.NewReader(fixedBody))
		retryReq.ContentLength = int64(len(fixedBody))

		if eventChan := flow.GetEventChan(c); eventChan != nil {
			eventChan.SendRequestInfo(&domain.RequestInfo{
				Method:  retryReq.Method,
				URL:     retryReq.URL.String(),
				Headers: sanitizeHeadersForEvent(retryReq.Header),
				Body:    string(fixedBody),
			})
		}

		retryResp, err := client.Do(retryReq)
		if err != nil {
			return err
		}

		if retryResp.StatusCode < 400 {
			// Success — handle the response
			defer retryResp.Body.Close()
			if isStreamRequest(fixedBody) {
				return a.handleStreamResponse(c, retryResp, clientType, isOAuthToken)
			}
			return a.handleNonStreamResponse(c, retryResp, clientType, isOAuthToken)
		}

		// Still failing — read the new error body for next round
		reader, decompErr := decompressResponse(retryResp)
		if decompErr != nil {
			retryResp.Body.Close()
			return fmt.Errorf("retry failed with status %d", retryResp.StatusCode)
		}
		newErrBody, _ := io.ReadAll(reader)
		reader.Close()
		retryResp.Body.Close()

		currentResp = retryResp
		currentErrBody = newErrBody
		currentReq = retryReq
		currentBody = fixedBody
	}

	return fmt.Errorf("retry exhausted after %d rounds", maxFixerRounds)
}

func fixerNames(fixers []error_fixer.ErrorFixer) []string {
	names := make([]string, len(fixers))
	for i, f := range fixers {
		names[i] = f.Name()
	}
	return names
}

func (a *CustomAdapter) getBaseURL(clientType domain.ClientType) string {
	config := a.provider.Config.Custom
	if url, ok := config.ClientBaseURL[clientType]; ok && url != "" {
		return url
	}
	return config.BaseURL
}

// customPassthroughFlag returns the provider's Codex Responses passthrough flag
// (nil when unconfigured → treated as default-on by ResponsesPassthroughEnabled).
func (a *CustomAdapter) customPassthroughFlag() *bool {
	if cfg := a.provider.Config.Custom; cfg != nil {
		return cfg.ResponsesPassthrough
	}
	return nil
}

func (a *CustomAdapter) handleNonStreamResponse(c *flow.Ctx, resp *http.Response, clientType domain.ClientType, isOAuthToken bool) error {
	// Decompress response body if needed
	reader, err := decompressResponse(resp)
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(err, false, "failed to decompress response")
		proxyErr.Scope = domain.ScopeRequest
		return proxyErr
	}
	defer reader.Close()

	body, err := io.ReadAll(reader)
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to read upstream response")
		proxyErr.Scope = domain.ScopeProvider
		proxyErr.Reason = domain.CooldownReasonNetworkError
		return proxyErr
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
				InputImageTokens:     metrics.InputImageTokens,
				OutputImageTokens:    metrics.OutputImageTokens,
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
	if _, err := c.Writer.Write(body); err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(err, false, "client disconnected")
		proxyErr.Scope = domain.ScopeRequest
		return proxyErr
	}
	return nil
}

func (a *CustomAdapter) handleStreamResponse(c *flow.Ctx, resp *http.Response, clientType domain.ClientType, isOAuthToken bool) error {
	// Decompress response body if needed
	reader, err := decompressResponse(resp)
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(err, false, "failed to decompress response")
		proxyErr.Scope = domain.ScopeRequest
		return proxyErr
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
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, false, "streaming not supported")
		proxyErr.Scope = domain.ScopeRequest
		return proxyErr
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
				InputImageTokens:     metrics.InputImageTokens,
				OutputImageTokens:    metrics.OutputImageTokens,
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
				proxyErr := domain.NewProxyErrorWithMessage(
					fmt.Errorf("SSE error (code=%d): %s", code, msg),
					isRetryableSSEError(code, errType, msg),
					msg,
				)
				proxyErr.Scope = domain.ScopeProvider
				proxyErr.Reason = domain.CooldownReasonServerError
				return proxyErr
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
				proxyErr.Scope = domain.ScopeProvider
				proxyErr.Reason = domain.CooldownReasonServerError
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
			proxyErr := domain.NewProxyErrorWithMessage(ctx.Err(), false, "client disconnected")
			proxyErr.Scope = domain.ScopeRequest
			return proxyErr
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
						proxyErr := domain.NewProxyErrorWithMessage(writeErr, false, "client disconnected")
						proxyErr.Scope = domain.ScopeRequest
						return proxyErr
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
				proxyErr := domain.NewProxyErrorWithMessage(ctx.Err(), false, "client disconnected")
				proxyErr.Scope = domain.ScopeRequest
				return proxyErr
			}
			sendFinalEvents()
			// Return SSE error if one was detected during streaming
			if sseError != nil {
				return sseError
			}
			if !firstChunkSent {
				proxyErr := domain.NewProxyErrorWithMessage(err, true, "upstream stream read error before response started")
				proxyErr.Scope = domain.ScopeProvider
				proxyErr.Reason = domain.CooldownReasonNetworkError
				return proxyErr
			}
			proxyErr := domain.NewProxyErrorWithMessage(err, false, "upstream stream read error after response started")
			proxyErr.Scope = domain.ScopeProvider
			proxyErr.Reason = domain.CooldownReasonNetworkError
			return proxyErr
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

// updateModelInMultipartForm rewrites the "model" form field of a
// multipart/form-data body (OpenAI images/edits) to the mapped model, copying
// every other part — including the uploaded image — through unchanged. It
// reuses the request's original boundary so the existing Content-Type header
// stays valid (no header rewrite needed). If the body carries no "model" field,
// one is appended so a configured mapping still applies.
func updateModelInMultipartForm(body []byte, req *http.Request, model string) ([]byte, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request for multipart model rewrite")
	}
	_, params, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}
	boundary := params["boundary"]
	if boundary == "" {
		return nil, fmt.Errorf("multipart body without boundary")
	}

	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	var out bytes.Buffer
	mw := multipart.NewWriter(&out)
	// Preserve the original boundary so the inbound Content-Type header (copied
	// to the upstream request) still matches the re-encoded body.
	if err := mw.SetBoundary(boundary); err != nil {
		return nil, err
	}

	replaced := false
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		w, err := mw.CreatePart(part.Header)
		if err != nil {
			_ = part.Close()
			return nil, err
		}
		if part.FormName() == "model" {
			_, err = io.WriteString(w, model)
			replaced = true
		} else {
			_, err = io.Copy(w, part)
		}
		_ = part.Close()
		if err != nil {
			return nil, err
		}
	}
	if !replaced {
		if err := mw.WriteField("model", model); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func buildUpstreamURL(baseURL string, requestPath string) string {
	return strings.TrimSuffix(baseURL, "/") + requestPath
}

// isMultipartForm reports whether the request body is multipart/form-data
// (e.g. OpenAI images/edits image upload), which must not be JSON-rewritten.
func isMultipartForm(req *http.Request) bool {
	if req == nil {
		return false
	}
	ct := strings.ToLower(strings.TrimSpace(req.Header.Get("Content-Type")))
	return strings.HasPrefix(ct, "multipart/")
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

// sanitizeHeadersForEvent returns a header map safe for request event logging,
// redacting upstream credentials that may be injected from provider config.
func sanitizeHeadersForEvent(h http.Header) map[string]string {
	result := flattenHeaders(h)
	for key := range result {
		if isSensitiveEventHeader(key) {
			result[key] = "[REDACTED]"
		}
	}
	return result
}

func isSensitiveEventHeader(key string) bool {
	switch strings.ToLower(key) {
	case "authorization",
		"proxy-authorization",
		"x-api-key",
		"x-goog-api-key",
		"api-key",
		"anthropic-api-key",
		"openai-api-key",
		"x-amz-security-token",
		"cookie",
		"set-cookie":
		return true
	default:
		return false
	}
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

// classifyHTTPError creates a structured ProxyError from an HTTP error response.
// It determines Scope (what's broken) and Reason (why) from status code and body.
func classifyHTTPError(statusCode int, body []byte, headers http.Header, clientType domain.ClientType, model string) *domain.ProxyError {
	bodyStr := string(body)
	bodyLower := strings.ToLower(bodyStr)

	proxyErr := &domain.ProxyError{
		Err:            fmt.Errorf("upstream error: %s", bodyStr),
		Message:        fmt.Sprintf("upstream returned status %d", statusCode),
		HTTPStatusCode: statusCode,
		Retryable:      isRetryableStatusCode(statusCode),
	}

	// Parse Retry-After header (used by several branches)
	if retryAfter, until := parseRetryAfterHeader(headers.Get("Retry-After")); retryAfter > 0 {
		proxyErr.RetryAfter = retryAfter
		proxyErr.CooldownUntil = until
	}

	switch {
	// 401 — invalid key / expired token
	case statusCode == 401:
		proxyErr.Scope = domain.ScopeKey
		proxyErr.Reason = domain.CooldownReasonAuthFailure
		proxyErr.Retryable = false

	// 403 — check if model-specific or account-level
	case statusCode == 403:
		if containsAny(bodyLower, "model", "access denied for model", "permission denied for model") {
			proxyErr.Scope = domain.ScopeModel
			proxyErr.Reason = domain.CooldownReasonModelUnavailable
			proxyErr.Model = model
		} else {
			proxyErr.Scope = domain.ScopeKey
			proxyErr.Reason = domain.CooldownReasonAuthFailure
		}
		proxyErr.Retryable = false

	// 404 — model not found
	case statusCode == 404:
		if containsAny(bodyLower, "model", "not found") {
			proxyErr.Scope = domain.ScopeModel
			proxyErr.Reason = domain.CooldownReasonModelUnavailable
			proxyErr.Model = model
			proxyErr.Retryable = false
		} else {
			proxyErr.Scope = domain.ScopeEndpoint
			proxyErr.Reason = domain.CooldownReasonServerError
		}

	// 400, 408, 413, 422 — request-level errors
	case statusCode == 400 || statusCode == 408 || statusCode == 413 || statusCode == 422:
		proxyErr.Scope = domain.ScopeRequest
		proxyErr.Retryable = false

	// 429 — rate limit (need to disambiguate)
	case statusCode == 429:
		proxyErr.Retryable = true
		proxyErr.ClientType = string(clientType)
		classify429Error(proxyErr, body, bodyLower, headers, model)

	// 503 — check if model overloaded or full outage
	case statusCode == 503:
		if containsAny(bodyLower, "model", "overloaded", "capacity") {
			proxyErr.Scope = domain.ScopeModel
			proxyErr.Reason = domain.CooldownReasonServerError
			proxyErr.Model = model
		} else {
			proxyErr.Scope = domain.ScopeProvider
			proxyErr.Reason = domain.CooldownReasonServerError
		}

	// 500, 502, 504 — provider-level server errors
	case statusCode >= 500:
		proxyErr.Scope = domain.ScopeProvider
		proxyErr.Reason = domain.CooldownReasonServerError

	// Other 4xx — request-level
	default:
		proxyErr.Scope = domain.ScopeRequest
		proxyErr.Retryable = false
	}

	return proxyErr
}

// classify429Error determines the scope and reason for 429 rate limit errors.
func classify429Error(proxyErr *domain.ProxyError, body []byte, bodyLower string, headers http.Header, model string) {
	// Default to key-level rate limit
	proxyErr.Scope = domain.ScopeKey
	proxyErr.Reason = domain.CooldownReasonRateLimitExceeded

	// Check for quota exhaustion
	if containsAny(bodyLower, "quota", "exceeded your", "insufficient_quota") {
		proxyErr.Reason = domain.CooldownReasonQuotaExhausted
	} else if containsAny(bodyLower, "concurrent") {
		proxyErr.Reason = domain.CooldownReasonConcurrentLimit
	}

	// Try to parse structured error for more detail
	var errResp struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &errResp) == nil {
		if errResp.Error.Type == "insufficient_quota" || errResp.Error.Code == "insufficient_quota" {
			proxyErr.Reason = domain.CooldownReasonQuotaExhausted
		}
		// Extract time from message if no Retry-After header
		if proxyErr.CooldownUntil == nil {
			if t := extractTimeFromMessage(errResp.Error.Message); !t.IsZero() {
				proxyErr.CooldownUntil = &t
			}
		}
	}

	// Try structured reset time fields
	if proxyErr.CooldownUntil == nil {
		if t := extractStructuredResetTime(body); !t.IsZero() {
			proxyErr.CooldownUntil = &t
		}
	}

	// Per-model rate limit detection (if body mentions specific model)
	if containsAny(bodyLower, "model", "tokens per minute", "tpm") {
		proxyErr.Scope = domain.ScopeModel
		proxyErr.Model = model
	}
}

// containsAny returns true if s contains any of the substrings.
func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
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
