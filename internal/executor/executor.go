package executor

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/converter"
	"github.com/awsl-project/maxx/internal/cooldown"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/event"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/repository"
	"github.com/awsl-project/maxx/internal/requestmeta"
	"github.com/awsl-project/maxx/internal/router"
	"github.com/awsl-project/maxx/internal/waiter"
)

// Executor handles request execution with retry logic
type Executor struct {
	router           *router.Router
	proxyRequestRepo repository.ProxyRequestRepository
	attemptRepo      repository.ProxyUpstreamAttemptRepository
	apiTokenRepo     repository.APITokenRepository
	retryConfigRepo  repository.RetryConfigRepository
	sessionRepo      repository.SessionRepository
	modelMappingRepo repository.ModelMappingRepository
	settingsRepo     repository.SystemSettingRepository
	broadcaster      event.Broadcaster
	projectWaiter    *waiter.ProjectWaiter
	instanceID       string
	converter        *converter.Registry
	engine           *flow.Engine
	middlewares      []flow.HandlerFunc
	cooldownSem      chan struct{} // semaphore to limit concurrent cooldown update goroutines

	smartMappingMu      sync.RWMutex
	smartMappingSuccess map[string]string
}

// NewExecutor creates a new executor
func NewExecutor(
	r *router.Router,
	prr repository.ProxyRequestRepository,
	ar repository.ProxyUpstreamAttemptRepository,
	apiTokenRepo repository.APITokenRepository,
	rcr repository.RetryConfigRepository,
	sessionRepo repository.SessionRepository,
	modelMappingRepo repository.ModelMappingRepository,
	settingsRepo repository.SystemSettingRepository,
	bc event.Broadcaster,
	projectWaiter *waiter.ProjectWaiter,
	instanceID string,
) *Executor {
	return &Executor{
		router:              r,
		proxyRequestRepo:    prr,
		attemptRepo:         ar,
		apiTokenRepo:        apiTokenRepo,
		retryConfigRepo:     rcr,
		sessionRepo:         sessionRepo,
		modelMappingRepo:    modelMappingRepo,
		settingsRepo:        settingsRepo,
		broadcaster:         bc,
		projectWaiter:       projectWaiter,
		instanceID:          instanceID,
		converter:           converter.GetGlobalRegistry(),
		engine:              flow.NewEngine(),
		cooldownSem:         make(chan struct{}, 10),
		smartMappingSuccess: make(map[string]string),
	}
}

func (e *Executor) Use(handlers ...flow.HandlerFunc) {
	e.middlewares = append(e.middlewares, handlers...)
}

func (e *Executor) CloseResponsesWebSocketConnection(connectionID string) {
	if e != nil && e.router != nil {
		e.router.CloseResponsesWebSocketConnection(connectionID)
	}
}

// HasResponsesWebSocketProvider reports whether any Codex route can serve
// Responses WebSocket for the given project scope (same rules as Match).
// Used to reject upgrades with HTTP 426 for Codex fallback.
func (e *Executor) HasResponsesWebSocketProvider(tenantID, projectID uint64) bool {
	if e == nil || e.router == nil {
		return false
	}
	return e.router.HasResponsesWebSocketProvider(tenantID, projectID)
}

// Execute runs the executor middleware chain with a new flow context.
func (e *Executor) Execute(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	c := flow.NewCtx(w, req)
	if ctx != nil {
		c.Set(flow.KeyProxyContext, ctx)
	}
	return e.ExecuteWith(c)
}

// ExecuteWith runs the executor middleware chain using an existing flow context.
func (e *Executor) ExecuteWith(c *flow.Ctx) error {
	if c == nil {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrInvalidInput, false, "flow context missing")
		proxyErr.Scope = domain.ScopeRequest
		return proxyErr
	}
	ctx := context.Background()
	if v, ok := c.Get(flow.KeyProxyContext); ok {
		if stored, ok := v.(context.Context); ok && stored != nil {
			ctx = stored
		}
	}
	state := &execState{ctx: ctx}
	c.Set(flow.KeyExecutorState, state)
	chain := []flow.HandlerFunc{e.egress, e.ingress}
	chain = append(chain, e.middlewares...)
	chain = append(chain, e.routeMatch, e.dispatch)
	e.engine.HandleWith(c, chain...)
	return state.lastErr
}

