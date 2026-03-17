package router

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	provideradapter "github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/cooldown"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/health"
	"github.com/awsl-project/maxx/internal/repository/cached"
)

func TestMatchOrdersRoutesByHealthScoreBeforePosition(t *testing.T) {
	t.Parallel()

	tracker := &routerFakeHealthTracker{
		scores: map[string]float64{
			"1:claude": 10,
			"2:claude": 900,
		},
	}
	router := newRouterForTest(t, tracker, []*domain.Provider{
		newProvider(1),
		newProvider(2),
	}, []*domain.Route{
		newRoute(101, 1, 1),
		newRoute(102, 2, 2),
	})

	matched, err := router.Match(&MatchContext{
		TenantID:     1,
		ClientType:   domain.ClientTypeClaude,
		RequestModel: "claude-sonnet-4",
	})
	if err != nil {
		t.Fatalf("Match returned error: %v", err)
	}
	if len(matched) != 2 {
		t.Fatalf("matched routes = %d, want 2", len(matched))
	}
	if matched[0].Provider.ID != 2 || matched[1].Provider.ID != 1 {
		t.Fatalf("provider order = [%d %d], want [2 1]", matched[0].Provider.ID, matched[1].Provider.ID)
	}
}

func TestMatchSkipsProvidersWithOpenCircuit(t *testing.T) {
	t.Parallel()

	tracker := &routerFakeHealthTracker{
		opens: map[string]bool{
			"1:claude": true,
		},
		scores: map[string]float64{
			"1:claude": 900,
			"2:claude": 100,
		},
	}
	router := newRouterForTest(t, tracker, []*domain.Provider{
		newProvider(1),
		newProvider(2),
	}, []*domain.Route{
		newRoute(101, 1, 1),
		newRoute(102, 2, 2),
	})

	matched, err := router.Match(&MatchContext{
		TenantID:     1,
		ClientType:   domain.ClientTypeClaude,
		RequestModel: "claude-sonnet-4",
	})
	if err != nil {
		t.Fatalf("Match returned error: %v", err)
	}
	if len(matched) != 1 {
		t.Fatalf("matched routes = %d, want 1 after open-circuit filtering", len(matched))
	}
	if matched[0].Provider.ID != 2 {
		t.Fatalf("remaining provider ID = %d, want 2", matched[0].Provider.ID)
	}
}

func TestMatchKeepsPositionOrderWhenHealthScoresTie(t *testing.T) {
	t.Parallel()

	tracker := &routerFakeHealthTracker{
		scores: map[string]float64{
			"1:claude": 100,
			"2:claude": 100,
		},
	}
	router := newRouterForTest(t, tracker, []*domain.Provider{
		newProvider(1),
		newProvider(2),
	}, []*domain.Route{
		newRoute(101, 1, 1),
		newRoute(102, 2, 2),
	})

	matched, err := router.Match(&MatchContext{
		TenantID:     1,
		ClientType:   domain.ClientTypeClaude,
		RequestModel: "claude-sonnet-4",
	})
	if err != nil {
		t.Fatalf("Match returned error: %v", err)
	}
	if len(matched) != 2 {
		t.Fatalf("matched routes = %d, want 2", len(matched))
	}
	if matched[0].Route.Position != 1 || matched[1].Route.Position != 2 {
		t.Fatalf("route positions = [%d %d], want original priority order [1 2]", matched[0].Route.Position, matched[1].Route.Position)
	}
}

