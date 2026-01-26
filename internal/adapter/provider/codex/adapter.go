package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider"
	ctxutil "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/usage"
)

func init() {
	provider.RegisterAdapterFactory("codex", NewAdapter)
}

// TokenCache caches access tokens
type TokenCache struct {
	AccessToken string
	ExpiresAt   time.Time
}

// ProviderUpdateFunc is a callback to persist token updates to the provider config
type ProviderUpdateFunc func(provider *domain.Provider) error

// CodexAdapter handles communication with OpenAI Codex API
type CodexAdapter struct {
	provider       *domain.Provider
	tokenCache     *TokenCache
	tokenMu        sync.RWMutex
	httpClient     *http.Client
	providerUpdate ProviderUpdateFunc
}

// SetProviderUpdateFunc sets the callback for persisting provider updates
func (a *CodexAdapter) SetProviderUpdateFunc(fn ProviderUpdateFunc) {
	a.providerUpdate = fn
}

func NewAdapter(p *domain.Provider) (provider.ProviderAdapter, error) {
	if p.Config == nil || p.Config.Codex == nil {
		return nil, fmt.Errorf("provider %s missing codex config", p.Name)
	}

	adapter := &CodexAdapter{
		provider:   p,
		tokenCache: &TokenCache{},
		httpClient: newUpstreamHTTPClient(),
	}

	// Initialize token cache from persisted config if available
	config := p.Config.Codex
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

func (a *CodexAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeCodex}
}

func (a *CodexAdapter) Execute(ctx context.Context, w http.ResponseWriter, req *http.Request, provider *domain.Provider) error {
	requestBody := ctxutil.GetRequestBody(ctx)
	stream := ctxutil.GetIsStream(ctx)

	// Get access token
	accessToken, err := a.getAccessToken(ctx)
	if err != nil {
		return domain.NewProxyErrorWithMessage(err, true, "failed to get access token")
	}

	// Build upstream URL
	upstreamURL := CodexBaseURL + "/responses"

	// Create upstream request
	upstreamReq, err := http.NewRequestWithContext(ctx, "POST", upstreamURL, bytes.NewReader(requestBody))
	if err != nil {
		return domain.NewProxyErrorWithMessage(err, true, "failed to create upstream request")
	}

	// Apply headers with passthrough support (client headers take priority)
	config := provider.Config.Codex
	a.applyCodexHeaders(upstreamReq, req, accessToken, config.AccountID)

	// Send request info via EventChannel
	if eventChan := ctxutil.GetEventChan(ctx); eventChan != nil {
		eventChan.SendRequestInfo(&domain.RequestInfo{
			Method:  upstreamReq.Method,
			URL:     upstreamURL,
			Headers: flattenHeaders(upstreamReq.Header),
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
	defer resp.Body.Close()

	// Handle 401 (token expired) - refresh and retry once
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

		// Retry request
		upstreamReq, _ = http.NewRequestWithContext(ctx, "POST", upstreamURL, bytes.NewReader(requestBody))
		a.applyCodexHeaders(upstreamReq, req, accessToken, config.AccountID)

		resp, err = a.httpClient.Do(upstreamReq)
		if err != nil {
			proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to connect to upstream after token refresh")
			proxyErr.IsNetworkError = true
			return proxyErr
		}
		defer resp.Body.Close()
	}

	// Handle error responses
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
		proxyErr.HTTPStatusCode = resp.StatusCode
		proxyErr.IsServerError = resp.StatusCode >= 500 && resp.StatusCode < 600

		// Handle rate limiting
		if resp.StatusCode == http.StatusTooManyRequests {
			proxyErr.RateLimitInfo = &domain.RateLimitInfo{
				Type:             "rate_limit",
				QuotaResetTime:   time.Now().Add(time.Minute),
				RetryHintMessage: "Rate limited by Codex API",
				ClientType:       string(domain.ClientTypeCodex),
			}
		}

		return proxyErr
	}

	// Handle response
	if stream {
		return a.handleStreamResponse(ctx, w, resp)
	}
	return a.handleNonStreamResponse(ctx, w, resp)
}

func (a *CodexAdapter) getAccessToken(ctx context.Context) (string, error) {
	// Check cache
	a.tokenMu.RLock()
	if a.tokenCache.AccessToken != "" && time.Now().Add(60*time.Second).Before(a.tokenCache.ExpiresAt) {
		token := a.tokenCache.AccessToken
		a.tokenMu.RUnlock()
		return token, nil
	}
	a.tokenMu.RUnlock()

	// Refresh token
	config := a.provider.Config.Codex
	tokenResp, err := RefreshAccessToken(ctx, config.RefreshToken)
	if err != nil {
		return "", err
	}

	// Calculate expiration time (with 60s buffer)
	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn-60) * time.Second)

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
		// Note: We intentionally ignore errors here as token persistence is best-effort
		// The token will still work in memory even if DB update fails
		_ = a.providerUpdate(a.provider)
	}

	return tokenResp.AccessToken, nil
}

func (a *CodexAdapter) handleNonStreamResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to read upstream response")
	}

	// Send events via EventChannel
	eventChan := ctxutil.GetEventChan(ctx)
	eventChan.SendResponseInfo(&domain.ResponseInfo{
		Status:  resp.StatusCode,
		Headers: flattenHeaders(resp.Header),
		Body:    string(body),
	})

	// Extract token usage from response
	if metrics := usage.ExtractFromResponse(string(body)); metrics != nil {
		eventChan.SendMetrics(&domain.AdapterMetrics{
			InputTokens:  metrics.InputTokens,
			OutputTokens: metrics.OutputTokens,
		})
	}

	// Extract model from response
	if model := extractModelFromResponse(body); model != "" {
		eventChan.SendResponseModel(model)
	}

	// Copy response headers
	copyResponseHeaders(w.Header(), resp.Header)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
	return nil
}

