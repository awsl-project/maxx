package claude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/usage"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func init() {
	provider.RegisterAdapterFactory("claude", NewAdapter)
}

// TokenCache caches access tokens
type TokenCache struct {
	AccessToken string
	ExpiresAt   time.Time
}

// ProviderUpdateFunc is a callback to persist token updates to the provider config
type ProviderUpdateFunc func(provider *domain.Provider) error

// ClaudeAdapter handles communication with Anthropic Claude API
type ClaudeAdapter struct {
	provider       *domain.Provider
	tokenCache     *TokenCache
	tokenMu        sync.RWMutex
	httpClient     *http.Client
	providerUpdate ProviderUpdateFunc
}

// SetProviderUpdateFunc sets the callback for persisting provider updates
func (a *ClaudeAdapter) SetProviderUpdateFunc(fn ProviderUpdateFunc) {
	a.providerUpdate = fn
}

func NewAdapter(p *domain.Provider) (provider.ProviderAdapter, error) {
	if p.Config == nil || p.Config.Claude == nil {
		return nil, fmt.Errorf("provider %s missing claude config", p.Name)
	}

	config := p.Config.Claude

	adapter := &ClaudeAdapter{
		provider:   p,
		tokenCache: &TokenCache{},
		httpClient: newUpstreamHTTPClient(),
	}

	// Initialize token cache from persisted config if available
	if config.AccessToken != "" && config.ExpiresAt != "" {
		expiresAt, err := time.Parse(time.RFC3339, config.ExpiresAt)
		if err == nil && time.Now().Before(expiresAt) {
			adapter.tokenCache = &TokenCache{
				AccessToken: config.AccessToken,
				ExpiresAt:   expiresAt,
			}
		}
	}

	return adapter, nil
}

func (a *ClaudeAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeClaude}
}

func (a *ClaudeAdapter) Execute(c *flow.Ctx, provider *domain.Provider) error {
	requestBody := flow.GetRequestBody(c)
	clientWantsStream := flow.GetIsStream(c)
	request := c.Request
	ctx := context.Background()
	if request != nil {
		ctx = request.Context()
	}

	// Get access token
	accessToken, err := a.getAccessToken(ctx, false)
	if err != nil {
		return domain.NewProxyErrorWithMessage(err, true, "failed to get access token")
	}

	// Extract beta headers from request body before sending
	var extraBetas []string
	requestBody, extraBetas = extractAndRemoveBetas(requestBody)

	// Build upstream URL
	upstreamURL := ClaudeBaseURL + "/v1/messages?beta=true"

	// Ensure stream field matches client preference
	if len(requestBody) > 0 {
		if updated, err := sjson.SetBytes(requestBody, "stream", clientWantsStream); err == nil {
			requestBody = updated
		}
	}

	// Create upstream request
	upstreamReq, err := http.NewRequestWithContext(ctx, "POST", upstreamURL, bytes.NewReader(requestBody))
	if err != nil {
		return domain.NewProxyErrorWithMessage(err, true, "failed to create upstream request")
	}

	// Apply headers
	a.applyClaudeHeaders(upstreamReq, request, accessToken, clientWantsStream, extraBetas)

	// Send request info via EventChannel
	if eventChan := flow.GetEventChan(c); eventChan != nil {
		eventChan.SendRequestInfo(&domain.RequestInfo{
			Method:  upstreamReq.Method,
			URL:     upstreamURL,
			Headers: sanitizeHeadersForEvent(upstreamReq.Header),
			Body:    string(requestBody),
		})
	}

	// Execute request
	resp, err := a.httpClient.Do(upstreamReq)
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to connect to upstream")
		proxyErr.IsNetworkError = true
		return proxyErr
	}
	resp.Body = flow.WrapResponseBody(c, resp.Body)
	defer resp.Body.Close()

	// Handle 401 (token expired) - refresh and retry once
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()

		// Invalidate token cache
		a.tokenMu.Lock()
		a.tokenCache = &TokenCache{}
		a.tokenMu.Unlock()

		// Get new token (force refresh to skip persisted token)
		accessToken, err = a.getAccessToken(ctx, true)
		if err != nil {
			return domain.NewProxyErrorWithMessage(err, true, "failed to refresh access token")
		}

		// Retry request
		upstreamReq, reqErr := http.NewRequestWithContext(ctx, "POST", upstreamURL, bytes.NewReader(requestBody))
		if reqErr != nil {
			return domain.NewProxyErrorWithMessage(reqErr, false, fmt.Sprintf("failed to create retry request: %v", reqErr))
		}
		a.applyClaudeHeaders(upstreamReq, request, accessToken, clientWantsStream, extraBetas)

		resp, err = a.httpClient.Do(upstreamReq)
		if err != nil {
			proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to connect to upstream after token refresh")
			proxyErr.IsNetworkError = true
			return proxyErr
		}
		resp.Body = flow.WrapResponseBody(c, resp.Body)
		defer resp.Body.Close()
	}

	// Handle error responses
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)

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
		proxyErr.HTTPStatusCode = resp.StatusCode
		proxyErr.IsServerError = resp.StatusCode >= 500 && resp.StatusCode < 600

		// Handle rate limiting
		if resp.StatusCode == http.StatusTooManyRequests {
			proxyErr.RateLimitInfo = &domain.RateLimitInfo{
				Type:             "rate_limit",
				QuotaResetTime:   time.Now().Add(time.Minute),
				RetryHintMessage: "Rate limited by Claude API",
				ClientType:       string(domain.ClientTypeClaude),
			}
		}

		return proxyErr
	}

	// Handle response
	if clientWantsStream {
		return a.handleStreamResponse(c, resp)
	}
	return a.handleNonStreamResponse(c, resp)
}