func TestMatchPreservesWeightedRandomBaselineAmongEqualHealthScores(t *testing.T) {
	baselineTracker := &routerFakeHealthTracker{
		scores: map[string]float64{
			"1:claude": 100,
			"2:claude": 100,
			"3:claude": 100,
		},
	}
	baselineRouter := newRouterForTestWithStrategy(t, baselineTracker, []*domain.Provider{
		newProvider(1),
		newProvider(2),
		newProvider(3),
	}, []*domain.Route{
		newRoute(101, 1, 1),
		newRoute(102, 2, 2),
		newRoute(103, 3, 3),
	}, domain.RoutingStrategyWeightedRandom)
	baselineRouter.shuffle = rand.New(rand.NewSource(7))

	healthTracker := &routerFakeHealthTracker{
		scores: map[string]float64{
			"1:claude": 500,
			"2:claude": 100,
			"3:claude": 100,
		},
	}
	healthRouter := newRouterForTestWithStrategy(t, healthTracker, []*domain.Provider{
		newProvider(1),
		newProvider(2),
		newProvider(3),
	}, []*domain.Route{
		newRoute(101, 1, 1),
		newRoute(102, 2, 2),
		newRoute(103, 3, 3),
	}, domain.RoutingStrategyWeightedRandom)
	healthRouter.shuffle = rand.New(rand.NewSource(7))

	baselineMatched, err := baselineRouter.Match(&MatchContext{
		TenantID:     1,
		ClientType:   domain.ClientTypeClaude,
		RequestModel: "claude-sonnet-4",
	})
	if err != nil {
		t.Fatalf("baseline Match returned error: %v", err)
	}
	tiedBaseline := tiedProviderOrder(baselineMatched, 2, 3)
	if len(tiedBaseline) != 2 {
		t.Fatalf("baseline tied order = %v, want two tied providers", tiedBaseline)
	}

	matched, err := healthRouter.Match(&MatchContext{
		TenantID:     1,
		ClientType:   domain.ClientTypeClaude,
		RequestModel: "claude-sonnet-4",
	})
	if err != nil {
		t.Fatalf("health Match returned error: %v", err)
	}
	if len(matched) != 3 {
		t.Fatalf("health matched routes = %d, want 3", len(matched))
	}
	if matched[0].Provider.ID != 1 {
		t.Fatalf("top provider ID = %d, want highest-health provider 1", matched[0].Provider.ID)
	}

	tiedAfterHealth := tiedProviderOrder(matched, 2, 3)
	if len(tiedAfterHealth) != 2 {
		t.Fatalf("health tied order = %v, want two tied providers", tiedAfterHealth)
	}
	if tiedAfterHealth[0] != tiedBaseline[0] || tiedAfterHealth[1] != tiedBaseline[1] {
		t.Fatalf("tied provider order after health reorder = %v, baseline = %v, want stable preservation of weighted_random order", tiedAfterHealth, tiedBaseline)
	}
}

func TestMatchWeightedRandomPrefersHigherWeightRoutes(t *testing.T) {
	router := newRouterForTestWithStrategy(t, nil, []*domain.Provider{
		newProvider(1),
		newProvider(2),
		newProvider(3),
	}, []*domain.Route{
		newWeightedRoute(101, 1, 1, 12),
		newWeightedRoute(102, 2, 2, 1),
		newWeightedRoute(103, 3, 3, 1),
	}, domain.RoutingStrategyWeightedRandom)
	router.shuffle = rand.New(rand.NewSource(11))

	firstCounts := map[uint64]int{}
	for range 1200 {
		matched, err := router.Match(&MatchContext{
			TenantID:     1,
			ClientType:   domain.ClientTypeClaude,
			RequestModel: "claude-sonnet-4",
		})
		if err != nil {
			t.Fatalf("Match returned error: %v", err)
		}
		firstCounts[matched[0].Provider.ID]++
	}

	if firstCounts[1] <= firstCounts[2] || firstCounts[1] <= firstCounts[3] {
		t.Fatalf("first provider counts = %v, want highest-weight provider to be chosen most often", firstCounts)
	}
	if firstCounts[1] < 800 {
		t.Fatalf("first provider counts = %v, want strong bias toward weight-12 route", firstCounts)
	}
}

func TestMatchWeightedRandomRetainsWeightBiasWhenHealthDiffIsSmall(t *testing.T) {
	tracker := &routerFakeHealthTracker{
		scores: map[string]float64{
			"1:claude": 100,
			"2:claude": 101,
		},
	}
	router := newRouterForTestWithStrategy(t, tracker, []*domain.Provider{
		newProvider(1),
		newProvider(2),
	}, []*domain.Route{
		newWeightedRoute(101, 1, 1, 10),
		newWeightedRoute(102, 2, 2, 1),
	}, domain.RoutingStrategyWeightedRandom)
	router.shuffle = rand.New(rand.NewSource(29))

	firstCounts := map[uint64]int{}
	for range 600 {
		matched, err := router.Match(&MatchContext{
			TenantID:     1,
			ClientType:   domain.ClientTypeClaude,
			RequestModel: "claude-sonnet-4",
		})
		if err != nil {
			t.Fatalf("Match returned error: %v", err)
		}
		firstCounts[matched[0].Provider.ID]++
	}

	if firstCounts[1] <= firstCounts[2] {
		t.Fatalf("first provider counts = %v, want weight-10 route to stay more likely when health delta is small", firstCounts)
	}
}

func newRouterForTest(t *testing.T, tracker health.ProviderTracker, providers []*domain.Provider, routes []*domain.Route) *Router {
	return newRouterForTestWithStrategy(t, tracker, providers, routes, domain.RoutingStrategyPriority)
}

