package antigravity

import (
	"bytes"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Bowl42/maxx-next/internal/adapter/provider"
	ctxutil "github.com/Bowl42/maxx-next/internal/context"
	"github.com/Bowl42/maxx-next/internal/cooldown"
	"github.com/Bowl42/maxx-next/internal/domain"
	"github.com/Bowl42/maxx-next/internal/usage"
)

func init() {
	provider.RegisterAdapterFactory("antigravity", NewAdapter)
}

// TokenCache caches access tokens
type TokenCache struct {
	AccessToken string
	ExpiresAt   time.Time
}

type AntigravityAdapter struct {
	provider   *domain.Provider
	tokenCache *TokenCache
	tokenMu    sync.RWMutex
}

func NewAdapter(p *domain.Provider) (provider.ProviderAdapter, error) {
	if p.Config == nil || p.Config.Antigravity == nil {
		return nil, fmt.Errorf("provider %s missing antigravity config", p.Name)
	}
	return &AntigravityAdapter{
		provider:   p,
		tokenCache: &TokenCache{},
	}, nil
}

func (a *AntigravityAdapter) SupportedClientTypes() []domain.ClientType {
	// Antigravity natively supports Claude, OpenAI, and Gemini by converting to Gemini/v1internal API
	return []domain.ClientType{domain.ClientTypeClaude, domain.ClientTypeOpenAI, domain.ClientTypeGemini}
}

