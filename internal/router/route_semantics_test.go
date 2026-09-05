package router

import (
	"errors"
	"strings"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository/cached"
)

func TestMatchTreatsRouteClientTypeAsInboundBucket(t *testing.T) {
	routeRepo := cached.NewRouteRepository(&wsRouterRouteRepo{routes: []*domain.Route{
		{ID: 1, TenantID: 1, ProviderID: 101, ClientType: domain.ClientTypeCodex, IsEnabled: true, Position: 1},
	}})
	providerRepo := cached.NewProviderRepository(&wsRouterProviderRepo{providers: []*domain.Provider{
		{ID: 101, TenantID: 1, Type: wsRouterNativeType, Name: "codex-only", SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}},
	}})
	retryRepo := cached.NewRetryConfigRepository(wsRouterRetryRepo{})
	strategyRepo := cached.NewRoutingStrategyRepository(wsRouterStrategyRepo{})
	projectRepo := cached.NewProjectRepository(&wsRouterProjectRepo{})
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

	_, err := r.Match(&MatchContext{TenantID: 1, ClientType: domain.ClientTypeOpenAI, RequestModel: "gpt-5"})
	if !errors.Is(err, domain.ErrNoRoutes) {
		t.Fatalf("OpenAI request matched Codex route: err=%v, want ErrNoRoutes", err)
	}

	result, err := r.Match(&MatchContext{TenantID: 1, ClientType: domain.ClientTypeCodex, RequestModel: "gpt-5"})
	if err != nil {
		t.Fatalf("Codex request did not match Codex route: %v", err)
	}
	if len(result.Routes) != 1 || result.Routes[0].Route.ID != 1 {
		t.Fatalf("routes = %+v, want Codex route 1", result.Routes)
	}
}

func TestMatchNoAvailableProvidersIncludesRejectionDiagnostics(t *testing.T) {
	routeRepo := cached.NewRouteRepository(&wsRouterRouteRepo{routes: []*domain.Route{
		{ID: 1, TenantID: 1, ProviderID: 901, ClientType: domain.ClientTypeCodex, IsEnabled: true, Position: 1},
	}})
	providerRepo := cached.NewProviderRepository(&wsRouterProviderRepo{providers: []*domain.Provider{
		{ID: 901, TenantID: 1, Type: wsRouterBadType, Name: "broken-config", SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}},
	}})
	retryRepo := cached.NewRetryConfigRepository(wsRouterRetryRepo{})
	strategyRepo := cached.NewRoutingStrategyRepository(wsRouterStrategyRepo{})
	projectRepo := cached.NewProjectRepository(&wsRouterProjectRepo{})
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

	_, err := r.Match(&MatchContext{TenantID: 1, ClientType: domain.ClientTypeCodex, RequestModel: "gpt-5"})
	if !errors.Is(err, domain.ErrNoAvailableProviders) {
		t.Fatalf("Match() error = %v, want ErrNoAvailableProviders", err)
	}
	if !strings.Contains(err.Error(), "rejections: adapter_missing=1") {
		t.Fatalf("error = %q, want adapter_missing diagnostic", err.Error())
	}
}
