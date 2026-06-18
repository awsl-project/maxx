package codex

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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider"
	cliproxyapi "github.com/awsl-project/maxx/internal/adapter/provider/cliproxyapi_codex"
	"github.com/awsl-project/maxx/internal/codexutil"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/payloadoverride"
	"github.com/awsl-project/maxx/internal/usage"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func init() {
	provider.RegisterAdapterFactory("codex", NewAdapter)
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			codexCacheMu.Lock()
			now := time.Now()
			for k, v := range codexCaches {
				if now.After(v.Expire) {
					delete(codexCaches, k)
				}
			}
			codexCacheMu.Unlock()
		}
	}()
}

// TokenCache caches access tokens
type TokenCache struct {
	AccessToken string
	ExpiresAt   time.Time
}

// ProviderUpdateFunc is a callback to persist token updates to the provider config.
// It is a type alias (not a defined type) so the router's duck-typed
// `SetProviderUpdateFunc(func(*domain.Provider) error)` interface — which uses the
// literal func type — matches this method's signature exactly; a defined type
// would silently fail the assertion and leave token persistence wired to nothing.
type ProviderUpdateFunc = func(provider *domain.Provider) error

// ProviderReloadFunc re-reads the freshest provider record (e.g. from the
// repository). It lets the adapter pick up a token another path rotated and
// persisted while this adapter was waiting on the refresh lock.
//
// It is a type alias (not a defined type) so the router's duck-typed
// `SetProviderReloadFunc(func() (*domain.Provider, error))` interface — which
// uses the literal func type — matches this method's signature exactly.
type ProviderReloadFunc = func() (*domain.Provider, error)

// CodexAdapter handles communication with OpenAI Codex API
type CodexAdapter struct {
	provider       *domain.Provider
	tokenCache     *TokenCache
	tokenMu        sync.RWMutex
	httpClient     *http.Client
	providerUpdate ProviderUpdateFunc
	providerReload ProviderReloadFunc
}

// SetProviderUpdateFunc sets the callback for persisting provider updates
func (a *CodexAdapter) SetProviderUpdateFunc(fn ProviderUpdateFunc) {
	a.providerUpdate = fn
}

// SetProviderReloadFunc sets the callback used to re-read the freshest provider
// record under the refresh lock.
func (a *CodexAdapter) SetProviderReloadFunc(fn ProviderReloadFunc) {
	a.providerReload = fn
}