func (e *Executor) mapModel(tenantID uint64, requestModel string, route *domain.Route, provider *domain.Provider, clientType domain.ClientType, projectID uint64, apiTokenID uint64) string {
	candidates := e.mapModelCandidates(tenantID, requestModel, route, provider, clientType, projectID, apiTokenID)
	return candidates[0]
}

func (e *Executor) mapModelCandidates(tenantID uint64, requestModel string, route *domain.Route, provider *domain.Provider, clientType domain.ClientType, projectID uint64, apiTokenID uint64) []string {
	query := &domain.ModelMappingQuery{
		ClientType:   clientType,
		ProviderType: provider.Type,
		ProviderID:   provider.ID,
		ProjectID:    projectID,
		RouteID:      route.ID,
		APITokenID:   apiTokenID,
	}
	mappings, _ := e.modelMappingRepo.ListByQuery(tenantID, query)
	candidates := make([]string, 0, len(mappings))
	seen := make(map[string]struct{}, len(mappings))
	for _, m := range mappings {
		if m == nil || !domain.MatchWildcard(m.Pattern, requestModel) {
			continue
		}
		if _, exists := seen[m.Target]; exists {
			continue
		}
		seen[m.Target] = struct{}{}
		candidates = append(candidates, m.Target)
	}
	if len(candidates) == 0 {
		return []string{requestModel}
	}
	return candidates
}

func smartMappingCacheKey(tenantID uint64, requestModel string, route *domain.Route, provider *domain.Provider, clientType domain.ClientType, projectID uint64, apiTokenID uint64, candidates []string) string {
	routeID := uint64(0)
	providerID := uint64(0)
	providerType := ""
	if route != nil {
		routeID = route.ID
	}
	if provider != nil {
		providerID = provider.ID
		providerType = provider.Type
	}
	parts := []string{
		strconv.FormatUint(tenantID, 10),
		strconv.FormatUint(projectID, 10),
		strconv.FormatUint(apiTokenID, 10),
		strconv.FormatUint(routeID, 10),
		strconv.FormatUint(providerID, 10),
		providerType,
		string(clientType),
		requestModel,
		strings.Join(candidates, "\x00"),
	}
	return strings.Join(parts, "\x1f")
}

func (e *Executor) smartMappingStartIndex(key string, candidates []string) int {
	if e == nil || key == "" || len(candidates) == 0 {
		return 0
	}
	e.smartMappingMu.RLock()
	last := e.smartMappingSuccess[key]
	e.smartMappingMu.RUnlock()
	if last == "" {
		return 0
	}
	for i, candidate := range candidates {
		if candidate == last {
			return i
		}
	}
	return 0
}

func (e *Executor) recordSmartMappingSuccess(key string, mappedModel string) {
	if e == nil || key == "" || mappedModel == "" {
		return
	}
	e.smartMappingMu.Lock()
	if e.smartMappingSuccess == nil {
		e.smartMappingSuccess = make(map[string]string)
	}
	e.smartMappingSuccess[key] = mappedModel
	e.smartMappingMu.Unlock()
}

func (e *Executor) getRetryConfig(tenantID uint64, config *domain.RetryConfig) *domain.RetryConfig {
	if config != nil {
		return config
	}

	// Get default config
	defaultConfig, err := e.retryConfigRepo.GetDefault(tenantID)
	if err == nil && defaultConfig != nil {
		return defaultConfig
	}

	// No default config means no retry
	return &domain.RetryConfig{
		MaxRetries:      0,
		InitialInterval: 0,
		BackoffRate:     1.0,
		MaxInterval:     0,
	}
}

func (e *Executor) calculateBackoff(config *domain.RetryConfig, attempt int) time.Duration {
	wait := float64(config.InitialInterval)
	for i := 0; i < attempt; i++ {
		wait *= config.BackoffRate
	}
	if time.Duration(wait) > config.MaxInterval {
		return config.MaxInterval
	}
	return time.Duration(wait)
}

