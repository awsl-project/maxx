package executor

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/awsl-project/maxx/internal/converter"
	"github.com/awsl-project/maxx/internal/cooldown"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/event"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/repository"
	"github.com/awsl-project/maxx/internal/router"
	"github.com/awsl-project/maxx/internal/stats"
	"github.com/awsl-project/maxx/internal/waiter"
)

// Executor handles request execution with retry logic
type Executor struct {
	router           *router.Router
	proxyRequestRepo repository.ProxyRequestRepository
	attemptRepo      repository.ProxyUpstreamAttemptRepository
	retryConfigRepo  repository.RetryConfigRepository
	sessionRepo      repository.SessionRepository
	modelMappingRepo repository.ModelMappingRepository
	settingsRepo     repository.SystemSettingRepository
	broadcaster      event.Broadcaster
	projectWaiter    *waiter.ProjectWaiter
	instanceID       string
	statsAggregator  *stats.StatsAggregator
	converter        *converter.Registry
	engine           *flow.Engine
	middlewares      []flow.HandlerFunc
}

// NewExecutor creates a new executor
func NewExecutor(
	r *router.Router,
	prr repository.ProxyRequestRepository,
	ar repository.ProxyUpstreamAttemptRepository,
	rcr repository.RetryConfigRepository,
	sessionRepo repository.SessionRepository,
	modelMappingRepo repository.ModelMappingRepository,
	settingsRepo repository.SystemSettingRepository,
	bc event.Broadcaster,
	projectWaiter *waiter.ProjectWaiter,
	instanceID string,
	statsAggregator *stats.StatsAggregator,
) *Executor {
	return &Executor{
		router:           r,
		proxyRequestRepo: prr,
		attemptRepo:      ar,
		retryConfigRepo:  rcr,
		sessionRepo:      sessionRepo,
		modelMappingRepo: modelMappingRepo,
		settingsRepo:     settingsRepo,
		broadcaster:      bc,
		projectWaiter:    projectWaiter,
		instanceID:       instanceID,
		statsAggregator:  statsAggregator,
		converter:        converter.GetGlobalRegistry(),
		engine:           flow.NewEngine(),
	}
}