func NewAdapter(p *domain.Provider) (provider.ProviderAdapter, error) {
	config := ensureCodexConfig(p)

	// Persist the synthesized config back onto the provider so downstream update callbacks
	// and retry logic observe a consistent shape.
	p.Config.Codex = config

	// If UseCLIProxyAPI is enabled, directly return CLIProxyAPI adapter
	if config.UseCLIProxyAPI {
		return cliproxyapi.NewAdapter(p)
	}

	adapter := &CodexAdapter{
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

func (a *CodexAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeCodex}
}

func (a *CodexAdapter) Execute(c *flow.Ctx, provider *domain.Provider) error {
	requestBody := flow.GetRequestBody(c)
	clientWantsStream := flow.GetIsStream(c)
	request := c.Request
	ctx := context.Background()
	if request != nil {
		ctx = request.Context()
	}

	// Get access token
	accessToken, err := a.getAccessToken(ctx, false, "")
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(err, false, "failed to get access token")
		proxyErr.Scope = domain.ScopeKey
		proxyErr.Reason = domain.CooldownReasonAuthFailure
		return proxyErr
	}

	// Apply Codex CLI payload adjustments (CLIProxyAPI-aligned)
	cacheID, updatedBody := applyCodexRequestTuning(c, requestBody)
	requestBody = updatedBody

	// Apply provider-level overrides for reasoning and service_tier
	config := provider.Config.Codex
	if config.Reasoning != "" {
		if updated, err := sjson.SetBytes(requestBody, "reasoning.effort", config.Reasoning); err == nil {
			requestBody = updated
		}
	}
	if config.ServiceTier != "" {
		if updated, err := sjson.SetBytes(requestBody, "service_tier", config.ServiceTier); err == nil {
			requestBody = updated
		}
	}
	requestBody = payloadoverride.ApplyGlobal(requestBody, "codex", flow.GetMappedModel(c))

	// Build upstream URL and stream mode.
	//
	// Custom downstream with passthrough on (default): forward the exact Responses
	// path the client used (preserving /v1, since New API / OpenAI-compatible
	// gateways serve /v1/responses, not /responses). Stream vs non-stream is
	// conveyed via the body's "stream" flag, not /responses/compact (which is
	// ChatGPT-specific and 404s elsewhere).
	//
	// Official ChatGPT backend (no custom BaseURL), or custom downstream with
	// passthrough explicitly disabled: use the ChatGPT contract — /responses for
	// streaming, /responses/compact for non-streaming.
	upstreamStream := clientWantsStream
	baseURL := CodexBaseURL
	custom := config.BaseURL != ""
	if custom {
		baseURL = strings.TrimRight(config.BaseURL, "/")
	}

	var upstreamURL string
	if custom && domain.ResponsesPassthroughEnabled(config.ResponsesPassthrough) {
		path := flow.GetResponsesClientPath(c)
		if path == "" {
			// No client Responses path captured (e.g. converted from another
			// client type) — default to the OpenAI-compatible endpoint.
			path = "/v1/responses"
		}
		upstreamURL = baseURL + path
	} else {
		upstreamURL = baseURL + "/responses"
		if !clientWantsStream {
			upstreamURL = baseURL + "/responses/compact"
			upstreamStream = false
		}
	}
	if len(requestBody) > 0 {
		if updated, err := sjson.SetBytes(requestBody, "stream", upstreamStream); err == nil {
			requestBody = updated
		}
	}

	// Create upstream request
	upstreamReq, err := http.NewRequestWithContext(ctx, "POST", upstreamURL, bytes.NewReader(requestBody))
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(err, false, "failed to create upstream request")
		proxyErr.Scope = domain.ScopeEndpoint
		proxyErr.Reason = domain.CooldownReasonServerError
		return proxyErr
	}

	// Apply headers with passthrough support (client headers take priority)
	a.applyCodexHeaders(upstreamReq, request, accessToken, config.AccountID, upstreamStream, cacheID)

	// Send request info via EventChannel
	if eventChan := flow.GetEventChan(c); eventChan != nil {
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
		proxyErr := domain.NewScopedProxyError(domain.ErrUpstreamError, domain.ScopeProvider, domain.CooldownReasonNetworkError)
		proxyErr.Message = "failed to connect to upstream"
		return proxyErr
	}
	defer resp.Body.Close()

	// Handle 401 (token expired) - refresh and retry once
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()

		// Force a real refresh: skip the cache and the persisted access token
		// (the latter is the same credential the upstream just rejected). Pass
		// the rejected token so a refresh another goroutine already completed is
		// still accepted, and only the actually-failed token is refused.
		accessToken, err = a.getAccessToken(ctx, true, accessToken)
		if err != nil {
			proxyErr := domain.NewProxyErrorWithMessage(err, false, "failed to refresh access token")
			proxyErr.Scope = domain.ScopeKey
			proxyErr.Reason = domain.CooldownReasonAuthFailure
			return proxyErr
		}

		// Retry request
		upstreamReq, reqErr := http.NewRequestWithContext(ctx, "POST", upstreamURL, bytes.NewReader(requestBody))
		if reqErr != nil {
			proxyErr := domain.NewProxyErrorWithMessage(reqErr, false, fmt.Sprintf("failed to create retry request: %v", reqErr))
			proxyErr.Scope = domain.ScopeEndpoint
			proxyErr.Reason = domain.CooldownReasonServerError
			return proxyErr
		}
		a.applyCodexHeaders(upstreamReq, request, accessToken, config.AccountID, upstreamStream, cacheID)

		resp, err = a.httpClient.Do(upstreamReq)
		if err != nil {
			proxyErr := domain.NewScopedProxyError(domain.ErrUpstreamError, domain.ScopeProvider, domain.CooldownReasonNetworkError)
			proxyErr.Message = "failed to connect to upstream after token refresh"
			return proxyErr
		}
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

		return classifyCodexHTTPError(resp.StatusCode, body, resp.Header, flow.GetMappedModel(c))
	}

	// Handle response
	if clientWantsStream {
		return a.handleStreamResponse(c, resp)
	}
	return a.handleNonStreamResponse(c, resp)
}