func generateRequestID() string {
	return time.Now().Format("20060102150405.000000")
}

// flattenHeaders converts http.Header to map[string]string (taking first value)
func flattenHeaders(h http.Header) map[string]string {
	if h == nil {
		return nil
	}
	result := make(map[string]string)
	for key, values := range h {
		if len(values) > 0 {
			result[key] = values[0]
		}
	}
	return result
}

// RecordRejectedProxyRequest persists an early-rejected request so the requests page
// can explain why the token saw an immediate 429 before dispatching upstream.
func (e *Executor) RecordRejectedProxyRequest(c *flow.Ctx, apiToken *domain.APIToken, statusCode int, errMsg string) {
	if e == nil || c == nil {
		return
	}

	tenantID := domain.DefaultTenantID
	apiTokenID := uint64(0)
	projectID := flow.GetProjectID(c)
	devMode := false
	if apiToken != nil {
		if apiToken.TenantID > 0 {
			tenantID = apiToken.TenantID
		}
		apiTokenID = apiToken.ID
		devMode = apiToken.DevMode
		if projectID == 0 {
			projectID = apiToken.ProjectID
		}
	}

	now := time.Now()
	reasoningBody := flow.GetOriginalRequestBody(c)
	if len(reasoningBody) == 0 {
		reasoningBody = flow.GetRequestBody(c)
	}
	isStream := flow.GetIsStream(c)
	isWebSocket := flow.GetResponsesWebSocketExchange(c) != nil
	proxyReq := &domain.ProxyRequest{
		TenantID:        tenantID,
		InstanceID:      e.instanceID,
		RequestID:       generateRequestID(),
		SessionID:       flow.GetSessionID(c),
		ClientType:      flow.GetClientType(c),
		ProjectID:       projectID,
		RequestModel:    flow.GetRequestModel(c),
		ReasoningEffort: requestmeta.ReasoningEffort(reasoningBody),
		StartTime:       now,
		EndTime:         now,
		Duration:        0,
		IsStream:        isStream,
		Protocol:        domain.ResolveProxyRequestProtocol(isStream, isWebSocket),
		Status:          "REJECTED",
		StatusCode:      statusCode,
		Error:           errMsg,
		APITokenID:      apiTokenID,
		DevMode:         devMode,
	}

	clearDetail := e.shouldClearFailedRequestDetailFor(&execState{apiTokenDevMode: devMode})
	if !clearDetail {
		rawHeaders := flow.GetRequestHeaders(c)
		requestHeaders := flattenHeaders(rawHeaders)
		requestURI := flow.GetRequestURI(c)
		requestBody := flow.GetRequestBody(c)
		if c.Request != nil {
			if c.Request.Host != "" {
				if requestHeaders == nil {
					requestHeaders = make(map[string]string)
				}
				requestHeaders["Host"] = c.Request.Host
			}
			proxyReq.RequestInfo = &domain.RequestInfo{
				Method:  c.Request.Method,
				URL:     requestURI,
				Headers: requestHeaders,
				Body:    domain.RequestBodySnapshot(requestBody, rawHeaders.Get("Content-Type"), devMode),
			}
		}
		proxyReq.ResponseInfo = &domain.ResponseInfo{Status: statusCode}
	}

	if err := e.proxyRequestRepo.Create(proxyReq); err != nil {
		log.Printf("[Executor] Failed to create rejected proxy request: %v", err)
		return
	}
	if e.broadcaster != nil {
		e.broadcaster.BroadcastProxyRequest(proxyReq)
	}
}

