package router

import (
	"context"
	"crypto/hmac"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/cooldown"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
	"github.com/awsl-project/maxx/internal/repository/cached"
	"github.com/awsl-project/maxx/internal/sticky"
	"github.com/awsl-project/maxx/internal/systemsettingcache"
)

// MatchedRoute contains all data needed to execute a proxy request
type MatchedRoute struct {
	Route           *domain.Route
	Provider        *domain.Provider
	ProviderAdapter provider.ProviderAdapter
	RetryConfig     *domain.RetryConfig
}

// MatchResult is the output of Match. Routes are the ordered candidates the
// dispatcher should try (first = preferred). Sticky, when non-nil, carries
// the key the dispatcher must SETEX after a successful upstream call so the
// affinity layer learns the binding.
type MatchResult struct {
	Routes []*MatchedRoute
	Sticky *StickyWrite
}

// StickyWrite is the write-back context handed to the dispatcher. It is only
// populated when the routing strategy has sticky enabled.
type StickyWrite struct {
	Key sticky.Key
	TTL time.Duration
}

// MatchContext contains all context needed for route matching.
// Ctx is the originating request's context — Match honors its cancellation
// when doing best-effort IO (currently just the sticky lookup). If Ctx is
// nil we fall back to context.Background; nil is allowed so existing
// non-proxy call sites don't have to plumb a context in.
type MatchContext struct {
	Ctx                       context.Context
	TenantID                  uint64
	ClientType                domain.ClientType
	ProjectID                 uint64
	RequestModel              string
	ModelCandidates           func(route *domain.Route, provider *domain.Provider, clientType domain.ClientType, requestModel string) []string
	APITokenID                uint64
	SessionID                 string
	RequireResponsesWebSocket bool
	RequiredProviderID        uint64
	StrictSupportModels       bool
}

// Router handles route matching and selection
type Router struct {
	routeRepo           *cached.RouteRepository
	providerRepo        *cached.ProviderRepository
	routingStrategyRepo *cached.RoutingStrategyRepository
	retryConfigRepo     *cached.RetryConfigRepository
	projectRepo         *cached.ProjectRepository
	settingRepo         repository.SystemSettingRepository

	// Adapter cache
	adapters            map[uint64]provider.ProviderAdapter
	adapterFingerprints map[uint64]string
	mu                  sync.RWMutex
	limiter             *ProviderLimiter

	// Cooldown manager
	cooldownManager *cooldown.Manager
}

// NewRouter creates a new router
func NewRouter(
	routeRepo *cached.RouteRepository,
	providerRepo *cached.ProviderRepository,
	routingStrategyRepo *cached.RoutingStrategyRepository,
	retryConfigRepo *cached.RetryConfigRepository,
	projectRepo *cached.ProjectRepository,
	settingRepo ...repository.SystemSettingRepository,
) *Router {
	var settings repository.SystemSettingRepository
	if len(settingRepo) > 0 {
		settings = settingRepo[0]
	}
	return &Router{
		routeRepo:           routeRepo,
		providerRepo:        providerRepo,
		routingStrategyRepo: routingStrategyRepo,
		retryConfigRepo:     retryConfigRepo,
		projectRepo:         projectRepo,
		settingRepo:         settings,
		adapters:            make(map[uint64]provider.ProviderAdapter),
		adapterFingerprints: make(map[uint64]string),
		limiter:             NewProviderLimiter(),
		cooldownManager:     cooldown.Default(),
	}
}

// TryAcquireProvider reserves an upstream session slot for a provider.
func (r *Router) TryAcquireProvider(p *domain.Provider) (func(), bool) {
	if r == nil || p == nil {
		return nil, false
	}
	return r.limiter.TryAcquire(p.ID, p.MaxConcurrency)
}

func (r *Router) isProviderAtConcurrencyLimit(p *domain.Provider) bool {
	return r != nil && p != nil && r.limiter.IsAtLimit(p.ID, p.MaxConcurrency)
}

