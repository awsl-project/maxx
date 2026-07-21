package service

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
)

func setupRouteNativeTestEnv(t *testing.T) (
	*AdminService,
	*sqlite.ProviderRepository,
	*sqlite.RouteRepository,
	*sqlite.ProjectRepository,
) {
	t.Helper()

	db, err := sqlite.NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("NewDBWithDSN() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	providerRepo := sqlite.NewProviderRepository(db)
	routeRepo := sqlite.NewRouteRepository(db)
	projectRepo := sqlite.NewProjectRepository(db)
	svc := NewAdminService(
		providerRepo, routeRepo, projectRepo,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		"", nil, nil, nil,
	)
	return svc, providerRepo, routeRepo, projectRepo
}

func newTestCodexProvider(name string) *domain.Provider {
	return &domain.Provider{
		TenantID: domain.DefaultTenantID,
		Name:     name,
		Type:     "codex",
		Config: &domain.ProviderConfig{Codex: &domain.ProviderConfigCodex{
			Email:        name + "@example.com",
			RefreshToken: "rt-test",
			AccessToken:  "at-test",
		}},
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex},
		SupportModels:        []string{"*"},
	}
}

func newTestOpenAIOnlyCustomProvider(name string) *domain.Provider {
	return &domain.Provider{
		TenantID: domain.DefaultTenantID,
		Name:     name,
		Type:     "custom",
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL: "https://api.example.com/" + name,
			APIKey:  "sk-test",
		}},
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeOpenAI},
		SupportModels:        []string{"*"},
	}
}

func mustCreateProvider(t *testing.T, repo *sqlite.ProviderRepository, provider *domain.Provider) *domain.Provider {
	t.Helper()
	if err := repo.Create(provider); err != nil {
		t.Fatalf("Create(provider %s) error = %v", provider.Name, err)
	}
	return provider
}

func TestCreateRouteDerivesNativeFromProvider(t *testing.T) {
	svc, providerRepo, routeRepo, _ := setupRouteNativeTestEnv(t)

	codexProvider := mustCreateProvider(t, providerRepo, newTestCodexProvider("codex-native"))
	openaiProvider := mustCreateProvider(t, providerRepo, newTestOpenAIOnlyCustomProvider("openai-only"))

	codexRoute := &domain.Route{
		ProviderID: codexProvider.ID,
		ClientType: domain.ClientTypeCodex,
		ProjectID:  0,
		Position:   1,
		Weight:     1,
		IsEnabled:  true,
	}
	if err := svc.CreateRoute(domain.DefaultTenantID, codexRoute); err != nil {
		t.Fatalf("CreateRoute(codex) error = %v", err)
	}
	if !codexRoute.IsNative {
		t.Fatalf("CreateRoute(codex→codex) IsNative = false, want true")
	}

	stored, err := routeRepo.GetByID(domain.DefaultTenantID, codexRoute.ID)
	if err != nil {
		t.Fatalf("GetByID(codex route) error = %v", err)
	}
	if !stored.IsNative {
		t.Fatalf("stored codex route IsNative = false, want true")
	}

	conversionRoute := &domain.Route{
		ProviderID: openaiProvider.ID,
		ClientType: domain.ClientTypeCodex,
		ProjectID:  0,
		Position:   2,
		Weight:     1,
		IsEnabled:  true,
	}
	if err := svc.CreateRoute(domain.DefaultTenantID, conversionRoute); err != nil {
		t.Fatalf("CreateRoute(conversion) error = %v", err)
	}
	if conversionRoute.IsNative {
		t.Fatalf("CreateRoute(openai→codex) IsNative = true, want false")
	}

	storedConversion, err := routeRepo.GetByID(domain.DefaultTenantID, conversionRoute.ID)
	if err != nil {
		t.Fatalf("GetByID(conversion route) error = %v", err)
	}
	if storedConversion.IsNative {
		t.Fatalf("stored conversion route IsNative = true, want false")
	}
}