// handleCooldown processes cooldown information from ProxyError and sets provider cooldown
func (e *Executor) handleCooldown(proxyErr *domain.ProxyError, provider *domain.Provider, clientType domain.ClientType, model string) {
	if proxyErr.Scope == domain.ScopeRequest {
		return // no cooldown for request-level errors
	}

	selectedClientType := proxyErr.ClientType
	if selectedClientType == "" {
		selectedClientType = string(clientType)
	}

	// Map domain CooldownReason to cooldown package CooldownReason
	reason := cooldown.CooldownReason(proxyErr.Reason)

	// Use explicit upstream cooldown time first. Retry-After comes next.
	// The configurable setting only replaces the local policy fallback for
	// short request/concurrency throttles (the fixed 5s defaults in
	// cooldown.DefaultPolicies), not quota reset/default times.
	var explicitUntil *time.Time
	if proxyErr.CooldownUntil != nil {
		explicitUntil = proxyErr.CooldownUntil
	} else if proxyErr.RetryAfter > 0 {
		t := time.Now().Add(proxyErr.RetryAfter)
		explicitUntil = &t
	} else if isConfigurableRateLimitFallback(reason) {
		explicitUntil = e.rateLimitDefaultCooldownUntil()
	}

	// Determine model for cooldown key
	cooldownModel := ""
	if proxyErr.Scope == domain.ScopeModel {
		cooldownModel = proxyErr.Model
		if cooldownModel == "" {
			cooldownModel = model // fallback to the request's mapped model
		}
	}

	cooldown.Default().RecordFailure(provider.ID, selectedClientType, cooldownModel, reason, proxyErr.Scope, explicitUntil)

	// If there's an async update channel, listen for updates (bounded by semaphore)
	if proxyErr.CooldownUpdateChan != nil {
		select {
		case e.cooldownSem <- struct{}{}:
			go e.handleAsyncCooldownUpdate(proxyErr.CooldownUpdateChan, provider, selectedClientType, cooldownModel)
		default:
		}
	}
}

func isConfigurableRateLimitFallback(reason cooldown.CooldownReason) bool {
	return reason == cooldown.ReasonRateLimit || reason == cooldown.ReasonConcurrentLimit || reason == cooldown.ReasonNetworkError
}

func (e *Executor) rateLimitDefaultCooldownUntil() *time.Time {
	duration := 5 * time.Second
	if e != nil && e.settingsRepo != nil {
		value, err := e.settingsRepo.Get(domain.SettingKeyRateLimitCooldownDefaultSeconds)
		if err == nil && strings.TrimSpace(value) != "" {
			if seconds, parseErr := strconv.Atoi(strings.TrimSpace(value)); parseErr == nil && seconds >= 1 && seconds <= 86400 {
				duration = time.Duration(seconds) * time.Second
			}
		}
	}
	until := time.Now().Add(duration)
	return &until
}

func shouldSkipErrorCooldown(provider *domain.Provider) bool {
	return provider != nil && provider.Config != nil && provider.Config.DisableErrorCooldown
}

func shouldSkipErrorCooldownUpdate(provider *domain.Provider, proxyErr *domain.ProxyError) bool {
	return shouldSkipErrorCooldown(provider)
}

func applyDisabledErrorCooldownRetryPolicy(provider *domain.Provider, proxyErr *domain.ProxyError) {
	if !shouldSkipErrorCooldown(provider) || proxyErr == nil {
		return
	}
	if !isDisabledErrorCooldownRetryableError(proxyErr) {
		return
	}

	// disableErrorCooldown is an explicit operator escape hatch: failures from
	// this provider should not create cooldown state and should keep retrying the
	// same provider beyond the matched retry config until the request succeeds or
	// its context is cancelled.
	proxyErr.Retryable = true
}

func isDisabledErrorCooldownRetryableError(proxyErr *domain.ProxyError) bool {
	if proxyErr == nil {
		return false
	}
	if isBedrockAdaptiveThinkingSchemaError(proxyErr) {
		return false
	}
	// Non-retryable client errors (4xx) must NOT be force-retried, even under
	// disableErrorCooldown. Retrying or failing over on a bad client request
	// (400 invalid image, 404 "No endpoints found that support tool use",
	// 413/415/422 payload rejects, 401/403 auth) cannot succeed — the request
	// itself is the problem, so retrying only amplifies load. A single such
	// request once retried 151,780 times over 4 hours before this guard existed.
	//
	// Genuinely retryable upstream-side failures keep the escape hatch:
	//   - 5xx server errors (provider is transiently broken)
	//   - 429 rate limit / 408 request timeout (worth retry / failover)
	//   - network + committed-stream-read errors (no HTTP status)
	if code := proxyErr.HTTPStatusCode; code >= 400 && code < 500 {
		if isRetryable4xxStatusCode(code) {
			return true
		}
		return isCommittedStreamReadRetryableError(proxyErr)
	}
	if proxyErr.HTTPStatusCode >= 500 && proxyErr.HTTPStatusCode < 600 {
		return true
	}
	return isCommittedStreamReadRetryableError(proxyErr)
}

