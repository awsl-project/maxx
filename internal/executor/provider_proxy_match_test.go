package executor

import (
	"context"
	"testing"

	provideradapter "github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/repository/cached"
	"github.com/awsl-project/maxx/internal/router"
)

const providerProxyMatchTestType = "provider-proxy-route-match-test"

func init() {
	provideradapter.RegisterAdapterFactory(providerProxyMatchTestType, func(*domain.Provider) (provideradapter.ProviderAdapter, error) {
		return providerProxyMatchTestAdapter{}, nil
	})
}

type providerProxyMatchTestAdapter struct{}

func (providerProxyMatchTestAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeOpenAI}
}

func (providerProxyMatchTestAdapter) Execute(*flow.Ctx, *domain.Provider) error { return nil }

type providerProxyMatchRouteRepo struct{ routes []*domain.Route }

func (r *providerProxyMatchRouteRepo) Create(route *domain.Route) error {
	r.routes = append(r.routes, route)
	return nil
}
func (r *providerProxyMatchRouteRepo) Update(route *domain.Route) error { return nil }
func (r *providerProxyMatchRouteRepo) Delete(uint64, uint64) error      { return nil }
func (r *providerProxyMatchRouteRepo) BulkDelete(uint64, domain.RouteBulkDeleteRequest) (*domain.RouteBulkDeleteResult, error) {
	return &domain.RouteBulkDeleteResult{}, nil
}
func (r *providerProxyMatchRouteRepo) GetByID(tenantID uint64, id uint64) (*domain.Route, error) {
	for _, route := range r.routes {
		if route.TenantID == tenantID && route.ID == id {
			return route, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *providerProxyMatchRouteRepo) FindByKey(tenantID uint64, projectID, providerID uint64, clientType domain.ClientType) (*domain.Route, error) {
	for _, route := range r.routes {
		if route.TenantID == tenantID && route.ProjectID == projectID && route.ProviderID == providerID && route.ClientType == clientType {
			return route, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *providerProxyMatchRouteRepo) List(tenantID uint64) ([]*domain.Route, error) {
	var out []*domain.Route
	for _, route := range r.routes {
		if tenantID == domain.TenantIDAll || route.TenantID == tenantID {
			out = append(out, route)
		}
	}
	return out, nil
}
func (r *providerProxyMatchRouteRepo) BatchUpdatePositions(uint64, []domain.RoutePositionUpdate) error {
	return nil
}

type providerProxyMatchProviderRepo struct{ providers []*domain.Provider }

func (r *providerProxyMatchProviderRepo) Create(provider *domain.Provider) error {
	r.providers = append(r.providers, provider)
	return nil
}
func (r *providerProxyMatchProviderRepo) Update(provider *domain.Provider) error { return nil }
func (r *providerProxyMatchProviderRepo) Delete(uint64, uint64) error            { return nil }
func (r *providerProxyMatchProviderRepo) GetByID(tenantID uint64, id uint64) (*domain.Provider, error) {
	for _, provider := range r.providers {
		if provider.TenantID == tenantID && provider.ID == id {
			return provider, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *providerProxyMatchProviderRepo) List(tenantID uint64) ([]*domain.Provider, error) {
	var out []*domain.Provider
	for _, provider := range r.providers {
		if tenantID == domain.TenantIDAll || provider.TenantID == tenantID {
			out = append(out, provider)
		}
	}
	return out, nil
}

type providerProxyMatchRetryRepo struct{ configs []*domain.RetryConfig }

func (r *providerProxyMatchRetryRepo) Create(config *domain.RetryConfig) error {
	r.configs = append(r.configs, config)
	return nil
}
func (r *providerProxyMatchRetryRepo) Update(config *domain.RetryConfig) error { return nil }
func (r *providerProxyMatchRetryRepo) Delete(uint64, uint64) error             { return nil }
func (r *providerProxyMatchRetryRepo) GetByID(tenantID uint64, id uint64) (*domain.RetryConfig, error) {
	for _, config := range r.configs {
		if config.TenantID == tenantID && config.ID == id {
			return config, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *providerProxyMatchRetryRepo) GetDefault(tenantID uint64) (*domain.RetryConfig, error) {
	for _, config := range r.configs {
		if config.TenantID == tenantID && config.IsDefault {
			return config, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *providerProxyMatchRetryRepo) List(tenantID uint64) ([]*domain.RetryConfig, error) {
	var out []*domain.RetryConfig
	for _, config := range r.configs {
		if tenantID == domain.TenantIDAll || config.TenantID == tenantID {
			out = append(out, config)
		}
	}
	return out, nil
}

type providerProxyMatchStrategyRepo struct{}

func (providerProxyMatchStrategyRepo) Create(*domain.RoutingStrategy) error { return nil }
func (providerProxyMatchStrategyRepo) Update(*domain.RoutingStrategy) error { return nil }
func (providerProxyMatchStrategyRepo) Delete(uint64, uint64) error          { return nil }
func (providerProxyMatchStrategyRepo) GetByID(uint64, uint64) (*domain.RoutingStrategy, error) {
	return nil, domain.ErrNotFound
}
func (providerProxyMatchStrategyRepo) GetByProjectID(uint64, uint64) (*domain.RoutingStrategy, error) {
	return nil, domain.ErrNotFound
}
func (providerProxyMatchStrategyRepo) List(uint64) ([]*domain.RoutingStrategy, error) {
	return nil, nil
}

type providerProxyMatchProjectRepo struct{}

func (providerProxyMatchProjectRepo) Create(*domain.Project) error { return nil }
func (providerProxyMatchProjectRepo) Update(*domain.Project) error { return nil }
func (providerProxyMatchProjectRepo) Delete(uint64, uint64) error  { return nil }
func (providerProxyMatchProjectRepo) GetByID(tenantID uint64, id uint64) (*domain.Project, error) {
	if tenantID == 1 && id == 123 {
		return &domain.Project{ID: 123, TenantID: 1, EnabledCustomRoutes: []domain.ClientType{domain.ClientTypeOpenAI}}, nil
	}
	return nil, domain.ErrNotFound
}
func (providerProxyMatchProjectRepo) GetBySlug(uint64, string) (*domain.Project, error) {
	return nil, domain.ErrNotFound
}
func (providerProxyMatchProjectRepo) List(uint64) ([]*domain.Project, error) { return nil, nil }

func TestMatchProviderProxyRouteUsesProjectRouteRetryPolicy(t *testing.T) {
	routeRepo := cached.NewRouteRepository(&providerProxyMatchRouteRepo{routes: []*domain.Route{
		{ID: 10, TenantID: 1, ProjectID: 0, ProviderID: 79007, ClientType: domain.ClientTypeOpenAI, IsEnabled: true},
		{ID: 11, TenantID: 1, ProjectID: 123, ProviderID: 79007, ClientType: domain.ClientTypeOpenAI, RetryConfigID: 99, IsEnabled: true},
	}})
	providerRepo := cached.NewProviderRepository(&providerProxyMatchProviderRepo{providers: []*domain.Provider{
		{ID: 79007, TenantID: 1, Type: providerProxyMatchTestType, Name: "provider", SupportedClientTypes: []domain.ClientType{domain.ClientTypeOpenAI}},
	}})
	retryRepo := cached.NewRetryConfigRepository(&providerProxyMatchRetryRepo{configs: []*domain.RetryConfig{
		{ID: 1, TenantID: 1, IsDefault: true, MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
		{ID: 99, TenantID: 1, MaxRetries: 2, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
	}})
	strategyRepo := cached.NewRoutingStrategyRepository(providerProxyMatchStrategyRepo{})
	projectRepo := cached.NewProjectRepository(providerProxyMatchProjectRepo{})

	for name, load := range map[string]func() error{
		"routes":     routeRepo.Load,
		"providers":  providerRepo.Load,
		"retry":      retryRepo.Load,
		"strategies": strategyRepo.Load,
		"projects":   projectRepo.Load,
	} {
		if err := load(); err != nil {
			t.Fatalf("load %s: %v", name, err)
		}
	}
	r := router.NewRouter(routeRepo, providerRepo, strategyRepo, retryRepo, projectRepo)
	if err := r.InitAdapters(); err != nil {
		t.Fatalf("init adapters: %v", err)
	}
	e := &Executor{router: r}

	matched, err := e.MatchProviderProxyRoute(context.Background(), 1, 79007, domain.ClientTypeOpenAI, 123, "minimaxai/minimax-m3", 46, "session-1")
	if err != nil {
		t.Fatalf("MatchProviderProxyRoute returned error: %v", err)
	}
	if matched.Route.ID != 11 {
		t.Fatalf("matched route ID = %d, want project route 11", matched.Route.ID)
	}
	if matched.RetryConfig == nil || matched.RetryConfig.MaxRetries != 2 {
		t.Fatalf("matched retry config = %+v, want project MaxRetries=2", matched.RetryConfig)
	}
}
