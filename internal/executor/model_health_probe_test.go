package executor

import (
	"context"
	"encoding/json"
	"testing"

	provideradapter "github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/repository/cached"
	"github.com/awsl-project/maxx/internal/router"
)

func TestBuildModelHealthProbeRequestUsesSourceClientType(t *testing.T) {
	tests := []struct {
		name       string
		clientType domain.ClientType
		wantURI    string
		wantField  string
	}{
		{name: "openai", clientType: domain.ClientTypeOpenAI, wantURI: "/v1/chat/completions", wantField: "messages"},
		{name: "claude", clientType: domain.ClientTypeClaude, wantURI: "/v1/messages", wantField: "messages"},
		{name: "codex", clientType: domain.ClientTypeCodex, wantURI: "/responses", wantField: "input"},
		{name: "gemini", clientType: domain.ClientTypeGemini, wantURI: "/v1beta/models/probe-model:generateContent", wantField: "contents"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uri, body, err := buildModelHealthProbeRequest(tt.clientType, "probe-model")
			if err != nil {
				t.Fatalf("buildModelHealthProbeRequest: %v", err)
			}
			if uri != tt.wantURI {
				t.Fatalf("uri = %q, want %q", uri, tt.wantURI)
			}
			var decoded map[string]any
			if err := json.Unmarshal(body, &decoded); err != nil {
				t.Fatalf("body is not JSON: %v", err)
			}
			if _, ok := decoded[tt.wantField]; !ok {
				t.Fatalf("body = %s, missing %q", body, tt.wantField)
			}
		})
	}
}

const modelHealthProbeCodexPathProviderType = "model-health-probe-codex-path-test"

var modelHealthProbeCodexPathSeen struct {
	requestURI          string
	responsesClientPath string
}

type modelHealthProbeCodexPathAdapter struct{}

func init() {
	provideradapter.RegisterAdapterFactory(modelHealthProbeCodexPathProviderType, func(*domain.Provider) (provideradapter.ProviderAdapter, error) {
		return modelHealthProbeCodexPathAdapter{}, nil
	})
}

func (modelHealthProbeCodexPathAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeCodex}
}

func (modelHealthProbeCodexPathAdapter) Execute(c *flow.Ctx, _ *domain.Provider) error {
	modelHealthProbeCodexPathSeen.requestURI = flow.GetRequestURI(c)
	modelHealthProbeCodexPathSeen.responsesClientPath = flow.GetResponsesClientPath(c)
	return nil
}

func TestModelHealthProbeCodexUsesNormalResponsesRoutePath(t *testing.T) {
	modelHealthProbeCodexPathSeen.requestURI = ""
	modelHealthProbeCodexPathSeen.responsesClientPath = ""

	routeRepo := cached.NewRouteRepository(&providerProxyMatchRouteRepo{routes: []*domain.Route{{ID: 101, TenantID: 1, ProviderID: 201, ClientType: domain.ClientTypeCodex, IsEnabled: true, Position: 1}}})
	providerRepo := cached.NewProviderRepository(&providerProxyMatchProviderRepo{providers: []*domain.Provider{{ID: 201, TenantID: 1, Type: modelHealthProbeCodexPathProviderType, Name: "codex-default-route", SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}, SupportModels: []string{"gpt-5"}}}})
	retryRepo := cached.NewRetryConfigRepository(&providerProxyMatchRetryRepo{configs: []*domain.RetryConfig{{ID: 1, TenantID: 1, IsDefault: true, MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0}}})
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
	r := router.NewRouter(routeRepo, providerRepo, strategyRepo, retryRepo, projectRepo, &stubExecutorSettingsRepo{})
	if err := r.InitAdapters(); err != nil {
		t.Fatalf("init adapters: %v", err)
	}
	exec := &Executor{router: r}

	result := exec.ProbeModelHealth(context.Background(), 1, 0, domain.ModelHealthCheckTarget{Model: "gpt-5", ClientType: domain.ClientTypeCodex, RouteID: 101, ProviderID: 201, ProviderName: "codex-default-route"})
	if result.Status != domain.ModelHealthStatusOK {
		t.Fatalf("ProbeModelHealth status = %s error = %q, want ok", result.Status, result.Error)
	}
	if modelHealthProbeCodexPathSeen.requestURI != "/responses" {
		t.Fatalf("request URI = %q, want normalized default route /responses", modelHealthProbeCodexPathSeen.requestURI)
	}
	if modelHealthProbeCodexPathSeen.responsesClientPath != "/v1/responses" {
		t.Fatalf("responses client path = %q, want ordinary user entry /v1/responses", modelHealthProbeCodexPathSeen.responsesClientPath)
	}
}