// WarmToken pre-warms the access token cache to avoid blocking during Execute
func (a *CodexAdapter) WarmToken(ctx context.Context) error {
	_, err := a.getAccessToken(ctx, false, "")
	return err
}

// getAccessToken returns a valid access token, refreshing via the OAuth
// refresh_token when necessary. When forceRefresh is true the in-memory cache
// and the persisted access token are bypassed entirely — used on a 401 retry
// where the current token has just been rejected upstream. rejectedToken is the
// token that just failed (only meaningful when forceRefresh is true): a cached
// token equal to it is treated as stale so the forced refresh actually refreshes,
// while a newer token minted concurrently is still accepted.
func (a *CodexAdapter) getAccessToken(ctx context.Context, forceRefresh bool, rejectedToken string) (string, error) {
	if !forceRefresh {
		// Check cache
		a.tokenMu.RLock()
		if a.tokenCache.AccessToken != "" {
			if !isFallbackCodexAccessToken(a.tokenCache.AccessToken) && (a.tokenCache.ExpiresAt.IsZero() || time.Now().Add(60*time.Second).Before(a.tokenCache.ExpiresAt)) {
				token := a.tokenCache.AccessToken
				a.tokenMu.RUnlock()
				return token, nil
			}
		}
		a.tokenMu.RUnlock()
	}

	// Work against a local provider snapshot. a.provider is set once at
	// construction and never reassigned, so it can be read without a lock; the
	// reload-under-lock path below rebinds this local (not the shared field) to
	// the freshest record, avoiding a data race with concurrent callers.
	prov := a.provider
	config := ensureCodexConfig(prov)

	if !forceRefresh {
		// Use persisted access token only if present AND still valid. Caching an
		// expired persisted token here could clobber a fresher token that a
		// concurrent refresh just stored, so only write the cache when usable.
		if strings.TrimSpace(config.AccessToken) != "" && !isFallbackCodexAccessToken(config.AccessToken) {
			var expiresAt time.Time
			if strings.TrimSpace(config.ExpiresAt) != "" {
				if parsed, err := time.Parse(time.RFC3339, config.ExpiresAt); err == nil {
					expiresAt = parsed
				}
			}
			if expiresAt.IsZero() || time.Now().Add(60*time.Second).Before(expiresAt) {
				a.tokenMu.Lock()
				a.tokenCache = &TokenCache{
					AccessToken: config.AccessToken,
					ExpiresAt:   expiresAt,
				}
				a.tokenMu.Unlock()
				return config.AccessToken, nil
			}
		}
	}

	// Refresh token
	if strings.TrimSpace(config.RefreshToken) == "" {
		log.Printf("[Codex] level=INFO trigger=fallback provider=%q provider_id=%d reason=missing_refresh_token message=%q",
			prov.Name,
			prov.ID,
			"codex provider config missing refresh token; using placeholder local token for fallback flow",
		)
		fallbackToken := buildFallbackCodexAccessToken(prov)
		a.tokenMu.Lock()
		a.tokenCache = &TokenCache{AccessToken: fallbackToken}
		a.tokenMu.Unlock()
		if a.providerUpdate != nil {
			// Copy-on-write: mutate a clone, not the shared provider that
			// concurrent requests read lock-free; repo.Update swaps the pointer.
			cp, cpCfg := CloneForTokenPersist(prov)
			cpCfg.AccessToken = fallbackToken
			cpCfg.ExpiresAt = time.Now().Add(5 * time.Second).Format(time.RFC3339)
			if err := a.providerUpdate(cp); err != nil {
				log.Printf("[Codex] failed to persist fallback token: %v", err)
			}
		}
		return fallbackToken, nil
	}

	// Serialize refreshes per account: refresh_tokens rotate and reusing an old
	// one is rejected upstream, so concurrent refreshes (across requests, the
	// quota task, and the quota handler) must not run in parallel.
	unlock := AcquireRefreshLock(RefreshLockKey(config.AccountID, config.RefreshToken))
	defer unlock()

	// Re-read the freshest provider under the lock: another path (quota task or
	// handler, or another adapter instance) may have rotated and persisted a new
	// token while we waited, leaving our snapshot's refresh_token stale.
	if a.providerReload != nil {
		if fresh, rerr := a.providerReload(); rerr == nil && fresh != nil {
			freshCfg := ensureCodexConfig(fresh)
			if strings.TrimSpace(freshCfg.RefreshToken) != "" {
				prov = fresh
				config = freshCfg
				// If the freshly persisted access token is usable (and, on a
				// forced refresh, differs from the rejected one), adopt it and
				// skip a needless refresh that would rotate the token again.
				if strings.TrimSpace(config.AccessToken) != "" && !isFallbackCodexAccessToken(config.AccessToken) &&
					(!forceRefresh || config.AccessToken != rejectedToken) {
					var expiresAt time.Time
					if strings.TrimSpace(config.ExpiresAt) != "" {
						if parsed, perr := time.Parse(time.RFC3339, config.ExpiresAt); perr == nil {
							expiresAt = parsed
						}
					}
					if expiresAt.IsZero() || time.Now().Add(60*time.Second).Before(expiresAt) {
						a.tokenMu.Lock()
						a.tokenCache = &TokenCache{AccessToken: config.AccessToken, ExpiresAt: expiresAt}
						a.tokenMu.Unlock()
						return config.AccessToken, nil
					}
				}
			}
		}
	}

	// Double-check: another goroutine may have produced a fresh token while we
	// waited for the lock. Reuse it instead of spending another refresh_token.
	// On a forced refresh, only accept a token that differs from the rejected
	// one (otherwise it is the same credential the upstream just refused).
	a.tokenMu.RLock()
	cachedToken := a.tokenCache.AccessToken
	cachedExpiry := a.tokenCache.ExpiresAt
	a.tokenMu.RUnlock()
	if cachedToken != "" && !isFallbackCodexAccessToken(cachedToken) &&
		(!forceRefresh || cachedToken != rejectedToken) &&
		(cachedExpiry.IsZero() || time.Now().Add(60*time.Second).Before(cachedExpiry)) {
		return cachedToken, nil
	}

	tokenResp, err := RefreshAccessTokenWithRetry(ctx, config.RefreshToken, 3)
	if err != nil {
		// On a forced refresh, don't fall back to the persisted token: it was
		// already rejected upstream and would only trigger another 401.
		if !forceRefresh && strings.TrimSpace(config.AccessToken) != "" && !isFallbackCodexAccessToken(config.AccessToken) {
			return config.AccessToken, nil
		}
		return "", err
	}

	// Store the raw expiry; every read path already applies a 60s safety margin
	// before treating the token as expired (avoids double-buffering, and matches
	// how the quota refreshers persist ExpiresAt).
	expiresAt := TokenExpiresAt(tokenResp.ExpiresIn)

	if err := a.persistRefreshedToken(prov, tokenResp, expiresAt); err != nil {
		return "", err
	}

	// Update cache only after persistence succeeds. A rotated refresh_token that
	// works only in memory is a trap: after restart/cache expiry the next caller
	// would read the old persisted token and hit invalid_grant/reuse upstream.
	a.tokenMu.Lock()
	a.tokenCache = &TokenCache{
		AccessToken: tokenResp.AccessToken,
		ExpiresAt:   expiresAt,
	}
	a.tokenMu.Unlock()

	return tokenResp.AccessToken, nil
}