func (a *AntigravityAdapter) Execute(ctx context.Context, w http.ResponseWriter, req *http.Request, provider *domain.Provider) error {
	clientType := ctxutil.GetClientType(ctx)
	baseCtx := ctx
	requestModel := ctxutil.GetRequestModel(ctx) // Original model from request (e.g., "claude-3-5-sonnet-20241022-online")
	mappedModel := ctxutil.GetMappedModel(ctx)   // Mapped model after route resolution
	requestBody := ctxutil.GetRequestBody(ctx)

	// [Model Mapping] Apply Antigravity model mapping (like Antigravity-Manager)
	// We'll attempt at most twice: original + retry without thinking on signature errors
	retriedWithoutThinking := false

	for attemptIdx := 0; attemptIdx < 2; attemptIdx++ {
		ctx = ctxutil.WithRequestModel(baseCtx, requestModel)
		ctx = ctxutil.WithRequestBody(ctx, requestBody)

		// Only map if route didn't provide a mapping (mappedModel empty or same as request)
		config := provider.Config.Antigravity
		if mappedModel == "" || mappedModel == requestModel {
			// Route didn't provide mapping, use our internal mapping with haikuTarget config
			haikuTarget := ""
			if config != nil {
				haikuTarget = config.HaikuTarget
			}
			mappedModel = MapClaudeModelToGeminiWithConfig(requestModel, haikuTarget)
		}
		// If route provided a different mappedModel, trust it and don't re-map
		// (user/route has explicitly configured the target model)

	// Get streaming flag from context (already detected correctly for Gemini URL path)
	stream := ctxutil.GetIsStream(ctx)
	clientWantsStream := stream
	actualStream := stream
	if !clientWantsStream {
		// Auto-convert non-stream to stream internally for better quota (like Manager)
		actualStream = true
	}

		// Get access token
		accessToken, err := a.getAccessToken(ctx)
		if err != nil {
			return domain.NewProxyErrorWithMessage(err, true, "failed to get access token")
		}

		// [SessionID Support] Extract metadata.user_id from original request for sessionId (like Antigravity-Manager)
		sessionID := extractSessionID(requestBody)

		// Transform request based on client type
		var geminiBody []byte
		if clientType == domain.ClientTypeClaude {
			// Use direct transformation (no converter dependency)
			// This combines cache control cleanup, thinking filter, tool loop recovery,
			// system instruction building, content transformation, tool building, and generation config
			geminiBody, err = TransformClaudeToGemini(requestBody, mappedModel, actualStream, sessionID, GlobalSignatureCache())
			if err != nil {
				return domain.NewProxyErrorWithMessage(err, true, fmt.Sprintf("failed to transform Claude request: %v", err))
			}

			// Apply additional post-processing for advanced features
			// (interleaved hint, toolConfig, signature validation)
			hasThinking := HasThinkingEnabledWithModel(requestBody, mappedModel)
			if hasThinking && !TargetModelSupportsThinking(mappedModel) {
				hasThinking = false
			}

			// Check if thinking should be disabled due to history
			if hasThinking {
				var req map[string]interface{}
				if err := json.Unmarshal(geminiBody, &req); err == nil {
					if contents, ok := req["contents"].([]interface{}); ok {
						if ShouldDisableThinkingDueToHistory(contents) {
							hasThinking = false
						}
					}
				}
			}

			// Apply minimal post-processing for features not yet fully integrated
			geminiBody = applyClaudePostProcess(geminiBody, sessionID, hasThinking, requestBody, mappedModel)
		} else if clientType == domain.ClientTypeOpenAI {
			// TODO: Implement OpenAI transformation in the future
			return domain.NewProxyErrorWithMessage(domain.ErrFormatConversion, true, "OpenAI transformation not yet implemented")
		} else {
			// For Gemini, unwrap CLI envelope if present
			geminiBody = unwrapGeminiCLIEnvelope(requestBody)
		}

		// Wrap request in v1internal format
		upstreamBody, err := wrapV1InternalRequest(geminiBody, config.ProjectID, requestModel, mappedModel, sessionID)
		if err != nil {
			return domain.NewProxyErrorWithMessage(domain.ErrFormatConversion, true, "failed to wrap request for v1internal")
		}

		// Build upstream URLs (prod first, daily fallback)
		baseURLs := []string{V1InternalBaseURLProd, V1InternalBaseURLDaily}
		client := &http.Client{}
		var lastErr error

		for idx, base := range baseURLs {
			upstreamURL := a.buildUpstreamURL(base, actualStream)

			upstreamReq, reqErr := http.NewRequestWithContext(ctx, "POST", upstreamURL, bytes.NewReader(upstreamBody))
			if reqErr != nil {
				lastErr = reqErr
				continue
			}

			// Set only the required headers (like Antigravity-Manager)
			upstreamReq.Header.Set("Content-Type", "application/json")
			upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)
			upstreamReq.Header.Set("User-Agent", AntigravityUserAgent)

			// Capture request info for attempt record (only once)
			if attempt := ctxutil.GetUpstreamAttempt(ctx); attempt != nil && attempt.RequestInfo == nil {
				attempt.RequestInfo = &domain.RequestInfo{
					Method:  upstreamReq.Method,
					URL:     upstreamURL,
					Headers: flattenHeaders(upstreamReq.Header),
					Body:    string(upstreamBody),
				}
			}

			resp, err := client.Do(upstreamReq)
			if err != nil {
				lastErr = err
				if hasNextEndpoint(idx, len(baseURLs)) {
					continue
				}
				return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to connect to upstream")
			}
			defer resp.Body.Close()

			// Check for 401 (token expired) and retry once
			if resp.StatusCode == http.StatusUnauthorized {
				resp.Body.Close()

				// Invalidate token cache
				a.tokenMu.Lock()
				a.tokenCache = &TokenCache{}
				a.tokenMu.Unlock()

				// Get new token
				accessToken, err = a.getAccessToken(ctx)
				if err != nil {
					return domain.NewProxyErrorWithMessage(err, true, "failed to refresh access token")
				}

				// Retry request with only required headers
				upstreamReq, _ = http.NewRequestWithContext(ctx, "POST", upstreamURL, bytes.NewReader(upstreamBody))
				upstreamReq.Header.Set("Content-Type", "application/json")
				upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)
				upstreamReq.Header.Set("User-Agent", AntigravityUserAgent)
				resp, err = client.Do(upstreamReq)
				if err != nil {
					lastErr = err
					if hasNextEndpoint(idx, len(baseURLs)) {
						continue
					}
					return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to connect to upstream after token refresh")
				}
				defer resp.Body.Close()
			}

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

				// Check for RESOURCE_EXHAUSTED (429) and handle cooldown
				if resp.StatusCode == http.StatusTooManyRequests {
					a.handleResourceExhausted(ctx, body, provider)
				}

				// Parse retry info for 429/5xx responses (like Antigravity-Manager)
				var retryAfter time.Duration
				if retryInfo := ParseRetryInfo(resp.StatusCode, body); retryInfo != nil {
					retryAfter = ApplyJitter(retryInfo.Delay)
				}

				proxyErr := domain.NewProxyErrorWithMessage(
					fmt.Errorf("upstream error: %s", string(body)),
					isRetryableStatusCode(resp.StatusCode),
					fmt.Sprintf("upstream returned status %d", resp.StatusCode),
				)

				// Set retry info on error for upstream handling
				if retryAfter > 0 {
					proxyErr.RetryAfter = retryAfter
				}

				lastErr = proxyErr

				// Signature failure recovery: retry once without thinking (like Manager)
				if resp.StatusCode == http.StatusBadRequest && !retriedWithoutThinking && isThinkingSignatureError(body) {
					retriedWithoutThinking = true
					requestBody = stripThinkingFromClaude(requestBody)
					if newModel := extractModelFromBody(requestBody); newModel != "" {
						requestModel = newModel
					}
					mappedModel = "" // force remap
					continue
				}

				// Fallback to next endpoint if available and retryable
				if hasNextEndpoint(idx, len(baseURLs)) && shouldTryNextEndpoint(resp.StatusCode) {
					resp.Body.Close()
					continue
				}

				return proxyErr
			}

			// Handle response
			if actualStream && !clientWantsStream {
				return a.handleCollectedStreamResponse(ctx, w, resp, clientType, requestModel)
			}
			if actualStream {
				return a.handleStreamResponse(ctx, w, resp, clientType)
			}
			return a.handleNonStreamResponse(ctx, w, resp, clientType)
		}

		// All endpoints failed in this iteration
		if lastErr != nil {
			if proxyErr, ok := lastErr.(*domain.ProxyError); ok {
				return proxyErr
			}
			return domain.NewProxyErrorWithMessage(lastErr, true, "all upstream endpoints failed")
		}
	}

	return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "all upstream endpoints failed")
}

