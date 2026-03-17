package service

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
)

func newBackupServiceTestDB(t *testing.T, name string) *sqlite.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), name)
	db, err := sqlite.NewDB(dbPath)
	if err != nil {
		t.Fatalf("create test db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func newBackupServiceForTest(t *testing.T, db *sqlite.DB) *BackupService {
	t.Helper()

	return NewBackupService(
		sqlite.NewProviderRepository(db),
		sqlite.NewRouteRepository(db),
		sqlite.NewProjectRepository(db),
		sqlite.NewRetryConfigRepository(db),
		sqlite.NewRoutingStrategyRepository(db),
		sqlite.NewSystemSettingRepository(db),
		sqlite.NewAPITokenRepository(db),
		sqlite.NewModelMappingRepository(db),
		sqlite.NewModelPriceRepository(db),
		nil,
	)
}

func seedBackupRoundtripData(t *testing.T, db *sqlite.DB) {
	t.Helper()

	settingRepo := sqlite.NewSystemSettingRepository(db)
	providerRepo := sqlite.NewProviderRepository(db)
	projectRepo := sqlite.NewProjectRepository(db)
	retryConfigRepo := sqlite.NewRetryConfigRepository(db)
	routeRepo := sqlite.NewRouteRepository(db)
	routingStrategyRepo := sqlite.NewRoutingStrategyRepository(db)
	apiTokenRepo := sqlite.NewAPITokenRepository(db)
	modelMappingRepo := sqlite.NewModelMappingRepository(db)
	modelPriceRepo := sqlite.NewModelPriceRepository(db)

	if err := settingRepo.Set("timezone", "UTC"); err != nil {
		t.Fatalf("seed system setting: %v", err)
	}

	provider := &domain.Provider{
		TenantID: domain.DefaultTenantID,
		Name:     "p-custom",
		Type:     "custom",
		Logo:     "https://example.com/logo.png",
		Config: &domain.ProviderConfig{
			Custom: &domain.ProviderConfigCustom{
				BaseURL: "https://api.example.com/v1",
				APIKey:  "secret-key",
			},
		},
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeOpenAI, domain.ClientTypeClaude},
		SupportModels:        []string{"gpt-4o*", "claude-*"},
	}
	if err := providerRepo.Create(provider); err != nil {
		t.Fatalf("seed provider: %v", err)
	}

	project := &domain.Project{
		TenantID:            domain.DefaultTenantID,
		Name:                "Project One",
		Slug:                "project-one",
		EnabledCustomRoutes: []domain.ClientType{domain.ClientTypeOpenAI},
	}
	if err := projectRepo.Create(project); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	retryConfig := &domain.RetryConfig{
		TenantID:        domain.DefaultTenantID,
		Name:            "retry-fast",
		IsDefault:       true,
		MaxRetries:      3,
		InitialInterval: 100 * time.Millisecond,
		BackoffRate:     2.0,
		MaxInterval:     800 * time.Millisecond,
	}
	if err := retryConfigRepo.Create(retryConfig); err != nil {
		t.Fatalf("seed retry config: %v", err)
	}

	route := &domain.Route{
		TenantID:      domain.DefaultTenantID,
		IsEnabled:     true,
		IsNative:      false,
		ProjectID:     project.ID,
		ClientType:    domain.ClientTypeOpenAI,
		ProviderID:    provider.ID,
		Position:      7,
		RetryConfigID: retryConfig.ID,
	}
	if err := routeRepo.Create(route); err != nil {
		t.Fatalf("seed route: %v", err)
	}

	routingStrategy := &domain.RoutingStrategy{
		TenantID:  domain.DefaultTenantID,
		ProjectID: project.ID,
		Type:      domain.RoutingStrategyPriority,
		Config:    &domain.RoutingStrategyConfig{},
	}
	if err := routingStrategyRepo.Create(routingStrategy); err != nil {
		t.Fatalf("seed routing strategy: %v", err)
	}

	apiToken := &domain.APIToken{
		TenantID:    domain.DefaultTenantID,
		Token:       "maxx_test_token_abc",
		TokenPrefix: "maxx_test...",
		Name:        "token-main",
		Description: "main token",
		ProjectID:   project.ID,
		IsEnabled:   true,
	}
	if err := apiTokenRepo.Create(apiToken); err != nil {
		t.Fatalf("seed api token: %v", err)
	}

	modelMapping := &domain.ModelMapping{
		TenantID:     domain.DefaultTenantID,
		Scope:        domain.ModelMappingScopeRoute,
		ClientType:   domain.ClientTypeOpenAI,
		ProviderType: "custom",
		ProviderID:   provider.ID,
		ProjectID:    project.ID,
		RouteID:      route.ID,
		APITokenID:   apiToken.ID,
		Pattern:      "gpt-4o",
		Target:       "gpt-4.1",
		Priority:     10,
	}
	if err := modelMappingRepo.Create(modelMapping); err != nil {
		t.Fatalf("seed model mapping: %v", err)
	}

	modelPrice := &domain.ModelPrice{
		ModelID:                "gpt-4.1",
		InputPriceMicro:        2000000,
		OutputPriceMicro:       8000000,
		CacheReadPriceMicro:    100000,
		Cache5mWritePriceMicro: 250000,
		Cache1hWritePriceMicro: 500000,
		Has1MContext:           true,
		Context1MThreshold:     1000000,
		InputPremiumNum:        2,
		InputPremiumDenom:      1,
		OutputPremiumNum:       3,
		OutputPremiumDenom:     2,
	}
	if err := modelPriceRepo.Create(modelPrice); err != nil {
		t.Fatalf("seed model price: %v", err)
	}
}