func (e *Executor) Use(handlers ...flow.HandlerFunc) {
	e.middlewares = append(e.middlewares, handlers...)
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
		return domain.NewProxyErrorWithMessage(domain.ErrInvalidInput, false, "flow context missing")
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
	// Database model mapping with full query conditions
	query := &domain.ModelMappingQuery{
		ClientType:   clientType,
		ProviderType: provider.Type,
		ProviderID:   provider.ID,
		ProjectID:    projectID,
		RouteID:      route.ID,
		APITokenID:   apiTokenID,
	}
	mappings, _ := e.modelMappingRepo.ListByQuery(tenantID, query)
	for _, m := range mappings {
		if domain.MatchWildcard(m.Pattern, requestModel) {
			return m.Target
		}
	}

	// No mapping, use original
	return requestModel
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

// handleCooldown processes cooldown information from ProxyError and sets provider cooldown
// Priority: 1) Explicit time from API, 2) Policy-based calculation based on failure reason
func (e *Executor) handleCooldown(proxyErr *domain.ProxyError, provider *domain.Provider, clientType domain.ClientType, originalClientType domain.ClientType) {
	selectedClientType := proxyErr.CooldownClientType
	if proxyErr.RateLimitInfo != nil && proxyErr.RateLimitInfo.ClientType != "" {
		selectedClientType = proxyErr.RateLimitInfo.ClientType
	}
	if selectedClientType == "" {
		if originalClientType != "" {
			selectedClientType = string(originalClientType)
		} else {
			selectedClientType = string(clientType)
		}
	}

	// Determine cooldown reason and explicit time
	var reason cooldown.CooldownReason
	var explicitUntil *time.Time

	// Priority 1: Check for explicit cooldown time from API
	if proxyErr.CooldownUntil != nil {
		// Has explicit time from API (e.g., from CooldownUntil field)
		explicitUntil = proxyErr.CooldownUntil
		reason = cooldown.ReasonQuotaExhausted // Default, may be overridden below
		if proxyErr.RateLimitInfo != nil {
			reason = mapRateLimitTypeToReason(proxyErr.RateLimitInfo.Type)
		}
	} else if proxyErr.RateLimitInfo != nil && !proxyErr.RateLimitInfo.QuotaResetTime.IsZero() {
		// Has explicit quota reset time from API
		explicitUntil = &proxyErr.RateLimitInfo.QuotaResetTime
		reason = mapRateLimitTypeToReason(proxyErr.RateLimitInfo.Type)
	} else if proxyErr.RetryAfter > 0 {
		// Has Retry-After duration from API
		untilTime := time.Now().Add(proxyErr.RetryAfter)
		explicitUntil = &untilTime
		reason = cooldown.ReasonRateLimit
	} else if proxyErr.IsServerError {
		// Server error (5xx) - no explicit time, use policy
		reason = cooldown.ReasonServerError
		explicitUntil = nil
	} else if proxyErr.IsNetworkError {
		// Network error - no explicit time, use policy
		reason = cooldown.ReasonNetworkError
		explicitUntil = nil
	} else {
		// Unknown error type - use policy
		reason = cooldown.ReasonUnknown
		explicitUntil = nil
	}

	// Record failure and apply cooldown
	// If explicitUntil is not nil, it will be used directly
	// Otherwise, cooldown duration is calculated based on policy and failure count
	cooldown.Default().RecordFailure(provider.ID, selectedClientType, reason, explicitUntil)

	// If there's an async update channel, listen for updates
	if proxyErr.CooldownUpdateChan != nil {
		go e.handleAsyncCooldownUpdate(proxyErr.CooldownUpdateChan, provider, selectedClientType)
	}
}

func shouldSkipErrorCooldown(provider *domain.Provider) bool {
	return provider != nil && provider.Config != nil && provider.Config.DisableErrorCooldown
}

// mapRateLimitTypeToReason maps RateLimitInfo.Type to CooldownReason
func mapRateLimitTypeToReason(rateLimitType string) cooldown.CooldownReason {
	switch rateLimitType {
	case "quota_exhausted":
		return cooldown.ReasonQuotaExhausted
	case "rate_limit_exceeded":
		return cooldown.ReasonRateLimit
	case "concurrent_limit":
		return cooldown.ReasonConcurrentLimit
	default:
		return cooldown.ReasonRateLimit // Default to rate limit
	}
}

// handleAsyncCooldownUpdate listens for async cooldown updates from providers
func (e *Executor) handleAsyncCooldownUpdate(updateChan chan time.Time, provider *domain.Provider, clientType string) {
	select {
	case newCooldownTime := <-updateChan:
		if !newCooldownTime.IsZero() {
			cooldown.Default().UpdateCooldown(provider.ID, clientType, newCooldownTime)
		}
	case <-time.After(15 * time.Second):
		// Timeout waiting for update
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
					attempt.CacheReadCount = event.Metrics.CacheReadCount
					attempt.CacheWriteCount = event.Metrics.CacheCreationCount
					attempt.Cache5mWriteCount = event.Metrics.Cache5mCreationCount
					attempt.Cache1hWriteCount = event.Metrics.Cache1hCreationCount
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
					attempt.CacheReadCount = ev.Metrics.CacheReadCount
					attempt.CacheWriteCount = ev.Metrics.CacheCreationCount
					attempt.Cache5mWriteCount = ev.Metrics.Cache5mCreationCount
					attempt.Cache1hWriteCount = ev.Metrics.Cache1hCreationCount
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

// getRequestDetailRetentionSeconds 获取请求详情保留秒数
// 返回值：-1=永久保存，0=不保存，>0=保留秒数
func (e *Executor) getRequestDetailRetentionSeconds() int {
	if e.settingsRepo == nil {
		return -1 // 默认永久保存
	}
	val, err := e.settingsRepo.Get(domain.SettingKeyRequestDetailRetentionSeconds)
	if err != nil || val == "" {
		return -1 // 默认永久保存
	}
	seconds, err := strconv.Atoi(val)
	if err != nil {
		return -1
	}
	return seconds
}

// shouldClearRequestDetailFor 检查是否应该立即清理请求详情（考虑 Token 开发者模式）
func (e *Executor) shouldClearRequestDetailFor(state *execState) bool {
	if state != nil && state.apiTokenDevMode {
		return false
	}
	return e.shouldClearRequestDetail()
}

// shouldClearRequestDetail 检查是否应该立即清理请求详情（全局配置）
// 当设置为 0 时返回 true
func (e *Executor) shouldClearRequestDetail() bool {
	return e.getRequestDetailRetentionSeconds() == 0
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