// InitAdapters initializes adapters for all providers
func (r *Router) InitAdapters() error {
	providers := r.providerRepo.GetAll()
	next := make(map[uint64]provider.ProviderAdapter, len(providers))
	nextFingerprints := make(map[uint64]string, len(providers))

	for _, p := range providers {
		factory, ok := provider.GetAdapterFactory(p.Type)
		if !ok {
			continue // Skip providers without registered adapters
		}
		a, err := factory(p)
		if err != nil {
			// A single mis-configured provider (e.g. empty config) must not
			// abort building adapters for every other provider. Log and skip
			// it so the rest still get live adapters.
			log.Printf("[Router] InitAdapters: skipping provider %d (%s): factory error: %v", p.ID, p.Type, err)
			continue
		}
		r.injectProviderUpdate(a, p)
		next[p.ID] = a
		nextFingerprints[p.ID] = adapterFingerprint(p)
	}

	r.mu.Lock()
	r.adapters = next
	r.adapterFingerprints = nextFingerprints
	r.mu.Unlock()
	return nil
}

// ReconcileAdapters refreshes the adapter map to match the current provider
// cache. It is used after cross-instance provider invalidation: reloading the
// provider repository alone is not enough because Match also requires a live
// adapter for each candidate provider.
func (r *Router) ReconcileAdapters() error {
	providers := r.providerRepo.GetAll()
	r.mu.RLock()
	currentAdapters := make(map[uint64]provider.ProviderAdapter, len(r.adapters))
	for id, adapter := range r.adapters {
		currentAdapters[id] = adapter
	}
	currentFingerprints := make(map[uint64]string, len(r.adapterFingerprints))
	for id, fingerprint := range r.adapterFingerprints {
		currentFingerprints[id] = fingerprint
	}
	r.mu.RUnlock()

	next := make(map[uint64]provider.ProviderAdapter, len(providers))
	nextFingerprints := make(map[uint64]string, len(providers))
	var changedProviderIDs []uint64
	for _, p := range providers {
		fingerprint := adapterFingerprint(p)
		if current, ok := currentAdapters[p.ID]; ok && currentFingerprints[p.ID] == fingerprint {
			next[p.ID] = current
			nextFingerprints[p.ID] = fingerprint
			continue
		}

		factory, ok := provider.GetAdapterFactory(p.Type)
		if !ok {
			changedProviderIDs = append(changedProviderIDs, p.ID)
			continue
		}
		a, err := factory(p)
		if err != nil {
			// A single mis-configured provider must not abort reconciliation
			// for every other provider (which would freeze hot-reload until a
			// full restart). Log and skip it so the rest still reconcile.
			log.Printf("[Router] ReconcileAdapters: skipping provider %d (%s): factory error: %v", p.ID, p.Type, err)
			continue
		}
		r.injectProviderUpdate(a, p)
		next[p.ID] = a
		nextFingerprints[p.ID] = fingerprint
		changedProviderIDs = append(changedProviderIDs, p.ID)
	}
	for id := range currentAdapters {
		if _, ok := providers[id]; !ok {
			changedProviderIDs = append(changedProviderIDs, id)
		}
	}

	r.mu.Lock()
	r.adapters = next
	r.adapterFingerprints = nextFingerprints
	r.mu.Unlock()
	for _, providerID := range changedProviderIDs {
		provider.ClearResponsesWebSocketTransportCooldown(providerID)
	}
	return nil
}

// RefreshAdapter refreshes the adapter for a specific provider
func (r *Router) RefreshAdapter(p *domain.Provider) error {
	factory, ok := provider.GetAdapterFactory(p.Type)
	if !ok {
		return nil
	}
	a, err := factory(p)
	if err != nil {
		return err
	}
	r.injectProviderUpdate(a, p)
	provider.ClearResponsesWebSocketTransportCooldown(p.ID)
	r.mu.Lock()
	r.adapters[p.ID] = a
	r.adapterFingerprints[p.ID] = adapterFingerprint(p)
	r.mu.Unlock()
	return nil
}

// RemoveAdapter removes the adapter for a provider
func (r *Router) RemoveAdapter(providerID uint64) {
	provider.ClearResponsesWebSocketTransportCooldown(providerID)
	r.mu.Lock()
	delete(r.adapters, providerID)
	delete(r.adapterFingerprints, providerID)
	r.mu.Unlock()
}

// GetAdapter returns the cached adapter for a provider, if any. Used by
// admin endpoints that need to reach into adapter-specific state (e.g.
// Bedrock runtime model discovery).
func (r *Router) GetAdapter(providerID uint64) (provider.ProviderAdapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.adapters[providerID]
	return a, ok
}