func TestBackupService_ExportImportRoundtrip_PreservesCoreConfig(t *testing.T) {
	sourceDB := newBackupServiceTestDB(t, "source.db")
	seedBackupRoundtripData(t, sourceDB)

	sourceSvc := newBackupServiceForTest(t, sourceDB)
	backup, err := sourceSvc.Export(domain.DefaultTenantID)
	if err != nil {
		t.Fatalf("export backup: %v", err)
	}

	targetDB := newBackupServiceTestDB(t, "target.db")
	targetSvc := newBackupServiceForTest(t, targetDB)

	result, err := targetSvc.Import(domain.DefaultTenantID, backup, domain.ImportOptions{ConflictStrategy: "skip"})
	if err != nil {
		t.Fatalf("import backup: %v", err)
	}
	if !result.Success {
		t.Fatalf("import result success=false, errors=%v", result.Errors)
	}

	roundtrip, err := targetSvc.Export(domain.DefaultTenantID)
	if err != nil {
		t.Fatalf("re-export backup: %v", err)
	}

	if len(roundtrip.Data.Providers) != 1 {
		t.Fatalf("providers count = %d, want 1", len(roundtrip.Data.Providers))
	}
	if roundtrip.Data.Providers[0].Logo != "https://example.com/logo.png" {
		t.Fatalf("provider logo = %q, want preserved", roundtrip.Data.Providers[0].Logo)
	}

	if len(roundtrip.Data.ModelPrices) != 1 {
		t.Fatalf("modelPrices count = %d, want 1", len(roundtrip.Data.ModelPrices))
	}
	mp := roundtrip.Data.ModelPrices[0]
	if mp.ModelID != "gpt-4.1" || mp.InputPriceMicro != 2000000 || mp.OutputPriceMicro != 8000000 {
		t.Fatalf("model price not preserved: %+v", mp)
	}

	if len(roundtrip.Data.APITokens) != 1 {
		t.Fatalf("apiTokens count = %d, want 1", len(roundtrip.Data.APITokens))
	}
	if roundtrip.Data.APITokens[0].Token != "maxx_test_token_abc" {
		t.Fatalf("api token not restored, got %q", roundtrip.Data.APITokens[0].Token)
	}

	foundCustomMapping := 0
	for _, mapping := range roundtrip.Data.ModelMappings {
		if mapping.Pattern == "gpt-4o" && mapping.Target == "gpt-4.1" {
			foundCustomMapping++
			if mapping.RouteName == "" {
				t.Fatalf("model mapping route reference lost: %+v", mapping)
			}
		}
	}
	if foundCustomMapping != 1 {
		t.Fatalf("custom model mapping count = %d, want 1", foundCustomMapping)
	}
}

