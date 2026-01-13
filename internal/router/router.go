package router

import (
	"log"
	"math/rand"
	"sort"
	"sync"

	"github.com/Bowl42/maxx-next/internal/adapter/provider"
	"github.com/Bowl42/maxx-next/internal/cooldown"
	"github.com/Bowl42/maxx-next/internal/domain"
	"github.com/Bowl42/maxx-next/internal/repository/cached"
)

// MatchedRoute contains all data needed to execute a proxy request
type MatchedRoute struct {
	Route           *domain.Route
	Provider        *domain.Provider
	ProviderAdapter provider.ProviderAdapter
	RetryConfig     *domain.RetryConfig
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
			log.Printf("[Router] InitAdapters: factory error for provider %d: %v", p.ID, err)
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
func (r *Router) Match(clientType domain.ClientType, projectID uint64) ([]*MatchedRoute, error) {
	routes := r.routeRepo.GetAll()

	log.Printf("[Router] Match called: clientType=%s, projectID=%d, total routes in cache=%d", clientType, projectID, len(routes))

	// Debug: print all routes in cache
	for _, rt := range routes {
		log.Printf("[Router] Route in cache: id=%d, clientType=%s, projectID=%d, providerID=%d, isEnabled=%v",
			rt.ID, rt.ClientType, rt.ProjectID, rt.ProviderID, rt.IsEnabled)
	}

	// Check if ClientType has custom routes enabled for this project
	useProjectRoutes := false
	if projectID != 0 {
		project, err := r.projectRepo.GetByID(projectID)
		if err != nil {
			log.Printf("[Router] Failed to get project %d: %v", projectID, err)
		} else if project == nil {
			log.Printf("[Router] Project %d not found in cache", projectID)
		} else {
			log.Printf("[Router] Project %d found: name=%s, EnabledCustomRoutes=%v", project.ID, project.Name, project.EnabledCustomRoutes)
			// If EnabledCustomRoutes is empty, all ClientTypes use global routes
			// If EnabledCustomRoutes is not empty, only listed ClientTypes can have custom routes
			if len(project.EnabledCustomRoutes) == 0 {
				useProjectRoutes = false
				log.Printf("[Router] Project %d has empty EnabledCustomRoutes, using global routes", projectID)
			} else {
				for _, ct := range project.EnabledCustomRoutes {
					if ct == clientType {
						useProjectRoutes = true
						break
					}
				}
			}
			if !useProjectRoutes && len(project.EnabledCustomRoutes) > 0 {
				log.Printf("[Router] ClientType %s not in EnabledCustomRoutes %v for project %d, falling back to global routes", clientType, project.EnabledCustomRoutes, projectID)
			}
		}
	} else {
		log.Printf("[Router] projectID is 0, using global routes")
	}

	// Filter routes
	var filtered []*domain.Route
	var hasProjectRoutes bool

	log.Printf("[Router] useProjectRoutes=%v for clientType=%s, projectID=%d", useProjectRoutes, clientType, projectID)

	// Only look for project-specific routes if ClientType is in EnabledCustomRoutes
	if useProjectRoutes {
		log.Printf("[Router] Looking for project-specific routes for projectID=%d, clientType=%s", projectID, clientType)
		for _, route := range routes {
			if !route.IsEnabled {
				log.Printf("[Router] Skipping disabled route id=%d", route.ID)
				continue
			}
			if route.ClientType != clientType {
				continue
			}
			if route.ProjectID == projectID && projectID != 0 {
				log.Printf("[Router] Found matching project route: id=%d, providerID=%d", route.ID, route.ProviderID)
				filtered = append(filtered, route)
				hasProjectRoutes = true
			} else {
				log.Printf("[Router] Route id=%d has projectID=%d, not matching requested projectID=%d", route.ID, route.ProjectID, projectID)
			}
		}
	}

	// If no project-specific routes or ClientType not enabled for custom routes, use global routes
	if !hasProjectRoutes {
		log.Printf("[Router] No project routes found, falling back to global routes (projectID=0)")
		for _, route := range routes {
			if !route.IsEnabled {
				continue
			}
			if route.ClientType != clientType {
				continue
			}
			if route.ProjectID == 0 {
				log.Printf("[Router] Found global route: id=%d, providerID=%d", route.ID, route.ProviderID)
				filtered = append(filtered, route)
			}
		}
	}

	log.Printf("[Router] Filtered routes count: %d, hasProjectRoutes=%v", len(filtered), hasProjectRoutes)

	if len(filtered) == 0 {
		return nil, domain.ErrNoRoutes
	}

	// Get routing strategy
	strategy := r.getRoutingStrategy(projectID)

	// Sort routes by strategy
	r.sortRoutes(filtered, strategy)

	// Get default retry config
	defaultRetry, err := r.retryConfigRepo.GetDefault()
	if err != nil {
		log.Printf("[Router] Failed to get default retry config: %v", err)
	} else if defaultRetry != nil {
		log.Printf("[Router] Default retry config: ID=%d, MaxRetries=%d", defaultRetry.ID, defaultRetry.MaxRetries)
	} else {
		log.Printf("[Router] No default retry config found")
	}

	// Build matched routes
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matched []*MatchedRoute
	providers := r.providerRepo.GetAll()

	log.Printf("[Router] Providers in cache: %d, Adapters: %d", len(providers), len(r.adapters))

	for _, route := range filtered {
		provider, ok := providers[route.ProviderID]
		if !ok {
			log.Printf("[Router] Provider not found for route %d (providerID=%d)", route.ID, route.ProviderID)
			continue
		}

		// Skip providers in cooldown
		if r.cooldownManager.IsInCooldown(route.ProviderID, string(clientType)) {
			until := r.cooldownManager.GetCooldownUntil(route.ProviderID, string(clientType))
			log.Printf("[Router] Provider %d (%s) is in cooldown for clientType=%s until %s, skipping",
				route.ProviderID, provider.Name, clientType, until.Format("15:04:05"))
			continue
		}

		adp, ok := r.adapters[route.ProviderID]
		if !ok {
			log.Printf("[Router] Adapter not found for provider %d", route.ProviderID)
			continue
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
			Provider:        provider,
			ProviderAdapter: adp,
			RetryConfig:     retryConfig,
		})
	}

	log.Printf("[Router] Final matched routes: %d", len(matched))

	if len(matched) == 0 {
		return nil, domain.ErrNoRoutes
	}

	return matched, nil
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

