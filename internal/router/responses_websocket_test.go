package router

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	provideradapter "github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/repository/cached"
)

const (
	wsRouterNativeType   = "responses-ws-router-native-test"
	wsRouterHTTPOnlyType = "responses-ws-router-http-only-test"
	wsRouterStatefulType = "responses-ws-router-stateful-test"
)

var wsRouterStatefulFactoryCalls atomic.Int64

func init() {
	provideradapter.RegisterAdapterFactory(wsRouterNativeType, func(p *domain.Provider) (provideradapter.ProviderAdapter, error) {
		return &wsRouterNativeAdapter{providerID: p.ID}, nil
	})
	provideradapter.RegisterAdapterFactory(wsRouterHTTPOnlyType, func(*domain.Provider) (provideradapter.ProviderAdapter, error) {
		return wsRouterHTTPOnlyAdapter{}, nil
	})
	provideradapter.RegisterAdapterFactory(wsRouterStatefulType, func(p *domain.Provider) (provideradapter.ProviderAdapter, error) {
		return &wsRouterStatefulAdapter{
			providerID: p.ID,
			instance:   wsRouterStatefulFactoryCalls.Add(1),
		}, nil
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

type wsRouterStatefulAdapter struct {
	providerID uint64
	instance   int64
}

func (wsRouterStatefulAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeCodex}
}

func (wsRouterStatefulAdapter) Execute(*flow.Ctx, *domain.Provider) error { return nil }

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

type wsRouterProjectRepo struct{ projects []*domain.Project }

func (r *wsRouterProjectRepo) Create(p *domain.Project) error {
	r.projects = append(r.projects, p)
	return nil
}
func (r *wsRouterProjectRepo) Update(*domain.Project) error { return nil }
func (r *wsRouterProjectRepo) Delete(uint64, uint64) error  { return nil }
func (r *wsRouterProjectRepo) GetByID(tenantID, id uint64) (*domain.Project, error) {
	for _, p := range r.projects {
		if p.TenantID == tenantID && p.ID == id {
			return p, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *wsRouterProjectRepo) GetBySlug(uint64, string) (*domain.Project, error) {
	return nil, domain.ErrNotFound
}
func (r *wsRouterProjectRepo) List(tenantID uint64) ([]*domain.Project, error) {
	var out []*domain.Project
	for _, p := range r.projects {
		if tenantID == domain.TenantIDAll || p.TenantID == tenantID {
			out = append(out, p)
		}
	}
	return out, nil
}

func newResponsesWebSocketTestRouter(t *testing.T, routes []*domain.Route, providers []*domain.Provider) *Router {
	t.Helper()
	return newResponsesWebSocketTestRouterWithProjects(t, routes, providers, nil)
}

func newResponsesWebSocketTestRouterWithProjects(
	t *testing.T,
	routes []*domain.Route,
	providers []*domain.Provider,
	projects []*domain.Project,
) *Router {
	t.Helper()
	routeRepo := cached.NewRouteRepository(&wsRouterRouteRepo{routes: routes})
	providerRepo := cached.NewProviderRepository(&wsRouterProviderRepo{providers: providers})
	retryRepo := cached.NewRetryConfigRepository(wsRouterRetryRepo{})
	strategyRepo := cached.NewRoutingStrategyRepository(wsRouterStrategyRepo{})
	projectRepo := cached.NewProjectRepository(&wsRouterProjectRepo{projects: projects})
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

func TestReconcileAdaptersPreservesUnchangedStatefulAdapters(t *testing.T) {
	wsRouterStatefulFactoryCalls.Store(0)
	provideradapter.MarkResponsesWebSocketTransportUnavailable(101)
	provideradapter.MarkResponsesWebSocketTransportUnavailable(102)
	t.Cleanup(func() {
		provideradapter.ClearResponsesWebSocketTransportCooldown(101)
		provideradapter.ClearResponsesWebSocketTransportCooldown(102)
	})

	providers := &wsRouterProviderRepo{providers: []*domain.Provider{
		{
			ID:                   101,
			TenantID:             1,
			Type:                 wsRouterStatefulType,
			Name:                 "keep-state",
			SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex},
			Config:               &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{APIKey: "keep"}},
		},
		{
			ID:                   102,
			TenantID:             1,
			Type:                 wsRouterStatefulType,
			Name:                 "changed",
			SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex},
			Config:               &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{APIKey: "before"}},
		},
	}}

	providerRepo := cached.NewProviderRepository(providers)
	if err := providerRepo.Load(); err != nil {
		t.Fatalf("load providers: %v", err)
	}
	r := NewRouter(
		cached.NewRouteRepository(&wsRouterRouteRepo{}),
		providerRepo,
		cached.NewRoutingStrategyRepository(wsRouterStrategyRepo{}),
		cached.NewRetryConfigRepository(wsRouterRetryRepo{}),
		cached.NewProjectRepository(&wsRouterProjectRepo{}),
	)
	if err := r.InitAdapters(); err != nil {
		t.Fatalf("InitAdapters: %v", err)
	}
	originalKeep, ok := r.GetAdapter(101)
	if !ok {
		t.Fatal("missing initial unchanged provider adapter")
	}
	originalChanged, ok := r.GetAdapter(102)
	if !ok {
		t.Fatal("missing initial changed provider adapter")
	}

	providers.providers = []*domain.Provider{
		{
			ID:                   101,
			TenantID:             1,
			Type:                 wsRouterStatefulType,
			Name:                 "keep-state-renamed",
			SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex},
			Config:               &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{APIKey: "keep"}},
		},
		{
			ID:                   102,
			TenantID:             1,
			Type:                 wsRouterStatefulType,
			Name:                 "changed",
			SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex},
			Config:               &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{APIKey: "after"}},
		},
		{
			ID:                   103,
			TenantID:             1,
			Type:                 wsRouterStatefulType,
			Name:                 "added",
			SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex},
			Config:               &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{APIKey: "new"}},
		},
	}
	if err := providerRepo.Load(); err != nil {
		t.Fatalf("reload providers: %v", err)
	}
	if err := r.ReconcileAdapters(); err != nil {
		t.Fatalf("ReconcileAdapters: %v", err)
	}

	kept, ok := r.GetAdapter(101)
	if !ok {
		t.Fatal("unchanged provider adapter disappeared")
	}
	if kept != originalKeep {
		t.Fatal("unchanged provider adapter was replaced")
	}
	if provideradapter.ResponsesWebSocketTransportAvailable(101) {
		t.Fatal("unchanged provider websocket transport cooldown was cleared")
	}

	replaced, ok := r.GetAdapter(102)
	if !ok {
		t.Fatal("changed provider adapter disappeared")
	}
	if replaced == originalChanged {
		t.Fatal("changed provider adapter was not replaced")
	}
	if !provideradapter.ResponsesWebSocketTransportAvailable(102) {
		t.Fatal("changed provider websocket transport cooldown was not cleared")
	}
	if _, ok := r.GetAdapter(103); !ok {
		t.Fatal("added provider adapter was not created")
	}
	if got := wsRouterStatefulFactoryCalls.Load(); got != 4 {
		t.Fatalf("adapter factory calls = %d, want 4", got)
	}
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
	if !r.HasResponsesWebSocketProvider(1, 0) {
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
	if rHTTPOnly.HasResponsesWebSocketProvider(1, 0) {
		t.Fatal("HTTP-only adapter should not count as WebSocket-capable")
	}

	rEmpty := newResponsesWebSocketTestRouter(t, nil, nil)
	if rEmpty.HasResponsesWebSocketProvider(1, 0) {
		t.Fatal("empty routes should not report WebSocket capability")
	}

	// Project-only WS routes must not make the global (projectID=0) pre-check pass.
	// Otherwise upgrade succeeds (101) and Codex will not auto-fallback to SSE.
	rProjectOnly := newResponsesWebSocketTestRouter(t,
		[]*domain.Route{
			{ID: 1, TenantID: 1, ProjectID: 9, ProviderID: 301, ClientType: domain.ClientTypeCodex, IsEnabled: true, Position: 1},
		},
		[]*domain.Provider{
			{ID: 301, TenantID: 1, Type: wsRouterNativeType, Name: "project-ws", SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}},
		},
	)
	if rProjectOnly.HasResponsesWebSocketProvider(1, 0) {
		t.Fatal("project-only WS route must not satisfy global projectID=0 check")
	}

	// Project custom Codex routes exist but are HTTP-only, while global is WS.
	// Match uses only project routes → no WS. Pre-check must be false so callers
	// return 426 (not 101 + later failure).
	rProjectHTTPOnlyWithGlobalWS := newResponsesWebSocketTestRouterWithProjects(t,
		[]*domain.Route{
			{ID: 1, TenantID: 1, ProjectID: 9, ProviderID: 401, ClientType: domain.ClientTypeCodex, IsEnabled: true, Position: 1},
			{ID: 2, TenantID: 1, ProjectID: 0, ProviderID: 402, ClientType: domain.ClientTypeCodex, IsEnabled: true, Position: 2},
		},
		[]*domain.Provider{
			{ID: 401, TenantID: 1, Type: wsRouterHTTPOnlyType, Name: "project-http", SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}},
			{ID: 402, TenantID: 1, Type: wsRouterNativeType, Name: "global-ws", SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}},
		},
		[]*domain.Project{
			{ID: 9, TenantID: 1, EnabledCustomRoutes: []domain.ClientType{domain.ClientTypeCodex}},
		},
	)
	if rProjectHTTPOnlyWithGlobalWS.HasResponsesWebSocketProvider(1, 9) {
		t.Fatal("project HTTP-only custom routes must force 426 even when global WS exists")
	}
	if !rProjectHTTPOnlyWithGlobalWS.HasResponsesWebSocketProvider(1, 0) {
		t.Fatal("global projectID=0 should still see the global WS route")
	}
}