// WarmToken pre-warms the access token cache to avoid blocking during Execute
func (a *ClaudeAdapter) WarmToken(ctx context.Context) error {
	_, err := a.getAccessToken(ctx, false)
	return err
}

func (a *ClaudeAdapter) getAccessToken(ctx context.Context, forceRefresh bool) (string, error) {
	config := a.provider.Config.Claude

	if !forceRefresh {
		// Check cache
		a.tokenMu.RLock()
		if a.tokenCache.AccessToken != "" {
			if a.tokenCache.ExpiresAt.IsZero() || time.Now().Add(60*time.Second).Before(a.tokenCache.ExpiresAt) {
				token := a.tokenCache.AccessToken
				a.tokenMu.RUnlock()
				return token, nil
			}
		}
		a.tokenMu.RUnlock()

		// Use persisted access token if present (even if expiry is unknown)
		if strings.TrimSpace(config.AccessToken) != "" {
			var expiresAt time.Time
			if strings.TrimSpace(config.ExpiresAt) != "" {
				if parsed, err := time.Parse(time.RFC3339, config.ExpiresAt); err == nil {
					expiresAt = parsed
				}
			}
			a.tokenMu.Lock()
			a.tokenCache = &TokenCache{
				AccessToken: config.AccessToken,
				ExpiresAt:   expiresAt,
			}
			a.tokenMu.Unlock()

			if expiresAt.IsZero() || time.Now().Add(60*time.Second).Before(expiresAt) {
				return config.AccessToken, nil
			}
		}
	}

	// Refresh token
	tokenResp, err := RefreshAccessToken(ctx, config.RefreshToken)
	if err != nil {
		// On force refresh (401 retry), don't fall back to the old token
		// since it was already rejected by the upstream
		if !forceRefresh && strings.TrimSpace(config.AccessToken) != "" {
			return config.AccessToken, nil
		}
		return "", err
	}

	// Calculate expiration time (with 60s buffer)
	ttl := tokenResp.ExpiresIn - 60
	if ttl < 1 {
		ttl = 1
	}
	expiresAt := time.Now().Add(time.Duration(ttl) * time.Second)

	// Update cache
	a.tokenMu.Lock()
	a.tokenCache = &TokenCache{
		AccessToken: tokenResp.AccessToken,
		ExpiresAt:   expiresAt,
	}
	a.tokenMu.Unlock()

	// Persist token to database if update function is set
	if a.providerUpdate != nil {
		config.AccessToken = tokenResp.AccessToken
		config.ExpiresAt = expiresAt.Format(time.RFC3339)
		if tokenResp.RefreshToken != "" {
			config.RefreshToken = tokenResp.RefreshToken
		}
		if tokenResp.Account != nil {
			if v := strings.TrimSpace(tokenResp.Account.EmailAddress); v != "" {
				config.Email = v
			}
		}
		if tokenResp.Organization != nil {
			if v := strings.TrimSpace(tokenResp.Organization.UUID); v != "" {
				config.OrganizationID = v
			}
		}
		if err := a.providerUpdate(a.provider); err != nil {
			log.Printf("[Claude] failed to persist refreshed token: %v", err)
		}
	}

	return tokenResp.AccessToken, nil
}