// isRetryable4xxStatusCode reports whether a 4xx status is worth retrying or
// failing over. Only rate limiting (429) and upstream request timeouts (408)
// qualify; every other 4xx is a client-request error where the same request on
// any provider yields the same rejection.
func isRetryable4xxStatusCode(code int) bool {
	switch code {
	case http.StatusTooManyRequests, // 429 — rate limit, back off / fail over
		http.StatusRequestTimeout: // 408 — upstream timed out before responding
		return true
	default:
		return false
	}
}

func applyCommittedStreamReadRetryPolicy(proxyErr *domain.ProxyError) {
	if isCommittedStreamReadRetryableError(proxyErr) {
		proxyErr.Retryable = true
	}
}

func isCommittedStreamReadRetryableError(proxyErr *domain.ProxyError) bool {
	if proxyErr == nil {
		return false
	}
	if proxyErr.Scope != domain.ScopeProvider {
		return false
	}
	if proxyErr.Reason != domain.CooldownReasonNetworkError {
		return false
	}
	msg := proxyErr.Message
	if proxyErr.Err != nil {
		msg += " " + proxyErr.Err.Error()
	}
	msg = strings.ToLower(msg)
	return strings.Contains(msg, "stream read error") ||
		strings.Contains(msg, "upstream stream") ||
		strings.Contains(msg, "unexpected eof") ||
		strings.Contains(msg, "upstream response stream was interrupted") ||
		(strings.Contains(msg, "response stream") && strings.Contains(msg, "interrupt"))
}

func shouldRetryCommittedResponseError(proxyErr *domain.ProxyError) bool {
	return proxyErr != nil && proxyErr.Retryable && isCommittedStreamReadRetryableError(proxyErr)
}

func isBedrockAdaptiveThinkingSchemaError(proxyErr *domain.ProxyError) bool {
	if proxyErr == nil || proxyErr.Scope != domain.ScopeRequest || proxyErr.HTTPStatusCode != http.StatusBadRequest {
		return false
	}
	text := strings.ToLower(proxyErr.Message)
	if proxyErr.Err != nil {
		text += " " + strings.ToLower(proxyErr.Err.Error())
	}
	return strings.Contains(text, "bedrock runtime") &&
		strings.Contains(text, "validationexception") &&
		strings.Contains(text, "enabled") &&
		strings.Contains(text, "adaptive") &&
		strings.Contains(text, "output_config.effort")
}

// handleAsyncCooldownUpdate listens for async cooldown updates from providers
func (e *Executor) handleAsyncCooldownUpdate(updateChan chan time.Time, provider *domain.Provider, clientType string, model string) {
	defer func() { <-e.cooldownSem }()
	select {
	case newCooldownTime := <-updateChan:
		if !newCooldownTime.IsZero() {
			cooldown.Default().UpdateCooldown(provider.ID, clientType, model, newCooldownTime)
		}
	case <-time.After(15 * time.Second):
	}
}

