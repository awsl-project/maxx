package router

import (
	"errors"
	"testing"

	provideradapter "github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/repository/cached"
)

const (
	wsRouterNativeType   = "responses-ws-router-native-test"
	wsRouterHTTPOnlyType = "responses-ws-router-http-only-test"
)

func init() {
	provideradapter.RegisterAdapterFactory(wsRouterNativeType, func(p *domain.Provider) (provideradapter.ProviderAdapter, error) {
		return &wsRouterNativeAdapter{providerID: p.ID}, nil
	})
	provideradapter.RegisterAdapterFactory(wsRouterHTTPOnlyType, func(*domain.Provider) (provideradapter.ProviderAdapter, error) {
		return wsRouterHTTPOnlyAdapter{}, nil
	})
}

// wsRouterNativeAdapter is Codex-native and implements ResponsesWebSocketAdapter.
type wsRouterNativeAdapter struct {
	providerID uint64
}

func (wsRouterNativeAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeCodex}
}

func (wsRouterNativeAdapter) Execute(*flow.Ctx, *domain.Provider) error { return nil }

func (a *wsRouterNativeAdapter) ExecuteResponsesWebSocket(
	*flow.Ctx,
	*domain.Provider,
	*domain.ResponsesWebSocketExchange,
) (*domain.ResponsesWebSocketResult, error) {
	return &domain.ResponsesWebSocketResult{ProviderID: a.providerID}, nil
}

// wsRouterHTTPOnlyAdapter speaks Codex over HTTP only — no WS capability.
type wsRouterHTTPOnlyAdapter struct{}

func (wsRouterHTTPOnlyAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeCodex}
}

func (wsRouterHTTPOnlyAdapter) Execute(*flow.Ctx, *domain.Provider) error { return nil }

type wsRouterRouteRepo struct{ routes []*domain.Route }