func (a *ClaudeAdapter) handleNonStreamResponse(c *flow.Ctx, resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to read upstream response")
	}

	// Send events via EventChannel
	if eventChan := flow.GetEventChan(c); eventChan != nil {
		eventChan.SendResponseInfo(&domain.ResponseInfo{
			Status:  resp.StatusCode,
			Headers: flattenHeaders(resp.Header),
			Body:    string(body),
		})
		// Extract token usage from response
		if metrics := usage.ExtractFromResponse(string(body)); metrics != nil {
			eventChan.SendMetrics(&domain.AdapterMetrics{
				InputTokens:          metrics.InputTokens,
				OutputTokens:         metrics.OutputTokens,
				CacheReadCount:       metrics.CacheReadCount,
				CacheCreationCount:   metrics.CacheCreationCount,
				Cache5mCreationCount: metrics.Cache5mCreationCount,
				Cache1hCreationCount: metrics.Cache1hCreationCount,
			})
		}
		// Extract model from response
		if model := extractModelFromResponse(body); model != "" {
			eventChan.SendResponseModel(model)
		}
	}

	// Copy response headers
	copyResponseHeaders(c.Writer.Header(), resp.Header)
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = c.Writer.Write(body)
	return nil
}

func (a *ClaudeAdapter) handleStreamResponse(c *flow.Ctx, resp *http.Response) error {
	eventChan := flow.GetEventChan(c)
	if eventChan != nil {
		eventChan.SendResponseInfo(&domain.ResponseInfo{
			Status:  resp.StatusCode,
			Headers: flattenHeaders(resp.Header),
			Body:    "[streaming]",
		})
	}

	copyResponseHeaders(c.Writer.Header(), resp.Header)
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, false, "streaming not supported")
	}

	// Incrementally extract metrics and model from SSE lines (no full-stream buffering)
	var collector usage.StreamCollector
	var model string
	reader := bufio.NewReader(resp.Body)
	firstChunkSent := false
	responseCompleted := false

	ctx := context.Background()
	if c.Request != nil {
		ctx = c.Request.Context()
	}
	for {
		select {
		case <-ctx.Done():
			a.sendFinalStreamEvents(eventChan, &collector, &model, resp)
			if responseCompleted {
				return nil
			}
			return domain.NewProxyErrorWithMessage(ctx.Err(), false, "client disconnected")
		default:
		}

		line, err := reader.ReadString('\n')
		if line != "" {
			// Extract metrics and model incrementally per line
			collector.ProcessSSELine(line)
			extractModelFromSSELine(line, &model)

			if isClaudeResponseCompletedLine(line) {
				responseCompleted = true
			}

			// Write to client
			_, writeErr := c.Writer.Write([]byte(line))
			if writeErr != nil {
				a.sendFinalStreamEvents(eventChan, &collector, &model, resp)
				if responseCompleted {
					return nil
				}
				return domain.NewProxyErrorWithMessage(writeErr, false, "client disconnected")
			}
			flusher.Flush()

			// Track TTFT
			if !firstChunkSent {
				firstChunkSent = true
				if eventChan != nil {
					eventChan.SendFirstToken(time.Now().UnixMilli())
				}
			}
		}

		if err != nil {
			a.sendFinalStreamEvents(eventChan, &collector, &model, resp)
			if err == io.EOF || responseCompleted {
				return nil
			}
			if ctx.Err() != nil {
				return domain.NewProxyErrorWithMessage(ctx.Err(), false, "client disconnected")
			}
			return domain.NewProxyErrorWithMessage(err, true, "stream read error")
		}
	}
}