func TestCreateRouteOverridesClientFalseForCodex(t *testing.T) {
	svc, providerRepo, routeRepo, _ := setupRouteNativeTestEnv(t)

	codexProvider := mustCreateProvider(t, providerRepo, newTestCodexProvider("codex-override-false"))

	route := &domain.Route{
		ProviderID: codexProvider.ID,
		ClientType: domain.ClientTypeCodex,
		ProjectID:  0,
		Position:   1,
		Weight:     1,
		IsEnabled:  true,
		IsNative:   false, // client-submitted value must be ignored
	}
	if err := svc.CreateRoute(domain.DefaultTenantID, route); err != nil {
		t.Fatalf("CreateRoute() error = %v", err)
	}
	if !route.IsNative {
		t.Fatalf("CreateRoute() IsNative = false, want true (override client false)")
	}

	stored, err := routeRepo.GetByID(domain.DefaultTenantID, route.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if !stored.IsNative {
		t.Fatalf("stored IsNative = false, want true")
	}
}

func TestCreateRouteOverridesClientTrueForConversion(t *testing.T) {
	svc, providerRepo, routeRepo, _ := setupRouteNativeTestEnv(t)

	openaiProvider := mustCreateProvider(t, providerRepo, newTestOpenAIOnlyCustomProvider("openai-override-true"))

	route := &domain.Route{
		ProviderID: openaiProvider.ID,
		ClientType: domain.ClientTypeCodex,
		ProjectID:  0,
		Position:   1,
		Weight:     1,
		IsEnabled:  true,
		IsNative:   true, // client-submitted value must be ignored
	}
	if err := svc.CreateRoute(domain.DefaultTenantID, route); err != nil {
		t.Fatalf("CreateRoute() error = %v", err)
	}
	if route.IsNative {
		t.Fatalf("CreateRoute() IsNative = true, want false (override client true)")
	}

	stored, err := routeRepo.GetByID(domain.DefaultTenantID, route.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if stored.IsNative {
		t.Fatalf("stored IsNative = true, want false")
	}
}

func TestUpdateRouteRecomputesNativeWhenProviderChanges(t *testing.T) {
	svc, providerRepo, routeRepo, _ := setupRouteNativeTestEnv(t)

	codexProvider := mustCreateProvider(t, providerRepo, newTestCodexProvider("codex-for-update"))
	openaiProvider := mustCreateProvider(t, providerRepo, newTestOpenAIOnlyCustomProvider("openai-for-update"))

	route := &domain.Route{
		ProviderID: codexProvider.ID,
		ClientType: domain.ClientTypeCodex,
		ProjectID:  0,
		Position:   1,
		Weight:     1,
		IsEnabled:  true,
	}
	if err := svc.CreateRoute(domain.DefaultTenantID, route); err != nil {
		t.Fatalf("CreateRoute() error = %v", err)
	}
	if !route.IsNative {
		t.Fatalf("initial IsNative = false, want true")
	}

	route.ProviderID = openaiProvider.ID
	route.IsNative = true // client false claim must still be recomputed
	if err := svc.UpdateRoute(domain.DefaultTenantID, route); err != nil {
		t.Fatalf("UpdateRoute() error = %v", err)
	}
	if route.IsNative {
		t.Fatalf("after provider change IsNative = true, want false")
	}

	stored, err := routeRepo.GetByID(domain.DefaultTenantID, route.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if stored.ProviderID != openaiProvider.ID {
		t.Fatalf("stored ProviderID = %d, want %d", stored.ProviderID, openaiProvider.ID)
	}
	if stored.IsNative {
		t.Fatalf("stored IsNative = true, want false")
	}

	route.ProviderID = codexProvider.ID
	route.IsNative = false
	if err := svc.UpdateRoute(domain.DefaultTenantID, route); err != nil {
		t.Fatalf("UpdateRoute(back to codex) error = %v", err)
	}
	if !route.IsNative {
		t.Fatalf("after switch back to codex IsNative = false, want true")
	}
}

func TestUpdateRouteRecomputesNativeWhenClientTypeChanges(t *testing.T) {
	svc, providerRepo, routeRepo, _ := setupRouteNativeTestEnv(t)

	codexProvider := mustCreateProvider(t, providerRepo, newTestCodexProvider("codex-client-type"))

	route := &domain.Route{
		ProviderID: codexProvider.ID,
		ClientType: domain.ClientTypeCodex,
		ProjectID:  0,
		Position:   1,
		Weight:     1,
		IsEnabled:  true,
	}
	if err := svc.CreateRoute(domain.DefaultTenantID, route); err != nil {
		t.Fatalf("CreateRoute() error = %v", err)
	}
	if !route.IsNative {
		t.Fatalf("initial IsNative = false, want true")
	}

	route.ClientType = domain.ClientTypeOpenAI
	route.IsNative = true
	if err := svc.UpdateRoute(domain.DefaultTenantID, route); err != nil {
		t.Fatalf("UpdateRoute() error = %v", err)
	}
	if route.IsNative {
		t.Fatalf("after ClientType→openai IsNative = true, want false")
	}

	stored, err := routeRepo.GetByID(domain.DefaultTenantID, route.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if stored.ClientType != domain.ClientTypeOpenAI {
		t.Fatalf("stored ClientType = %s, want %s", stored.ClientType, domain.ClientTypeOpenAI)
	}
	if stored.IsNative {
		t.Fatalf("stored IsNative = true, want false")
	}

	route.ClientType = domain.ClientTypeCodex
	route.IsNative = false
	if err := svc.UpdateRoute(domain.DefaultTenantID, route); err != nil {
		t.Fatalf("UpdateRoute(back to codex) error = %v", err)
	}
	if !route.IsNative {
		t.Fatalf("after ClientType→codex IsNative = false, want true")
	}
}

func TestGetRoutesReturnsCanonicalCopies(t *testing.T) {
	svc, providerRepo, routeRepo, _ := setupRouteNativeTestEnv(t)

	codexProvider := mustCreateProvider(t, providerRepo, newTestCodexProvider("codex-get-routes"))
	openaiProvider := mustCreateProvider(t, providerRepo, newTestOpenAIOnlyCustomProvider("openai-get-routes"))

	// Seed stale is_native snapshots directly in the repository.
	staleNativeFalse := &domain.Route{
		TenantID:   domain.DefaultTenantID,
		ProviderID: codexProvider.ID,
		ClientType: domain.ClientTypeCodex,
		ProjectID:  0,
		Position:   1,
		Weight:     1,
		IsEnabled:  true,
		IsNative:   false,
	}
	staleNativeTrue := &domain.Route{
		TenantID:   domain.DefaultTenantID,
		ProviderID: openaiProvider.ID,
		ClientType: domain.ClientTypeCodex,
		ProjectID:  0,
		Position:   2,
		Weight:     1,
		IsEnabled:  true,
		IsNative:   true,
	}
	if err := routeRepo.Create(staleNativeFalse); err != nil {
		t.Fatalf("Create(stale false) error = %v", err)
	}
	if err := routeRepo.Create(staleNativeTrue); err != nil {
		t.Fatalf("Create(stale true) error = %v", err)
	}

	routes, err := svc.GetRoutes(domain.DefaultTenantID)
	if err != nil {
		t.Fatalf("GetRoutes() error = %v", err)
	}
	if len(routes) != 2 {
		t.Fatalf("GetRoutes() len = %d, want 2", len(routes))
	}

	byProvider := make(map[uint64]*domain.Route, len(routes))
	for _, route := range routes {
		byProvider[route.ProviderID] = route
	}

	if got := byProvider[codexProvider.ID]; got == nil || !got.IsNative {
		t.Fatalf("codex route IsNative = %v, want true (canonical)", got)
	}
	if got := byProvider[openaiProvider.ID]; got == nil || got.IsNative {
		t.Fatalf("openai→codex route IsNative = %v, want false (canonical)", got)
	}

	// Mutating the returned slice must not change the next GetRoutes result.
	byProvider[codexProvider.ID].IsNative = false
	byProvider[openaiProvider.ID].IsNative = true
	byProvider[codexProvider.ID].Weight = 99

	routes2, err := svc.GetRoutes(domain.DefaultTenantID)
	if err != nil {
		t.Fatalf("GetRoutes() second call error = %v", err)
	}
	byProvider2 := make(map[uint64]*domain.Route, len(routes2))
	for _, route := range routes2 {
		byProvider2[route.ProviderID] = route
	}
	if !byProvider2[codexProvider.ID].IsNative {
		t.Fatal("second GetRoutes codex IsNative mutated via previous return value")
	}
	if byProvider2[openaiProvider.ID].IsNative {
		t.Fatal("second GetRoutes conversion IsNative mutated via previous return value")
	}
	if byProvider2[codexProvider.ID].Weight != 1 {
		t.Fatalf("second GetRoutes Weight = %d, want 1", byProvider2[codexProvider.ID].Weight)
	}
}

func TestGetRouteDoesNotExposeCachedPointer(t *testing.T) {
	svc, providerRepo, _, _ := setupRouteNativeTestEnv(t)

	codexProvider := mustCreateProvider(t, providerRepo, newTestCodexProvider("codex-get-route-ptr"))

	route := &domain.Route{
		ProviderID: codexProvider.ID,
		ClientType: domain.ClientTypeCodex,
		ProjectID:  0,
		Position:   1,
		Weight:     1,
		IsEnabled:  true,
	}
	if err := svc.CreateRoute(domain.DefaultTenantID, route); err != nil {
		t.Fatalf("CreateRoute() error = %v", err)
	}

	first, err := svc.GetRoute(domain.DefaultTenantID, route.ID)
	if err != nil {
		t.Fatalf("GetRoute() first error = %v", err)
	}
	if !first.IsNative {
		t.Fatalf("first IsNative = false, want true")
	}

	first.IsNative = false
	first.Weight = 42
	first.IsEnabled = false
	first.ProviderID = 999999

	second, err := svc.GetRoute(domain.DefaultTenantID, route.ID)
	if err != nil {
		t.Fatalf("GetRoute() second error = %v", err)
	}
	if second == first {
		t.Fatal("GetRoute returned same pointer twice")
	}
	if !second.IsNative {
		t.Fatal("mutating first GetRoute result changed next GetRoute IsNative")
	}
	if second.Weight != 1 {
		t.Fatalf("second Weight = %d, want 1", second.Weight)
	}
	if !second.IsEnabled {
		t.Fatal("second IsEnabled = false, want true")
	}
	if second.ProviderID != codexProvider.ID {
		t.Fatalf("second ProviderID = %d, want %d", second.ProviderID, codexProvider.ID)
	}
}

func TestProviderUpdateReconcilesRouteNativeSnapshot(t *testing.T) {
	svc, providerRepo, routeRepo, _ := setupRouteNativeTestEnv(t)

	provider := mustCreateProvider(t, providerRepo, newTestOpenAIOnlyCustomProvider("reconcile-provider"))

	// Stale snapshot: custom openai-only provider with a codex route marked native.
	route := &domain.Route{
		TenantID:   domain.DefaultTenantID,
		ProviderID: provider.ID,
		ClientType: domain.ClientTypeCodex,
		ProjectID:  0,
		Position:   1,
		Weight:     1,
		IsEnabled:  true,
		IsNative:   true,
	}
	if err := routeRepo.Create(route); err != nil {
		t.Fatalf("Create(route) error = %v", err)
	}

	// Re-load provider for UpdateProvider so write-only fields are preserved.
	existing, err := providerRepo.GetByID(domain.DefaultTenantID, provider.ID)
	if err != nil {
		t.Fatalf("GetByID(provider) error = %v", err)
	}
	existing.SupportedClientTypes = []domain.ClientType{domain.ClientTypeOpenAI}
	if err := svc.UpdateProvider(domain.DefaultTenantID, existing); err != nil {
		t.Fatalf("UpdateProvider() error = %v", err)
	}

	stored, err := routeRepo.GetByID(domain.DefaultTenantID, route.ID)
	if err != nil {
		t.Fatalf("GetByID(route) error = %v", err)
	}
	if stored.IsNative {
		t.Fatal("after reconcile with openai-only provider, IsNative still true, want false")
	}

	// Expand capability to natively support codex; snapshot must flip to true.
	existing, err = providerRepo.GetByID(domain.DefaultTenantID, provider.ID)
	if err != nil {
		t.Fatalf("GetByID(provider) reload error = %v", err)
	}
	existing.SupportedClientTypes = []domain.ClientType{domain.ClientTypeCodex}
	if err := svc.UpdateProvider(domain.DefaultTenantID, existing); err != nil {
		t.Fatalf("UpdateProvider(codex capable) error = %v", err)
	}

	stored, err = routeRepo.GetByID(domain.DefaultTenantID, route.ID)
	if err != nil {
		t.Fatalf("GetByID(route) after codex expand error = %v", err)
	}
	if !stored.IsNative {
		t.Fatal("after reconcile with codex-capable provider, IsNative still false, want true")
	}

	// API read path should also report the canonical value.
	got, err := svc.GetRoute(domain.DefaultTenantID, route.ID)
	if err != nil {
		t.Fatalf("GetRoute() error = %v", err)
	}
	if !got.IsNative {
		t.Fatal("GetRoute IsNative = false after reconcile, want true")
	}
}

func TestSyncRoutesRecomputesNative(t *testing.T) {
	svc, providerRepo, routeRepo, projectRepo := setupRouteNativeTestEnv(t)

	codexProvider := mustCreateProvider(t, providerRepo, newTestCodexProvider("codex-sync"))
	openaiProvider := mustCreateProvider(t, providerRepo, newTestOpenAIOnlyCustomProvider("openai-sync"))

	// Global source routes with intentionally wrong is_native snapshots.
	sourceNative := &domain.Route{
		TenantID:   domain.DefaultTenantID,
		ProviderID: codexProvider.ID,
		ClientType: domain.ClientTypeCodex,
		ProjectID:  0,
		Position:   1,
		Weight:     2,
		IsEnabled:  true,
		IsNative:   false,
	}
	sourceConversion := &domain.Route{
		TenantID:   domain.DefaultTenantID,
		ProviderID: openaiProvider.ID,
		ClientType: domain.ClientTypeCodex,
		ProjectID:  0,
		Position:   2,
		Weight:     1,
		IsEnabled:  true,
		IsNative:   true,
	}
	if err := routeRepo.Create(sourceNative); err != nil {
		t.Fatalf("Create(source native) error = %v", err)
	}
	if err := routeRepo.Create(sourceConversion); err != nil {
		t.Fatalf("Create(source conversion) error = %v", err)
	}

	// Target project starts with a stale conversion route that should be updated.
	targetProject := &domain.Project{
		TenantID: domain.DefaultTenantID,
		Name:     "sync-target",
		Slug:     "sync-target",
	}
	if err := projectRepo.Create(targetProject); err != nil {
		t.Fatalf("Create(project) error = %v", err)
	}

	staleTarget := &domain.Route{
		TenantID:   domain.DefaultTenantID,
		ProviderID: openaiProvider.ID,
		ClientType: domain.ClientTypeCodex,
		ProjectID:  targetProject.ID,
		Position:   9,
		Weight:     9,
		IsEnabled:  false,
		IsNative:   true,
	}
	if err := routeRepo.Create(staleTarget); err != nil {
		t.Fatalf("Create(stale target) error = %v", err)
	}

	result, err := svc.SyncRoutesFromProject(domain.DefaultTenantID, domain.RouteSyncRequest{
		SourceProjectID: 0,
		TargetProjectID: targetProject.ID,
		ClientType:      domain.ClientTypeCodex,
		Mode:            domain.RouteSyncModeOverwrite,
	})
	if err != nil {
		t.Fatalf("SyncRoutesFromProject() error = %v", err)
	}
	if result.CreatedCount != 1 || result.UpdatedCount != 1 {
		t.Fatalf("result created/updated = %d/%d, want 1/1: %+v", result.CreatedCount, result.UpdatedCount, result)
	}
	if len(result.Routes) != 2 {
		t.Fatalf("result.Routes len = %d, want 2", len(result.Routes))
	}

	byProvider := make(map[uint64]*domain.Route, len(result.Routes))
	for _, route := range result.Routes {
		byProvider[route.ProviderID] = route
	}
	if got := byProvider[codexProvider.ID]; got == nil || !got.IsNative {
		t.Fatalf("synced codex route IsNative = %v, want true", got)
	}
	if got := byProvider[openaiProvider.ID]; got == nil || got.IsNative {
		t.Fatalf("synced openai→codex route IsNative = %v, want false", got)
	}

	// Persist layer must also hold recomputed snapshots.
	targetRoutes, err := routeRepo.List(domain.DefaultTenantID)
	if err != nil {
		t.Fatalf("List(routes) error = %v", err)
	}
	var codexTarget, openaiTarget *domain.Route
	for _, route := range targetRoutes {
		if route.ProjectID != targetProject.ID || route.ClientType != domain.ClientTypeCodex {
			continue
		}
		switch route.ProviderID {
		case codexProvider.ID:
			codexTarget = route
		case openaiProvider.ID:
			openaiTarget = route
		}
	}
	if codexTarget == nil || !codexTarget.IsNative {
		t.Fatalf("persisted codex target = %+v, want IsNative true", codexTarget)
	}
	if openaiTarget == nil || openaiTarget.IsNative {
		t.Fatalf("persisted openai target = %+v, want IsNative false", openaiTarget)
	}
}