func TestBackupService_Export_SkipsReferencesToExcludedProviders(t *testing.T) {
	db := newBackupServiceTestDB(t, "excluded-provider.db")
	providerRepo := sqlite.NewProviderRepository(db)
	projectRepo := sqlite.NewProjectRepository(db)
	routeRepo := sqlite.NewRouteRepository(db)
	modelMappingRepo := sqlite.NewModelMappingRepository(db)

	includedProvider := &domain.Provider{
		TenantID: domain.DefaultTenantID,
		Name:     "included-provider",
		Type:     "custom",
		Config: &domain.ProviderConfig{
			Custom: &domain.ProviderConfigCustom{
				BaseURL: "https://api.included.example",
				APIKey:  "included-key",
			},
		},
	}
	if err := providerRepo.Create(includedProvider); err != nil {
		t.Fatalf("create included provider: %v", err)
	}

	excludedProvider := &domain.Provider{
		TenantID:          domain.DefaultTenantID,
		Name:              "excluded-provider",
		Type:              "custom",
		ExcludeFromExport: true,
		Config: &domain.ProviderConfig{
			Custom: &domain.ProviderConfigCustom{
				BaseURL: "https://api.excluded.example",
				APIKey:  "excluded-key",
			},
		},
	}
	if err := providerRepo.Create(excludedProvider); err != nil {
		t.Fatalf("create excluded provider: %v", err)
	}

	project := &domain.Project{
		TenantID: domain.DefaultTenantID,
		Name:     "Project One",
		Slug:     "project-one",
	}
	if err := projectRepo.Create(project); err != nil {
		t.Fatalf("create project: %v", err)
	}

	includedRoute := &domain.Route{
		TenantID:   domain.DefaultTenantID,
		IsEnabled:  true,
		ClientType: domain.ClientTypeOpenAI,
		ProviderID: includedProvider.ID,
		ProjectID:  project.ID,
		Position:   1,
	}
	if err := routeRepo.Create(includedRoute); err != nil {
		t.Fatalf("create included route: %v", err)
	}

	excludedRoute := &domain.Route{
		TenantID:   domain.DefaultTenantID,
		IsEnabled:  true,
		ClientType: domain.ClientTypeOpenAI,
		ProviderID: excludedProvider.ID,
		ProjectID:  project.ID,
		Position:   2,
	}
	if err := routeRepo.Create(excludedRoute); err != nil {
		t.Fatalf("create excluded route: %v", err)
	}

	includedMapping := &domain.ModelMapping{
		TenantID:     domain.DefaultTenantID,
		Scope:        domain.ModelMappingScopeRoute,
		ClientType:   domain.ClientTypeOpenAI,
		ProviderType: "custom",
		ProviderID:   includedProvider.ID,
		ProjectID:    project.ID,
		RouteID:      includedRoute.ID,
		Pattern:      "keep-me",
		Target:       "gpt-4.1",
		Priority:     1,
	}
	if err := modelMappingRepo.Create(includedMapping); err != nil {
		t.Fatalf("create included mapping: %v", err)
	}

	excludedProviderMapping := &domain.ModelMapping{
		TenantID:     domain.DefaultTenantID,
		Scope:        domain.ModelMappingScopeProvider,
		ProviderType: "custom",
		ProviderID:   excludedProvider.ID,
		Pattern:      "drop-provider-ref",
		Target:       "gpt-4.1-mini",
		Priority:     2,
	}
	if err := modelMappingRepo.Create(excludedProviderMapping); err != nil {
		t.Fatalf("create excluded provider mapping: %v", err)
	}

	excludedRouteMapping := &domain.ModelMapping{
		TenantID:   domain.DefaultTenantID,
		Scope:      domain.ModelMappingScopeRoute,
		ClientType: domain.ClientTypeOpenAI,
		RouteID:    excludedRoute.ID,
		Pattern:    "drop-route-ref",
		Target:     "gpt-4.1-nano",
		Priority:   3,
	}
	if err := modelMappingRepo.Create(excludedRouteMapping); err != nil {
		t.Fatalf("create excluded route mapping: %v", err)
	}

	svc := newBackupServiceForTest(t, db)
	backup, err := svc.Export(domain.DefaultTenantID)
	if err != nil {
		t.Fatalf("export backup: %v", err)
	}

	if len(backup.Data.Providers) != 1 || backup.Data.Providers[0].Name != "included-provider" {
		t.Fatalf("providers = %+v, want only included provider", backup.Data.Providers)
	}
	if len(backup.Data.Routes) != 1 || backup.Data.Routes[0].ProviderName != "included-provider" {
		t.Fatalf("routes = %+v, want only included route", backup.Data.Routes)
	}
	if len(backup.Data.ModelMappings) != 1 || backup.Data.ModelMappings[0].Pattern != "keep-me" {
		t.Fatalf("modelMappings = %+v, want only included mapping", backup.Data.ModelMappings)
	}
	if backup.Data.ModelMappings[0].RouteName == "" {
		t.Fatalf("expected exported mapping to retain route reference: %+v", backup.Data.ModelMappings[0])
	}
}

func TestBackupService_Import_ModelMappingsSkipDuplicates(t *testing.T) {
	db := newBackupServiceTestDB(t, "dupe.db")
	seedBackupRoundtripData(t, db)

	svc := newBackupServiceForTest(t, db)
	backup, err := svc.Export(domain.DefaultTenantID)
	if err != nil {
		t.Fatalf("export backup: %v", err)
	}

	result, err := svc.Import(domain.DefaultTenantID, backup, domain.ImportOptions{ConflictStrategy: "skip"})
	if err != nil {
		t.Fatalf("import backup: %v", err)
	}

	mappingSummary, ok := result.Summary["modelMappings"]
	if !ok {
		t.Fatalf("missing modelMappings summary: %+v", result.Summary)
	}
	if mappingSummary.Skipped == 0 {
		t.Fatalf("expected duplicate model mapping skip, got summary=%+v", mappingSummary)
	}
}

func TestBuildModelMappingKey_NoSeparatorCollision(t *testing.T) {
	left := domain.BackupModelMapping{
		Scope:        domain.ModelMappingScopeGlobal,
		ProviderName: "a|b",
		Pattern:      "foo",
		Target:       "bar",
		Priority:     1,
	}
	right := domain.BackupModelMapping{
		Scope:        domain.ModelMappingScopeGlobal,
		ProviderName: "a",
		Pattern:      "b|foo",
		Target:       "bar",
		Priority:     1,
	}

	leftKey := buildModelMappingKey(left)
	rightKey := buildModelMappingKey(right)

	if leftKey == "" || rightKey == "" {
		t.Fatalf("mapping key should not be empty")
	}
	if leftKey == rightKey {
		t.Fatalf("mapping keys should differ, left=%q right=%q", leftKey, rightKey)
	}
}