// isClaudeResponseCompletedLine checks if a SSE line indicates stream completion.
// Claude uses "message_stop" event type to signal end of stream.
func isClaudeResponseCompletedLine(line string) bool {
	if !strings.HasPrefix(line, "data:") {
		return false
	}
	data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if data == "" || data == "[DONE]" {
		return false
	}
	if !gjson.Valid(data) {
		return false
	}
	return gjson.Get(data, "type").String() == "message_stop"
}

func (a *ClaudeAdapter) sendFinalStreamEvents(eventChan domain.AdapterEventChan, collector *usage.StreamCollector, model *string, resp *http.Response) {
	if eventChan == nil {
		return
	}

	// Send response info (body not accumulated to avoid unbounded memory growth)
	eventChan.SendResponseInfo(&domain.ResponseInfo{
		Status:  resp.StatusCode,
		Headers: flattenHeaders(resp.Header),
		Body:    "[streaming]",
	})

	// Send token usage collected incrementally
	if collector.Metrics != nil && !collector.Metrics.IsEmpty() {
		eventChan.SendMetrics(&domain.AdapterMetrics{
			InputTokens:          collector.Metrics.InputTokens,
			OutputTokens:         collector.Metrics.OutputTokens,
			CacheReadCount:       collector.Metrics.CacheReadCount,
			CacheCreationCount:   collector.Metrics.CacheCreationCount,
			Cache5mCreationCount: collector.Metrics.Cache5mCreationCount,
			Cache1hCreationCount: collector.Metrics.Cache1hCreationCount,
		})
	}

	// Send model collected incrementally
	if *model != "" {
		eventChan.SendResponseModel(*model)
	}
}

// extractModelFromSSELine extracts model from a single SSE line, updating the model pointer if found.
func extractModelFromSSELine(line string, model *string) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "data:") {
		return
	}
	data := strings.TrimPrefix(line, "data: ")
	data = strings.TrimPrefix(data, "data:")
	data = strings.TrimSpace(data)
	if data == "[DONE]" || data == "" {
		return
	}
	// Claude puts model in message_start event
	if gjson.Valid(data) {
		eventType := gjson.Get(data, "type").String()
		if eventType == "message_start" {
			if m := gjson.Get(data, "message.model").String(); m != "" {
				*model = m
			}
		}
	}
}

// extractAndRemoveBetas extracts beta values from request body and removes them.
// Claude API betas are specified in request body but should be sent as headers.
func extractAndRemoveBetas(body []byte) ([]byte, []string) {
	if len(body) == 0 {
		return body, nil
	}

	betasResult := gjson.GetBytes(body, "betas")
	if !betasResult.Exists() {
		return body, nil
	}

	var betas []string
	if betasResult.IsArray() {
		for _, b := range betasResult.Array() {
			if v := strings.TrimSpace(b.String()); v != "" {
				betas = append(betas, v)
			}
		}
	}

	// Remove betas from body
	body, _ = sjson.DeleteBytes(body, "betas")
	return body, betas
}