func (a *CodexAdapter) persistRefreshedToken(prov *domain.Provider, tokenResp *TokenResponse, expiresAt time.Time) error {
	if tokenResp == nil {
		return fmt.Errorf("failed to persist refreshed token: empty token response")
	}

	oldRefreshToken := ""
	if prov != nil && prov.Config != nil && prov.Config.Codex != nil {
		oldRefreshToken = prov.Config.Codex.RefreshToken
	}
	refreshRotated := tokenResp.RefreshToken != "" && tokenResp.RefreshToken != oldRefreshToken

	if a.providerUpdate == nil {
		if refreshRotated {
			return fmt.Errorf("failed to persist refreshed token: provider update callback not configured")
		}
		return nil
	}

	// Copy-on-write: the cached repository hands out the same *Provider to every
	// caller and the request hot path reads these fields lock-free, so mutate a
	// clone instead of the shared struct. repo.Update atomically swaps the cache
	// pointer; readers holding the old pointer see a consistent (if briefly stale)
	// struct.
	cp, cpCfg := CloneForTokenPersist(prov)
	cpCfg.AccessToken = tokenResp.AccessToken
	cpCfg.ExpiresAt = expiresAt.Format(time.RFC3339)
	if tokenResp.RefreshToken != "" {
		cpCfg.RefreshToken = tokenResp.RefreshToken
	}
	if tokenResp.IDToken != "" {
		if claims, parseErr := ParseIDToken(tokenResp.IDToken); parseErr == nil && claims != nil {
			if v := strings.TrimSpace(claims.GetAccountID()); v != "" {
				cpCfg.AccountID = v
			}
			if v := strings.TrimSpace(claims.GetUserID()); v != "" {
				cpCfg.UserID = v
			}
			if v := strings.TrimSpace(claims.Email); v != "" {
				cpCfg.Email = v
			}
			if v := strings.TrimSpace(claims.Name); v != "" {
				cpCfg.Name = v
			}
			if v := strings.TrimSpace(claims.Picture); v != "" {
				cpCfg.Picture = v
			}
			if v := strings.TrimSpace(claims.GetPlanType()); v != "" {
				cpCfg.PlanType = v
			}
			if v := strings.TrimSpace(claims.GetSubscriptionStart()); v != "" {
				cpCfg.SubscriptionStart = v
			}
			if v := strings.TrimSpace(claims.GetSubscriptionEnd()); v != "" {
				cpCfg.SubscriptionEnd = v
			}
		}
	}
	if err := a.providerUpdate(cp); err != nil {
		return fmt.Errorf("failed to persist refreshed token: %w", err)
	}
	return nil
}

