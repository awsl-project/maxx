package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "github.com/awsl-project/maxx/internal/adapter/provider/custom"
	"github.com/awsl-project/maxx/internal/cooldown"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository/cached"
	"github.com/awsl-project/maxx/internal/router"
)

type modelAvailabilityRouteRepo struct{ routes []*domain.Route }

func (r *modelAvailabilityRouteRepo) Create(route *domain.Route) error        { return nil }
func (r *modelAvailabilityRouteRepo) Update(route *domain.Route) error        { return nil }
func (r *modelAvailabilityRouteRepo) Delete(tenantID uint64, id uint64) error { return nil }
func (r *modelAvailabilityRouteRepo) BulkDelete(tenantID uint64, req domain.RouteBulkDeleteRequest) (*domain.RouteBulkDeleteResult, error) {
	return nil, nil
}
func (r *modelAvailabilityRouteRepo) GetByID(tenantID uint64, id uint64) (*domain.Route, error) {
	return nil, domain.ErrNotFound
}
func (r *modelAvailabilityRouteRepo) FindByKey(tenantID uint64, projectID, providerID uint64, clientType domain.ClientType) (*domain.Route, error) {
	return nil, domain.ErrNotFound
}
func (r *modelAvailabilityRouteRepo) List(tenantID uint64) ([]*domain.Route, error) {
	return append([]*domain.Route(nil), r.routes...), nil
}
func (r *modelAvailabilityRouteRepo) BatchUpdatePositions(tenantID uint64, updates []domain.RoutePositionUpdate) error {
	return nil
}

type modelAvailabilityProviderRepo struct{ providers []*domain.Provider }

func (r *modelAvailabilityProviderRepo) Create(provider *domain.Provider) error  { return nil }
func (r *modelAvailabilityProviderRepo) Update(provider *domain.Provider) error  { return nil }
func (r *modelAvailabilityProviderRepo) Delete(tenantID uint64, id uint64) error { return nil }
func (r *modelAvailabilityProviderRepo) GetByID(tenantID uint64, id uint64) (*domain.Provider, error) {
	for _, p := range r.providers {
		if p.ID == id && (tenantID == domain.TenantIDAll || p.TenantID == tenantID) {
			return p, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *modelAvailabilityProviderRepo) List(tenantID uint64) ([]*domain.Provider, error) {
	return append([]*domain.Provider(nil), r.providers...), nil
}

type modelAvailabilityStrategyRepo struct{}

func (r *modelAvailabilityStrategyRepo) Create(strategy *domain.RoutingStrategy) error { return nil }
func (r *modelAvailabilityStrategyRepo) Update(strategy *domain.RoutingStrategy) error { return nil }
func (r *modelAvailabilityStrategyRepo) Delete(tenantID uint64, id uint64) error       { return nil }
func (r *modelAvailabilityStrategyRepo) GetByID(tenantID uint64, id uint64) (*domain.RoutingStrategy, error) {
	return nil, domain.ErrNotFound
}
func (r *modelAvailabilityStrategyRepo) GetByProjectID(tenantID uint64, projectID uint64) (*domain.RoutingStrategy, error) {
	return nil, domain.ErrNotFound
}
func (r *modelAvailabilityStrategyRepo) List(tenantID uint64) ([]*domain.RoutingStrategy, error) {
	return nil, nil
}

type modelAvailabilityRetryRepo struct{}

func (r *modelAvailabilityRetryRepo) Create(config *domain.RetryConfig) error { return nil }
func (r *modelAvailabilityRetryRepo) Update(config *domain.RetryConfig) error { return nil }
func (r *modelAvailabilityRetryRepo) Delete(tenantID uint64, id uint64) error { return nil }
func (r *modelAvailabilityRetryRepo) GetByID(tenantID uint64, id uint64) (*domain.RetryConfig, error) {
	return nil, domain.ErrNotFound
}
func (r *modelAvailabilityRetryRepo) GetDefault(tenantID uint64) (*domain.RetryConfig, error) {
	return nil, domain.ErrNotFound
}
func (r *modelAvailabilityRetryRepo) List(tenantID uint64) ([]*domain.RetryConfig, error) {
	return nil, nil
}

type modelAvailabilityProjectRepo struct{ projects []*domain.Project }

func (r *modelAvailabilityProjectRepo) Create(project *domain.Project) error    { return nil }
func (r *modelAvailabilityProjectRepo) Update(project *domain.Project) error    { return nil }
func (r *modelAvailabilityProjectRepo) Delete(tenantID uint64, id uint64) error { return nil }
func (r *modelAvailabilityProjectRepo) GetByID(tenantID uint64, id uint64) (*domain.Project, error) {
	for _, p := range r.projects {
		if p.ID == id && (tenantID == domain.TenantIDAll || p.TenantID == tenantID) {
			return p, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *modelAvailabilityProjectRepo) GetBySlug(tenantID uint64, slug string) (*domain.Project, error) {
	return nil, domain.ErrNotFound
}
func (r *modelAvailabilityProjectRepo) List(tenantID uint64) ([]*domain.Project, error) {
	return append([]*domain.Project(nil), r.projects...), nil
}

func newModelAvailabilityRouter(t *testing.T, providers []*domain.Provider, routes []*domain.Route, projects []*domain.Project) (*router.Router, *cached.ProviderRepository) {
	t.Helper()
	providerRepo := cached.NewProviderRepository(&modelAvailabilityProviderRepo{providers: providers})
	routeRepo := cached.NewRouteRepository(&modelAvailabilityRouteRepo{routes: routes})
	strategyRepo := cached.NewRoutingStrategyRepository(&modelAvailabilityStrategyRepo{})
	retryRepo := cached.NewRetryConfigRepository(&modelAvailabilityRetryRepo{})
	projectRepo := cached.NewProjectRepository(&modelAvailabilityProjectRepo{projects: projects})
	for _, loader := range []struct {
		name string
		load func() error
	}{
		{"providers", providerRepo.Load},
		{"routes", routeRepo.Load},
		{"strategies", strategyRepo.Load},
		{"retries", retryRepo.Load},
		{"projects", projectRepo.Load},
	} {
		if err := loader.load(); err != nil {
			t.Fatalf("load %s: %v", loader.name, err)
		}
	}
	r := router.NewRouter(routeRepo, providerRepo, strategyRepo, retryRepo, projectRepo)
	if err := r.InitAdapters(); err != nil {
		t.Fatalf("InitAdapters: %v", err)
	}
	return r, providerRepo
}

func testCustomProvider(id uint64, name string, models []string) *domain.Provider {
	return &domain.Provider{
		ID:                   id,
		TenantID:             1,
		Name:                 name,
		Type:                 "custom",
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeOpenAI},
		SupportModels:        models,
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL: "https://example.invalid/v1",
			APIKey:  "test-key",
		}},
	}
}

func decodeOpenAIModelIDs(t *testing.T, body []byte) []string {
	t.Helper()
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("invalid model list payload: %v", err)
	}
	ids := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		ids = append(ids, item.ID)
	}
	return ids
}