// Match returns matched routes for a client type and project, plus optional
// sticky write-back context.
func (r *Router) Match(ctx *MatchContext) (*MatchResult, error) {
	tenantID := ctx.TenantID
	clientType := ctx.ClientType
	projectID := ctx.ProjectID
	requestModel := ctx.RequestModel

	routes := r.routeRepo.GetAll()

	// Check if ClientType has custom routes enabled for this project
	useProjectRoutes := false
	if projectID != 0 {
		project, err := r.projectRepo.GetByID(tenantID, projectID)
		if err == nil && project != nil {
			// If EnabledCustomRoutes is empty, all ClientTypes use global routes
			// If EnabledCustomRoutes is not empty, only listed ClientTypes can have custom routes
			if len(project.EnabledCustomRoutes) > 0 {
				for _, ct := range project.EnabledCustomRoutes {
					if ct == clientType {
						useProjectRoutes = true
						break
					}
				}
			}
		}
	}

	// Filter routes
	var filtered []*domain.Route
	var hasProjectRoutes bool

	// Only look for project-specific routes if ClientType is in EnabledCustomRoutes
	if useProjectRoutes {
		for _, route := range routes {
			if !route.IsEnabled {
				continue
			}
			if tenantID > 0 && route.TenantID != tenantID {
				continue
			}
			if route.ClientType != clientType {
				continue
			}
			if route.ProjectID == projectID && projectID != 0 {
				filtered = append(filtered, route)
				hasProjectRoutes = true
			}
		}
	}

	// If no project-specific routes or ClientType not enabled for custom routes, use global routes
	if !hasProjectRoutes {
		for _, route := range routes {
			if !route.IsEnabled {
				continue
			}
			if tenantID > 0 && route.TenantID != tenantID {
				continue
			}
			if route.ClientType != clientType {
				continue
			}
			if route.ProjectID == 0 {
				filtered = append(filtered, route)
			}
		}
	}

	if len(filtered) == 0 {
		if ctx.RequireResponsesWebSocket {
			return nil, domain.ErrNoResponsesWebSocketProviders
		}
		return nil, domain.ErrNoRoutes
	}

	// Get routing strategy
	strategy := r.getRoutingStrategy(tenantID, projectID)

	// Sort routes by strategy. For weighted_random we seed the RNG via an
	// HMAC of the caller identity + routing context so the same session
	// sees a stable fallback order while different sessions diverge —
	// implicit per-session affinity without shared state, and natural
	// load spread across the active session population. The HMAC salt
	// prevents an authenticated client from grinding session ids to
	// steer their traffic onto a specific provider.
	seed := makeSessionSeed(ctx)
	r.sortRoutes(filtered, strategy, seed)

	// Get default retry config
	defaultRetry, _ := r.retryConfigRepo.GetDefault(tenantID)

	// Build matched routes under r.mu so adapter map snapshots are stable.
	// Release the lock as soon as the slice is built — the sticky lookup
	// below can take up to 100ms on a degraded Redis and we don't want
	// that blocking adapter refresh/removal on the write side.
	r.mu.RLock()
	var matched []*MatchedRoute
	providers := r.providerRepo.GetAll()

	// Track why candidates were dropped so an empty result can be reported
	// precisely: sawModelReject means a route was skipped purely because the model
	// is not in the provider's SupportModels allowlist (a client request error);
	// sawTransientSkip means a route was skipped for a transient reason (cooldown)
	// that might otherwise have served the model — in which case we stay with the
	// generic ErrNoAvailableProviders rather than blaming the model.
	sawModelReject := false
	sawTransientSkip := false
	rejects := make(map[string]int)
	noteReject := func(reason string) {
		rejects[reason]++
	}

	for _, route := range filtered {
		prov, ok := providers[route.ProviderID]
		if !ok {
			noteReject("provider_missing")
			continue
		}
		if r.isProviderAtConcurrencyLimit(prov) {
			sawTransientSkip = true
			noteReject("provider_concurrency_limit")
			continue
		}

		modelCandidates := r.modelCandidatesForMatch(route, prov, clientType, requestModel, ctx.ModelCandidates)

		// Skip providers in cooldown (checks provider, key, and outbound model-level cooldowns)
		if r.isAnyModelCandidateInCooldown(route.ProviderID, clientType, modelCandidates) {
			sawTransientSkip = true
			noteReject("cooldown")
			continue
		}

		adp, ok := r.adapters[route.ProviderID]
		if !ok {
			noteReject("adapter_missing")
			continue
		}
		if ctx.RequiredProviderID != 0 && prov.ID != ctx.RequiredProviderID {
			noteReject("required_provider_mismatch")
			continue
		}
		// Derive native capability from provider + client type; never trust
		// the historical routes.is_native snapshot for WebSocket eligibility.
		native := domain.RouteIsNative(prov, route)
		if ctx.RequireResponsesWebSocket {
			if !provider.ResponsesWebSocketTransportAvailable(prov.ID) {
				sawTransientSkip = true
				noteReject("websocket_transport_unavailable")
				continue
			}
			wsAdapter := adapterSupportsResponsesWebSocket(adp)
			wsEnabled := domain.ProviderResponsesWebSocketEnabled(prov)
			if !native || !wsAdapter || !wsEnabled {
				log.Printf(
					"[Router] skip codex websocket candidate provider=%d type=%s native=%v wsAdapter=%v wsEnabled=%v adapter=%T",
					prov.ID, prov.Type, native, wsAdapter, wsEnabled, adp,
				)
				switch {
				case !native:
					noteReject("websocket_non_native_route")
				case !wsAdapter:
					noteReject("websocket_adapter_unsupported")
				case !wsEnabled:
					noteReject("websocket_provider_disabled")
				}
				continue
			}
		}

		// Check if provider supports the outbound model only when the adapter
		// natively speaks the request protocol. Model mappings are route/provider
		// scoped and dispatch applies them after route matching, so route matching
		// must evaluate the same candidate list here; otherwise a request such as
		// gpt-5 -> moonshotai/kimi-k3 can be routed by the pre-mapped gpt-5 allowlist
		// to a provider/key that cannot serve the mapped model, while a direct
		// moonshotai/kimi-k3 request would choose a different working route.
		if (r.strictSupportModelsRoutingEnabled() || ctx.StrictSupportModels) && adapterSupportsClientType(adp, clientType) && len(prov.SupportModels) > 0 && requestModel != "" {
			if !r.isAnyModelCandidateSupported(prov, modelCandidates) {
				sawModelReject = true
				noteReject("support_models_mismatch")
				continue
			}
		}

		var retryConfig *domain.RetryConfig
		if route.RetryConfigID != 0 {
			retryConfig, _ = r.retryConfigRepo.GetByID(tenantID, route.RetryConfigID)
		}
		if retryConfig == nil {
			retryConfig = defaultRetry
		}

		matched = append(matched, &MatchedRoute{
			Route:           route,
			Provider:        prov,
			ProviderAdapter: adp,
			RetryConfig:     retryConfig,
		})
	}
	r.mu.RUnlock()

	if len(matched) == 0 {
		if ctx.RequireResponsesWebSocket {
			if ctx.RequiredProviderID != 0 {
				return nil, domain.ErrResponsesWebSocketSessionUnavailable
			}
			return nil, domain.ErrNoResponsesWebSocketProviders
		}
		// Only blame the model when the emptiness is entirely due to SupportModels
		// rejections; a transient skip (cooldown) may hide a provider that does
		// support it, so fall back to the generic error to avoid mislabeling.
		if sawModelReject && !sawTransientSkip {
			return nil, domain.ErrModelNotSupported
		}
		return nil, noAvailableProvidersError(rejects)
	}

	// Sticky / session-affinity layer. Only meaningful when:
	//   - strategy is weighted_random (priority is already deterministic; sticky
	//     would be a no-op in steady state)
	//   - sticky is explicitly enabled in the strategy config
	//   - we have a stable principal (api token id) to anchor the binding to
	//
	// On hit (and the pointed-to provider is still in the matched set, i.e. not
	// in cooldown and still supports the model), we prepend it; otherwise the
	// existing seeded-weighted order stands and the dispatcher's first success
	// will write a fresh sticky.
	var stickyWrite *StickyWrite
	if strategy != nil && strategy.Type == domain.RoutingStrategyWeightedRandom &&
		strategy.Config != nil && strategy.Config.StickyEnabled && ctx.APITokenID != 0 {
		key := sticky.Key{
			TenantID:   tenantID,
			ClientType: string(clientType),
			ProjectID:  projectID,
			PolicyVer:  policyFingerprint(filtered),
			BaseKey:    sticky.BaseKey(strategy.Config.StickyScope, ctx.APITokenID, ctx.SessionID),
		}
		ttl := sticky.TTLFromConfig(strategy.Config.StickyTTLSeconds)
		stickyWrite = &StickyWrite{Key: key, TTL: ttl}

		// Bound the sticky Get: a slow/unavailable Redis must not stall the
		// match path. We derive from the caller's request context so a
		// client cancel propagates here too; if no context was supplied,
		// Background is the safe fallback. On timeout/error sticky.Get
		// returns (0,false) and we fall through to the normal
		// weighted_random order — affinity is best-effort by design.
		parentCtx := ctx.Ctx
		if parentCtx == nil {
			parentCtx = context.Background()
		}
		stickyCtx, stickyCancel := context.WithTimeout(parentCtx, 100*time.Millisecond)
		if pinned, ok := sticky.Default().Get(stickyCtx, key); ok {
			matched = promoteByProvider(matched, pinned)
		}
		stickyCancel()
	}

	return &MatchResult{Routes: matched, Sticky: stickyWrite}, nil
}

