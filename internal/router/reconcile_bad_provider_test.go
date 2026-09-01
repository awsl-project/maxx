package router

import (
	"errors"

	provideradapter "github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository/cached"

	"testing"
)

// wsRouterBadType simulates a provider whose adapter factory always fails,
// e.g. the real-world fal provider created with an empty config where
// NewAdapter returns an error when Config.Fal == nil.
const wsRouterBadType = "responses-ws-router-bad-config-test"

func init() {
	provideradapter.RegisterAdapterFactory(wsRouterBadType, func(*domain.Provider) (provideradapter.ProviderAdapter, error) {
		return nil, errors.New("bad provider: nil config")
	})
}

// TestInitAdaptersSkipsBadProvider proves a single provider whose factory
// errors does not abort adapter building for the rest.
func TestInitAdaptersSkipsBadProvider(t *testing.T) {
	providers := &wsRouterProviderRepo{providers: []*domain.Provider{
		{
			ID:                   901,
			TenantID:             1,
			Type:                 wsRouterBadType,
			Name:                 "broken-config",
			SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex},
		},
		{
			ID:                   902,
			TenantID:             1,
			Type:                 wsRouterNativeType,
			Name:                 "healthy",
			SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex},
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

	// The broken provider must not abort InitAdapters.
	if err := r.InitAdapters(); err != nil {
		t.Fatalf("InitAdapters returned error for one bad provider: %v", err)
	}
	if _, ok := r.GetAdapter(901); ok {
		t.Fatal("bad provider unexpectedly got a live adapter")
	}
	if _, ok := r.GetAdapter(902); !ok {
		t.Fatal("healthy provider was poisoned by the bad provider (no live adapter)")
	}
}

// TestReconcileAdaptersSkipsBadProvider proves that when a bad provider is
// added on a provider-invalidation reconcile, a newly added good provider
// still gets a live adapter (the poison bug from the hubitos incident).
func TestReconcileAdaptersSkipsBadProvider(t *testing.T) {
	providers := &wsRouterProviderRepo{providers: []*domain.Provider{
		{
			ID:                   911,
			TenantID:             1,
			Type:                 wsRouterNativeType,
			Name:                 "existing-healthy",
			SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex},
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
	if _, ok := r.GetAdapter(911); !ok {
		t.Fatal("missing initial healthy adapter")
	}

	// Add a bad provider (empty-config fal) AND a new good provider, mimicking
	// a cross-instance provider-invalidation broadcast.
	providers.providers = []*domain.Provider{
		{
			ID:                   911,
			TenantID:             1,
			Type:                 wsRouterNativeType,
			Name:                 "existing-healthy",
			SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex},
		},
		{
			ID:                   912,
			TenantID:             1,
			Type:                 wsRouterBadType,
			Name:                 "broken-config",
			SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex},
		},
		{
			ID:                   913,
			TenantID:             1,
			Type:                 wsRouterNativeType,
			Name:                 "newly-added-healthy",
			SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex},
		},
	}
	if err := providerRepo.Load(); err != nil {
		t.Fatalf("reload providers: %v", err)
	}

	if err := r.ReconcileAdapters(); err != nil {
		t.Fatalf("ReconcileAdapters aborted wholesale on one bad provider: %v", err)
	}

	if _, ok := r.GetAdapter(911); !ok {
		t.Fatal("existing healthy provider lost its adapter after reconcile")
	}
	if _, ok := r.GetAdapter(912); ok {
		t.Fatal("bad provider unexpectedly got a live adapter")
	}
	if _, ok := r.GetAdapter(913); !ok {
		t.Fatal("newly added healthy provider never got a live adapter (poison bug)")
	}
}