func newRouterForTestWithStrategy(t *testing.T, tracker health.ProviderTracker, providers []*domain.Provider, routes []*domain.Route, strategyType domain.RoutingStrategyType) *Router {
	t.Helper()

	routeRepo := cached.NewRouteRepository(&routerRouteRepoStub{})
	providerRepo := cached.NewProviderRepository(&routerProviderRepoStub{})
	routingStrategyRepo := cached.NewRoutingStrategyRepository(&routerStrategyRepoStub{})
	retryConfigRepo := cached.NewRetryConfigRepository(&routerRetryConfigRepoStub{})
	projectRepo := cached.NewProjectRepository(&routerProjectRepoStub{})

	for _, provider := range providers {
		if err := providerRepo.Create(provider); err != nil {
			t.Fatalf("Create provider failed: %v", err)
		}
	}
	for _, route := range routes {
		if err := routeRepo.Create(route); err != nil {
			t.Fatalf("Create route failed: %v", err)
		}
	}
	if err := routingStrategyRepo.Create(&domain.RoutingStrategy{
		ID:        1,
		TenantID:  1,
		ProjectID: 0,
		Type:      strategyType,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("Create routing strategy failed: %v", err)
	}

	adapters := make(map[uint64]provideradapter.ProviderAdapter, len(providers))
	for _, provider := range providers {
		adapters[provider.ID] = &routerStubProviderAdapter{}
	}

	return &Router{
		routeRepo:           routeRepo,
		providerRepo:        providerRepo,
		routingStrategyRepo: routingStrategyRepo,
		retryConfigRepo:     retryConfigRepo,
		projectRepo:         projectRepo,
		adapters:            adapters,
		cooldownManager:     cooldown.NewManager(),
		healthTracker:       tracker,
		shuffle:             rand.New(rand.NewSource(1)),
	}
}

func tiedProviderOrder(matched []*MatchedRoute, providerIDs ...uint64) []uint64 {
	allowed := make(map[uint64]struct{}, len(providerIDs))
	for _, providerID := range providerIDs {
		allowed[providerID] = struct{}{}
	}

	order := make([]uint64, 0, len(providerIDs))
	for _, matchedRoute := range matched {
		if _, ok := allowed[matchedRoute.Provider.ID]; ok {
			order = append(order, matchedRoute.Provider.ID)
		}
	}
	return order
}

func newProvider(id uint64) *domain.Provider {
	return &domain.Provider{
		ID:                   id,
		TenantID:             1,
		Type:                 "custom",
		Name:                 "provider",
		Config:               &domain.ProviderConfig{DisableErrorCooldown: true},
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeClaude},
	}
}

func newRoute(id uint64, providerID uint64, position int) *domain.Route {
	return newWeightedRoute(id, providerID, position, 1)
}

func newWeightedRoute(id uint64, providerID uint64, position int, weight int) *domain.Route {
	return &domain.Route{
		ID:         id,
		TenantID:   1,
		IsEnabled:  true,
		IsNative:   true,
		ProjectID:  0,
		ClientType: domain.ClientTypeClaude,
		ProviderID: providerID,
		Position:   position,
		Weight:     weight,
	}
}

type routerStubProviderAdapter struct{}

func (a *routerStubProviderAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeClaude}
}

func (a *routerStubProviderAdapter) Execute(c *flow.Ctx, provider *domain.Provider) error {
	return nil
}

type routerFakeHealthTracker struct {
	scores map[string]float64
	opens  map[string]bool
}

func (t *routerFakeHealthTracker) BeginAttempt(providerID uint64, clientType string) func() {
	return func() {}
}

func (t *routerFakeHealthTracker) Record(result health.AttemptResult) {}

func (t *routerFakeHealthTracker) Score(providerID uint64, clientType string) float64 {
	if t == nil || t.scores == nil {
		return 0
	}
	return t.scores[routerTrackerKey(providerID, clientType)]
}

func (t *routerFakeHealthTracker) IsCircuitOpen(providerID uint64, clientType string) bool {
	if t == nil || t.opens == nil {
		return false
	}
	return t.opens[routerTrackerKey(providerID, clientType)]
}

func (t *routerFakeHealthTracker) AllowAttempt(providerID uint64, clientType string) bool {
	return !t.IsCircuitOpen(providerID, clientType)
}

func (t *routerFakeHealthTracker) ReleaseHalfOpenProbe(providerID uint64, clientType string, startedAt time.Time) {
}

func routerTrackerKey(providerID uint64, clientType string) string {
	return fmt.Sprintf("%d:%s", providerID, clientType)
}

type routerRouteRepoStub struct {
	routes []*domain.Route
}