func (a *CodexAdapter) handleStreamResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response) error {
	eventChan := ctxutil.GetEventChan(ctx)

	// Send initial response info
	eventChan.SendResponseInfo(&domain.ResponseInfo{
		Status:  resp.StatusCode,
		Headers: flattenHeaders(resp.Header),
		Body:    "[streaming]",
	})

	// Set streaming headers
	copyResponseHeaders(w.Header(), resp.Header)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, false, "streaming not supported")
	}

	// Collect SSE for token extraction
	var sseBuffer strings.Builder
	var lineBuffer bytes.Buffer
	buf := make([]byte, 4096)
	firstChunkSent := false
	responseCompleted := false

	for {
		// Check context
		select {
		case <-ctx.Done():
			a.sendFinalStreamEvents(eventChan, &sseBuffer, resp)
			if responseCompleted {
				return nil
			}
			return domain.NewProxyErrorWithMessage(ctx.Err(), false, "client disconnected")
		default:
		}

		n, err := resp.Body.Read(buf)
		if n > 0 {
			lineBuffer.Write(buf[:n])

			// Process complete lines
			for {
				line, readErr := lineBuffer.ReadString('\n')
				if readErr != nil {
					lineBuffer.WriteString(line)
					break
				}

				sseBuffer.WriteString(line)

				// Check for response.completed in data line
				if strings.HasPrefix(line, "data:") && strings.Contains(line, "response.completed") {
					responseCompleted = true
				}

				// Write to client
				_, writeErr := w.Write([]byte(line))
				if writeErr != nil {
					a.sendFinalStreamEvents(eventChan, &sseBuffer, resp)
					if responseCompleted {
						return nil
					}
					return domain.NewProxyErrorWithMessage(writeErr, false, "client disconnected")
				}
				flusher.Flush()

				// Track TTFT
				if !firstChunkSent {
					firstChunkSent = true
					eventChan.SendFirstToken(time.Now().UnixMilli())
				}
			}
		}

		if err != nil {
			a.sendFinalStreamEvents(eventChan, &sseBuffer, resp)
			if err == io.EOF || responseCompleted {
				return nil
			}
			if ctx.Err() != nil {
				return domain.NewProxyErrorWithMessage(ctx.Err(), false, "client disconnected")
			}
			return nil
		}
	}
}

func (a *CodexAdapter) sendFinalStreamEvents(eventChan domain.AdapterEventChan, sseBuffer *strings.Builder, resp *http.Response) {
	if sseBuffer.Len() > 0 {
		// Update response body with collected SSE
		eventChan.SendResponseInfo(&domain.ResponseInfo{
			Status:  resp.StatusCode,
			Headers: flattenHeaders(resp.Header),
			Body:    sseBuffer.String(),
		})

		// Extract token usage from stream
		if metrics := usage.ExtractFromStreamContent(sseBuffer.String()); metrics != nil {
			eventChan.SendMetrics(&domain.AdapterMetrics{
				InputTokens:  metrics.InputTokens,
				OutputTokens: metrics.OutputTokens,
			})
		}

		// Extract model from stream
		if model := extractModelFromSSE(sseBuffer.String()); model != "" {
			eventChan.SendResponseModel(model)
		}
	}
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

func copyResponseHeaders(dst, src http.Header) {
	for k, vv := range src {
		// Skip hop-by-hop headers
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

func extractModelFromSSE(sseContent string) string {
	var lastModel string
	for _, line := range strings.Split(sseContent, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			continue
		}

		var chunk struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err == nil && chunk.Model != "" {
			lastModel = chunk.Model
		}
	}
	return lastModel
}

// applyCodexHeaders applies headers for Codex API requests
// It follows the CLIProxyAPI pattern: passthrough client headers, use defaults only when missing
func (a *CodexAdapter) applyCodexHeaders(upstreamReq, clientReq *http.Request, accessToken, accountID string) {
	// First, copy passthrough headers from client request (excluding hop-by-hop and auth)
	if clientReq != nil {
		for k, vv := range clientReq.Header {
			lk := strings.ToLower(k)
			// Skip hop-by-hop headers and authorization (we'll set our own)
			switch lk {
			case "connection", "keep-alive", "transfer-encoding", "upgrade",
				"authorization", "host", "content-length":
				continue
			}
			for _, v := range vv {
				upstreamReq.Header.Add(k, v)
			}
		}
	}

	// Set required headers (these always override)
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)
	upstreamReq.Header.Set("Accept", "text/event-stream")
	upstreamReq.Header.Set("Connection", "Keep-Alive")

	// Set Codex-specific headers only if client didn't provide them
	ensureHeader(upstreamReq.Header, clientReq, "Version", CodexVersion)
	ensureHeader(upstreamReq.Header, clientReq, "Openai-Beta", OpenAIBetaHeader)
	ensureHeader(upstreamReq.Header, clientReq, "User-Agent", CodexUserAgent)
	ensureHeader(upstreamReq.Header, clientReq, "Originator", CodexOriginator)

	// Set account ID if available (required for OAuth auth, not for API key)
	if accountID != "" {
		upstreamReq.Header.Set("Chatgpt-Account-Id", accountID)
	}
}

// ensureHeader sets a header only if the client request doesn't already have it
func ensureHeader(dst http.Header, clientReq *http.Request, key, defaultValue string) {
	if clientReq != nil && clientReq.Header.Get(key) != "" {
		// Client provided this header, it's already copied, don't override
		return
	}
	dst.Set(key, defaultValue)
}