func (r *wsRouterRouteRepo) Create(route *domain.Route) error {
	r.routes = append(r.routes, route)
	return nil
}
func (r *wsRouterRouteRepo) Update(*domain.Route) error { return nil }
func (r *wsRouterRouteRepo) Delete(uint64, uint64) error {
	return nil
}
func (r *wsRouterRouteRepo) BulkDelete(uint64, domain.RouteBulkDeleteRequest) (*domain.RouteBulkDeleteResult, error) {
	return &domain.RouteBulkDeleteResult{}, nil
}
func (r *wsRouterRouteRepo) GetByID(tenantID, id uint64) (*domain.Route, error) {
	for _, route := range r.routes {
		if route.TenantID == tenantID && route.ID == id {
			return route, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *wsRouterRouteRepo) FindByKey(tenantID, projectID, providerID uint64, clientType domain.ClientType) (*domain.Route, error) {
	for _, route := range r.routes {
		if route.TenantID == tenantID && route.ProjectID == projectID && route.ProviderID == providerID && route.ClientType == clientType {
			return route, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *wsRouterRouteRepo) List(tenantID uint64) ([]*domain.Route, error) {
	var out []*domain.Route
	for _, route := range r.routes {
		if tenantID == domain.TenantIDAll || route.TenantID == tenantID {
			out = append(out, route)
		}
	}
	return out, nil
}
func (r *wsRouterRouteRepo) BatchUpdatePositions(uint64, []domain.RoutePositionUpdate) error {
	return nil
}

type wsRouterProviderRepo struct{ providers []*domain.Provider }

func (r *wsRouterProviderRepo) Create(p *domain.Provider) error {
	r.providers = append(r.providers, p)
	return nil
}
func (r *wsRouterProviderRepo) Update(*domain.Provider) error { return nil }
func (r *wsRouterProviderRepo) Delete(uint64, uint64) error   { return nil }
func (r *wsRouterProviderRepo) GetByID(tenantID, id uint64) (*domain.Provider, error) {
	for _, p := range r.providers {
		if p.TenantID == tenantID && p.ID == id {
			return p, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *wsRouterProviderRepo) List(tenantID uint64) ([]*domain.Provider, error) {
	var out []*domain.Provider
	for _, p := range r.providers {
		if tenantID == domain.TenantIDAll || p.TenantID == tenantID {
			out = append(out, p)
		}
	}
	return out, nil
}

type wsRouterRetryRepo struct{}

func (wsRouterRetryRepo) Create(*domain.RetryConfig) error { return nil }
func (wsRouterRetryRepo) Update(*domain.RetryConfig) error { return nil }
func (wsRouterRetryRepo) Delete(uint64, uint64) error      { return nil }
func (wsRouterRetryRepo) GetByID(uint64, uint64) (*domain.RetryConfig, error) {
	return nil, domain.ErrNotFound
}
func (wsRouterRetryRepo) GetDefault(uint64) (*domain.RetryConfig, error) {
	return nil, domain.ErrNotFound
}
func (wsRouterRetryRepo) List(uint64) ([]*domain.RetryConfig, error) { return nil, nil }

type wsRouterStrategyRepo struct{}

func (wsRouterStrategyRepo) Create(*domain.RoutingStrategy) error { return nil }
func (wsRouterStrategyRepo) Update(*domain.RoutingStrategy) error { return nil }
func (wsRouterStrategyRepo) Delete(uint64, uint64) error          { return nil }
func (wsRouterStrategyRepo) GetByID(uint64, uint64) (*domain.RoutingStrategy, error) {
	return nil, domain.ErrNotFound
}
func (wsRouterStrategyRepo) GetByProjectID(uint64, uint64) (*domain.RoutingStrategy, error) {
	return nil, domain.ErrNotFound
}
func (wsRouterStrategyRepo) List(uint64) ([]*domain.RoutingStrategy, error) { return nil, nil }

type wsRouterProjectRepo struct{}

func (wsRouterProjectRepo) Create(*domain.Project) error { return nil }
func (wsRouterProjectRepo) Update(*domain.Project) error { return nil }
func (wsRouterProjectRepo) Delete(uint64, uint64) error  { return nil }
func (wsRouterProjectRepo) GetByID(uint64, uint64) (*domain.Project, error) {
	return nil, domain.ErrNotFound
}
func (wsRouterProjectRepo) GetBySlug(uint64, string) (*domain.Project, error) {
	return nil, domain.ErrNotFound
}
func (wsRouterProjectRepo) List(uint64) ([]*domain.Project, error) { return nil, nil }

func newResponsesWebSocketTestRouter(t *testing.T, routes []*domain.Route, providers []*domain.Provider) *Router {
	t.Helper()
	routeRepo := cached.NewRouteRepository(&wsRouterRouteRepo{routes: routes})
	providerRepo := cached.NewProviderRepository(&wsRouterProviderRepo{providers: providers})
	retryRepo := cached.NewRetryConfigRepository(wsRouterRetryRepo{})
	strategyRepo := cached.NewRoutingStrategyRepository(wsRouterStrategyRepo{})
	projectRepo := cached.NewProjectRepository(wsRouterProjectRepo{})
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
	r := NewRouter(routeRepo, providerRepo, strategyRepo, retryRepo, projectRepo)
	if err := r.InitAdapters(); err != nil {
		t.Fatalf("InitAdapters: %v", err)
	}
	return r
}

func matchedProviderIDs(result *MatchResult) []uint64 {
	if result == nil {
		return nil
	}
	ids := make([]uint64, 0, len(result.Routes))
	for _, mr := range result.Routes {
		ids = append(ids, mr.Provider.ID)
	}
	return ids
}

func TestHasResponsesWebSocketProvider(t *testing.T) {
	r := newResponsesWebSocketTestRouter(t,
		[]*domain.Route{
			{ID: 1, TenantID: 1, ProviderID: 101, ClientType: domain.ClientTypeCodex, IsEnabled: true, Position: 1},
		},
		[]*domain.Provider{
			{ID: 101, TenantID: 1, Type: wsRouterNativeType, Name: "ws", SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}},
		},
	)
	if !r.HasResponsesWebSocketProvider(1) {
		t.Fatal("expected HasResponsesWebSocketProvider true for native WS adapter")
	}

	rHTTPOnly := newResponsesWebSocketTestRouter(t,
		[]*domain.Route{
			{ID: 1, TenantID: 1, ProviderID: 201, ClientType: domain.ClientTypeCodex, IsEnabled: true, Position: 1},
		},
		[]*domain.Provider{
			{ID: 201, TenantID: 1, Type: wsRouterHTTPOnlyType, Name: "http-only", SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}},
		},
	)
	if rHTTPOnly.HasResponsesWebSocketProvider(1) {
		t.Fatal("HTTP-only adapter should not count as WebSocket-capable")
	}

	rEmpty := newResponsesWebSocketTestRouter(t, nil, nil)
	if rEmpty.HasResponsesWebSocketProvider(1) {
		t.Fatal("empty routes should not report WebSocket capability")
	}
}

// Only providers that natively support Codex and whose adapter implements
// ResponsesWebSocketAdapter are eligible. Stale is_native=false is ignored.
func TestMatch_ResponsesWebSocketOnlyNativeCapableAdaptersEligible(t *testing.T) {
	const (
		nativeWSProviderID  = uint64(101)
		httpOnlyProviderID  = uint64(102)
		nonNativeWSProvider = uint64(103)
	)
	r := newResponsesWebSocketTestRouter(t,
		[]*domain.Route{
			{ID: 1, TenantID: 1, ProviderID: nativeWSProviderID, ClientType: domain.ClientTypeCodex, IsEnabled: true, IsNative: false, Position: 1},
			{ID: 2, TenantID: 1, ProviderID: httpOnlyProviderID, ClientType: domain.ClientTypeCodex, IsEnabled: true, IsNative: true, Position: 2},
			{ID: 3, TenantID: 1, ProviderID: nonNativeWSProvider, ClientType: domain.ClientTypeCodex, IsEnabled: true, IsNative: true, Position: 3},
		},
		[]*domain.Provider{
			// Stale IsNative=false still eligible via canonical native check.
			{ID: nativeWSProviderID, TenantID: 1, Type: wsRouterNativeType, Name: "native-ws", SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}},
			{ID: httpOnlyProviderID, TenantID: 1, Type: wsRouterHTTPOnlyType, Name: "http-only", SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}},
			// Stale IsNative=true but provider does not natively support Codex.
			{ID: nonNativeWSProvider, TenantID: 1, Type: wsRouterNativeType, Name: "non-native-ws", SupportedClientTypes: []domain.ClientType{domain.ClientTypeOpenAI}},
		},
	)

	result, err := r.Match(&MatchContext{
		TenantID:                  1,
		ClientType:                domain.ClientTypeCodex,
		RequestModel:              "gpt-test",
		RequireResponsesWebSocket: true,
	})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	ids := matchedProviderIDs(result)
	if len(ids) != 1 || ids[0] != nativeWSProviderID {
		t.Fatalf("matched provider IDs = %v, want only native capable %d", ids, nativeWSProviderID)
	}
}

func TestMatch_ResponsesWebSocketReturnsOneProvider(t *testing.T) {
	const (
		providerA = uint64(401)
		providerB = uint64(402)
	)
	r := newResponsesWebSocketTestRouter(t,
		[]*domain.Route{
			{ID: 41, TenantID: 1, ProviderID: providerA, ClientType: domain.ClientTypeCodex, IsEnabled: true, IsNative: true, Position: 1},
			{ID: 42, TenantID: 1, ProviderID: providerB, ClientType: domain.ClientTypeCodex, IsEnabled: true, IsNative: true, Position: 2},
		},
		[]*domain.Provider{
			{ID: providerA, TenantID: 1, Type: wsRouterNativeType, Name: "ws-a", SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}},
			{ID: providerB, TenantID: 1, Type: wsRouterNativeType, Name: "ws-b", SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}},
		},
	)

	result, err := r.Match(&MatchContext{
		TenantID:                  1,
		ClientType:                domain.ClientTypeCodex,
		RequestModel:              "gpt-test",
		RequireResponsesWebSocket: true,
	})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	ids := matchedProviderIDs(result)
	if len(ids) != 1 || ids[0] != providerA {
		t.Fatalf("matched provider IDs = %v, want single first provider %d", ids, providerA)
	}
}