func (a *CodexAdapter) handleNonStreamResponse(c *flow.Ctx, resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, true, "failed to read upstream response")
		proxyErr.Scope = domain.ScopeProvider
		proxyErr.Reason = domain.CooldownReasonNetworkError
		return proxyErr
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
			metrics = usage.AdjustForClientType(metrics, domain.ClientTypeCodex)
			eventChan.SendMetrics(&domain.AdapterMetrics{
				InputTokens:    metrics.InputTokens,
				OutputTokens:   metrics.OutputTokens,
				CacheReadCount: metrics.CacheReadCount,
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

func (a *CodexAdapter) handleStreamResponse(c *flow.Ctx, resp *http.Response) error {
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
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, false, "streaming not supported")
		proxyErr.Scope = domain.ScopeRequest
		return proxyErr
	}

	// Incrementally extract metrics and model from SSE lines (no full-stream buffering)
	var collector usage.StreamCollector
	var model string
	var lastStreamErr *codexStreamError
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
			proxyErr := domain.NewProxyErrorWithMessage(ctx.Err(), false, "client disconnected")
			proxyErr.Scope = domain.ScopeRequest
			return proxyErr
		default:
		}

		line, err := reader.ReadString('\n')
		if line != "" {
			// Extract metrics and model incrementally per line
			collector.ProcessSSELine(line)
			extractModelFromSSELine(line, &model)

			if isCodexResponseCompletedLine(line) {
				responseCompleted = true
			}

			if e := parseCodexStreamErrorLine(line); e != nil {
				lastStreamErr = e
			}

			// Write to client
			_, writeErr := c.Writer.Write([]byte(line))
			if writeErr != nil {
				a.sendFinalStreamEvents(eventChan, &collector, &model, resp)
				if responseCompleted {
					return nil
				}
				proxyErr := domain.NewProxyErrorWithMessage(writeErr, false, "client disconnected")
				proxyErr.Scope = domain.ScopeRequest
				return proxyErr
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
			if responseCompleted {
				return nil
			}
			if ctx.Err() != nil {
				proxyErr := domain.NewProxyErrorWithMessage(ctx.Err(), false, "client disconnected")
				proxyErr.Scope = domain.ScopeRequest
				return proxyErr
			}
			proxyErr := domain.NewProxyErrorWithMessage(err, true, "stream closed before response.completed")
			// If the upstream emitted a structured error event in the stream
			// (e.g. response.failed with code=model_not_supported), narrow the
			// cooldown scope so an unrelated model on the same provider is not
			// frozen along with the failing one. Anything we can't classify
			// confidently keeps the conservative ScopeProvider fallback.
			if scope, reason, ok := classifyCodexStreamError(lastStreamErr); ok {
				if scope == domain.ScopeModel {
					proxyErr.Model = lastStreamErr.model
					if proxyErr.Model == "" {
						proxyErr.Model = model
					}
					if proxyErr.Model == "" {
						proxyErr.Model = flow.GetMappedModel(c)
					}
					// Without a model to attribute, ScopeModel would collapse
					// to a (provider,"","") cooldown key — i.e. provider-wide —
					// defeating the point of refining the scope. Fall back to
					// the conservative ScopeProvider in that case.
					if proxyErr.Model == "" {
						proxyErr.Scope = domain.ScopeProvider
						proxyErr.Reason = domain.CooldownReasonNetworkError
						return proxyErr
					}
				}
				proxyErr.Scope = scope
				proxyErr.Reason = reason
			} else {
				proxyErr.Scope = domain.ScopeProvider
				proxyErr.Reason = domain.CooldownReasonNetworkError
			}
			return proxyErr
		}
	}
}