func (r *routerRouteRepoStub) Create(route *domain.Route) error {
	r.routes = append(r.routes, route)
	return nil
}
func (r *routerRouteRepoStub) Update(route *domain.Route) error        { return nil }
func (r *routerRouteRepoStub) Delete(tenantID uint64, id uint64) error { return nil }
func (r *routerRouteRepoStub) GetByID(tenantID uint64, id uint64) (*domain.Route, error) {
	for _, route := range r.routes {
		if route.ID == id {
			return route, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *routerRouteRepoStub) FindByKey(tenantID uint64, projectID, providerID uint64, clientType domain.ClientType) (*domain.Route, error) {
	for _, route := range r.routes {
		if route.ProjectID == projectID && route.ProviderID == providerID && route.ClientType == clientType {
			return route, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *routerRouteRepoStub) List(tenantID uint64) ([]*domain.Route, error) {
	return append([]*domain.Route(nil), r.routes...), nil
}
func (r *routerRouteRepoStub) BatchUpdatePositions(tenantID uint64, updates []domain.RoutePositionUpdate) error {
	return nil
}

type routerProviderRepoStub struct {
	providers map[uint64]*domain.Provider
}

func (r *routerProviderRepoStub) Create(provider *domain.Provider) error {
	if r.providers == nil {
		r.providers = make(map[uint64]*domain.Provider)
	}
	r.providers[provider.ID] = provider
	return nil
}
func (r *routerProviderRepoStub) Update(provider *domain.Provider) error {
	if r.providers == nil {
		r.providers = make(map[uint64]*domain.Provider)
	}
	r.providers[provider.ID] = provider
	return nil
}
func (r *routerProviderRepoStub) Delete(tenantID uint64, id uint64) error { return nil }
func (r *routerProviderRepoStub) GetByID(tenantID uint64, id uint64) (*domain.Provider, error) {
	if provider, ok := r.providers[id]; ok {
		return provider, nil
	}
	return nil, domain.ErrNotFound
}
func (r *routerProviderRepoStub) List(tenantID uint64) ([]*domain.Provider, error) {
	list := make([]*domain.Provider, 0, len(r.providers))
	for _, provider := range r.providers {
		list = append(list, provider)
	}
	return list, nil
}

type routerStrategyRepoStub struct {
	strategies []*domain.RoutingStrategy
}

func (r *routerStrategyRepoStub) Create(strategy *domain.RoutingStrategy) error {
	r.strategies = append(r.strategies, strategy)
	return nil
}
func (r *routerStrategyRepoStub) Update(strategy *domain.RoutingStrategy) error { return nil }
func (r *routerStrategyRepoStub) Delete(tenantID uint64, id uint64) error       { return nil }
func (r *routerStrategyRepoStub) GetByProjectID(tenantID uint64, projectID uint64) (*domain.RoutingStrategy, error) {
	for _, strategy := range r.strategies {
		if strategy.ProjectID == projectID && strategy.TenantID == tenantID {
			return strategy, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *routerStrategyRepoStub) List(tenantID uint64) ([]*domain.RoutingStrategy, error) {
	return append([]*domain.RoutingStrategy(nil), r.strategies...), nil
}

type routerRetryConfigRepoStub struct{}

func (r *routerRetryConfigRepoStub) Create(config *domain.RetryConfig) error { return nil }
func (r *routerRetryConfigRepoStub) Update(config *domain.RetryConfig) error { return nil }
func (r *routerRetryConfigRepoStub) Delete(tenantID uint64, id uint64) error { return nil }
func (r *routerRetryConfigRepoStub) GetByID(tenantID uint64, id uint64) (*domain.RetryConfig, error) {
	return nil, domain.ErrNotFound
}
func (r *routerRetryConfigRepoStub) GetDefault(tenantID uint64) (*domain.RetryConfig, error) {
	return nil, domain.ErrNotFound
}
func (r *routerRetryConfigRepoStub) List(tenantID uint64) ([]*domain.RetryConfig, error) {
	return nil, nil
}

type routerProjectRepoStub struct{}

func (r *routerProjectRepoStub) Create(project *domain.Project) error    { return nil }
func (r *routerProjectRepoStub) Update(project *domain.Project) error    { return nil }
func (r *routerProjectRepoStub) Delete(tenantID uint64, id uint64) error { return nil }
func (r *routerProjectRepoStub) GetByID(tenantID uint64, id uint64) (*domain.Project, error) {
	return nil, domain.ErrNotFound
}
func (r *routerProjectRepoStub) GetBySlug(tenantID uint64, slug string) (*domain.Project, error) {
	return nil, domain.ErrNotFound
}
func (r *routerProjectRepoStub) List(tenantID uint64) ([]*domain.Project, error) {
	return nil, nil
}