// applyClaudeHeaders applies headers for Claude API requests
func (a *ClaudeAdapter) applyClaudeHeaders(upstreamReq, clientReq *http.Request, accessToken string, stream bool, extraBetas []string) {
	// First, copy passthrough headers from client request (excluding filtered headers)
	if clientReq != nil {
		for k, vv := range clientReq.Header {
			lk := strings.ToLower(k)
			if claudeFilteredHeaders[lk] {
				continue
			}
			for _, v := range vv {
				upstreamReq.Header.Add(k, v)
			}
		}
	}

	// Set required headers (always override)
	upstreamReq.Header.Set("Content-Type", "application/json")

	// Auth: OAuth tokens use Bearer, API keys use x-api-key
	if isClaudeOAuthToken(accessToken) {
		upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)
	} else {
		upstreamReq.Header.Set("x-api-key", accessToken)
	}

	if stream {
		upstreamReq.Header.Set("Accept", "text/event-stream")
	} else {
		upstreamReq.Header.Set("Accept", "application/json")
	}
	upstreamReq.Header.Set("Connection", "keep-alive")

	// Anthropic-specific headers
	ensureHeader(upstreamReq.Header, clientReq, "Anthropic-Version", ClaudeAPIVersion)

	// Build beta header: base betas + extra betas from request body
	betaSet := make(map[string]struct{})
	var allBetas []string
	for _, b := range strings.Split(ClaudeBetaHeader, ",") {
		b = strings.TrimSpace(b)
		if b != "" {
			if _, exists := betaSet[b]; !exists {
				betaSet[b] = struct{}{}
				allBetas = append(allBetas, b)
			}
		}
	}
	for _, b := range extraBetas {
		b = strings.TrimSpace(b)
		if b != "" {
			if _, exists := betaSet[b]; !exists {
				betaSet[b] = struct{}{}
				allBetas = append(allBetas, b)
			}
		}
	}
	if len(allBetas) > 0 {
		upstreamReq.Header.Set("Anthropic-Beta", strings.Join(allBetas, ","))
	}

	// Set User-Agent
	upstreamReq.Header.Set("User-Agent", resolveClaudeUserAgent(clientReq))
}

// isClaudeOAuthToken checks if the token is an OAuth access token
func isClaudeOAuthToken(token string) bool {
	return strings.HasPrefix(token, "sk-ant-oat")
}

// ensureHeader sets a header only if the client request doesn't already have it
func ensureHeader(dst http.Header, clientReq *http.Request, key, defaultValue string) {
	if clientReq != nil && clientReq.Header.Get(key) != "" {
		return
	}
	dst.Set(key, defaultValue)
}

func resolveClaudeUserAgent(clientReq *http.Request) string {
	if clientReq != nil {
		if ua := strings.TrimSpace(clientReq.Header.Get("User-Agent")); isClaudeCLIUserAgent(ua) {
			return ua
		}
	}
	return ClaudeUserAgent
}

func isClaudeCLIUserAgent(userAgent string) bool {
	ua := strings.ToLower(strings.TrimSpace(userAgent))
	return strings.HasPrefix(ua, "claude-cli/") || strings.HasPrefix(ua, "claude-code/")
}

func newUpstreamHTTPClient() *http.Client {
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

func flattenHeaders(h http.Header) map[string]string {
	result := make(map[string]string)
	for k, v := range h {
		if len(v) > 0 {
			result[k] = v[0]
		}
	}
	return result
}

// sanitizeHeadersForEvent returns a header map safe for event channel logging,
// redacting sensitive auth headers to prevent credential leakage.
func sanitizeHeadersForEvent(h http.Header) map[string]string {
	result := flattenHeaders(h)
	for _, key := range []string{"Authorization", "X-Api-Key"} {
		if _, ok := result[key]; ok {
			result[key] = "[REDACTED]"
		}
	}
	return result
}

func copyResponseHeaders(dst, src http.Header) {
	for k, vv := range src {
		switch strings.ToLower(k) {
		case "connection", "keep-alive", "transfer-encoding", "upgrade":
			continue
		}
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func isRetryableStatusCode(status int) bool {
	switch status {
	case http.StatusTooManyRequests,
		http.StatusRequestTimeout,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return status >= 500
	}
}

func extractModelFromResponse(body []byte) string {
	var resp struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &resp); err == nil && resp.Model != "" {
		return resp.Model
	}
	return ""
}

var claudeFilteredHeaders = map[string]bool{
	// Hop-by-hop headers
	"connection":        true,
	"keep-alive":        true,
	"transfer-encoding": true,
	"upgrade":           true,

	// Headers set by HTTP client
	"host":           true,
	"content-length": true,

	// Explicitly controlled headers
	"user-agent":    true,
	"authorization": true,
	"x-api-key":     true,

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

	// Credential/session headers
	"cookie":     true,
	"set-cookie": true,
	"origin":     true,
	"referer":    true,

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