func TestResponsesWebSocketTransportCooldownAffectsOnlyWebSocket(t *testing.T) {
	const providerID = uint64(501)
	r := newResponsesWebSocketTestRouter(t,
		[]*domain.Route{
			{ID: 1, TenantID: 1, ProviderID: providerID, ClientType: domain.ClientTypeCodex, IsEnabled: true, Position: 1},
		},
		[]*domain.Provider{
			{ID: providerID, TenantID: 1, Type: wsRouterNativeType, Name: "ws", SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}},
		},
	)
	provideradapter.MarkResponsesWebSocketTransportUnavailable(providerID)
	t.Cleanup(func() { provideradapter.ClearResponsesWebSocketTransportCooldown(providerID) })

	if r.HasResponsesWebSocketProvider(1, 0) {
		t.Fatal("websocket handshake pre-check ignored transport cooldown")
	}
	_, err := r.Match(&MatchContext{
		TenantID:                  1,
		ClientType:                domain.ClientTypeCodex,
		RequestModel:              "gpt-test",
		RequireResponsesWebSocket: true,
	})
	if !errors.Is(err, domain.ErrNoResponsesWebSocketProviders) {
		t.Fatalf("websocket Match error = %v, want ErrNoResponsesWebSocketProviders", err)
	}
	result, err := r.Match(&MatchContext{
		TenantID:     1,
		ClientType:   domain.ClientTypeCodex,
		RequestModel: "gpt-test",
	})
	if err != nil {
		t.Fatalf("HTTP/SSE Match: %v", err)
	}
	if ids := matchedProviderIDs(result); len(ids) != 1 || ids[0] != providerID {
		t.Fatalf("HTTP/SSE matched provider IDs = %v, want %d", ids, providerID)
	}
}

