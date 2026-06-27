package service

import (
	"fmt"
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

func TestAdminServiceBulkDeleteProvidersCleansReferencesInOnePass(t *testing.T) {
	db, err := sqlite.NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("NewDBWithDSN() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	providerRepo := sqlite.NewProviderRepository(db)
	routeRepo := sqlite.NewRouteRepository(db)
	modelMappingRepo := sqlite.NewModelMappingRepository(db)

	providers := []*domain.Provider{
		newTestCustomProvider("bulk-delete-a"),
		newTestCustomProvider("bulk-delete-b"),
		newTestCustomProvider("bulk-keep"),
	}
	for _, provider := range providers {
		if err := providerRepo.Create(provider); err != nil {
			t.Fatalf("Create(provider %s) error = %v", provider.Name, err)
		}
	}

	routes := []*domain.Route{
		{TenantID: domain.DefaultTenantID, ProviderID: providers[0].ID, ClientType: domain.ClientTypeClaude, ProjectID: 0, Position: 1, Weight: 1, IsEnabled: true},
		{TenantID: domain.DefaultTenantID, ProviderID: providers[0].ID, ClientType: domain.ClientTypeOpenAI, ProjectID: 0, Position: 2, Weight: 1, IsEnabled: true},
		{TenantID: domain.DefaultTenantID, ProviderID: providers[1].ID, ClientType: domain.ClientTypeClaude, ProjectID: 7, Position: 3, Weight: 1, IsEnabled: true},
		{TenantID: domain.DefaultTenantID, ProviderID: providers[2].ID, ClientType: domain.ClientTypeClaude, ProjectID: 0, Position: 4, Weight: 1, IsEnabled: true},
	}
	for _, route := range routes {
		if err := routeRepo.Create(route); err != nil {
			t.Fatalf("Create(route) error = %v", err)
		}
	}

	mappings := []*domain.ModelMapping{
		{TenantID: domain.DefaultTenantID, Scope: domain.ModelMappingScopeProvider, ProviderID: providers[0].ID, Pattern: "a-*", Target: "upstream-a"},
		{TenantID: domain.DefaultTenantID, Scope: domain.ModelMappingScopeProvider, ProviderID: providers[1].ID, Pattern: "b-*", Target: "upstream-b"},
		{TenantID: domain.DefaultTenantID, Scope: domain.ModelMappingScopeProvider, ProviderID: providers[2].ID, Pattern: "keep-*", Target: "keep-upstream"},
		{TenantID: domain.DefaultTenantID, Scope: domain.ModelMappingScopeGlobal, Pattern: "global-*", Target: "global-upstream"},
	}
	for _, mapping := range mappings {
		if err := modelMappingRepo.Create(mapping); err != nil {
			t.Fatalf("Create(mapping) error = %v", err)
		}
	}

	svc := NewAdminService(providerRepo, routeRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, modelMappingRepo, nil, nil, nil, "", nil, nil, nil)
	result, err := svc.BulkDeleteProviders(domain.DefaultTenantID, domain.ProviderBulkDeleteRequest{
		IDs: []uint64{providers[0].ID, providers[1].ID, providers[0].ID, 999999, 0},
	})
	if err != nil {
		t.Fatalf("BulkDeleteProviders() error = %v", err)
	}

	if result.DeletedCount != 2 || result.RouteDeletedCount != 3 || result.ModelMappingDeletedCount != 2 {
		t.Fatalf("unexpected result: %#v", result)
	}
	assertServiceContainsID(t, result.DeletedIDs, providers[0].ID)
	assertServiceContainsID(t, result.DeletedIDs, providers[1].ID)
	assertServiceContainsID(t, result.NotFoundIDs, 999999)

	remainingProviders, err := providerRepo.List(domain.DefaultTenantID)
	if err != nil {
		t.Fatalf("List(providers) error = %v", err)
	}
	if len(remainingProviders) != 1 || remainingProviders[0].ID != providers[2].ID {
		t.Fatalf("providers after bulk delete = %+v, want only keep provider", remainingProviders)
	}

	remainingRoutes, err := routeRepo.List(domain.DefaultTenantID)
	if err != nil {
		t.Fatalf("List(routes) error = %v", err)
	}
	if len(remainingRoutes) != 1 || remainingRoutes[0].ID != routes[3].ID {
		t.Fatalf("routes after bulk delete = %+v, want only keep route", remainingRoutes)
	}

	remainingMappings, err := modelMappingRepo.List(domain.DefaultTenantID)
	if err != nil {
		t.Fatalf("List(mappings) error = %v", err)
	}
	if containsModelMappingID(remainingMappings, mappings[0].ID) || containsModelMappingID(remainingMappings, mappings[1].ID) {
		t.Fatalf("deleted provider mappings still visible: %+v", remainingMappings)
	}
	if !containsModelMappingID(remainingMappings, mappings[2].ID) || !containsModelMappingID(remainingMappings, mappings[3].ID) {
		t.Fatalf("unrelated mappings were deleted: %+v", remainingMappings)
	}
}

func TestAdminServiceBulkDeleteProvidersRollsBackReferencesWhenProviderDeleteFails(t *testing.T) {
	db, err := sqlite.NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("NewDBWithDSN() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	providerRepo := sqlite.NewProviderRepository(db)
	routeRepo := sqlite.NewRouteRepository(db)
	modelMappingRepo := sqlite.NewModelMappingRepository(db)

	provider := newTestCustomProvider("rollback-provider")
	if err := providerRepo.Create(provider); err != nil {
		t.Fatalf("Create(provider) error = %v", err)
	}

	route := &domain.Route{TenantID: domain.DefaultTenantID, ProviderID: provider.ID, ClientType: domain.ClientTypeClaude, ProjectID: 0, Position: 1, Weight: 1, IsEnabled: true}
	if err := routeRepo.Create(route); err != nil {
		t.Fatalf("Create(route) error = %v", err)
	}

	mapping := &domain.ModelMapping{TenantID: domain.DefaultTenantID, Scope: domain.ModelMappingScopeProvider, ProviderID: provider.ID, Pattern: "rollback-*", Target: "upstream"}
	if err := modelMappingRepo.Create(mapping); err != nil {
		t.Fatalf("Create(mapping) error = %v", err)
	}

	triggerSQL := fmt.Sprintf(`
		CREATE TRIGGER fail_provider_soft_delete
		BEFORE UPDATE OF deleted_at ON providers
		WHEN OLD.id = %d AND NEW.deleted_at != 0
		BEGIN
			SELECT RAISE(ABORT, 'provider delete blocked');
		END;
	`, provider.ID)
	if err := db.GormDB().Exec(triggerSQL).Error; err != nil {
		t.Fatalf("create trigger error = %v", err)
	}

	svc := NewAdminService(providerRepo, routeRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, modelMappingRepo, nil, nil, nil, "", nil, nil, nil)
	if _, err := svc.BulkDeleteProviders(domain.DefaultTenantID, domain.ProviderBulkDeleteRequest{IDs: []uint64{provider.ID}}); err == nil {
		t.Fatal("BulkDeleteProviders() error = nil, want provider delete failure")
	}

	providers, err := providerRepo.List(domain.DefaultTenantID)
	if err != nil {
		t.Fatalf("List(providers) error = %v", err)
	}
	if !containsProviderID(providers, provider.ID) {
		t.Fatalf("provider was deleted despite transaction rollback: %+v", providers)
	}

	routes, err := routeRepo.List(domain.DefaultTenantID)
	if err != nil {
		t.Fatalf("List(routes) error = %v", err)
	}
	if len(routes) != 1 || routes[0].ID != route.ID {
		t.Fatalf("routes after failed bulk delete = %+v, want original route", routes)
	}

	mappings, err := modelMappingRepo.List(domain.DefaultTenantID)
	if err != nil {
		t.Fatalf("List(mappings) error = %v", err)
	}
	if !containsModelMappingID(mappings, mapping.ID) {
		t.Fatalf("provider mapping was deleted despite transaction rollback: %+v", mappings)
	}
}

func containsProviderID(providers []*domain.Provider, id uint64) bool {
	for _, provider := range providers {
		if provider.ID == id {
			return true
		}
	}
	return false
}

func newTestCustomProvider(name string) *domain.Provider {
	return &domain.Provider{
		TenantID: domain.DefaultTenantID,
		Name:     name,
		Type:     "custom",
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL: "https://api.example.com/" + name,
			APIKey:  "sk-test",
		}},
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeClaude, domain.ClientTypeOpenAI},
		SupportModels:        []string{"*"},
	}
}

func assertServiceContainsID(t *testing.T, ids []uint64, want uint64) {
	t.Helper()
	for _, id := range ids {
		if id == want {
			return
		}
	}
	t.Fatalf("ids %v does not contain %d", ids, want)
}
