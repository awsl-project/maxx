package antigravity

import (
	"bufio"
	"bytes"
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
	"github.com/Bowl42/maxx-next/internal/converter"
	ctxutil "github.com/Bowl42/maxx-next/internal/context"
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
	converter  *converter.Registry
	tokenCache *TokenCache
	tokenMu    sync.RWMutex
}

func NewAdapter(p *domain.Provider) (provider.ProviderAdapter, error) {
	if p.Config == nil || p.Config.Antigravity == nil {
		return nil, fmt.Errorf("provider %s missing antigravity config", p.Name)
	}
	return &AntigravityAdapter{
		provider:   p,
		converter:  converter.NewRegistry(),
		tokenCache: &TokenCache{},
	}, nil
}

func (a *AntigravityAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeGemini}
}

func (a *AntigravityAdapter) Execute(ctx context.Context, w http.ResponseWriter, req *http.Request, provider *domain.Provider) error {
	clientType := ctxutil.GetClientType(ctx)
	mappedModel := ctxutil.GetMappedModel(ctx)
	requestBody := ctxutil.GetRequestBody(ctx)

	// Determine if streaming
	stream := isStreamRequest(requestBody)

	// Get access token
	accessToken, err := a.getAccessToken(ctx)
	if err != nil {
		return domain.NewProxyErrorWithMessage(err, true, "failed to get access token")
	}

	// Antigravity uses Gemini format
	targetType := domain.ClientTypeGemini
	needsConversion := clientType != targetType

	// Transform request if needed
	var upstreamBody []byte
	if needsConversion {
		upstreamBody, err = a.converter.TransformRequest(clientType, targetType, requestBody, mappedModel, stream)
		if err != nil {
			return domain.NewProxyErrorWithMessage(domain.ErrFormatConversion, true, "failed to transform request")
		}
	} else {
		upstreamBody = requestBody
	}

	// Build upstream URL
	config := provider.Config.Antigravity
	upstreamURL := a.buildUpstreamURL(config, mappedModel, stream)

	// Create upstream request
	upstreamReq, err := http.NewRequestWithContext(ctx, "POST", upstreamURL, bytes.NewReader(upstreamBody))
	if err != nil {
		return domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to create upstream request")
	}

	// Forward original headers (filtered) - preserves application headers
	originalHeaders := ctxutil.GetRequestHeaders(ctx)
	copyHeadersFiltered(upstreamReq.Header, originalHeaders)

	// Set content-type if not already set
	if upstreamReq.Header.Get("Content-Type") == "" {
		upstreamReq.Header.Set("Content-Type", "application/json")
	}
	// Disable compression to avoid gzip decode issues
	upstreamReq.Header.Set("Accept-Encoding", "identity")
	// Override auth with provider's token
	upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)

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

	// Check for 401 (token expired) and retry once
	if resp.StatusCode == http.StatusUnauthorized {
		// Invalidate token cache
		a.tokenMu.Lock()
		a.tokenCache = &TokenCache{}
		a.tokenMu.Unlock()

		// Get new token
		accessToken, err = a.getAccessToken(ctx)
		if err != nil {
			return domain.NewProxyErrorWithMessage(err, true, "failed to refresh access token")
		}

		// Retry request with same headers
		upstreamReq, _ = http.NewRequestWithContext(ctx, "POST", upstreamURL, bytes.NewReader(upstreamBody))
		copyHeadersFiltered(upstreamReq.Header, originalHeaders)
		if upstreamReq.Header.Get("Content-Type") == "" {
			upstreamReq.Header.Set("Content-Type", "application/json")
		}
		upstreamReq.Header.Set("Accept-Encoding", "identity")
		upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)
		resp, err = client.Do(upstreamReq)
		if err != nil {
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
	data.Set("client_id", "77185425430.apps.googleusercontent.com")
	data.Set("client_secret", "OTJgUOQcT7lO7GsGZq2G4IlT")

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

func (a *AntigravityAdapter) buildUpstreamURL(config *domain.ProviderConfigAntigravity, model string, stream bool) string {
	baseURL := config.Endpoint
	if baseURL == "" {
		baseURL = fmt.Sprintf("https://us-central1-aiplatform.googleapis.com/v1/projects/%s/locations/us-central1", config.ProjectID)
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	if stream {
		return fmt.Sprintf("%s/publishers/google/models/%s:streamGenerateContent?alt=sse", baseURL, model)
	}
	return fmt.Sprintf("%s/publishers/google/models/%s:generateContent", baseURL, model)
}

func (a *AntigravityAdapter) handleNonStreamResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response, clientType, targetType domain.ClientType, needsConversion bool) error {
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
			attempt.InputTokenCount = metrics.InputTokens
			attempt.OutputTokenCount = metrics.OutputTokens
			attempt.CacheReadCount = metrics.CacheReadCount
			attempt.CacheWriteCount = metrics.CacheCreationCount
			attempt.Cache5mWriteCount = metrics.Cache5mCreationCount
			attempt.Cache1hWriteCount = metrics.Cache1hCreationCount
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

func (a *AntigravityAdapter) handleStreamResponse(ctx context.Context, w http.ResponseWriter, resp *http.Response, clientType, targetType domain.ClientType, needsConversion bool) error {
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
		}
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
			extractTokens()
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
					return nil
				}
				// Upstream connection closed - check if client is still connected
				if ctx.Err() != nil {
					extractTokens()
					return domain.NewProxyErrorWithMessage(ctx.Err(), false, "client disconnected")
				}
				extractTokens()
				return nil // Upstream closed normally
			}

			// Collect all SSE content (preserve complete format including newlines)
			sseBuffer.Write(result.line)

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

func isRetryableStatusCode(code int) bool {
	switch code {
	case 429, 500, 502, 503, 504:
		return true
	default:
		return false
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

// Headers to filter out - only privacy/proxy related, NOT application headers
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

	// Headers that will be overridden
	"host":          true,
	"content-length": true,
	"authorization": true,
	"x-api-key":     true,
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