func (a *AntigravityAdapter) getAccessToken(ctx context.Context) (string, error) {
	// Check cache
	a.tokenMu.RLock()
	if a.tokenCache.AccessToken != "" && time.Now().Before(a.tokenCache.ExpiresAt) {
		token := a.tokenCache.AccessToken
		a.tokenMu.RUnlock()
		return token, nil
	}
	a.tokenMu.RUnlock()

	// Refresh token
	config := a.provider.Config.Antigravity
	accessToken, expiresIn, err := refreshGoogleToken(ctx, config.RefreshToken)
	if err != nil {
		return "", err
	}

	// Cache token
	a.tokenMu.Lock()
	a.tokenCache = &TokenCache{
		AccessToken: accessToken,
		ExpiresAt:   time.Now().Add(time.Duration(expiresIn-60) * time.Second), // 60s buffer
	}
	a.tokenMu.Unlock()

	return accessToken, nil
}

func refreshGoogleToken(ctx context.Context, refreshToken string) (string, int, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", OAuthClientID)
	data.Set("client_secret", OAuthClientSecret)

	req, err := http.NewRequestWithContext(ctx, "POST", "https://oauth2.googleapis.com/token", strings.NewReader(data.Encode()))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", 0, fmt.Errorf("token refresh failed: %s", string(body))
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", 0, err
	}

	return result.AccessToken, result.ExpiresIn, nil
}

// applyClaudePostProcess applies minimal post-processing for advanced features
// not yet fully integrated into the transform functions
func applyClaudePostProcess(geminiBody []byte, sessionID string, hasThinking bool, claudeRequest []byte, mappedModel string) []byte {
	var request map[string]interface{}
	if err := json.Unmarshal(geminiBody, &request); err != nil {
		return geminiBody
	}

	modified := false

	// 1. Inject interleaved thinking hint when tools + thinking enabled
	hasTools := hasToolDeclarations(request)
	if hasThinking && hasTools {
		if injectInterleavedHint(request) {
			modified = true
		}
	}

	// 2. Inject toolConfig with VALIDATED mode when tools exist
	if InjectToolConfig(request) {
		modified = true
	}

	// 3. Process contents for additional signature validation
	if contents, ok := request["contents"].([]interface{}); ok {
		if processContentsForSignatures(contents, sessionID, mappedModel) {
			modified = true
		}
	}

	// 4. Clean thinking fields if disabled
	if !hasThinking {
		CleanThinkingFieldsRecursive(request)
		modified = true
	}

	if !modified {
		return geminiBody
	}

	result, err := json.Marshal(request)
	if err != nil {
		return geminiBody
	}
	return result
}

// v1internal endpoints (prod + daily fallback, like Antigravity-Manager)
const (
	V1InternalBaseURLProd  = "https://cloudcode-pa.googleapis.com/v1internal"
	V1InternalBaseURLDaily = "https://daily-cloudcode-pa.googleapis.com/v1internal"
)