func adapterSupportsClientType(adapter provider.ProviderAdapter, clientType domain.ClientType) bool {
	for _, supported := range adapter.SupportedClientTypes() {
		if supported == clientType {
			return true
		}
	}
	return false
}

func adapterSupportsResponsesWebSocket(adapter provider.ProviderAdapter) bool {
	if adapter == nil || !adapterSupportsClientType(adapter, domain.ClientTypeCodex) {
		return false
	}
	_, ok := adapter.(provider.ResponsesWebSocketAdapter)
	return ok
}

// HasResponsesWebSocketProvider reports whether Match would find any Codex route
// that can serve Responses over WebSocket (native + adapter + opt-in flag).
//
// Route scope MUST match Match exactly: when the project enables custom Codex
// routes and has project-scoped Codex routes, only those are considered;
// otherwise only global (ProjectID == 0) routes. A false positive here lets the
// upgrade succeed (101) and Codex will not auto-fallback to HTTP/SSE — only an
// immediate 426 Upgrade Required triggers FallbackToHttp in official Codex.
func (r *Router) HasResponsesWebSocketProvider(tenantID, projectID uint64) bool {
	if r == nil {
		return false
	}
	routes, err := r.routeRepo.List(tenantID)
	if err != nil {
		return false
	}

	// Mirror Match's project custom-route gate for ClientTypeCodex.
	useProjectRoutes := false
	if projectID != 0 && r.projectRepo != nil {
		project, err := r.projectRepo.GetByID(tenantID, projectID)
		if err == nil && project != nil && len(project.EnabledCustomRoutes) > 0 {
			for _, ct := range project.EnabledCustomRoutes {
				if ct == domain.ClientTypeCodex {
					useProjectRoutes = true
					break
				}
			}
		}
	}

	// Select the same candidate set Match would use before capability filters.
	var candidates []*domain.Route
	if useProjectRoutes {
		for _, route := range routes {
			if route == nil || !route.IsEnabled {
				continue
			}
			if tenantID > 0 && route.TenantID != tenantID {
				continue
			}
			if route.ClientType != domain.ClientTypeCodex {
				continue
			}
			if route.ProjectID == projectID && projectID != 0 {
				candidates = append(candidates, route)
			}
		}
	}
	if len(candidates) == 0 {
		for _, route := range routes {
			if route == nil || !route.IsEnabled {
				continue
			}
			if tenantID > 0 && route.TenantID != tenantID {
				continue
			}
			if route.ClientType != domain.ClientTypeCodex {
				continue
			}
			if route.ProjectID == 0 {
				candidates = append(candidates, route)
			}
		}
	}

	providers := r.providerRepo.GetAll()
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, route := range candidates {
		prov, ok := providers[route.ProviderID]
		if !ok || prov == nil || r.isProviderAtConcurrencyLimit(prov) {
			continue
		}
		adp, ok := r.adapters[route.ProviderID]
		if !ok {
			continue
		}
		if !domain.RouteIsNative(prov, route) {
			continue
		}
		if !adapterSupportsResponsesWebSocket(adp) {
			continue
		}
		if !domain.ProviderResponsesWebSocketEnabled(prov) {
			continue
		}
		if !provider.ResponsesWebSocketTransportAvailable(prov.ID) {
			continue
		}
		return true
	}
	return false
}

