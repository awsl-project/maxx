package router

import (
	"context"
	"crypto/hmac"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"log"
	"math/rand"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/cooldown"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository/cached"
	"github.com/awsl-project/maxx/internal/sticky"
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
	Ctx          context.Context
	TenantID     uint64
	ClientType   domain.ClientType
	ProjectID    uint64
	RequestModel string
	APITokenID   uint64
	SessionID    string
}

// Router handles route matching and selection
type Router struct {
	routeRepo           *cached.RouteRepository
	providerRepo        *cached.ProviderRepository
	routingStrategyRepo *cached.RoutingStrategyRepository
	retryConfigRepo     *cached.RetryConfigRepository
	projectRepo         *cached.ProjectRepository

	// Adapter cache
	adapters map[uint64]provider.ProviderAdapter
	mu       sync.RWMutex

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
) *Router {
	return &Router{
		routeRepo:           routeRepo,
		providerRepo:        providerRepo,
		routingStrategyRepo: routingStrategyRepo,
		retryConfigRepo:     retryConfigRepo,
		projectRepo:         projectRepo,
		adapters:            make(map[uint64]provider.ProviderAdapter),
		cooldownManager:     cooldown.Default(),
	}
}

// InitAdapters initializes adapters for all providers
func (r *Router) InitAdapters() error {
	providers := r.providerRepo.GetAll()
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, p := range providers {
		factory, ok := provider.GetAdapterFactory(p.Type)
		if !ok {
			continue // Skip providers without registered adapters
		}
		a, err := factory(p)
		if err != nil {
			return err
		}
		r.injectProviderUpdate(a)
		r.adapters[p.ID] = a
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
	r.injectProviderUpdate(a)
	r.mu.Lock()
	r.adapters[p.ID] = a
	r.mu.Unlock()
	return nil
}

// RemoveAdapter removes the adapter for a provider
func (r *Router) RemoveAdapter(providerID uint64) {
	r.mu.Lock()
	delete(r.adapters, providerID)
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

	for _, route := range filtered {
		prov, ok := providers[route.ProviderID]
		if !ok {
			continue
		}

		// Skip providers in cooldown (checks provider, key, and model-level cooldowns)
		if r.cooldownManager.IsInCooldown(route.ProviderID, string(clientType), requestModel) {
			continue
		}

		adp, ok := r.adapters[route.ProviderID]
		if !ok {
			continue
		}

		// Check if provider supports the request model
		// SupportModels check is done BEFORE mapping
		// If SupportModels is configured, check if the request model is supported
		if len(prov.SupportModels) > 0 && requestModel != "" {
			if !r.isModelSupported(requestModel, prov.SupportModels) {
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
		return nil, domain.ErrNoRoutes
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

// isModelSupported checks if a model matches any pattern in the support list
func (r *Router) isModelSupported(model string, supportModels []string) bool {
	for _, pattern := range supportModels {
		if domain.MatchWildcard(pattern, model) {
			return true
		}
	}
	return false
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
	return nil
}

// injectProviderUpdate injects a provider-update callback into adapters that support it.
// Uses duck-typing: if the adapter has SetProviderUpdateFunc, inject repo.Update.
func (r *Router) injectProviderUpdate(a provider.ProviderAdapter) {
	type providerUpdater interface {
		SetProviderUpdateFunc(fn func(*domain.Provider) error)
	}
	if u, ok := a.(providerUpdater); ok {
		repo := r.providerRepo
		u.SetProviderUpdateFunc(func(p *domain.Provider) error {
			return repo.Update(p)
		})
	}
}