func isCodexResponseCompletedLine(line string) bool {
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
	return gjson.Get(data, "type").String() == "response.completed"
}

func (a *CodexAdapter) sendFinalStreamEvents(eventChan domain.AdapterEventChan, collector *usage.StreamCollector, model *string, resp *http.Response) {
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
		metrics := usage.AdjustForClientType(collector.Metrics, domain.ClientTypeCodex)
		eventChan.SendMetrics(&domain.AdapterMetrics{
			InputTokens:    metrics.InputTokens,
			OutputTokens:   metrics.OutputTokens,
			CacheReadCount: metrics.CacheReadCount,
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
	data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if data == "" || data == "[DONE]" {
		return
	}
	var chunk struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal([]byte(data), &chunk); err == nil && chunk.Model != "" {
		*model = chunk.Model
	}
}

type codexCache struct {
	ID     string
	Expire time.Time
}

var (
	codexCacheMu sync.Mutex
	codexCaches  = map[string]codexCache{}
)

func getCodexCache(key string) (codexCache, bool) {
	codexCacheMu.Lock()
	defer codexCacheMu.Unlock()
	cache, ok := codexCaches[key]
	if !ok {
		return codexCache{}, false
	}
	if time.Now().After(cache.Expire) {
		delete(codexCaches, key)
		return codexCache{}, false
	}
	return cache, true
}

func setCodexCache(key string, cache codexCache) {
	codexCacheMu.Lock()
	codexCaches[key] = cache
	codexCacheMu.Unlock()
}

func applyCodexRequestTuning(c *flow.Ctx, body []byte) (string, []byte) {
	if len(body) == 0 {
		return "", body
	}

	origBody := flow.GetOriginalRequestBody(c)
	origType := flow.GetOriginalClientType(c)

	cacheID := ""
	if origType == domain.ClientTypeClaude && len(origBody) > 0 {
		userID := gjson.GetBytes(origBody, "metadata.user_id")
		if userID.Exists() && strings.TrimSpace(userID.String()) != "" {
			model := gjson.GetBytes(body, "model").String()
			key := model + "-" + userID.String()
			if cache, ok := getCodexCache(key); ok {
				cacheID = cache.ID
			} else {
				cacheID = uuid.NewString()
				setCodexCache(key, codexCache{
					ID:     cacheID,
					Expire: time.Now().Add(1 * time.Hour),
				})
			}
		}
	} else if len(origBody) > 0 {
		if promptKey := gjson.GetBytes(origBody, "prompt_cache_key"); promptKey.Exists() {
			cacheID = promptKey.String()
		}
	}

	if cacheID != "" {
		if updated, err := sjson.SetBytes(body, "prompt_cache_key", cacheID); err == nil {
			body = updated
		}
	}

	if updated, err := sjson.SetBytes(body, "stream", true); err == nil {
		body = updated
	}
	body, _ = sjson.DeleteBytes(body, "previous_response_id")
	body, _ = sjson.DeleteBytes(body, "prompt_cache_retention")
	body, _ = sjson.DeleteBytes(body, "safety_identifier")
	if maxOut := gjson.GetBytes(body, "max_output_tokens"); maxOut.Exists() {
		if !gjson.GetBytes(body, "max_tokens").Exists() {
			if updated, err := sjson.SetBytes(body, "max_tokens", maxOut.Value()); err == nil {
				body = updated
			}
		}
		body, _ = sjson.DeleteBytes(body, "max_output_tokens")
	}
	if !gjson.GetBytes(body, "instructions").Exists() {
		body, _ = sjson.SetBytes(body, "instructions", "")
	}
	body = codexutil.NormalizeCodexInput(body)

	return cacheID, body
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

func classifyCodexHTTPError(statusCode int, body []byte, headers http.Header, model string) *domain.ProxyError {
	bodyLower := strings.ToLower(string(body))

	proxyErr := &domain.ProxyError{
		Err:            fmt.Errorf("upstream error: %s", string(body)),
		Message:        fmt.Sprintf("upstream returned status %d", statusCode),
		HTTPStatusCode: statusCode,
		Retryable:      isRetryableStatusCode(statusCode),
		ClientType:     string(domain.ClientTypeCodex),
	}

	switch {
	case statusCode == 400 || statusCode == 413 || statusCode == 422:
		proxyErr.Scope = domain.ScopeRequest
		proxyErr.Retryable = false

	case statusCode == 401:
		proxyErr.Scope = domain.ScopeKey
		proxyErr.Reason = domain.CooldownReasonAuthFailure
		proxyErr.Retryable = false

	case statusCode == 403:
		proxyErr.Scope = domain.ScopeKey
		proxyErr.Reason = domain.CooldownReasonAuthFailure
		proxyErr.Retryable = false

	case statusCode == 404:
		if model != "" && strings.Contains(bodyLower, "model") {
			proxyErr.Scope = domain.ScopeModel
			proxyErr.Reason = domain.CooldownReasonModelUnavailable
			proxyErr.Model = model
		} else {
			proxyErr.Scope = domain.ScopeEndpoint
			proxyErr.Reason = domain.CooldownReasonServerError
		}
		proxyErr.Retryable = false

	case statusCode == 429:
		proxyErr.Scope = domain.ScopeKey
		proxyErr.Reason = domain.CooldownReasonRateLimitExceeded
		proxyErr.Retryable = true
		// Parse Retry-After
		if retryAfter := headers.Get("Retry-After"); retryAfter != "" {
			if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
				proxyErr.RetryAfter = time.Duration(seconds) * time.Second
				until := time.Now().Add(proxyErr.RetryAfter)
				proxyErr.CooldownUntil = &until
			}
		}
		if proxyErr.CooldownUntil == nil {
			until := time.Now().Add(time.Minute)
			proxyErr.CooldownUntil = &until
		}

	case statusCode == 503:
		if model != "" && (strings.Contains(bodyLower, "overloaded") || strings.Contains(bodyLower, "model")) {
			proxyErr.Scope = domain.ScopeModel
			proxyErr.Reason = domain.CooldownReasonServerError
			proxyErr.Model = model
		} else {
			proxyErr.Scope = domain.ScopeProvider
			proxyErr.Reason = domain.CooldownReasonServerError
		}

	case statusCode >= 500:
		proxyErr.Scope = domain.ScopeProvider
		proxyErr.Reason = domain.CooldownReasonServerError

	default:
		proxyErr.Scope = domain.ScopeRequest
		proxyErr.Retryable = false
	}

	return proxyErr
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

// applyCodexHeaders applies headers for Codex API requests
// It follows the CLIProxyAPI pattern: passthrough client headers, use defaults only when missing
func (a *CodexAdapter) applyCodexHeaders(upstreamReq, clientReq *http.Request, accessToken, accountID string, stream bool, cacheID string) {
	hasAccessToken := strings.TrimSpace(accessToken) != ""

	// First, copy passthrough headers from client request (excluding hop-by-hop and auth)
	if clientReq != nil {
		for k, vv := range clientReq.Header {
			lk := strings.ToLower(k)
			if codexFilteredHeaders[lk] {
				continue
			}
			if lk == "authorization" && hasAccessToken {
				continue
			}
			for _, v := range vv {
				upstreamReq.Header.Add(k, v)
			}
		}
	}

	// Set required headers (these always override)
	upstreamReq.Header.Set("Content-Type", "application/json")
	if hasAccessToken {
		upstreamReq.Header.Set("Authorization", "Bearer "+accessToken)
	}
	if stream {
		upstreamReq.Header.Set("Accept", "text/event-stream")
	} else {
		upstreamReq.Header.Set("Accept", "application/json")
	}
	upstreamReq.Header.Set("Connection", "Keep-Alive")

	// Set Codex-specific headers only if client didn't provide them
	ensureHeader(upstreamReq.Header, clientReq, "Version", CodexVersion)
	ensureHeader(upstreamReq.Header, clientReq, "Openai-Beta", OpenAIBetaHeader)
	if cacheID != "" {
		upstreamReq.Header.Set("Conversation_id", cacheID)
		upstreamReq.Header.Set("Session_id", cacheID)
	} else {
		ensureHeader(upstreamReq.Header, clientReq, "Session_id", uuid.NewString())
	}
	upstreamReq.Header.Set("User-Agent", resolveCodexUserAgent(clientReq))
	if hasAccessToken {
		ensureHeader(upstreamReq.Header, clientReq, "Originator", CodexOriginator)
	}

	// Set account ID if available (required for OAuth auth, not for API key)
	if hasAccessToken && accountID != "" {
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

func resolveCodexUserAgent(clientReq *http.Request) string {
	if clientReq != nil {
		if ua := clientReq.Header.Get("User-Agent"); strings.TrimSpace(ua) != "" {
			return ua
		}
	}
	return CodexUserAgent
}

func isCodexCLIUserAgent(userAgent string) bool {
	ua := strings.ToLower(strings.TrimSpace(userAgent))
	return strings.HasPrefix(ua, "codex_cli_rs/") || strings.HasPrefix(ua, "codex-cli/")
}

var codexFilteredHeaders = map[string]bool{
	// Hop-by-hop headers
	"connection":        true,
	"keep-alive":        true,
	"transfer-encoding": true,
	"upgrade":           true,

	// Headers set by HTTP client
	"host":           true,
	"content-length": true,

	// Explicitly controlled headers
	"user-agent": true,

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