// RequiredProviderID pins eligibility to one provider; a missing pin is a
// dedicated session-unavailable error (not the generic no-providers error).
func TestMatch_ResponsesWebSocketRequiredProviderIDPinsProvider(t *testing.T) {
	const (
		providerA = uint64(201)
		providerB = uint64(202)
	)
	r := newResponsesWebSocketTestRouter(t,
		[]*domain.Route{
			{ID: 11, TenantID: 1, ProviderID: providerA, ClientType: domain.ClientTypeCodex, IsEnabled: true, IsNative: true, Position: 1},
			{ID: 12, TenantID: 1, ProviderID: providerB, ClientType: domain.ClientTypeCodex, IsEnabled: true, IsNative: true, Position: 2},
		},
		[]*domain.Provider{
			{ID: providerA, TenantID: 1, Type: wsRouterNativeType, Name: "ws-a", SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}},
			{ID: providerB, TenantID: 1, Type: wsRouterNativeType, Name: "ws-b", SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}},
		},
	)

	result, err := r.Match(&MatchContext{
		TenantID:                  1,
		ClientType:                domain.ClientTypeCodex,
		RequestModel:              "gpt-test",
		RequireResponsesWebSocket: true,
		RequiredProviderID:        providerB,
	})
	if err != nil {
		t.Fatalf("Match with pin: %v", err)
	}
	ids := matchedProviderIDs(result)
	if len(ids) != 1 || ids[0] != providerB {
		t.Fatalf("matched provider IDs = %v, want pinned %d", ids, providerB)
	}

	_, err = r.Match(&MatchContext{
		TenantID:                  1,
		ClientType:                domain.ClientTypeCodex,
		RequestModel:              "gpt-test",
		RequireResponsesWebSocket: true,
		RequiredProviderID:        99999,
	})
	if !errors.Is(err, domain.ErrResponsesWebSocketSessionUnavailable) {
		t.Fatalf("missing pin error = %v, want %v", err, domain.ErrResponsesWebSocketSessionUnavailable)
	}
}