func (a *AntigravityAdapter) buildUpstreamURL(base string, stream bool) string {
	if stream {
		return fmt.Sprintf("%s:streamGenerateContent?alt=sse", base)
	}
	return fmt.Sprintf("%s:generateContent", base)
}

func hasNextEndpoint(index, total int) bool {
	return index+1 < total
}

// shouldTryNextEndpoint decides if we should fall back to the next endpoint
// Mirrors Antigravity-Manager: retry on 429, 408, 404, and 5xx errors.
func shouldTryNextEndpoint(status int) bool {
	if status == http.StatusTooManyRequests || status == http.StatusRequestTimeout || status == http.StatusNotFound {
		return true
	}
	return status >= 500
}

// isThinkingSignatureError detects thinking signature related 400 errors (like Manager)
func isThinkingSignatureError(body []byte) bool {
	bodyStr := strings.ToLower(string(body))
	return strings.Contains(bodyStr, "invalid `signature`") ||
		strings.Contains(bodyStr, "thinking.signature") ||
		strings.Contains(bodyStr, "thinking.thinking") ||
		strings.Contains(bodyStr, "corrupted thought signature") ||
		strings.Contains(bodyStr, "failed to deserialise")
}

func (a *AntigravityAdapter) handleNonStreamResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response, clientType domain.ClientType) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to read upstream response")
	}

	// Unwrap v1internal response wrapper (extract "response" field)
	unwrappedBody := unwrapV1InternalResponse(body)

	// Capture response info and extract token usage
	if attempt := ctxutil.GetUpstreamAttempt(ctx); attempt != nil {
		attempt.ResponseInfo = &domain.ResponseInfo{
			Status:  resp.StatusCode,
			Headers: flattenHeaders(resp.Header),
			Body:    string(body), // Keep original for debugging
		}

		// Extract token usage from unwrapped response
		if metrics := usage.ExtractFromResponse(string(unwrappedBody)); metrics != nil {
			attempt.InputTokenCount = metrics.InputTokens
			attempt.OutputTokenCount = metrics.OutputTokens
			attempt.CacheReadCount = metrics.CacheReadCount
			attempt.CacheWriteCount = metrics.CacheCreationCount
			attempt.Cache5mWriteCount = metrics.Cache5mCreationCount
			attempt.Cache1hWriteCount = metrics.Cache1hCreationCount
		}

		// Broadcast attempt update with token info
		if bc := ctxutil.GetBroadcaster(ctx); bc != nil {
			bc.BroadcastProxyUpstreamAttempt(attempt)
		}
	}

	var responseBody []byte

	// Transform response based on client type
	if clientType == domain.ClientTypeClaude {
		requestModel := ctxutil.GetRequestModel(ctx)
		responseBody, err = convertGeminiToClaudeResponse(unwrappedBody, requestModel)
		if err != nil {
			return domain.NewProxyErrorWithMessage(domain.ErrFormatConversion, false, "failed to transform response")
		}
	} else if clientType == domain.ClientTypeOpenAI {
		// TODO: Implement OpenAI response transformation
		return domain.NewProxyErrorWithMessage(domain.ErrFormatConversion, false, "OpenAI response transformation not yet implemented")
	} else {
		// Gemini native
		responseBody = unwrappedBody
	}

	// Copy upstream headers (except those we override)
	copyResponseHeaders(w.Header(), resp.Header)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(responseBody)
	return nil
}