func (r *Router) CloseResponsesWebSocketConnection(connectionID string) {
	if r == nil || connectionID == "" {
		return
	}
	r.mu.RLock()
	cleaners := make([]provider.ResponsesWebSocketSessionCleaner, 0, len(r.adapters))
	for _, adapter := range r.adapters {
		if cleaner, ok := adapter.(provider.ResponsesWebSocketSessionCleaner); ok {
			cleaners = append(cleaners, cleaner)
		}
	}
	r.mu.RUnlock()
	for _, cleaner := range cleaners {
		cleaner.CloseResponsesWebSocketConnection(connectionID)
	}
}

func (r *Router) strictSupportModelsRoutingEnabled() bool {
	return systemsettingcache.GetBooleanDefault(r.settingRepo, domain.SettingKeyStrictSupportModelsRoutingEnabled, false)
}

func (r *Router) modelCandidatesForMatch(route *domain.Route, provider *domain.Provider, clientType domain.ClientType, requestModel string, candidatesFn func(route *domain.Route, provider *domain.Provider, clientType domain.ClientType, requestModel string) []string) []string {
	candidates := []string{requestModel}
	if candidatesFn != nil {
		if mapped := candidatesFn(route, provider, clientType, requestModel); len(mapped) > 0 {
			candidates = mapped
		}
	}
	return candidates
}