func TestMatch_ResponsesWebSocketNoEligibleProviders(t *testing.T) {
	r := newResponsesWebSocketTestRouter(t,
		[]*domain.Route{
			{ID: 21, TenantID: 1, ProviderID: 301, ClientType: domain.ClientTypeCodex, IsEnabled: true, IsNative: true, Position: 1},
			{ID: 22, TenantID: 1, ProviderID: 302, ClientType: domain.ClientTypeCodex, IsEnabled: true, IsNative: true, Position: 2},
		},
		[]*domain.Provider{
			// Native route but HTTP-only adapter → not WS-capable.
			{ID: 301, TenantID: 1, Type: wsRouterHTTPOnlyType, Name: "http-only", SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}},
			// WS adapter but provider does not natively support Codex.
			{ID: 302, TenantID: 1, Type: wsRouterNativeType, Name: "ws-non-native", SupportedClientTypes: []domain.ClientType{domain.ClientTypeOpenAI}},
		},
	)

	_, err := r.Match(&MatchContext{
		TenantID:                  1,
		ClientType:                domain.ClientTypeCodex,
		RequestModel:              "gpt-test",
		RequireResponsesWebSocket: true,
	})
	if !errors.Is(err, domain.ErrNoResponsesWebSocketProviders) {
		t.Fatalf("Match error = %v, want %v", err, domain.ErrNoResponsesWebSocketProviders)
	}
}
