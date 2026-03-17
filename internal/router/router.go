package router

import (
	"math"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/cooldown"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/health"
	"github.com/awsl-project/maxx/internal/repository/cached"
)

// MatchedRoute contains all data needed to execute a proxy request
type MatchedRoute struct {
	Route           *domain.Route
	Provider        *domain.Provider
	ProviderAdapter provider.ProviderAdapter
	RetryConfig     *domain.RetryConfig
}

// MatchContext contains all context needed for route matching
type MatchContext struct {
	TenantID     uint64
	ClientType   domain.ClientType
	ProjectID    uint64
	RequestModel string
	APITokenID   uint64
}

// Router handles route matching and selection
type Router struct {
	routeRepo           *cached.RouteRepository
	providerRepo        *cached.ProviderRepository
	routingStrategyRepo *cached.RoutingStrategyRepository
	retryConfigRepo     *cached.RetryConfigRepository
	projectRepo         *cached.ProjectRepository

	// Adapter cache
	adapters  map[uint64]provider.ProviderAdapter
	mu        sync.RWMutex
	shuffle   *rand.Rand
	shuffleMu sync.Mutex

	// Cooldown manager
	cooldownManager *cooldown.Manager
	healthTracker   health.ProviderTracker
}

// NewRouter creates a new router
func NewRouter(
	routeRepo *cached.RouteRepository,
	providerRepo *cached.ProviderRepository,
	routingStrategyRepo *cached.RoutingStrategyRepository,
	retryConfigRepo *cached.RetryConfigRepository,
	projectRepo *cached.ProjectRepository,
	healthTracker health.ProviderTracker,
) *Router {
	return &Router{
		routeRepo:           routeRepo,
		providerRepo:        providerRepo,
		routingStrategyRepo: routingStrategyRepo,
		retryConfigRepo:     retryConfigRepo,
		projectRepo:         projectRepo,
		adapters:            make(map[uint64]provider.ProviderAdapter),
		cooldownManager:     cooldown.Default(),
		healthTracker:       healthTracker,
		shuffle:             rand.New(rand.NewSource(time.Now().UnixNano())),
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

// Match returns matched routes for a client type and project
func (r *Router) Match(ctx *MatchContext) ([]*MatchedRoute, error) {
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

	// priority 路由先排顺序；weighted_random 在构建 matched 后再与健康分一起做联合排序，
	// 避免被后续 health reorder 退化成确定性排序。
	if strategy.Type != domain.RoutingStrategyWeightedRandom || r.healthTracker == nil {
		r.sortRoutes(filtered, strategy)
	}

	// Get default retry config
	defaultRetry, _ := r.retryConfigRepo.GetDefault(tenantID)

	// Build matched routes
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matched []*MatchedRoute
	providers := r.providerRepo.GetAll()

	for _, route := range filtered {
		prov, ok := providers[route.ProviderID]
		if !ok {
			continue
		}

		// Skip providers in cooldown
		if r.cooldownManager.IsInCooldown(route.ProviderID, string(clientType)) {
			continue
		}
		if r.healthTracker != nil && r.healthTracker.IsCircuitOpen(route.ProviderID, string(clientType)) {
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

	if len(matched) == 0 {
		return nil, domain.ErrNoRoutes
	}

	if r.healthTracker != nil {
		scores := make(map[uint64]float64, len(matched))
		for _, matchedRoute := range matched {
			scores[matchedRoute.Provider.ID] = r.healthTracker.Score(matchedRoute.Provider.ID, string(clientType))
		}
		if strategy.Type == domain.RoutingStrategyWeightedRandom {
			r.sortMatchedWeightedRandomWithHealth(matched, scores)
		} else {
			sort.SliceStable(matched, func(i, j int) bool {
				left := scores[matched[i].Provider.ID]
				right := scores[matched[j].Provider.ID]
				if left == right {
					return false
				}
				return left > right
			})
		}
	}

	return matched, nil
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

func (r *Router) sortRoutes(routes []*domain.Route, strategy *domain.RoutingStrategy) {
	switch strategy.Type {
	case domain.RoutingStrategyWeightedRandom:
		r.sortRoutesWeightedRandom(routes)
	default: // priority
		sort.Slice(routes, func(i, j int) bool {
			return routes[i].Position < routes[j].Position
		})
	}
}

func (r *Router) sortRoutesWeightedRandom(routes []*domain.Route) {
	if len(routes) < 2 {
		return
	}
	if r.shuffle == nil {
		r.shuffle = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	type weightedRoute struct {
		route *domain.Route
		key   float64
	}

	weighted := make([]weightedRoute, 0, len(routes))
	r.shuffleMu.Lock()
	for _, route := range routes {
		weight := route.Weight
		if weight <= 0 {
			weight = 1
		}
		u := r.shuffle.Float64()
		if u <= 0 {
			u = math.SmallestNonzeroFloat64
		}
		weighted = append(weighted, weightedRoute{
			route: route,
			key:   math.Pow(u, 1/float64(weight)),
		})
	}
	r.shuffleMu.Unlock()

	sort.SliceStable(weighted, func(i, j int) bool {
		return weighted[i].key > weighted[j].key
	})
	for i := range routes {
		routes[i] = weighted[i].route
	}
}

func (r *Router) sortMatchedWeightedRandomWithHealth(matched []*MatchedRoute, scores map[uint64]float64) {
	if len(matched) < 2 {
		return
	}
	if r.shuffle == nil {
		r.shuffle = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	type weightedMatch struct {
		route *MatchedRoute
		key   float64
	}

	weighted := make([]weightedMatch, 0, len(matched))
	r.shuffleMu.Lock()
	for _, matchedRoute := range matched {
		weight := matchedRoute.Route.Weight
		if weight <= 0 {
			weight = 1
		}
		effectiveWeight := float64(weight) * healthWeightMultiplier(scores[matchedRoute.Provider.ID])
		if effectiveWeight <= 0 {
			effectiveWeight = math.SmallestNonzeroFloat64
		}
		u := r.shuffle.Float64()
		if u <= 0 {
			u = math.SmallestNonzeroFloat64
		}
		weighted = append(weighted, weightedMatch{
			route: matchedRoute,
			key:   math.Pow(u, 1/effectiveWeight),
		})
	}
	r.shuffleMu.Unlock()

	sort.SliceStable(weighted, func(i, j int) bool {
		return weighted[i].key > weighted[j].key
	})
	for i := range matched {
		matched[i] = weighted[i].route
	}
}

func healthWeightMultiplier(score float64) float64 {
	const (
		scale    = 300.0
		maxScore = 1500.0
		minScore = -1500.0
	)

	switch {
	case score > maxScore:
		score = maxScore
	case score < minScore:
		score = minScore
	}
	return math.Exp(score / scale)
}

// GetCooldowns returns all active cooldowns
func (r *Router) GetCooldowns() ([]*domain.Cooldown, error) {
	return r.cooldownManager.GetAllCooldownsFromDB()
}

// ClearCooldown clears cooldown for a specific provider
// Clears all cooldowns (global + per-client-type) for the provider
func (r *Router) ClearCooldown(providerID uint64) error {
	r.cooldownManager.ClearCooldown(providerID, "")
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