func (a *AntigravityAdapter) handleStreamResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response, clientType domain.ClientType) error {
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

	// Use specialized Claude SSE handler for Claude clients
	isClaudeClient := clientType == domain.ClientTypeClaude

	// Extract sessionID for signature caching (like CLIProxyAPI)
	requestBody := ctxutil.GetRequestBody(ctx)
	sessionID := extractSessionID(requestBody)

	// Get original request model for Claude response (like Antigravity-Manager)
	requestModel := ctxutil.GetRequestModel(ctx)

	var claudeState *ClaudeStreamingState
	if isClaudeClient {
		claudeState = NewClaudeStreamingStateWithSession(sessionID, requestModel)
	}

	// Collect all SSE events for response body and token extraction
	var sseBuffer strings.Builder

	// Helper to extract tokens and update attempt with final response body
	extractTokens := func() {
		if attempt != nil && sseBuffer.Len() > 0 {
			// Update response body with collected SSE content
			if attempt.ResponseInfo != nil {
				attempt.ResponseInfo.Body = sseBuffer.String()
			}
			// Extract token usage
			if metrics := usage.ExtractFromStreamContent(sseBuffer.String()); metrics != nil {
				attempt.InputTokenCount = metrics.InputTokens
				attempt.OutputTokenCount = metrics.OutputTokens
				attempt.CacheReadCount = metrics.CacheReadCount
				attempt.CacheWriteCount = metrics.CacheCreationCount
				attempt.Cache5mWriteCount = metrics.Cache5mCreationCount
				attempt.Cache1hWriteCount = metrics.Cache1hCreationCount
			}
			// Broadcast attempt update with token info
			if bc := ctxutil.GetBroadcaster(ctx); bc != nil {
				bc.BroadcastProxyUpstreamAttempt(attempt)
			}
		}
	}

	// Use buffer-based approach like Antigravity-Manager
	// Read chunks and accumulate until we have complete lines
	var lineBuffer bytes.Buffer
	buf := make([]byte, 4096)

	for {
		// Check context before reading
		select {
		case <-ctx.Done():
			extractTokens()
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

				// Collect for token extraction
				sseBuffer.WriteString(line)

				// Process the complete line
				lineBytes := []byte(line)

				// Unwrap v1internal SSE chunk before processing
				unwrappedLine := unwrapV1InternalSSEChunk(lineBytes)

				var output []byte
				if isClaudeClient {
					// Use specialized Claude SSE transformation
					output = claudeState.ProcessGeminiSSELine(string(unwrappedLine))
				} else if clientType == domain.ClientTypeOpenAI {
					// TODO: Implement OpenAI streaming transformation
					continue
				} else {
					// Gemini native
					output = unwrappedLine
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

		if err != nil {
			if err == io.EOF {
				// Ensure Claude clients get termination events
				if isClaudeClient && claudeState != nil {
					if forceStop := claudeState.EmitForceStop(); len(forceStop) > 0 {
						_, _ = w.Write(forceStop)
						flusher.Flush()
					}
				}
				extractTokens()
				return nil
			}
			// Upstream connection closed - check if client is still connected
			if ctx.Err() != nil {
				// Try to send termination events for Claude clients
				if isClaudeClient && claudeState != nil {
					if forceStop := claudeState.EmitForceStop(); len(forceStop) > 0 {
						_, _ = w.Write(forceStop)
						flusher.Flush()
					}
				}
				extractTokens()
				return domain.NewProxyErrorWithMessage(ctx.Err(), false, "client disconnected")
			}
			// Ensure Claude clients get termination events
			if isClaudeClient && claudeState != nil {
				if forceStop := claudeState.EmitForceStop(); len(forceStop) > 0 {
					_, _ = w.Write(forceStop)
					flusher.Flush()
				}
			}
			extractTokens()
			return nil
		}
	}
}

// handleCollectedStreamResponse forwards upstream SSE but collects into a single response body (like Manager non-stream auto-convert)
func (a *AntigravityAdapter) handleCollectedStreamResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response, clientType domain.ClientType, requestModel string) error {
	attempt := ctxutil.GetUpstreamAttempt(ctx)

	if attempt != nil {
		attempt.ResponseInfo = &domain.ResponseInfo{
			Status:  resp.StatusCode,
			Headers: flattenHeaders(resp.Header),
			Body:    "[stream-collected]",
		}
	}

	var sseBuffer strings.Builder
	var lastData string

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			sseBuffer.WriteString(line)

			unwrapped := unwrapV1InternalSSEChunk([]byte(line))
			if len(unwrapped) == 0 {
				if err == io.EOF {
					break
				}
				if err != nil && err != io.EOF {
					return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to read upstream stream")
				}
				continue
			}

			lineStr := strings.TrimSpace(string(unwrapped))
			if !strings.HasPrefix(lineStr, "data: ") {
				if err == io.EOF {
					break
				}
				if err != nil && err != io.EOF {
					return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to read upstream stream")
				}
				continue
			}

			dataStr := strings.TrimSpace(strings.TrimPrefix(lineStr, "data: "))
			if dataStr == "" || dataStr == "[DONE]" {
				if err == io.EOF {
					break
				}
				if err != nil && err != io.EOF {
					return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to read upstream stream")
				}
				continue
			}

			lastData = dataStr
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to read upstream stream")
		}
	}

	// Update attempt with collected body and token usage
	if attempt != nil {
		if attempt.ResponseInfo != nil {
			attempt.ResponseInfo.Body = sseBuffer.String()
		}
		if metrics := usage.ExtractFromStreamContent(sseBuffer.String()); metrics != nil {
			attempt.InputTokenCount = metrics.InputTokens
			attempt.OutputTokenCount = metrics.OutputTokens
			attempt.CacheReadCount = metrics.CacheReadCount
			attempt.CacheWriteCount = metrics.CacheCreationCount
			attempt.Cache5mWriteCount = metrics.Cache5mCreationCount
			attempt.Cache1hWriteCount = metrics.Cache1hCreationCount
		}
		if bc := ctxutil.GetBroadcaster(ctx); bc != nil {
			bc.BroadcastProxyUpstreamAttempt(attempt)
		}
	}

	if lastData == "" {
		return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "empty upstream stream response")
	}

	var responseBody []byte
	var err error

	switch clientType {
	case domain.ClientTypeClaude:
		responseBody, err = convertGeminiToClaudeResponse([]byte(lastData), requestModel)
		if err != nil {
			return domain.NewProxyErrorWithMessage(domain.ErrFormatConversion, false, "failed to transform streamed response")
		}
	case domain.ClientTypeGemini:
		responseBody = []byte(lastData)
	case domain.ClientTypeOpenAI:
		return domain.NewProxyErrorWithMessage(domain.ErrFormatConversion, false, "OpenAI response transformation not yet implemented")
	default:
		responseBody = []byte(lastData)
	}

	copyResponseHeaders(w.Header(), resp.Header)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(responseBody)
	return nil
}