func TestResponsesWebSocketTransportCooldownKeepsOtherProviderAvailable(t *testing.T) {
	const (
		cooledProviderID = uint64(511)
		readyProviderID  = uint64(512)
	)
	r := newResponsesWebSocketTestRouter(t,
		[]*domain.Route{
			{ID: 1, TenantID: 1, ProviderID: cooledProviderID, ClientType: domain.ClientTypeCodex, IsEnabled: true, Position: 1},
			{ID: 2, TenantID: 1, ProviderID: readyProviderID, ClientType: domain.ClientTypeCodex, IsEnabled: true, Position: 2},
		},
		[]*domain.Provider{
			{ID: cooledProviderID, TenantID: 1, Type: wsRouterNativeType, Name: "cooled", SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}},
			{ID: readyProviderID, TenantID: 1, Type: wsRouterNativeType, Name: "ready", SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}},
		},
	)
	provideradapter.MarkResponsesWebSocketTransportUnavailable(cooledProviderID)
	t.Cleanup(func() { provideradapter.ClearResponsesWebSocketTransportCooldown(cooledProviderID) })

	if !r.HasResponsesWebSocketProvider(1, 0) {
		t.Fatal("ready websocket provider was hidden by another provider's cooldown")
	}
	result, err := r.Match(&MatchContext{
		TenantID:                  1,
		ClientType:                domain.ClientTypeCodex,
		RequestModel:              "gpt-test",
		RequireResponsesWebSocket: true,
	})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if ids := matchedProviderIDs(result); len(ids) != 1 || ids[0] != readyProviderID {
		t.Fatalf("matched provider IDs = %v, want only %d", ids, readyProviderID)
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

func TestMatch_ResponsesWebSocketReturnsOrderedProviders(t *testing.T) {
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
	if len(ids) != 2 || ids[0] != providerA || ids[1] != providerB {
		t.Fatalf("matched provider IDs = %v, want [%d %d]", ids, providerA, providerB)
	}
}

func TestMatch_ProviderConcurrencyLimitSkipsFullProvider(t *testing.T) {
	r := newResponsesWebSocketTestRouter(t,
		[]*domain.Route{
			{ID: 71, TenantID: 1, ProviderID: 701, ClientType: domain.ClientTypeCodex, IsEnabled: true, Position: 1},
			{ID: 72, TenantID: 1, ProviderID: 702, ClientType: domain.ClientTypeCodex, IsEnabled: true, Position: 2},
		},
		[]*domain.Provider{
			{ID: 701, TenantID: 1, Type: wsRouterNativeType, Name: "full", MaxConcurrency: 1, SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}},
			{ID: 702, TenantID: 1, Type: wsRouterNativeType, Name: "available", MaxConcurrency: 1, SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}},
		},
	)
	release, ok := r.TryAcquireProvider(&domain.Provider{ID: 701, MaxConcurrency: 1})
	if !ok {
		t.Fatal("failed to occupy provider slot")
	}
	defer release()

	result, err := r.Match(&MatchContext{TenantID: 1, ClientType: domain.ClientTypeCodex, RequestModel: "gpt-test"})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	ids := matchedProviderIDs(result)
	if len(ids) != 1 || ids[0] != 702 {
		t.Fatalf("matched provider IDs = %v, want [702]", ids)
	}
}

