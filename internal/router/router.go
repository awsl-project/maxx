package router

import (
	"math/rand"
	"sort"
	"sync"

	"github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/cooldown"
	"github.com/awsl-project/maxx/internal/domain"
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
	clientType := ctx.ClientType
	projectID := ctx.ProjectID
	requestModel := ctx.RequestModel

	routes := r.routeRepo.GetAll()

	// Check if ClientType has custom routes enabled for this project
	useProjectRoutes := false
	if projectID != 0 {
		project, err := r.projectRepo.GetByID(projectID)
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
	strategy := r.getRoutingStrategy(projectID)

	// Sort routes by strategy
	r.sortRoutes(filtered, strategy)

	// Get default retry config
	defaultRetry, _ := r.retryConfigRepo.GetDefault()

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
			retryConfig, _ = r.retryConfigRepo.GetByID(route.RetryConfigID)
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

func (r *Router) getRoutingStrategy(projectID uint64) *domain.RoutingStrategy {
	// Try project-specific strategy first
	if projectID != 0 {
		if s, err := r.routingStrategyRepo.GetByProjectID(projectID); err == nil {
			return s
		}
	}
	// Fall back to global strategy
	if s, err := r.routingStrategyRepo.GetByProjectID(0); err == nil {
		return s
	}
	// Default to priority
	return &domain.RoutingStrategy{Type: domain.RoutingStrategyPriority}
}

func (r *Router) sortRoutes(routes []*domain.Route, strategy *domain.RoutingStrategy) {
	switch strategy.Type {
	case domain.RoutingStrategyWeightedRandom:
		// Shuffle with weights (simplified - just shuffle for now)
		rand.Shuffle(len(routes), func(i, j int) {
			routes[i], routes[j] = routes[j], routes[i]
		})
	default: // priority
		sort.Slice(routes, func(i, j int) bool {
			return routes[i].Position < routes[j].Position
		})
	}
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