func (r *Router) isAnyModelCandidateInCooldown(providerID uint64, clientType domain.ClientType, candidates []string) bool {
	if len(candidates) == 0 {
		return r.cooldownManager.IsInCooldown(providerID, string(clientType), "")
	}
	for _, model := range candidates {
		if r.cooldownManager.IsInCooldown(providerID, string(clientType), model) {
			return true
		}
	}
	return false
}

func (r *Router) isAnyModelCandidateSupported(provider *domain.Provider, candidates []string) bool {
	for _, model := range candidates {
		if model != "" && r.isModelSupported(model, provider.SupportModels) {
			return true
		}
	}
	return false
}

// isModelSupported checks if a model matches any pattern in the support list
func (r *Router) isModelSupported(model string, supportModels []string) bool {
	for _, pattern := range supportModels {
		if domain.MatchWildcard(pattern, model) {
			return true
		}
	}
	return false
}

// adapterFingerprint covers the provider fields captured by adapter
// construction. Route-only fields such as SupportModels intentionally stay out
// so unrelated routing edits do not drop stateful adapter instances.
func adapterFingerprint(p *domain.Provider) string {
	if p == nil {
		return ""
	}
	supportedClientTypes := append([]domain.ClientType(nil), p.SupportedClientTypes...)
	sort.Slice(supportedClientTypes, func(i, j int) bool {
		return supportedClientTypes[i] < supportedClientTypes[j]
	})
	payload := struct {
		Type                 string
		Config               *domain.ProviderConfig
		SupportedClientTypes []domain.ClientType
	}{
		Type:                 p.Type,
		Config:               p.Config,
		SupportedClientTypes: supportedClientTypes,
	}
	data, _ := json.Marshal(payload)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (r *Router) getRoutingStrategy(tenantID uint64, projectID uint64) *domain.RoutingStrategy {
	// Try project-specific strategy first
	if projectID != 0 {
		if s, err := r.routingStrategyRepo.GetByProjectID(tenantID, projectID); err == nil {
			return s
		}
	}
	// Fall back to global strategy
	if s, err := r.routingStrategyRepo.GetByProjectID(tenantID, 0); err == nil {
		return s
	}
	// Default to priority
	return &domain.RoutingStrategy{Type: domain.RoutingStrategyPriority}
}

func (r *Router) sortRoutes(routes []*domain.Route, strategy *domain.RoutingStrategy, seed int64) {
	switch strategy.Type {
	case domain.RoutingStrategyWeightedRandom:
		// 按权重做概率排序：权重越大，排在前面的概率越高
		weightedShuffle(routes, rand.New(rand.NewSource(seed)))
	default: // priority
		sort.Slice(routes, func(i, j int) bool {
			return routes[i].Position < routes[j].Position
		})
	}
}

// policyFingerprint returns a short, stable hash of the routes that are in
// scope for this Match call. Sticky entries embed it so any user-driven
// config change (route added/removed, weight/position edited, provider
// re-pointed) naturally invalidates all bindings — no explicit cache flush.
//
// Cooldown state is *not* included: cooldown is transient and already
// handled by the matched-set filter at request time. Mixing it in would
// invalidate sticky every time a provider blips.
func policyFingerprint(routes []*domain.Route) string {
	// Sort by ID for a stable hash regardless of input order. We hash IDs
	// directly (instead of sorting routes) to avoid mutating the caller's
	// slice ordering.
	ids := make([]uint64, len(routes))
	byID := make(map[uint64]*domain.Route, len(routes))
	for i, r := range routes {
		ids[i] = r.ID
		byID[r.ID] = r
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	h := sha256.New()
	var buf [8]byte
	for _, id := range ids {
		r := byID[id]
		binary.LittleEndian.PutUint64(buf[:], r.ID)
		h.Write(buf[:])
		binary.LittleEndian.PutUint64(buf[:], r.ProviderID)
		h.Write(buf[:])
		binary.LittleEndian.PutUint64(buf[:], uint64(r.Position))
		h.Write(buf[:])
		binary.LittleEndian.PutUint64(buf[:], uint64(r.Weight))
		h.Write(buf[:])
		var flag byte
		if r.IsEnabled {
			flag = 1
		}
		h.Write([]byte{flag})
	}
	sum := h.Sum(nil)
	// 32 hex chars (128 bits): birthday collisions only matter past ~1.8e19
	// distinct configs, which we will never approach. Earlier 48-bit hashes
	// would have hit ~1.6e7 — Codex reviewer flagged this as a long-tail
	// adversarial concern even though normal tenants never get close.
	return hex.EncodeToString(sum[:16])
}

// promoteByProvider moves the matched route for providerID (if any) to the
// front, preserving the relative order of the rest. No-op if not present.
func promoteByProvider(matched []*MatchedRoute, providerID uint64) []*MatchedRoute {
	for i, mr := range matched {
		if mr.Provider.ID == providerID {
			if i == 0 {
				return matched
			}
			out := make([]*MatchedRoute, 0, len(matched))
			out = append(out, mr)
			out = append(out, matched[:i]...)
			out = append(out, matched[i+1:]...)
			return out
		}
	}
	return matched
}

// routingSeedSalt is a process-wide secret mixed into makeSessionSeed.
// Without it, an attacker holding any valid API token could grind through
// X-Session-Id values offline until the seeded shuffle lands traffic on
// whichever upstream they want to target (e.g. always the cheapest, or
// always the one with the largest prompt cache they want to poison).
//
// Resolution order, lazy on first use:
//  1. MAXX_ROUTING_SEED_SALT env var (operator-controlled; required for
//     full sticky-binding consistency across multi-instance deployments).
//  2. 32 bytes from crypto/rand. Per-process random — different instances
//     will compute different first-pick orders for the same session, but
//     each instance still picks deterministically per session and Redis
//     sticky writes (keyed by routes/policy, not salt) converge across
//     instances after the first success. Single-instance and dev
//     deployments are fully covered.
//
// Lazy resolution lets tests override via t.Setenv before the first
// Match() call.
var (
	routingSeedSalt     []byte
	routingSeedSaltOnce sync.Once
)

func getRoutingSeedSalt() []byte {
	routingSeedSaltOnce.Do(func() {
		if v := os.Getenv("MAXX_ROUTING_SEED_SALT"); v != "" {
			routingSeedSalt = []byte(v)
			return
		}
		buf := make([]byte, 32)
		if _, err := crand.Read(buf); err != nil {
			// crypto/rand failures are nearly impossible (an unprivileged
			// process is denied /dev/urandom and getrandom etc.). Build
			// a time-derived fallback that still fills all 32 bytes so
			// HMAC entropy doesn't collapse to 64 bits.
			now := uint64(time.Now().UnixNano())
			for i := 0; i < len(buf); i += 8 {
				binary.LittleEndian.PutUint64(buf[i:i+8], now)
				now = now*6364136223846793005 + 1442695040888963407 // splitmix-style step
			}
		}
		routingSeedSalt = buf
		log.Printf("[Router] MAXX_ROUTING_SEED_SALT not set — generated a per-process random salt. " +
			"For consistent first-pick behavior across multi-instance deployments, set MAXX_ROUTING_SEED_SALT to a shared secret.")
	})
	return routingSeedSalt
}

// makeSessionSeed derives a stable seed from the caller identity + the
// MatchContext so the weighted shuffle becomes deterministic per session
// (implicit affinity) but unpredictable to clients who don't know the salt.
//
// When no session anchor is available (no api token, no session id) we
// still want determinism per tenant/client/project so distribution doesn't
// degrade to global rand — incorporate the routing context as the anchor.
//
// Encoding is unambiguous by construction: every variable-length field is
// length-prefixed (uint64 LE), every fixed-length integer is uint64 LE.
// No clever separators, no field can collide with another's payload.
func makeSessionSeed(ctx *MatchContext) int64 {
	mac := hmac.New(sha256.New, getRoutingSeedSalt())
	var u64 [8]byte
	writeU64 := func(v uint64) {
		binary.LittleEndian.PutUint64(u64[:], v)
		mac.Write(u64[:])
	}
	writeBytes := func(b []byte) {
		writeU64(uint64(len(b)))
		mac.Write(b)
	}
	writeU64(ctx.TenantID)
	writeBytes([]byte(ctx.ClientType))
	writeU64(ctx.ProjectID)
	writeU64(ctx.APITokenID)
	writeBytes([]byte(ctx.SessionID))
	sum := mac.Sum(nil)
	return int64(binary.LittleEndian.Uint64(sum[:8]))
}

// weightedShuffle 按权重做加权随机排序
// 使用加权采样算法：每次从剩余路由中按权重概率选一个放到当前位置
func weightedShuffle(routes []*domain.Route, rng *rand.Rand) {
	n := len(routes)
	for i := 0; i < n-1; i++ {
		// 计算剩余路由的权重总和
		totalWeight := 0
		for j := i; j < n; j++ {
			w := routes[j].Weight
			if w <= 0 {
				w = 1
			}
			totalWeight += w
		}

		// 按权重随机选择一个
		pick := rng.Intn(totalWeight)
		cumulative := 0
		for j := i; j < n; j++ {
			w := routes[j].Weight
			if w <= 0 {
				w = 1
			}
			cumulative += w
			if pick < cumulative {
				routes[i], routes[j] = routes[j], routes[i]
				break
			}
		}
	}
}

// GetCooldowns returns all active cooldowns
func (r *Router) GetCooldowns() ([]*domain.Cooldown, error) {
	return r.cooldownManager.GetAllCooldownsFromDB()
}

// ClearCooldown clears cooldown for a specific provider
// Clears all cooldowns (global + per-client-type) for the provider
func (r *Router) ClearCooldown(providerID uint64) error {
	r.cooldownManager.ClearCooldown(providerID, "", "")
	provider.ClearResponsesWebSocketTransportCooldown(providerID)
	return nil
}

// injectProviderUpdate injects a provider-update callback into adapters that support it.
// Uses duck-typing: if the adapter has SetProviderUpdateFunc, inject repo.Update.
func (r *Router) injectProviderUpdate(a provider.ProviderAdapter, p *domain.Provider) {
	type providerUpdater interface {
		SetProviderUpdateFunc(fn func(*domain.Provider) error)
	}
	if u, ok := a.(providerUpdater); ok {
		repo := r.providerRepo
		u.SetProviderUpdateFunc(func(p *domain.Provider) error {
			return repo.Update(p)
		})
	}

	// providerReload lets the adapter re-read the freshest provider record (to
	// pick up a token another path rotated and persisted) while holding its
	// refresh lock.
	type providerReloader interface {
		SetProviderReloadFunc(fn func() (*domain.Provider, error))
	}
	if u, ok := a.(providerReloader); ok && p != nil {
		repo := r.providerRepo
		tenantID, id := p.TenantID, p.ID
		u.SetProviderReloadFunc(func() (*domain.Provider, error) {
			return repo.GetByID(tenantID, id)
		})
	}
}

func noAvailableProvidersError(rejects map[string]int) error {
	if len(rejects) == 0 {
		return domain.ErrNoAvailableProviders
	}
	keys := make([]string, 0, len(rejects))
	for key := range rejects {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, rejects[key]))
	}
	return fmt.Errorf("%w (rejections: %s)", domain.ErrNoAvailableProviders, strings.Join(parts, ", "))
}