func TestMatch_ResponsesWebSocketSkipsProviderAtConcurrencyLimit(t *testing.T) {
	r := newResponsesWebSocketTestRouter(t,
		[]*domain.Route{
			{ID: 51, TenantID: 1, ProviderID: 501, ClientType: domain.ClientTypeCodex, IsEnabled: true, Position: 1},
			{ID: 52, TenantID: 1, ProviderID: 502, ClientType: domain.ClientTypeCodex, IsEnabled: true, Position: 2},
		},
		[]*domain.Provider{
			{ID: 501, TenantID: 1, Type: wsRouterNativeType, Name: "full", MaxConcurrency: 1, SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}},
			{ID: 502, TenantID: 1, Type: wsRouterNativeType, Name: "available", MaxConcurrency: 1, SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}},
		},
	)
	release, ok := r.TryAcquireProvider(&domain.Provider{ID: 501, MaxConcurrency: 1})
	if !ok {
		t.Fatal("failed to occupy provider slot")
	}
	defer release()

	result, err := r.Match(&MatchContext{TenantID: 1, ClientType: domain.ClientTypeCodex, RequestModel: "gpt-test", RequireResponsesWebSocket: true})
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	ids := matchedProviderIDs(result)
	if len(ids) != 1 || ids[0] != 502 {
		t.Fatalf("matched provider IDs = %v, want [502]", ids)
	}
}

func TestMatch_ResponsesWebSocketRejectsWhenOnlyProviderIsAtConcurrencyLimit(t *testing.T) {
	r := newResponsesWebSocketTestRouter(t,
		[]*domain.Route{{ID: 61, TenantID: 1, ProviderID: 601, ClientType: domain.ClientTypeCodex, IsEnabled: true, Position: 1}},
		[]*domain.Provider{{ID: 601, TenantID: 1, Type: wsRouterNativeType, Name: "full", MaxConcurrency: 1, SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}}},
	)
	release, ok := r.TryAcquireProvider(&domain.Provider{ID: 601, MaxConcurrency: 1})
	if !ok {
		t.Fatal("failed to occupy provider slot")
	}
	defer release()

	_, err := r.Match(&MatchContext{TenantID: 1, ClientType: domain.ClientTypeCodex, RequestModel: "gpt-test", RequireResponsesWebSocket: true})
	if !errors.Is(err, domain.ErrNoResponsesWebSocketProviders) {
		t.Fatalf("Match error = %v, want ErrNoResponsesWebSocketProviders", err)
	}
	if !strings.Contains(err.Error(), "provider_concurrency_limit=1") {
		t.Fatalf("Match error = %q, want provider_concurrency_limit diagnostic", err.Error())
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
	if !strings.Contains(err.Error(), "required_provider_mismatch=2") {
		t.Fatalf("missing pin error = %q, want required_provider_mismatch diagnostic", err.Error())
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
