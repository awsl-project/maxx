package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
	"github.com/awsl-project/maxx/internal/service"
)

type selfServiceProviderRepo struct {
	providers []*domain.Provider
}

func (r *selfServiceProviderRepo) Create(provider *domain.Provider) error {
	r.providers = append(r.providers, provider)
	return nil
}

func (r *selfServiceProviderRepo) Update(provider *domain.Provider) error {
	for i, existing := range r.providers {
		if existing.ID == provider.ID {
			r.providers[i] = provider
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *selfServiceProviderRepo) Delete(tenantID uint64, id uint64) error {
	for i, provider := range r.providers {
		if provider.ID == id && provider.TenantID == tenantID {
			r.providers = append(r.providers[:i], r.providers[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *selfServiceProviderRepo) GetByID(tenantID uint64, id uint64) (*domain.Provider, error) {
	for _, provider := range r.providers {
		if provider.ID == id && provider.TenantID == tenantID {
			return provider, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *selfServiceProviderRepo) List(tenantID uint64) ([]*domain.Provider, error) {
	var result []*domain.Provider
	for _, provider := range r.providers {
		if provider.TenantID == tenantID {
			result = append(result, provider)
		}
	}
	return result, nil
}

type selfServiceProjectRepo struct {
	projects []*domain.Project
}

func (r *selfServiceProjectRepo) Create(project *domain.Project) error {
	r.projects = append(r.projects, project)
	return nil
}

func (r *selfServiceProjectRepo) Update(project *domain.Project) error {
	for i, existing := range r.projects {
		if existing.ID == project.ID {
			r.projects[i] = project
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *selfServiceProjectRepo) Delete(tenantID uint64, id uint64) error {
	for i, project := range r.projects {
		if project.ID == id && project.TenantID == tenantID {
			r.projects = append(r.projects[:i], r.projects[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *selfServiceProjectRepo) GetByID(tenantID uint64, id uint64) (*domain.Project, error) {
	for _, project := range r.projects {
		if project.ID == id && project.TenantID == tenantID {
			return project, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *selfServiceProjectRepo) GetBySlug(tenantID uint64, slug string) (*domain.Project, error) {
	for _, project := range r.projects {
		if project.Slug == slug && project.TenantID == tenantID {
			return project, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *selfServiceProjectRepo) List(tenantID uint64) ([]*domain.Project, error) {
	var result []*domain.Project
	for _, project := range r.projects {
		if project.TenantID == tenantID {
			result = append(result, project)
		}
	}
	return result, nil
}

type selfServiceRetryConfigRepo struct {
	configs []*domain.RetryConfig
}

func (r *selfServiceRetryConfigRepo) Create(config *domain.RetryConfig) error {
	r.configs = append(r.configs, config)
	return nil
}

func (r *selfServiceRetryConfigRepo) Update(config *domain.RetryConfig) error {
	for i, existing := range r.configs {
		if existing.ID == config.ID {
			r.configs[i] = config
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *selfServiceRetryConfigRepo) Delete(tenantID uint64, id uint64) error {
	for i, config := range r.configs {
		if config.ID == id && config.TenantID == tenantID {
			r.configs = append(r.configs[:i], r.configs[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *selfServiceRetryConfigRepo) GetByID(tenantID uint64, id uint64) (*domain.RetryConfig, error) {
	for _, config := range r.configs {
		if config.ID == id && config.TenantID == tenantID {
			return config, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *selfServiceRetryConfigRepo) GetDefault(tenantID uint64) (*domain.RetryConfig, error) {
	for _, config := range r.configs {
		if config.TenantID == tenantID && config.IsDefault {
			return config, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *selfServiceRetryConfigRepo) List(tenantID uint64) ([]*domain.RetryConfig, error) {
	var result []*domain.RetryConfig
	for _, config := range r.configs {
		if config.TenantID == tenantID {
			result = append(result, config)
		}
	}
	return result, nil
}

type selfServiceRouteRepo struct {
	routes []*domain.Route
}

func (r *selfServiceRouteRepo) Create(route *domain.Route) error {
	r.routes = append(r.routes, route)
	return nil
}

func (r *selfServiceRouteRepo) Update(route *domain.Route) error {
	for i, existing := range r.routes {
		if existing.ID == route.ID {
			r.routes[i] = route
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *selfServiceRouteRepo) Delete(tenantID uint64, id uint64) error {
	for i, route := range r.routes {
		if route.ID == id && route.TenantID == tenantID {
			r.routes = append(r.routes[:i], r.routes[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *selfServiceRouteRepo) GetByID(tenantID uint64, id uint64) (*domain.Route, error) {
	for _, route := range r.routes {
		if route.ID == id && route.TenantID == tenantID {
			return route, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *selfServiceRouteRepo) FindByKey(tenantID uint64, projectID, providerID uint64, clientType domain.ClientType) (*domain.Route, error) {
	for _, route := range r.routes {
		if route.TenantID == tenantID && route.ProjectID == projectID && route.ProviderID == providerID && route.ClientType == clientType {
			return route, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *selfServiceRouteRepo) List(tenantID uint64) ([]*domain.Route, error) {
	var result []*domain.Route
	for _, route := range r.routes {
		if route.TenantID == tenantID {
			result = append(result, route)
		}
	}
	return result, nil
}

func (r *selfServiceRouteRepo) BatchUpdatePositions(tenantID uint64, updates []domain.RoutePositionUpdate) error {
	for _, update := range updates {
		for _, route := range r.routes {
			if route.ID == update.ID && route.TenantID == tenantID {
				route.Position = update.Position
			}
		}
	}
	return nil
}

type selfServiceModelMappingRepo struct {
	mappings []*domain.ModelMapping
}

func (r *selfServiceModelMappingRepo) Create(mapping *domain.ModelMapping) error {
	r.mappings = append(r.mappings, mapping)
	return nil
}

func (r *selfServiceModelMappingRepo) Update(mapping *domain.ModelMapping) error {
	for i, existing := range r.mappings {
		if existing.ID == mapping.ID {
			r.mappings[i] = mapping
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *selfServiceModelMappingRepo) Delete(tenantID uint64, id uint64) error {
	for i, mapping := range r.mappings {
		if mapping.ID == id && mapping.TenantID == tenantID {
			r.mappings = append(r.mappings[:i], r.mappings[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *selfServiceModelMappingRepo) GetByID(tenantID uint64, id uint64) (*domain.ModelMapping, error) {
	for _, mapping := range r.mappings {
		if mapping.ID == id && mapping.TenantID == tenantID {
			return mapping, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *selfServiceModelMappingRepo) List(tenantID uint64) ([]*domain.ModelMapping, error) {
	var result []*domain.ModelMapping
	for _, mapping := range r.mappings {
		if mapping.TenantID == tenantID {
			result = append(result, mapping)
		}
	}
	return result, nil
}

func (r *selfServiceModelMappingRepo) ListEnabled(tenantID uint64) ([]*domain.ModelMapping, error) {
	return r.List(tenantID)
}

func (r *selfServiceModelMappingRepo) ListByClientType(tenantID uint64, clientType domain.ClientType) ([]*domain.ModelMapping, error) {
	var result []*domain.ModelMapping
	for _, mapping := range r.mappings {
		if mapping.TenantID == tenantID && mapping.ClientType == clientType {
			result = append(result, mapping)
		}
	}
	return result, nil
}

func (r *selfServiceModelMappingRepo) ListByQuery(tenantID uint64, _ *domain.ModelMappingQuery) ([]*domain.ModelMapping, error) {
	return r.List(tenantID)
}

func (r *selfServiceModelMappingRepo) Count(tenantID uint64) (int, error) {
	list, err := r.List(tenantID)
	if err != nil {
		return 0, err
	}
	return len(list), nil
}

func (r *selfServiceModelMappingRepo) DeleteAll(tenantID uint64) error {
	return r.ClearAll(tenantID)
}

func (r *selfServiceModelMappingRepo) ClearAll(tenantID uint64) error {
	filtered := make([]*domain.ModelMapping, 0, len(r.mappings))
	for _, mapping := range r.mappings {
		if mapping.TenantID != tenantID {
			filtered = append(filtered, mapping)
		}
	}
	r.mappings = filtered
	return nil
}

func (r *selfServiceModelMappingRepo) SeedDefaults(tenantID uint64) error {
	r.mappings = append(r.mappings, &domain.ModelMapping{
		ID:       uint64(len(r.mappings) + 1),
		TenantID: tenantID,
		Scope:    domain.ModelMappingScopeGlobal,
		Pattern:  "gpt-*",
		Target:   "gpt-4.1",
		Priority: 10,
	})
	return nil
}

type selfServiceSettingsRepo struct {
	values map[string]string
}

func (r *selfServiceSettingsRepo) Get(key string) (string, error) {
	value, ok := r.values[key]
	if !ok {
		return "", domain.ErrNotFound
	}
	return value, nil
}

func (r *selfServiceSettingsRepo) Set(key, value string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	r.values[key] = value
	return nil
}

func (r *selfServiceSettingsRepo) GetAll() ([]*domain.SystemSetting, error) {
	result := make([]*domain.SystemSetting, 0, len(r.values))
	for key, value := range r.values {
		result = append(result, &domain.SystemSetting{Key: key, Value: value})
	}
	return result, nil
}

func (r *selfServiceSettingsRepo) Delete(key string) error {
	delete(r.values, key)
	return nil
}

type selfServiceAPITokenRepo struct {
	tokens []*domain.APIToken
}

func (r *selfServiceAPITokenRepo) Create(token *domain.APIToken) error {
	r.tokens = append(r.tokens, token)
	return nil
}

func (r *selfServiceAPITokenRepo) Update(token *domain.APIToken) error {
	for i, existing := range r.tokens {
		if existing.ID == token.ID {
			r.tokens[i] = token
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *selfServiceAPITokenRepo) Delete(tenantID uint64, id uint64) error {
	for i, token := range r.tokens {
		if token.ID == id && token.TenantID == tenantID {
			r.tokens = append(r.tokens[:i], r.tokens[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *selfServiceAPITokenRepo) GetByID(tenantID uint64, id uint64) (*domain.APIToken, error) {
	for _, token := range r.tokens {
		if token.ID == id && token.TenantID == tenantID {
			return token, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *selfServiceAPITokenRepo) GetByToken(tenantID uint64, tokenValue string) (*domain.APIToken, error) {
	for _, token := range r.tokens {
		if token.TenantID == tenantID && token.Token == tokenValue {
			return token, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *selfServiceAPITokenRepo) List(tenantID uint64) ([]*domain.APIToken, error) {
	var result []*domain.APIToken
	for _, token := range r.tokens {
		if token.TenantID == tenantID {
			result = append(result, token)
		}
	}
	return result, nil
}

func (r *selfServiceAPITokenRepo) UpdateLastSeen(_ uint64, _ uint64, _ string, _ time.Time) error {
	return nil
}

type selfServiceUsageStatsRepo struct {
	providerStats  map[uint64]*domain.ProviderStats
	lastTenantID   uint64
	lastClientType string
	lastProjectID  uint64
}

func (r *selfServiceUsageStatsRepo) Upsert(_ *domain.UsageStats) error        { return nil }
func (r *selfServiceUsageStatsRepo) BatchUpsert(_ []*domain.UsageStats) error { return nil }
func (r *selfServiceUsageStatsRepo) Query(_ uint64, _ repository.UsageStatsFilter) ([]*domain.UsageStats, error) {
	return nil, nil
}
func (r *selfServiceUsageStatsRepo) QueryDashboardData(_ uint64) (*domain.DashboardData, error) {
	return nil, nil
}
func (r *selfServiceUsageStatsRepo) GetSummary(_ uint64, _ repository.UsageStatsFilter) (*domain.UsageStatsSummary, error) {
	return nil, nil
}
func (r *selfServiceUsageStatsRepo) GetSummaryByProvider(_ uint64, _ repository.UsageStatsFilter) (map[uint64]*domain.UsageStatsSummary, error) {
	return nil, nil
}
func (r *selfServiceUsageStatsRepo) GetSummaryByRoute(_ uint64, _ repository.UsageStatsFilter) (map[uint64]*domain.UsageStatsSummary, error) {
	return nil, nil
}
func (r *selfServiceUsageStatsRepo) GetSummaryByProject(_ uint64, _ repository.UsageStatsFilter) (map[uint64]*domain.UsageStatsSummary, error) {
	return nil, nil
}
func (r *selfServiceUsageStatsRepo) GetSummaryByAPIToken(_ uint64, _ repository.UsageStatsFilter) (map[uint64]*domain.UsageStatsSummary, error) {
	return nil, nil
}
func (r *selfServiceUsageStatsRepo) GetSummaryByClientType(_ uint64, _ repository.UsageStatsFilter) (map[string]*domain.UsageStatsSummary, error) {
	return nil, nil
}
func (r *selfServiceUsageStatsRepo) DeleteOlderThan(_ domain.Granularity, _ time.Time) (int64, error) {
	return 0, nil
}
func (r *selfServiceUsageStatsRepo) GetLatestTimeBucket(_ uint64, _ domain.Granularity) (*time.Time, error) {
	return nil, nil
}
func (r *selfServiceUsageStatsRepo) GetProviderStats(tenantID uint64, clientType string, projectID uint64) (map[uint64]*domain.ProviderStats, error) {
	r.lastTenantID = tenantID
	r.lastClientType = clientType
	r.lastProjectID = projectID
	return r.providerStats, nil
}
func (r *selfServiceUsageStatsRepo) AggregateAndRollUp(_ uint64) <-chan domain.AggregateEvent {
	ch := make(chan domain.AggregateEvent)
	close(ch)
	return ch
}
func (r *selfServiceUsageStatsRepo) ClearAndRecalculate(_ uint64) error { return nil }
func (r *selfServiceUsageStatsRepo) ClearAndRecalculateWithProgress(_ uint64, _ chan<- domain.Progress) error {
	return nil
}

type selfServiceModelPriceRepo struct {
	prices []*domain.ModelPrice
}

func (r *selfServiceModelPriceRepo) Create(price *domain.ModelPrice) error {
	r.prices = append(r.prices, price)
	return nil
}

func (r *selfServiceModelPriceRepo) BatchCreate(prices []*domain.ModelPrice) error {
	r.prices = append(r.prices, prices...)
	return nil
}

func (r *selfServiceModelPriceRepo) GetByID(id uint64) (*domain.ModelPrice, error) {
	for _, price := range r.prices {
		if price.ID == id {
			return price, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *selfServiceModelPriceRepo) GetCurrentByModelID(modelID string) (*domain.ModelPrice, error) {
	for _, price := range r.prices {
		if price.ModelID == modelID {
			return price, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *selfServiceModelPriceRepo) ListCurrentPrices() ([]*domain.ModelPrice, error) {
	return r.prices, nil
}

func (r *selfServiceModelPriceRepo) ListByModelID(modelID string) ([]*domain.ModelPrice, error) {
	var result []*domain.ModelPrice
	for _, price := range r.prices {
		if price.ModelID == modelID {
			result = append(result, price)
		}
	}
	return result, nil
}

func (r *selfServiceModelPriceRepo) Count() (int64, error) {
	return int64(len(r.prices)), nil
}

func (r *selfServiceModelPriceRepo) Delete(id uint64) error {
	for i, price := range r.prices {
		if price.ID == id {
			r.prices = append(r.prices[:i], r.prices[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *selfServiceModelPriceRepo) Update(price *domain.ModelPrice) error {
	for i, existing := range r.prices {
		if existing.ID == price.ID {
			r.prices[i] = price
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *selfServiceModelPriceRepo) SoftDeleteAll() error {
	r.prices = nil
	return nil
}

func (r *selfServiceModelPriceRepo) ResetToDefaults() ([]*domain.ModelPrice, error) {
	r.prices = []*domain.ModelPrice{}
	return r.prices, nil
}

type selfServiceResponseModelRepo struct {
	names []string
}

func (r *selfServiceResponseModelRepo) Upsert(_ string) error { return nil }

func (r *selfServiceResponseModelRepo) BatchUpsert(_ []string) error { return nil }

func (r *selfServiceResponseModelRepo) List() ([]*domain.ResponseModel, error) {
	result := make([]*domain.ResponseModel, 0, len(r.names))
	for _, name := range r.names {
		result = append(result, &domain.ResponseModel{Name: name})
	}
	return result, nil
}

func (r *selfServiceResponseModelRepo) ListNames() ([]string, error) {
	return r.names, nil
}

type selfServiceTestDeps struct {
	providerRepo      repository.ProviderRepository
	routeRepo         *selfServiceRouteRepo
	projectRepo       *selfServiceProjectRepo
	retryConfigRepo   *selfServiceRetryConfigRepo
	modelMappingRepo  *selfServiceModelMappingRepo
	settingsRepo      *selfServiceSettingsRepo
	apiTokenRepo      *selfServiceAPITokenRepo
	usageStatsRepo    *selfServiceUsageStatsRepo
	responseModelRepo *selfServiceResponseModelRepo
	modelPriceRepo    *selfServiceModelPriceRepo
}

type selfServiceProviderRepoWithListError struct {
	*selfServiceProviderRepo
	listErr error
}

func (r *selfServiceProviderRepoWithListError) List(tenantID uint64) ([]*domain.Provider, error) {
	return nil, r.listErr
}

func newSelfServiceHandlerForTests(deps selfServiceTestDeps) *SelfServiceHandler {
	adminSvc := service.NewAdminService(
		deps.providerRepo,
		deps.routeRepo,
		deps.projectRepo,
		nil,
		deps.retryConfigRepo,
		nil,
		nil,
		nil,
		deps.settingsRepo,
		deps.apiTokenRepo,
		nil,
		nil,
		deps.modelMappingRepo,
		deps.usageStatsRepo,
		deps.responseModelRepo,
		deps.modelPriceRepo,
		"",
		nil,
		nil,
		nil,
	)
	return NewSelfServiceHandler(adminSvc)
}

func newSelfServiceRequest(method, path string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	return withSelfServiceContext(req, domain.UserRoleMember)
}

func newSelfServiceRequestWithBody(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	return withSelfServiceContext(req, domain.UserRoleMember)
}

func newSelfServiceAdminRequest(method, path string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	return withSelfServiceContext(req, domain.UserRoleAdmin)
}

func withSelfServiceContext(req *http.Request, role domain.UserRole) *http.Request {
	ctx := maxxctx.WithTenantID(req.Context(), 1)
	ctx = maxxctx.WithUserID(ctx, 9)
	ctx = maxxctx.WithUserRole(ctx, string(role))
	return req.WithContext(ctx)
}

func TestSelfServiceHandler_ListProviders_MemberAllowed(t *testing.T) {
	handler := newSelfServiceHandlerForTests(selfServiceTestDeps{
		providerRepo: &selfServiceProviderRepo{
			providers: []*domain.Provider{
				{
					ID:       1,
					TenantID: 1,
					Name:     "tenant-provider",
					Type:     "custom",
					Config: &domain.ProviderConfig{
						Custom: &domain.ProviderConfigCustom{
							BaseURL: "https://example.com",
							APIKey:  "secret-api-key",
						},
					},
				},
				{ID: 2, TenantID: 2, Name: "other-tenant-provider", Type: "custom"},
			},
		},
		projectRepo: &selfServiceProjectRepo{},
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newSelfServiceRequest(http.MethodGet, "/providers"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var providers []domain.Provider
	if err := json.Unmarshal(rec.Body.Bytes(), &providers); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(providers) != 1 || providers[0].Name != "tenant-provider" {
		t.Fatalf("providers = %+v, want only tenant provider", providers)
	}
	if providers[0].Config == nil || providers[0].Config.Custom == nil {
		t.Fatalf("provider config missing: %+v", providers[0])
	}
	if providers[0].Config.Custom.APIKey != "" {
		t.Fatalf("provider API key leaked to member: %+v", providers[0].Config.Custom)
	}
}

func TestSelfServiceHandler_GetProvider_AdminKeepsSecrets(t *testing.T) {
	handler := newSelfServiceHandlerForTests(selfServiceTestDeps{
		providerRepo: &selfServiceProviderRepo{
			providers: []*domain.Provider{
				{
					ID:       1,
					TenantID: 1,
					Name:     "tenant-provider",
					Type:     "custom",
					Config: &domain.ProviderConfig{
						Custom: &domain.ProviderConfigCustom{
							BaseURL: "https://example.com",
							APIKey:  "secret-api-key",
						},
					},
				},
			},
		},
		projectRepo: &selfServiceProjectRepo{},
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newSelfServiceAdminRequest(http.MethodGet, "/providers/1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var provider domain.Provider
	if err := json.Unmarshal(rec.Body.Bytes(), &provider); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if provider.Config == nil || provider.Config.Custom == nil || provider.Config.Custom.APIKey != "secret-api-key" {
		t.Fatalf("admin provider config = %+v, want unredacted secret", provider.Config)
	}
}

func TestSelfServiceHandler_MemberForbiddenOnSensitiveProviderOperations(t *testing.T) {
	handler := newSelfServiceHandlerForTests(selfServiceTestDeps{
		providerRepo: &selfServiceProviderRepo{
			providers: []*domain.Provider{
				{ID: 1, TenantID: 1, Name: "tenant-provider", Type: "custom"},
			},
		},
		projectRepo: &selfServiceProjectRepo{},
	})

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "create provider", method: http.MethodPost, path: "/providers", body: `{"name":"x","type":"custom"}`},
		{name: "update provider", method: http.MethodPut, path: "/providers/1", body: `{"name":"x","type":"custom"}`},
		{name: "delete provider", method: http.MethodDelete, path: "/providers/1"},
		{name: "export providers", method: http.MethodGet, path: "/providers/export"},
		{name: "import providers", method: http.MethodPost, path: "/providers/import", body: `[]`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, newSelfServiceRequestWithBody(tc.method, tc.path, tc.body))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
			}
		})
	}
}

func TestSelfServiceHandler_ListProjects_MemberAllowed(t *testing.T) {
	handler := newSelfServiceHandlerForTests(selfServiceTestDeps{
		providerRepo: &selfServiceProviderRepo{},
		projectRepo: &selfServiceProjectRepo{
			projects: []*domain.Project{
				{ID: 1, TenantID: 1, Name: "tenant-project", Slug: "tenant-project"},
				{ID: 2, TenantID: 2, Name: "other-project", Slug: "other-project"},
			},
		},
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newSelfServiceRequest(http.MethodGet, "/projects"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var projects []domain.Project
	if err := json.Unmarshal(rec.Body.Bytes(), &projects); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(projects) != 1 || projects[0].Slug != "tenant-project" {
		t.Fatalf("projects = %+v, want only tenant project", projects)
	}
}

func TestSelfServiceHandler_GetProjectBySlug_MemberAllowed(t *testing.T) {
	handler := newSelfServiceHandlerForTests(selfServiceTestDeps{
		providerRepo: &selfServiceProviderRepo{},
		projectRepo: &selfServiceProjectRepo{
			projects: []*domain.Project{
				{ID: 1, TenantID: 1, Name: "tenant-project", Slug: "tenant-project"},
			},
		},
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newSelfServiceRequest(http.MethodGet, "/projects/by-slug/tenant-project"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var project domain.Project
	if err := json.Unmarshal(rec.Body.Bytes(), &project); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if project.Slug != "tenant-project" {
		t.Fatalf("project.slug = %q, want tenant-project", project.Slug)
	}
}

func TestSelfServiceHandler_ListRetryConfigs_MemberAllowed(t *testing.T) {
	handler := newSelfServiceHandlerForTests(selfServiceTestDeps{
		providerRepo: &selfServiceProviderRepo{},
		projectRepo:  &selfServiceProjectRepo{},
		retryConfigRepo: &selfServiceRetryConfigRepo{
			configs: []*domain.RetryConfig{
				{ID: 1, TenantID: 1, Name: "tenant-default", IsDefault: true},
				{ID: 2, TenantID: 2, Name: "other-default", IsDefault: true},
			},
		},
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newSelfServiceRequest(http.MethodGet, "/retry-configs"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var configs []domain.RetryConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &configs); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(configs) != 1 || configs[0].Name != "tenant-default" {
		t.Fatalf("configs = %+v, want only tenant retry config", configs)
	}
}

func TestSelfServiceHandler_ListModelMappings_MemberAllowed(t *testing.T) {
	handler := newSelfServiceHandlerForTests(selfServiceTestDeps{
		providerRepo: &selfServiceProviderRepo{},
		projectRepo:  &selfServiceProjectRepo{},
		modelMappingRepo: &selfServiceModelMappingRepo{
			mappings: []*domain.ModelMapping{
				{ID: 1, TenantID: 1, Scope: domain.ModelMappingScopeGlobal, Pattern: "gpt-*", Target: "gpt-4.1"},
				{ID: 2, TenantID: 2, Scope: domain.ModelMappingScopeGlobal, Pattern: "claude-*", Target: "claude-sonnet-4"},
			},
		},
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newSelfServiceRequest(http.MethodGet, "/model-mappings"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var mappings []domain.ModelMapping
	if err := json.Unmarshal(rec.Body.Bytes(), &mappings); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(mappings) != 1 || mappings[0].Pattern != "gpt-*" {
		t.Fatalf("mappings = %+v, want only tenant model mapping", mappings)
	}
}

func TestSelfServiceHandler_GetProviderStats_MemberAllowed(t *testing.T) {
	statsRepo := &selfServiceUsageStatsRepo{
		providerStats: map[uint64]*domain.ProviderStats{
			1: {ProviderID: 1, TotalRequests: 12, SuccessfulRequests: 11},
		},
	}
	handler := newSelfServiceHandlerForTests(selfServiceTestDeps{
		providerRepo:   &selfServiceProviderRepo{},
		projectRepo:    &selfServiceProjectRepo{},
		usageStatsRepo: statsRepo,
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newSelfServiceRequest(http.MethodGet, "/provider-stats?client_type=claude&project_id=42"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if statsRepo.lastTenantID != 1 || statsRepo.lastClientType != "claude" || statsRepo.lastProjectID != 42 {
		t.Fatalf(
			"provider stats args = tenant:%d client:%q project:%d, want tenant:1 client:claude project:42",
			statsRepo.lastTenantID,
			statsRepo.lastClientType,
			statsRepo.lastProjectID,
		)
	}

	var stats map[string]domain.ProviderStats
	if err := json.Unmarshal(rec.Body.Bytes(), &stats); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(stats) != 1 || stats["1"].TotalRequests != 12 {
		t.Fatalf("stats = %+v, want provider 1 stats", stats)
	}
}

func TestSelfServiceHandler_GetProviderStats_InvalidProjectID_ReturnsBadRequest(t *testing.T) {
	statsRepo := &selfServiceUsageStatsRepo{}
	handler := newSelfServiceHandlerForTests(selfServiceTestDeps{
		providerRepo:   &selfServiceProviderRepo{},
		projectRepo:    &selfServiceProjectRepo{},
		usageStatsRepo: statsRepo,
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newSelfServiceRequest(http.MethodGet, "/provider-stats?project_id=abc"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["error"] != "invalid project_id query parameter" {
		t.Fatalf("error = %q, want invalid project_id query parameter", body["error"])
	}
	if statsRepo.lastTenantID != 0 || statsRepo.lastClientType != "" || statsRepo.lastProjectID != 0 {
		t.Fatalf(
			"usage stats repo should not be called, got tenant:%d client:%q project:%d",
			statsRepo.lastTenantID,
			statsRepo.lastClientType,
			statsRepo.lastProjectID,
		)
	}
}

func TestSelfServiceHandler_GetPublicSettings_FiltersSensitiveKeys(t *testing.T) {
	handler := newSelfServiceHandlerForTests(selfServiceTestDeps{
		providerRepo: &selfServiceProviderRepo{},
		projectRepo:  &selfServiceProjectRepo{},
		settingsRepo: &selfServiceSettingsRepo{
			values: map[string]string{
				"api_token_auth_enabled": "true",
				"force_project_binding":  "true",
				"force_project_timeout":  "45",
				"auto_sort_antigravity":  "true",
				"auto_sort_codex":        "false",
				"jwt_secret":             "hidden",
				"pprof_password":         "secret",
			},
		},
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newSelfServiceRequest(http.MethodGet, "/settings"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var settings map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &settings); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(settings) != 5 {
		t.Fatalf("settings length = %d, want 5, settings = %+v", len(settings), settings)
	}
	if settings["api_token_auth_enabled"] != "true" ||
		settings["force_project_binding"] != "true" ||
		settings["force_project_timeout"] != "45" ||
		settings["auto_sort_antigravity"] != "true" ||
		settings["auto_sort_codex"] != "false" {
		t.Fatalf("settings = %+v, want public setting values", settings)
	}
	if _, ok := settings["jwt_secret"]; ok {
		t.Fatalf("settings = %+v, sensitive jwt_secret should be filtered", settings)
	}
	if _, ok := settings["pprof_password"]; ok {
		t.Fatalf("settings = %+v, sensitive pprof_password should be filtered", settings)
	}
}

func TestSelfServiceHandler_GetProxyStatus_MemberAllowed(t *testing.T) {
	handler := newSelfServiceHandlerForTests(selfServiceTestDeps{
		providerRepo: &selfServiceProviderRepo{},
		projectRepo:  &selfServiceProjectRepo{},
	})

	req := newSelfServiceRequest(http.MethodGet, "/proxy-status")
	req.Host = "proxy.example.test:4321"

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var status service.ProxyStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !status.Running {
		t.Fatalf("status.running = %v, want true", status.Running)
	}
	if status.Address != "proxy.example.test:4321" {
		t.Fatalf("status.address = %q, want proxy.example.test:4321", status.Address)
	}
	if status.Port != 4321 {
		t.Fatalf("status.port = %d, want 4321", status.Port)
	}
}

func TestSelfServiceHandler_ListModelPrices_MemberAllowed(t *testing.T) {
	handler := newSelfServiceHandlerForTests(selfServiceTestDeps{
		providerRepo: &selfServiceProviderRepo{},
		projectRepo:  &selfServiceProjectRepo{},
		modelPriceRepo: &selfServiceModelPriceRepo{
			prices: []*domain.ModelPrice{
				{ID: 1, ModelID: "gpt-4.1", InputPriceMicro: 1000000},
			},
		},
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newSelfServiceRequest(http.MethodGet, "/model-prices"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var prices []domain.ModelPrice
	if err := json.Unmarshal(rec.Body.Bytes(), &prices); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(prices) != 1 || prices[0].ModelID != "gpt-4.1" {
		t.Fatalf("prices = %+v, want model price list", prices)
	}
}

func TestSelfServiceHandler_ListRoutes_MemberAllowed(t *testing.T) {
	handler := newSelfServiceHandlerForTests(selfServiceTestDeps{
		providerRepo: &selfServiceProviderRepo{},
		routeRepo: &selfServiceRouteRepo{
			routes: []*domain.Route{
				{ID: 1, TenantID: 1, ClientType: domain.ClientTypeClaude, ProviderID: 10, Position: 1},
				{ID: 2, TenantID: 2, ClientType: domain.ClientTypeOpenAI, ProviderID: 20, Position: 1},
			},
		},
		projectRepo: &selfServiceProjectRepo{},
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newSelfServiceRequest(http.MethodGet, "/routes"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var routes []domain.Route
	if err := json.Unmarshal(rec.Body.Bytes(), &routes); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(routes) != 1 || routes[0].ProviderID != 10 {
		t.Fatalf("routes = %+v, want only tenant route", routes)
	}
}

func TestSelfServiceHandler_InvalidResourceIDs_ReturnBadRequest(t *testing.T) {
	handler := newSelfServiceHandlerForTests(selfServiceTestDeps{
		providerRepo:      &selfServiceProviderRepo{},
		routeRepo:         &selfServiceRouteRepo{},
		projectRepo:       &selfServiceProjectRepo{},
		retryConfigRepo:   &selfServiceRetryConfigRepo{},
		modelMappingRepo:  &selfServiceModelMappingRepo{},
		apiTokenRepo:      &selfServiceAPITokenRepo{},
		modelPriceRepo:    &selfServiceModelPriceRepo{},
		responseModelRepo: &selfServiceResponseModelRepo{},
	})

	cases := []struct {
		name string
		path string
	}{
		{name: "providers", path: "/providers/not-a-number"},
		{name: "providers zero", path: "/providers/0"},
		{name: "routes", path: "/routes/not-a-number"},
		{name: "routes zero", path: "/routes/0"},
		{name: "projects", path: "/projects/not-a-number"},
		{name: "projects zero", path: "/projects/0"},
		{name: "retry-configs", path: "/retry-configs/not-a-number"},
		{name: "retry-configs zero", path: "/retry-configs/0"},
		{name: "model-mappings", path: "/model-mappings/not-a-number"},
		{name: "model-mappings zero", path: "/model-mappings/0"},
		{name: "api-tokens", path: "/api-tokens/not-a-number"},
		{name: "api-tokens zero", path: "/api-tokens/0"},
		{name: "model-prices", path: "/model-prices/not-a-number"},
		{name: "model-prices zero", path: "/model-prices/0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, newSelfServiceRequest(http.MethodGet, tc.path))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func TestSelfServiceHandler_UnexpectedExtraSegmentsReturnNotFound(t *testing.T) {
	handler := newSelfServiceHandlerForTests(selfServiceTestDeps{
		providerRepo:      &selfServiceProviderRepo{},
		routeRepo:         &selfServiceRouteRepo{},
		projectRepo:       &selfServiceProjectRepo{},
		retryConfigRepo:   &selfServiceRetryConfigRepo{},
		modelMappingRepo:  &selfServiceModelMappingRepo{},
		settingsRepo:      &selfServiceSettingsRepo{},
		apiTokenRepo:      &selfServiceAPITokenRepo{},
		modelPriceRepo:    &selfServiceModelPriceRepo{},
		responseModelRepo: &selfServiceResponseModelRepo{},
	})

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{name: "provider export nested under id", method: http.MethodGet, path: "/providers/1/export"},
		{name: "project by slug extra segment", method: http.MethodGet, path: "/projects/by-slug/demo/extra"},
		{name: "batch route positions extra segment", method: http.MethodPut, path: "/routes/batch-positions/extra"},
		{name: "clear mappings extra segment", method: http.MethodDelete, path: "/model-mappings/clear-all/extra"},
		{name: "settings extra segment", method: http.MethodGet, path: "/settings/api_token_auth_enabled/extra"},
		{name: "response models extra segment", method: http.MethodGet, path: "/response-models/extra"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, newSelfServiceRequest(tc.method, tc.path))

			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
			}
		})
	}
}

func TestSelfServiceHandler_InternalErrorsAreSanitized(t *testing.T) {
	handler := newSelfServiceHandlerForTests(selfServiceTestDeps{
		providerRepo: &selfServiceProviderRepoWithListError{
			selfServiceProviderRepo: &selfServiceProviderRepo{},
			listErr:                 errors.New("database credentials leaked"),
		},
		projectRepo: &selfServiceProjectRepo{},
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newSelfServiceRequest(http.MethodGet, "/providers"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["error"] != "internal server error" {
		t.Fatalf("error = %q, want internal server error", body["error"])
	}
}

func TestSelfServiceHandler_MemberForbiddenOnConfigurationWrites(t *testing.T) {
	handler := newSelfServiceHandlerForTests(selfServiceTestDeps{
		providerRepo:     &selfServiceProviderRepo{},
		projectRepo:      &selfServiceProjectRepo{},
		retryConfigRepo:  &selfServiceRetryConfigRepo{},
		modelMappingRepo: &selfServiceModelMappingRepo{},
		routeRepo:        &selfServiceRouteRepo{},
	})

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "create project", method: http.MethodPost, path: "/projects", body: `{"name":"x","slug":"x"}`},
		{name: "update project", method: http.MethodPut, path: "/projects/1", body: `{"name":"x","slug":"x"}`},
		{name: "delete project", method: http.MethodDelete, path: "/projects/1"},
		{name: "create route", method: http.MethodPost, path: "/routes", body: `{"providerID":1,"clientType":"claude"}`},
		{name: "update route", method: http.MethodPut, path: "/routes/1", body: `{"isEnabled":true}`},
		{name: "delete route", method: http.MethodDelete, path: "/routes/1"},
		{name: "batch route positions", method: http.MethodPut, path: "/routes/batch-positions", body: `[]`},
		{name: "create retry config", method: http.MethodPost, path: "/retry-configs", body: `{"name":"cfg"}`},
		{name: "update retry config", method: http.MethodPut, path: "/retry-configs/1", body: `{"name":"cfg"}`},
		{name: "delete retry config", method: http.MethodDelete, path: "/retry-configs/1"},
		{name: "create mapping", method: http.MethodPost, path: "/model-mappings", body: `{"pattern":"a","target":"b"}`},
		{name: "update mapping", method: http.MethodPut, path: "/model-mappings/1", body: `{"pattern":"a"}`},
		{name: "delete mapping", method: http.MethodDelete, path: "/model-mappings/1"},
		{name: "clear mappings", method: http.MethodDelete, path: "/model-mappings/clear-all"},
		{name: "reset mappings", method: http.MethodPost, path: "/model-mappings/reset-defaults"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, newSelfServiceRequestWithBody(tc.method, tc.path, tc.body))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
			}
		})
	}
}

func TestSelfServiceHandler_ListAPITokens_MemberAllowed(t *testing.T) {
	handler := newSelfServiceHandlerForTests(selfServiceTestDeps{
		providerRepo: &selfServiceProviderRepo{},
		projectRepo:  &selfServiceProjectRepo{},
		apiTokenRepo: &selfServiceAPITokenRepo{
			tokens: []*domain.APIToken{
				{ID: 1, TenantID: 1, Name: "tenant-token", Token: "maxx_secret", TokenPrefix: "maxx_abc"},
				{ID: 2, TenantID: 2, Name: "other-token", TokenPrefix: "maxx_def"},
			},
		},
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newSelfServiceRequest(http.MethodGet, "/api-tokens"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var tokens []domain.APIToken
	if err := json.Unmarshal(rec.Body.Bytes(), &tokens); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(tokens) != 1 || tokens[0].Name != "tenant-token" {
		t.Fatalf("tokens = %+v, want only tenant token", tokens)
	}
	if tokens[0].Token != "" {
		t.Fatalf("token plaintext leaked in list response: %+v", tokens[0])
	}
	if tokens[0].TokenPrefix != "maxx_abc" {
		t.Fatalf("tokenPrefix = %q, want maxx_abc", tokens[0].TokenPrefix)
	}
}

func TestSelfServiceHandler_GetAPIToken_MemberRedactsPlaintextToken(t *testing.T) {
	handler := newSelfServiceHandlerForTests(selfServiceTestDeps{
		providerRepo: &selfServiceProviderRepo{},
		projectRepo:  &selfServiceProjectRepo{},
		apiTokenRepo: &selfServiceAPITokenRepo{
			tokens: []*domain.APIToken{
				{ID: 1, TenantID: 1, Name: "tenant-token", Token: "maxx_secret", TokenPrefix: "maxx_abc"},
			},
		},
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newSelfServiceRequest(http.MethodGet, "/api-tokens/1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var token domain.APIToken
	if err := json.Unmarshal(rec.Body.Bytes(), &token); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if token.Token != "" {
		t.Fatalf("token plaintext leaked in detail response: %+v", token)
	}
	if token.TokenPrefix != "maxx_abc" {
		t.Fatalf("tokenPrefix = %q, want maxx_abc", token.TokenPrefix)
	}
}

func TestSelfServiceHandler_ListResponseModels_MemberAllowed(t *testing.T) {
	handler := newSelfServiceHandlerForTests(selfServiceTestDeps{
		providerRepo:      &selfServiceProviderRepo{},
		projectRepo:       &selfServiceProjectRepo{},
		responseModelRepo: &selfServiceResponseModelRepo{names: []string{"gpt-4.1", "claude-sonnet-4"}},
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, newSelfServiceRequest(http.MethodGet, "/response-models"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var names []string
	if err := json.Unmarshal(rec.Body.Bytes(), &names); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(names) != 2 || names[0] != "gpt-4.1" {
		t.Fatalf("names = %+v, want response model names", names)
	}
}

func TestSelfServiceHandler_ListEndpoints_EmptySlicesSerializeAsJSONArray(t *testing.T) {
	handler := newSelfServiceHandlerForTests(selfServiceTestDeps{
		providerRepo:      &selfServiceProviderRepo{},
		routeRepo:         &selfServiceRouteRepo{},
		projectRepo:       &selfServiceProjectRepo{},
		apiTokenRepo:      &selfServiceAPITokenRepo{},
		responseModelRepo: &selfServiceResponseModelRepo{},
		modelPriceRepo:    &selfServiceModelPriceRepo{},
	})

	cases := []struct {
		name string
		path string
	}{
		{name: "providers", path: "/providers"},
		{name: "projects", path: "/projects"},
		{name: "routes", path: "/routes"},
		{name: "api tokens", path: "/api-tokens"},
		{name: "model prices", path: "/model-prices"},
		{name: "response models", path: "/response-models"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, newSelfServiceRequest(http.MethodGet, tc.path))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
			}
			if body := strings.TrimSpace(rec.Body.String()); body != "[]" {
				t.Fatalf("body = %q, want []", body)
			}
		})
	}
}
