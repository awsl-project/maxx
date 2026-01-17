package antigravity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider"
	ctxutil "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/usage"
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
	httpClient *http.Client
}

func NewAdapter(p *domain.Provider) (provider.ProviderAdapter, error) {
	if p.Config == nil || p.Config.Antigravity == nil {
		return nil, fmt.Errorf("provider %s missing antigravity config", p.Name)
	}
	return &AntigravityAdapter{
		provider:   p,
		tokenCache: &TokenCache{},
		httpClient: newUpstreamHTTPClient(),
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
	mappedModel := ctxutil.GetMappedModel(ctx)   // Mapped model after executor's unified mapping
	requestBody := ctxutil.GetRequestBody(ctx)
	backgroundDowngrade := false
	backgroundModel := ""

	// Background task downgrade (like Manager) - only for Claude clients
	if clientType == domain.ClientTypeClaude {
		if isBg, forcedModel, newBody := detectBackgroundTask(requestBody); isBg {
			requestBody = newBody
			mappedModel = forcedModel
			backgroundModel = forcedModel
			backgroundDowngrade = true
		}
	}

	// We'll attempt at most twice: original + retry without thinking on signature errors
	retriedWithoutThinking := false

	for attemptIdx := 0; attemptIdx < 2; attemptIdx++ {
		ctx = ctxutil.WithRequestModel(baseCtx, requestModel)
		ctx = ctxutil.WithRequestBody(ctx, requestBody)

		// Apply background downgrade override if needed
		config := provider.Config.Antigravity
		if backgroundDowngrade && backgroundModel != "" {
			mappedModel = backgroundModel
		}

		// Update attempt record with the final mapped model (in case of background downgrade)
		if attempt := ctxutil.GetUpstreamAttempt(ctx); attempt != nil {
			attempt.MappedModel = mappedModel
		}

		// Get streaming flag from context (already detected correctly for Gemini URL path)
		stream := ctxutil.GetIsStream(ctx)
		clientWantsStream := stream
		actualStream := stream
		if clientType == domain.ClientTypeClaude && !clientWantsStream {
			// Auto-convert Claude non-stream to stream internally for better quota (like Manager)
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
			var (
				effectiveMappedModel string
				hasThinking          bool
			)
			geminiBody, effectiveMappedModel, hasThinking, err = TransformClaudeToGemini(requestBody, mappedModel, actualStream, sessionID, GlobalSignatureCache())
			if err != nil {
				return domain.NewProxyErrorWithMessage(err, true, fmt.Sprintf("failed to transform Claude request: %v", err))
			}
			mappedModel = effectiveMappedModel

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
		var toolsForConfig []interface{}
		if clientType == domain.ClientTypeClaude {
			var raw map[string]interface{}
			if err := json.Unmarshal(requestBody, &raw); err == nil {
				if tools, ok := raw["tools"].([]interface{}); ok {
					toolsForConfig = tools
				}
			}
		}
		upstreamBody, err := wrapV1InternalRequest(geminiBody, config.ProjectID, requestModel, mappedModel, sessionID, toolsForConfig)
		if err != nil {
			return domain.NewProxyErrorWithMessage(domain.ErrFormatConversion, true, "failed to wrap request for v1internal")
		}

		// Build upstream URLs (prod first, daily fallback)
		baseURLs := []string{V1InternalBaseURLProd, V1InternalBaseURLDaily}
		client := a.httpClient
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

			// Send request info via EventChannel (only once per attempt)
			if eventChan := ctxutil.GetEventChan(ctx); eventChan != nil {
				eventChan.SendRequestInfo(&domain.RequestInfo{
					Method:  upstreamReq.Method,
					URL:     upstreamURL,
					Headers: flattenHeaders(upstreamReq.Header),
					Body:    string(upstreamBody),
				})
			}

			resp, err := client.Do(upstreamReq)
			if err != nil {
				lastErr = err
				if hasNextEndpoint(idx, len(baseURLs)) {
					continue
				}
				proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to connect to upstream")
				proxyErr.IsNetworkError = true // Mark as network error (connection timeout, DNS failure, etc.)
				return proxyErr
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
					proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to connect to upstream after token refresh")
					proxyErr.IsNetworkError = true // Mark as network error
					return proxyErr
				}
				defer resp.Body.Close()
			}

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

				// Check for RESOURCE_EXHAUSTED (429) and extract cooldown info
				var rateLimitInfo *domain.RateLimitInfo
				var cooldownUpdateChan chan time.Time
				if resp.StatusCode == http.StatusTooManyRequests {
					rateLimitInfo, cooldownUpdateChan = a.parseRateLimitInfo(ctx, body, provider)
				}

				// Parse retry info for 429/5xx responses (like Antigravity-Manager)
				var retryAfter time.Duration

				// 1) Prefer Retry-After header (seconds)
				if ra := strings.TrimSpace(resp.Header.Get("Retry-After")); ra != "" {
					if secs, err := strconv.Atoi(ra); err == nil && secs > 0 {
						retryAfter = time.Duration(secs) * time.Second
					}
				}

				// 2) Fallback to body parsing (google.rpc.RetryInfo / quotaResetDelay)
				if retryAfter == 0 {
					if retryInfo := ParseRetryInfo(resp.StatusCode, body); retryInfo != nil {
						retryAfter = retryInfo.Delay

						// Manager: add a small buffer and cap for 429 retries
						if resp.StatusCode == http.StatusTooManyRequests {
							retryAfter += 200 * time.Millisecond
							if retryAfter > 10*time.Second {
								retryAfter = 10 * time.Second
							}
						}

						retryAfter = ApplyJitter(retryAfter)
					}
				}

				proxyErr := domain.NewProxyErrorWithMessage(
					fmt.Errorf("upstream error: %s", string(body)),
					isRetryableStatusCode(resp.StatusCode),
					fmt.Sprintf("upstream returned status %d", resp.StatusCode),
				)

				// Set status code and check if it's a server error (5xx)
				proxyErr.HTTPStatusCode = resp.StatusCode
				proxyErr.IsServerError = resp.StatusCode >= 500 && resp.StatusCode < 600

				// Set retry info on error for upstream handling
				if retryAfter > 0 {
					proxyErr.RetryAfter = retryAfter
				}

				// Set rate limit info for cooldown handling
				if rateLimitInfo != nil {
					proxyErr.RateLimitInfo = rateLimitInfo
					proxyErr.CooldownUpdateChan = cooldownUpdateChan
				}

				lastErr = proxyErr

				// Signature failure recovery: retry once without thinking (like Manager)
				if resp.StatusCode == http.StatusBadRequest && !retriedWithoutThinking && isThinkingSignatureError(body) {
					retriedWithoutThinking = true

					// Manager uses a small fixed delay before retrying.
					select {
					case <-ctx.Done():
						return domain.NewProxyErrorWithMessage(ctx.Err(), false, "client disconnected")
					case <-time.After(200 * time.Millisecond):
					}

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

	client := &http.Client{Timeout: 15 * time.Second}
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

func newUpstreamHTTPClient() *http.Client {
	// Mirrors Antigravity-Manager's reqwest client settings:
	// connect_timeout=20s, pool_max_idle_per_host=16, pool_idle_timeout=90s, tcp_keepalive=60s, timeout=600s.
	dialer := &net.Dialer{
		Timeout:   20 * time.Second,
		KeepAlive: 60 * time.Second,
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   20 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   600 * time.Second,
	}
}

// applyClaudePostProcess applies minimal post-processing for advanced features
// not yet fully integrated into the transform functions
func applyClaudePostProcess(geminiBody []byte, sessionID string, hasThinking bool, _ []byte, mappedModel string) []byte {
	var request map[string]interface{}
	if err := json.Unmarshal(geminiBody, &request); err != nil {
		return geminiBody
	}

	modified := false

	// 1. Inject toolConfig with VALIDATED mode when tools exist
	if InjectToolConfig(request) {
		modified = true
	}

	// 2. Process contents for additional signature validation
	if contents, ok := request["contents"].([]interface{}); ok {
		if processContentsForSignatures(contents, sessionID, mappedModel) {
			modified = true
		}
	}

	// 3. Clean thinking fields if disabled
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
	V1InternalBaseURLDaily = "https://daily-cloudcode-pa.sandbox.googleapis.com/v1internal"
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

	// Send events via EventChannel (executor will process them)
	eventChan := ctxutil.GetEventChan(ctx)

	// Send response info event
	eventChan.SendResponseInfo(&domain.ResponseInfo{
		Status:  resp.StatusCode,
		Headers: flattenHeaders(resp.Header),
		Body:    string(body), // Keep original for debugging
	})

	// Extract and send token usage metrics
	if metrics := usage.ExtractFromResponse(string(unwrappedBody)); metrics != nil {
		eventChan.SendMetrics(&domain.AdapterMetrics{
			InputTokens:          metrics.InputTokens,
			OutputTokens:         metrics.OutputTokens,
			CacheReadCount:       metrics.CacheReadCount,
			CacheCreationCount:   metrics.CacheCreationCount,
			Cache5mCreationCount: metrics.Cache5mCreationCount,
			Cache1hCreationCount: metrics.Cache1hCreationCount,
		})
	}

	// Extract and send response model
	if modelVersion := extractModelVersion(unwrappedBody); modelVersion != "" {
		eventChan.SendResponseModel(modelVersion)
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
	eventChan := ctxutil.GetEventChan(ctx)

	// Send initial response info (for streaming, we only capture status and headers)
	eventChan.SendResponseInfo(&domain.ResponseInfo{
		Status:  resp.StatusCode,
		Headers: flattenHeaders(resp.Header),
		Body:    "[streaming]",
	})

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

	// Helper to extract tokens and send events
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
				eventChan.SendMetrics(&domain.AdapterMetrics{
					InputTokens:          metrics.InputTokens,
					OutputTokens:         metrics.OutputTokens,
					CacheReadCount:       metrics.CacheReadCount,
					CacheCreationCount:   metrics.CacheCreationCount,
					Cache5mCreationCount: metrics.Cache5mCreationCount,
					Cache1hCreationCount: metrics.Cache1hCreationCount,
				})
			}

			// Extract and send response model
			var modelVersion string
			if claudeState != nil {
				modelVersion = claudeState.GetModelVersion()
			} else {
				modelVersion = extractModelVersionFromSSE(sseBuffer.String())
			}
			if modelVersion != "" {
				eventChan.SendResponseModel(modelVersion)
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
			sendFinalEvents()
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

				// Process the complete line
				lineBytes := []byte(line)

				// Unwrap v1internal SSE chunk before processing
				unwrappedLine := unwrapV1InternalSSEChunk(lineBytes)

				// Collect original SSE for token extraction (extractor handles v1internal wrapper)
				sseBuffer.WriteString(line)

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
						sendFinalEvents()
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
				sendFinalEvents()
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
				sendFinalEvents()
				return domain.NewProxyErrorWithMessage(ctx.Err(), false, "client disconnected")
			}
			// Ensure Claude clients get termination events
			if isClaudeClient && claudeState != nil {
				if forceStop := claudeState.EmitForceStop(); len(forceStop) > 0 {
					_, _ = w.Write(forceStop)
					flusher.Flush()
				}
			}
			sendFinalEvents()
			return nil
		}
	}
}

// handleCollectedStreamResponse forwards upstream SSE but collects into a single response body (like Manager non-stream auto-convert)
func (a *AntigravityAdapter) handleCollectedStreamResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response, clientType domain.ClientType, requestModel string) error {
	eventChan := ctxutil.GetEventChan(ctx)

	// Send initial response info
	eventChan.SendResponseInfo(&domain.ResponseInfo{
		Status:  resp.StatusCode,
		Headers: flattenHeaders(resp.Header),
		Body:    "[stream-collected]",
	})

	// Copy upstream headers (except those we override)
	copyResponseHeaders(w.Header(), resp.Header)

	isClaudeClient := clientType == domain.ClientTypeClaude
	var claudeState *ClaudeStreamingState
	var claudeSSE strings.Builder
	if isClaudeClient {
		// Extract sessionID for signature caching (like CLIProxyAPI)
		requestBody := ctxutil.GetRequestBody(ctx)
		sessionID := extractSessionID(requestBody)
		claudeState = NewClaudeStreamingStateWithSession(sessionID, requestModel)
	}

	// Collect upstream SSE for attempt/debug and token extraction.
	var upstreamSSE strings.Builder
	var lastPayload []byte
	var responseBody []byte

	var lineBuffer bytes.Buffer
	buf := make([]byte, 4096)

	for {
		// Check context before reading
		select {
		case <-ctx.Done():
			return domain.NewProxyErrorWithMessage(ctx.Err(), false, "client disconnected")
		default:
		}

		n, err := resp.Body.Read(buf)
		if n > 0 {
			lineBuffer.Write(buf[:n])

			for {
				line, readErr := lineBuffer.ReadString('\n')
				if readErr != nil {
					lineBuffer.WriteString(line)
					break
				}

				upstreamSSE.WriteString(line)

				unwrappedLine := unwrapV1InternalSSEChunk([]byte(line))
				if len(unwrappedLine) == 0 {
					continue
				}

				// Track last Gemini payload for non-Claude responses (best-effort)
				lineStr := strings.TrimSpace(string(unwrappedLine))
				if strings.HasPrefix(lineStr, "data: ") {
					dataStr := strings.TrimSpace(strings.TrimPrefix(lineStr, "data: "))
					if dataStr != "" && dataStr != "[DONE]" {
						lastPayload = []byte(dataStr)
					}
				}

				if isClaudeClient && claudeState != nil {
					out := claudeState.ProcessGeminiSSELine(string(unwrappedLine))
					if len(out) > 0 {
						claudeSSE.Write(out)
					}
				}
			}
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to read upstream stream")
		}
	}

	// Ensure Claude clients get termination events
	if isClaudeClient && claudeState != nil {
		if forceStop := claudeState.EmitForceStop(); len(forceStop) > 0 {
			claudeSSE.Write(forceStop)
		}
	}

	// Send events via EventChannel
	// Send response info with collected body
	eventChan.SendResponseInfo(&domain.ResponseInfo{
		Status:  resp.StatusCode,
		Headers: flattenHeaders(resp.Header),
		Body:    upstreamSSE.String(),
	})

	// Extract and send token usage
	if metrics := usage.ExtractFromStreamContent(upstreamSSE.String()); metrics != nil {
		eventChan.SendMetrics(&domain.AdapterMetrics{
			InputTokens:          metrics.InputTokens,
			OutputTokens:         metrics.OutputTokens,
			CacheReadCount:       metrics.CacheReadCount,
			CacheCreationCount:   metrics.CacheCreationCount,
			Cache5mCreationCount: metrics.Cache5mCreationCount,
			Cache1hCreationCount: metrics.Cache1hCreationCount,
		})
	}

	// Extract and send response model
	var modelVersion string
	if claudeState != nil {
		modelVersion = claudeState.GetModelVersion()
	} else {
		modelVersion = extractModelVersionFromSSE(upstreamSSE.String())
	}
	if modelVersion != "" {
		eventChan.SendResponseModel(modelVersion)
	}

	if isClaudeClient {
		if claudeSSE.Len() == 0 {
			return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "empty upstream stream response")
		}
		collected, collectErr := collectClaudeSSEToJSON(claudeSSE.String())
		if collectErr != nil {
			return domain.NewProxyErrorWithMessage(domain.ErrFormatConversion, false, "failed to collect streamed response")
		}
		responseBody = collected
	} else {
		if len(lastPayload) == 0 {
			return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "empty upstream stream response")
		}
		switch clientType {
		case domain.ClientTypeGemini:
			responseBody = lastPayload
		case domain.ClientTypeOpenAI:
			return domain.NewProxyErrorWithMessage(domain.ErrFormatConversion, false, "OpenAI response transformation not yet implemented")
		default:
			responseBody = lastPayload
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(responseBody)
	return nil
}

// parseRateLimitInfo parses 429 RESOURCE_EXHAUSTED errors and extracts cooldown information
// Returns RateLimitInfo and optional channel for async cooldown updates
func (a *AntigravityAdapter) parseRateLimitInfo(ctx context.Context, body []byte, provider *domain.Provider) (*domain.RateLimitInfo, chan time.Time) {
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
		// Can't parse error, return nil
		return nil, nil
	}

	// Check if it's RESOURCE_EXHAUSTED
	if errResp.Error.Status != "RESOURCE_EXHAUSTED" {
		return nil, nil
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

	if !resetTime.IsZero() {
		// Found quota reset timestamp, return immediately
		return &domain.RateLimitInfo{
			Type:             "quota_exhausted",
			QuotaResetTime:   resetTime,
			RetryHintMessage: errResp.Error.Message,
			ClientType:       "", // Antigravity quota is global, affects all client types
		}, nil
	}

	// No quota reset timestamp found, query quota API asynchronously
	config := provider.Config.Antigravity
	if config == nil {
		// No config, return default 1-minute cooldown
		oneMinuteFromNow := time.Now().Add(time.Minute)
		return &domain.RateLimitInfo{
			Type:             "quota_exhausted",
			QuotaResetTime:   oneMinuteFromNow,
			RetryHintMessage: errResp.Error.Message,
			ClientType:       "",
		}, nil
	}

	// Create channel for async update
	updateChan := make(chan time.Time, 1)

	// Fetch quota in background
	go func() {
		defer close(updateChan)

		quotaCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		quota, err := FetchQuotaForProvider(quotaCtx, config.RefreshToken, config.ProjectID)
		if err != nil {
			// Failed to fetch quota, send 1-minute cooldown
			updateChan <- time.Now().Add(time.Minute)
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
			// Quota is 0, send cooldown until reset time
			updateChan <- earliestReset
		} else {
			// Quota is not 0, send 1-minute cooldown
			updateChan <- time.Now().Add(time.Minute)
		}
	}()

	// Return initial 1-minute cooldown with async update channel
	oneMinuteFromNow := time.Now().Add(time.Minute)
	return &domain.RateLimitInfo{
		Type:             "quota_exhausted",
		QuotaResetTime:   oneMinuteFromNow,
		RetryHintMessage: errResp.Error.Message,
		ClientType:       "", // Global cooldown
	}, updateChan
}

// extractModelVersion extracts modelVersion from Gemini response JSON
func extractModelVersion(body []byte) string {
	// Try direct format first: {"modelVersion": "..."}
	var resp struct {
		ModelVersion string `json:"modelVersion"`
	}
	if err := json.Unmarshal(body, &resp); err == nil && resp.ModelVersion != "" {
		return resp.ModelVersion
	}

	// Try v1internal wrapper format: {"response": {"modelVersion": "..."}}
	var wrapper struct {
		Response struct {
			ModelVersion string `json:"modelVersion"`
		} `json:"response"`
	}
	if err := json.Unmarshal(body, &wrapper); err == nil && wrapper.Response.ModelVersion != "" {
		return wrapper.Response.ModelVersion
	}

	return ""
}

// extractModelVersionFromSSE extracts modelVersion from SSE content
// Looks for the last "modelVersion" field in the SSE data
func extractModelVersionFromSSE(sseContent string) string {
	var lastModelVersion string
	for _, line := range strings.Split(sseContent, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		// Try direct format first: {"modelVersion": "..."}
		var chunk struct {
			ModelVersion string `json:"modelVersion"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err == nil && chunk.ModelVersion != "" {
			lastModelVersion = chunk.ModelVersion
			continue
		}

		// Try v1internal wrapper format: {"response": {"modelVersion": "..."}}
		var wrapper struct {
			Response struct {
				ModelVersion string `json:"modelVersion"`
			} `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &wrapper); err == nil && wrapper.Response.ModelVersion != "" {
			lastModelVersion = wrapper.Response.ModelVersion
		}
	}
	return lastModelVersion
}