// processAdapterEvents drains the event channel and updates attempt record
func (e *Executor) processAdapterEvents(eventChan domain.AdapterEventChan, attempt *domain.ProxyUpstreamAttempt) {
	if eventChan == nil || attempt == nil {
		return
	}

	// Drain all events from channel (non-blocking)
	for {
		select {
		case event, ok := <-eventChan:
			if !ok {
				return // Channel closed
			}
			if event == nil {
				continue
			}

			switch event.Type {
			case domain.EventRequestInfo:
				if event.RequestInfo != nil {
					attempt.RequestInfo = event.RequestInfo
				}
			case domain.EventResponseInfo:
				if event.ResponseInfo != nil {
					attempt.ResponseInfo = event.ResponseInfo
				}
			case domain.EventMetrics:
				if event.Metrics != nil {
					attempt.InputTokenCount = event.Metrics.InputTokens
					attempt.OutputTokenCount = event.Metrics.OutputTokens
					attempt.InputImageTokenCount = event.Metrics.InputImageTokens
					attempt.OutputImageTokenCount = event.Metrics.OutputImageTokens
					attempt.CacheReadCount = event.Metrics.CacheReadCount
					attempt.CacheWriteCount = event.Metrics.CacheCreationCount
					attempt.Cache5mWriteCount = event.Metrics.Cache5mCreationCount
					attempt.Cache1hWriteCount = event.Metrics.Cache1hCreationCount
					attempt.UpstreamCostNanoUSD = event.Metrics.UpstreamCostNanoUSD
				}
			case domain.EventResponseModel:
				if event.ResponseModel != "" {
					attempt.ResponseModel = event.ResponseModel
				}
			case domain.EventFirstToken:
				if event.FirstTokenTime > 0 {
					firstTokenTime := time.UnixMilli(event.FirstTokenTime)
					attempt.TTFT = firstTokenTime.Sub(attempt.StartTime)
				}
			}
		default:
			// No more events
			return
		}
	}
}

// processAdapterEventsRealtime processes events in real-time during adapter execution
// It broadcasts updates immediately when RequestInfo/ResponseInfo are received
func (e *Executor) processAdapterEventsRealtime(
	eventChan domain.AdapterEventChan,
	attempt *domain.ProxyUpstreamAttempt,
	done chan struct{},
	clearDetail bool,
) {
	defer close(done)

	if eventChan == nil || attempt == nil {
		return
	}

	// 事件节流：合并多个 adapter 事件为一次广播，避免在流式高并发下产生“广播风暴”
	const broadcastThrottle = 200 * time.Millisecond
	ticker := time.NewTicker(broadcastThrottle)
	defer ticker.Stop()

	dirty := false

	flush := func() {
		if !dirty || e.broadcaster == nil {
			dirty = false
			return
		}
		// 广播前做一次瘦身 + 快照，避免发送大字段、也避免指针被后续修改导致数据竞争
		snapshot := event.SanitizeProxyUpstreamAttemptForBroadcast(attempt)
		e.broadcaster.BroadcastProxyUpstreamAttempt(snapshot)
		dirty = false
	}

	for {
		select {
		case ev, ok := <-eventChan:
			if !ok {
				flush()
				return
			}
			if ev == nil {
				continue
			}

			switch ev.Type {
			case domain.EventRequestInfo:
				if !clearDetail && ev.RequestInfo != nil {
					attempt.RequestInfo = ev.RequestInfo
					dirty = true
				}
			case domain.EventResponseInfo:
				if !clearDetail && ev.ResponseInfo != nil {
					attempt.ResponseInfo = ev.ResponseInfo
					dirty = true
				}
			case domain.EventMetrics:
				if ev.Metrics != nil {
					attempt.InputTokenCount = ev.Metrics.InputTokens
					attempt.OutputTokenCount = ev.Metrics.OutputTokens
					attempt.InputImageTokenCount = ev.Metrics.InputImageTokens
					attempt.OutputImageTokenCount = ev.Metrics.OutputImageTokens
					attempt.CacheReadCount = ev.Metrics.CacheReadCount
					attempt.CacheWriteCount = ev.Metrics.CacheCreationCount
					attempt.Cache5mWriteCount = ev.Metrics.Cache5mCreationCount
					attempt.Cache1hWriteCount = ev.Metrics.Cache1hCreationCount
					attempt.UpstreamCostNanoUSD = ev.Metrics.UpstreamCostNanoUSD
					dirty = true
				}
			case domain.EventResponseModel:
				if ev.ResponseModel != "" {
					attempt.ResponseModel = ev.ResponseModel
					dirty = true
				}
			case domain.EventFirstToken:
				if ev.FirstTokenTime > 0 {
					// Calculate TTFT as duration from start time to first token time
					firstTokenTime := time.UnixMilli(ev.FirstTokenTime)
					attempt.TTFT = firstTokenTime.Sub(attempt.StartTime)
					dirty = true
				}
			}
		case <-ticker.C:
			flush()
		}
	}
}