func TestModelsHandlerFiltersUnavailableModelsByRouteMatch(t *testing.T) {
	providers := []*domain.Provider{
		testCustomProvider(10, "available", []string{"gpt-live", "gpt-cooldown"}),
		testCustomProvider(11, "disabled", []string{"gpt-disabled"}),
	}
	routes := []*domain.Route{
		{ID: 1, TenantID: 1, ProviderID: 10, ClientType: domain.ClientTypeOpenAI, ProjectID: 0, IsEnabled: true, Position: 1, Weight: 1},
		{ID: 2, TenantID: 1, ProviderID: 11, ClientType: domain.ClientTypeOpenAI, ProjectID: 0, IsEnabled: false, Position: 2, Weight: 1},
	}
	r, providerRepo := newModelAvailabilityRouter(t, providers, routes, nil)
	handler := NewModelsHandler(&fakeResponseModelRepo{names: []string{"gpt-live", "gpt-cooldown", "gpt-disabled", "gpt-history-only"}}, providerRepo, nil, r)

	cooldown.Default().SetCooldownUntil(10, string(domain.ClientTypeOpenAI), "gpt-cooldown", time.Now().Add(time.Minute))
	defer cooldown.Default().ClearCooldown(10, string(domain.ClientTypeOpenAI), "gpt-cooldown")

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("User-Agent", "codex_cli_rs/0.98.0")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	ids := decodeOpenAIModelIDs(t, rec.Body.Bytes())
	if !containsModel(ids, "gpt-live") {
		t.Fatalf("ids = %v, want gpt-live", ids)
	}
	for _, unavailable := range []string{"gpt-cooldown", "gpt-disabled", "gpt-history-only"} {
		if containsModel(ids, unavailable) {
			t.Fatalf("ids = %v, did not want %s", ids, unavailable)
		}
	}
}