// handleResourceExhausted handles 429 RESOURCE_EXHAUSTED errors with QUOTA_EXHAUSTED reason
// Only triggers cooldown when the error contains quotaResetTimeStamp in details
func (a *AntigravityAdapter) handleResourceExhausted(ctx context.Context, body []byte, provider *domain.Provider) {
	// Parse error response to check if it's QUOTA_EXHAUSTED with reset timestamp
	var errResp struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
			Details []struct {
				Type     string `json:"@type"`
				Reason   string `json:"reason,omitempty"`
				Metadata struct {
					Model               string `json:"model,omitempty"`
					QuotaResetDelay     string `json:"quotaResetDelay,omitempty"`
					QuotaResetTimeStamp string `json:"quotaResetTimeStamp,omitempty"`
				} `json:"metadata,omitempty"`
			} `json:"details,omitempty"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &errResp); err != nil {
		// Can't parse error, don't set cooldown
		return
	}

	// Check if it's RESOURCE_EXHAUSTED
	if errResp.Error.Status != "RESOURCE_EXHAUSTED" {
		return
	}

	// Look for QUOTA_EXHAUSTED with quotaResetTimeStamp in details
	var resetTime time.Time
	for _, detail := range errResp.Error.Details {
		if detail.Reason == "QUOTA_EXHAUSTED" && detail.Metadata.QuotaResetTimeStamp != "" {
			parsed, err := time.Parse(time.RFC3339, detail.Metadata.QuotaResetTimeStamp)
			if err == nil {
				resetTime = parsed
				break
			}
		}
	}

	if resetTime.IsZero() {
		// No quota reset timestamp found, query quota API
		config := provider.Config.Antigravity
		if config == nil {
			return
		}

		// Fetch quota in background to not block the response
		go func() {
			quotaCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			quota, err := FetchQuotaForProvider(quotaCtx, config.RefreshToken, config.ProjectID)
			if err != nil {
				// Failed to fetch quota, apply short cooldown
				cooldown.Default().SetCooldownDuration(provider.ID, time.Minute)
				return
			}

			// Check if any model has 0% quota
			var earliestReset time.Time
			hasZeroQuota := false

			for _, model := range quota.Models {
				if model.Percentage == 0 && model.ResetTime != "" {
					hasZeroQuota = true
					rt, err := time.Parse(time.RFC3339, model.ResetTime)
					if err != nil {
						continue
					}
					if earliestReset.IsZero() || rt.Before(earliestReset) {
						earliestReset = rt
					}
				}
			}

			if hasZeroQuota && !earliestReset.IsZero() {
				// Quota is 0, cooldown until reset time
				cooldown.Default().SetCooldown(provider.ID, earliestReset)
			} else {
				// Quota is not 0, apply short cooldown (1 minute)
				cooldown.Default().SetCooldownDuration(provider.ID, time.Minute)
			}
		}()
		return
	}

	// Found quota reset timestamp, set cooldown until that time
	cooldown.Default().SetCooldown(provider.ID, resetTime)
}
