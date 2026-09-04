package executor

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/repository/cached"
	"github.com/awsl-project/maxx/internal/router"
)

func TestExecutorRouteMatchFailureCreatesRejectedProxyRequest(t *testing.T) {
	routeRepo := cached.NewRouteRepository(nil)
	providerRepo := cached.NewProviderRepository(nil)
	strategyRepo := cached.NewRoutingStrategyRepository(nil)
	retryRepo := cached.NewRetryConfigRepository(nil)
	projectRepo := cached.NewProjectRepository(nil)
	r := router.NewRouter(routeRepo, providerRepo, strategyRepo, retryRepo, projectRepo)

	proxyRepo := &recordingProxyRequestRepo{}
	exec := &Executor{
		router:           r,
		proxyRequestRepo: proxyRepo,
		instanceID:       "test-instance",
	}

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"minimaxai/minimax-m3"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := flow.NewCtx(rec, req)
	state := &execState{
		ctx:            req.Context(),
		tenantID:       1,
		clientType:     domain.ClientTypeOpenAI,
		projectID:      0,
		sessionID:      "session-1",
		requestModel:   "minimaxai/minimax-m3",
		apiTokenID:     46,
		requestBody:    []byte(`{"model":"minimaxai/minimax-m3"}`),
		requestURI:     req.URL.RequestURI(),
		requestHeaders: req.Header,
	}
	c.Set(flow.KeyExecutorState, state)

	exec.routeMatch(c)

	if len(proxyRepo.created) != 1 {
		t.Fatalf("created proxy requests = %d, want 1", len(proxyRepo.created))
	}
	created := proxyRepo.created[0]
	if created.Status != "REJECTED" {
		t.Fatalf("status = %q, want REJECTED", created.Status)
	}
	if created.StatusCode != 503 {
		t.Fatalf("statusCode = %d, want 503", created.StatusCode)
	}
	if !strings.Contains(created.Error, "route match failed") || !strings.Contains(created.Error, "no routes available") {
		t.Fatalf("error = %q, want route match failure", created.Error)
	}
	if created.RequestModel != "minimaxai/minimax-m3" || created.APITokenID != 46 || created.ClientType != domain.ClientTypeOpenAI {
		t.Fatalf("created = %+v, want request context preserved", created)
	}
	if created.RequestInfo == nil || created.RequestInfo.URL != "/v1/chat/completions" {
		t.Fatalf("requestInfo = %+v, want captured request detail", created.RequestInfo)
	}
	if state.proxyReq == nil || state.proxyReq.Status != "REJECTED" {
		t.Fatalf("state.proxyReq = %+v, want rejected proxy request", state.proxyReq)
	}
	if c.Err == nil {
		t.Fatalf("c.Err is nil, want proxy error")
	}
}

func TestExecutorRouteMatchUsesMappedModelCandidatesForSupportModels(t *testing.T) {
	routeRepo := cached.NewRouteRepository(&providerProxyMatchRouteRepo{routes: []*domain.Route{
		{ID: 1, TenantID: 1, ProviderID: 101, ClientType: domain.ClientTypeOpenAI, IsEnabled: true, Position: 1},
		{ID: 2, TenantID: 1, ProviderID: 102, ClientType: domain.ClientTypeOpenAI, IsEnabled: true, Position: 2},
	}})
	providerRepo := cached.NewProviderRepository(&providerProxyMatchProviderRepo{providers: []*domain.Provider{
		{ID: 101, TenantID: 1, Type: providerProxyMatchTestType, Name: "pre-mapped-only", SupportedClientTypes: []domain.ClientType{domain.ClientTypeOpenAI}, SupportModels: []string{"gpt-5"}},
		{ID: 102, TenantID: 1, Type: providerProxyMatchTestType, Name: "mapped-target", SupportedClientTypes: []domain.ClientType{domain.ClientTypeOpenAI}, SupportModels: []string{"moonshotai/kimi-k3"}},
	}})
	retryRepo := cached.NewRetryConfigRepository(&providerProxyMatchRetryRepo{configs: []*domain.RetryConfig{
		{ID: 1, TenantID: 1, IsDefault: true, MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
	}})
	strategyRepo := cached.NewRoutingStrategyRepository(providerProxyMatchStrategyRepo{})
	projectRepo := cached.NewProjectRepository(providerProxyMatchProjectRepo{})

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
	r := router.NewRouter(routeRepo, providerRepo, strategyRepo, retryRepo, projectRepo, &stubExecutorSettingsRepo{values: map[string]string{
		domain.SettingKeyStrictSupportModelsRoutingEnabled: "true",
	}})
	if err := r.InitAdapters(); err != nil {
		t.Fatalf("init adapters: %v", err)
	}

	proxyRepo := &recordingProxyRequestRepo{}
	exec := &Executor{
		router:           r,
		proxyRequestRepo: proxyRepo,
		modelMappingRepo: &staticModelMappingRepo{mappings: []*domain.ModelMapping{
			{Pattern: "gpt-5", Target: "moonshotai/kimi-k3", IsEnabled: true},
		}},
		instanceID: "test-instance",
	}

	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := flow.NewCtx(rec, req)
	state := &execState{
		ctx:            req.Context(),
		tenantID:       1,
		clientType:     domain.ClientTypeOpenAI,
		requestModel:   "gpt-5",
		apiTokenID:     46,
		requestBody:    []byte(`{"model":"gpt-5"}`),
		requestURI:     req.URL.RequestURI(),
		requestHeaders: req.Header,
	}
	c.Set(flow.KeyExecutorState, state)

	exec.routeMatch(c)

	if c.Err != nil {
		t.Fatalf("routeMatch error = %v", c.Err)
	}
	if len(state.routes) != 1 || state.routes[0].Provider.ID != 102 {
		t.Fatalf("matched routes = %+v, want only mapped-target provider 102", state.routes)
	}
	if len(proxyRepo.created) != 1 || proxyRepo.created[0].ProviderID != 0 || proxyRepo.created[0].RequestModel != "gpt-5" {
		t.Fatalf("created proxy request = %+v, want pre-dispatch request preserved", proxyRepo.created)
	}
}