func TestModelsHandlerFiltersProviderScopedModelList(t *testing.T) {
	providers := []*domain.Provider{
		testCustomProvider(10, "provider-a", []string{"gpt-a"}),
		testCustomProvider(20, "provider-b", []string{"gpt-b"}),
	}
	routes := []*domain.Route{
		{ID: 1, TenantID: 1, ProviderID: 10, ClientType: domain.ClientTypeOpenAI, ProjectID: 0, IsEnabled: true, Position: 1, Weight: 1},
		{ID: 2, TenantID: 1, ProviderID: 20, ClientType: domain.ClientTypeOpenAI, ProjectID: 0, IsEnabled: true, Position: 2, Weight: 1},
	}
	r, providerRepo := newModelAvailabilityRouter(t, providers, routes, nil)
	handler := NewModelsHandler(&fakeResponseModelRepo{names: []string{"gpt-a", "gpt-b"}}, providerRepo, nil, r)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("X-Maxx-Provider-ID", "20")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	ids := decodeOpenAIModelIDs(t, rec.Body.Bytes())
	if containsModel(ids, "gpt-a") || !containsModel(ids, "gpt-b") {
		t.Fatalf("ids = %v, want only provider-b model", ids)
	}
}

func TestModelsHandlerFiltersProjectScopedModelList(t *testing.T) {
	providers := []*domain.Provider{
		testCustomProvider(10, "global", []string{"gpt-global"}),
		testCustomProvider(20, "project", []string{"gpt-project"}),
	}
	routes := []*domain.Route{
		{ID: 1, TenantID: 1, ProviderID: 10, ClientType: domain.ClientTypeOpenAI, ProjectID: 0, IsEnabled: true, Position: 1, Weight: 1},
		{ID: 2, TenantID: 1, ProviderID: 20, ClientType: domain.ClientTypeOpenAI, ProjectID: 42, IsEnabled: true, Position: 1, Weight: 1},
	}
	projects := []*domain.Project{{ID: 42, TenantID: 1, Name: "project", Slug: "project", EnabledCustomRoutes: []domain.ClientType{domain.ClientTypeOpenAI}}}
	r, providerRepo := newModelAvailabilityRouter(t, providers, routes, projects)
	handler := NewModelsHandler(&fakeResponseModelRepo{names: []string{"gpt-global", "gpt-project"}}, providerRepo, nil, r)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("X-Maxx-Project-ID", "42")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	ids := decodeOpenAIModelIDs(t, rec.Body.Bytes())
	if containsModel(ids, "gpt-global") || !containsModel(ids, "gpt-project") {
		t.Fatalf("ids = %v, want only project model", ids)
	}
}

func TestUserPanelAvailableModelsIncludesCodexOnlyVisibleRoute(t *testing.T) {
	providers := []*domain.Provider{{
		ID:                   10,
		TenantID:             1,
		Name:                 "codex-only",
		Type:                 "custom",
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex},
		SupportModels:        []string{"gpt-codex-panel"},
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL: "https://example.invalid/v1",
			APIKey:  "test-key",
		}},
	}}
	routes := []*domain.Route{{
		ID:         1,
		TenantID:   1,
		ProviderID: 10,
		ClientType: domain.ClientTypeCodex,
		ProjectID:  0,
		IsEnabled:  true,
		Position:   1,
		Weight:     1,
	}}
	r, providerRepo := newModelAvailabilityRouter(t, providers, routes, nil)
	modelsHandler := NewModelsHandler(&fakeResponseModelRepo{names: []string{"gpt-codex-panel"}}, providerRepo, nil, r)
	selfServiceHandler := newSelfServiceHandlerForTests(selfServiceTestDeps{
		settingsRepo: &selfServiceSettingsRepo{values: map[string]string{
			domain.SettingKeyProxyRouteOpenAIChatEnabled:     "false",
			domain.SettingKeyProxyRouteResponsesEnabled:      "true",
			domain.SettingKeyProxyRouteClaudeMessagesEnabled: "false",
			domain.SettingKeyProxyRouteGeminiEnabled:         "false",
		}},
	})
	selfServiceHandler.modelsHandler = modelsHandler

	names, err := selfServiceHandler.collectUserPanelAvailableModelNames(1, 0)
	if err != nil {
		t.Fatalf("collectUserPanelAvailableModelNames: %v", err)
	}
	if !containsModel(names, "gpt-codex-panel") {
		t.Fatalf("names = %v, want Codex-only model", names)
	}
}
