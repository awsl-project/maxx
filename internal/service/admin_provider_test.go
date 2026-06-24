package service

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
)

func TestAdminServiceDeleteProviderCleansRoutesAndProviderModelMappings(t *testing.T) {
	db, err := sqlite.NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("NewDBWithDSN() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	providerRepo := sqlite.NewProviderRepository(db)
	routeRepo := sqlite.NewRouteRepository(db)
	modelMappingRepo := sqlite.NewModelMappingRepository(db)

	provider := &domain.Provider{
		TenantID: domain.DefaultTenantID,
		Name:     "delete-me",
		Type:     "custom",
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL: "https://api.example.com",
			APIKey:  "sk-test",
		}},
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeClaude},
		SupportModels:        []string{"claude-*"},
	}
	if err := providerRepo.Create(provider); err != nil {
		t.Fatalf("Create(provider) error = %v", err)
	}

	otherProvider := &domain.Provider{
		TenantID: domain.DefaultTenantID,
		Name:     "keep-me",
		Type:     "custom",
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL: "https://api.other.example.com",
			APIKey:  "sk-other",
		}},
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeClaude},
		SupportModels:        []string{"claude-*"},
	}
	if err := providerRepo.Create(otherProvider); err != nil {
		t.Fatalf("Create(otherProvider) error = %v", err)
	}

	route := &domain.Route{TenantID: domain.DefaultTenantID, ProviderID: provider.ID, ClientType: domain.ClientTypeClaude, ProjectID: 0, Position: 1, Weight: 1, IsEnabled: true}
	if err := routeRepo.Create(route); err != nil {
		t.Fatalf("Create(route) error = %v", err)
	}
	otherRoute := &domain.Route{TenantID: domain.DefaultTenantID, ProviderID: otherProvider.ID, ClientType: domain.ClientTypeClaude, ProjectID: 0, Position: 2, Weight: 1, IsEnabled: true}
	if err := routeRepo.Create(otherRoute); err != nil {
		t.Fatalf("Create(otherRoute) error = %v", err)
	}

	providerMapping := &domain.ModelMapping{TenantID: domain.DefaultTenantID, Scope: domain.ModelMappingScopeProvider, ProviderID: provider.ID, Pattern: "claude-*", Target: "upstream"}
	if err := modelMappingRepo.Create(providerMapping); err != nil {
		t.Fatalf("Create(providerMapping) error = %v", err)
	}
	otherProviderMapping := &domain.ModelMapping{TenantID: domain.DefaultTenantID, Scope: domain.ModelMappingScopeProvider, ProviderID: otherProvider.ID, Pattern: "other-*", Target: "other-upstream"}
	if err := modelMappingRepo.Create(otherProviderMapping); err != nil {
		t.Fatalf("Create(otherProviderMapping) error = %v", err)
	}
	globalMapping := &domain.ModelMapping{TenantID: domain.DefaultTenantID, Scope: domain.ModelMappingScopeGlobal, Pattern: "global-*", Target: "global-upstream"}
	if err := modelMappingRepo.Create(globalMapping); err != nil {
		t.Fatalf("Create(globalMapping) error = %v", err)
	}

	svc := NewAdminService(providerRepo, routeRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, modelMappingRepo, nil, nil, nil, "", nil, nil, nil)
	if err := svc.DeleteProvider(domain.DefaultTenantID, provider.ID); err != nil {
		t.Fatalf("DeleteProvider() error = %v", err)
	}

	routes, err := routeRepo.List(domain.DefaultTenantID)
	if err != nil {
		t.Fatalf("List(routes) error = %v", err)
	}
	if len(routes) != 1 || routes[0].ID != otherRoute.ID {
		t.Fatalf("routes after delete = %+v, want only other route", routes)
	}

	mappings, err := modelMappingRepo.List(domain.DefaultTenantID)
	if err != nil {
		t.Fatalf("List(mappings) error = %v", err)
	}
	for _, mapping := range mappings {
		if mapping.ID == providerMapping.ID {
			t.Fatalf("provider-scoped mapping still exists after provider delete: %+v", mapping)
		}
	}
	if !containsModelMappingID(mappings, otherProviderMapping.ID) {
		t.Fatalf("other provider mapping was deleted unexpectedly: %+v", mappings)
	}
	if !containsModelMappingID(mappings, globalMapping.ID) {
		t.Fatalf("global mapping was deleted unexpectedly: %+v", mappings)
	}
}

func containsModelMappingID(mappings []*domain.ModelMapping, id uint64) bool {
	for _, mapping := range mappings {
		if mapping.ID == id {
			return true
		}
	}
	return false
}