// shouldClearRequestDetailFor 检查是否应该立即清理请求详情（考虑 Token 开发者模式）
func (e *Executor) shouldClearRequestDetailFor(state *execState) bool {
	if state != nil && state.apiTokenDevMode {
		return false
	}
	return e.shouldClearRequestDetail()
}

// shouldClearFailedRequestDetailFor 检查已知失败状态的请求详情是否应该立即清理。
func (e *Executor) shouldClearFailedRequestDetailFor(state *execState) bool {
	if state != nil && state.apiTokenDevMode {
		return false
	}
	return e.shouldClearFailedRequestDetail()
}

// shouldClearRequestDetail 检查是否应该立即清理请求详情（全局配置）
//
// 决策时机在 ingress / dispatch，此时 status 未知，因此不能按状态分别决策。
// 语义：
//   - split=false：当统一键 == 0 时立即清理（行为不变）
//   - split=true：仅当 success 和 failed 两个键都解析为 0 时才立即清理
//     （只要任一侧需要保留，就先存到 DB，后台 task 再按状态分别清理）
func (e *Executor) shouldClearRequestDetail() bool {
	cfg, ok := e.requestDetailRetentionConfig()
	if !ok {
		return false
	}
	if !cfg.split {
		return cfg.unified == 0
	}
	return cfg.successSec == 0 && cfg.failedSec == 0
}

// shouldClearFailedRequestDetail 检查已知失败/拒绝请求是否应该立即清理详情。
// 对 early rejection 这类已知失败状态，可以使用 failed 桶，而不是退化成
// “success 和 failed 都为 0 才清理”的未知状态策略。
func (e *Executor) shouldClearFailedRequestDetail() bool {
	cfg, ok := e.requestDetailRetentionConfig()
	if !ok {
		return false
	}
	if !cfg.split {
		return cfg.unified == 0
	}
	return cfg.failedSec == 0
}

type requestDetailRetentionConfig struct {
	unified    int
	split      bool
	successSec int
	failedSec  int
}

func (e *Executor) requestDetailRetentionConfig() (requestDetailRetentionConfig, bool) {
	if e.settingsRepo == nil {
		return requestDetailRetentionConfig{}, false
	}

	parse := func(key string, fallback int) int {
		v, err := e.settingsRepo.Get(key)
		if err != nil || v == "" {
			return fallback
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return fallback
		}
		return n
	}

	unified := parse(domain.SettingKeyRequestDetailRetentionSeconds, -1)
	splitVal, _ := e.settingsRepo.Get(domain.SettingKeyRequestDetailRetentionSplitEnabled)
	cfg := requestDetailRetentionConfig{
		unified:    unified,
		split:      splitVal == "true",
		successSec: unified,
		failedSec:  unified,
	}
	if cfg.split {
		cfg.successSec = parse(domain.SettingKeyRequestDetailRetentionSecondsSuccess, unified)
		cfg.failedSec = parse(domain.SettingKeyRequestDetailRetentionSecondsFailed, unified)
	}
	return cfg, true
}

// ShouldClearRequestDetailByConfig 检查是否应该按全局配置立即清理请求详情
func (e *Executor) ShouldClearRequestDetailByConfig() bool {
	return e.shouldClearRequestDetail()
}

// getProviderMultiplier 获取 Provider 针对特定 ClientType 的倍率
// 返回 10000 表示 1 倍，15000 表示 1.5 倍
func getProviderMultiplier(provider *domain.Provider, clientType domain.ClientType) uint64 {
	if provider == nil || provider.Config == nil || provider.Config.Custom == nil {
		return 10000 // 默认 1 倍
	}
	if provider.Config.Custom.ClientMultiplier == nil {
		return 10000
	}
	if multiplier, ok := provider.Config.Custom.ClientMultiplier[clientType]; ok && multiplier > 0 {
		return multiplier
	}
	return 10000
}
