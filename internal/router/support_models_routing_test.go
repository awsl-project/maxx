package router

import (
	"errors"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository/cached"
	"github.com/awsl-project/maxx/internal/systemsettingcache"
)

type supportModelSettingRepo struct{ values map[string]string }

func (r *supportModelSettingRepo) Get(key string) (string, error) {
	if r == nil || r.values == nil {
		return "", domain.ErrNotFound
	}
	value, ok := r.values[key]
	if !ok {
		return "", domain.ErrNotFound
	}
	return value, nil
}

func (r *supportModelSettingRepo) Set(key, value string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	r.values[key] = value
	return nil
}

func (r *supportModelSettingRepo) GetAll() ([]*domain.SystemSetting, error) {
	result := make([]*domain.SystemSetting, 0, len(r.values))
	for key, value := range r.values {
		result = append(result, &domain.SystemSetting{Key: key, Value: value})
	}
	return result, nil
}

func (r *supportModelSettingRepo) Delete(key string) error {
	delete(r.values, key)
	return nil
}

func newSupportModelRoutingTestRouter(t *testing.T, strict bool, routes []*domain.Route, providers []*domain.Provider) *Router {
	t.Helper()
	routeRepo := cached.NewRouteRepository(&wsRouterRouteRepo{routes: routes})
	providerRepo := cached.NewProviderRepository(&wsRouterProviderRepo{providers: providers})
	retryRepo := cached.NewRetryConfigRepository(wsRouterRetryRepo{})
	strategyRepo := cached.NewRoutingStrategyRepository(wsRouterStrategyRepo{})
	projectRepo := cached.NewProjectRepository(&wsRouterProjectRepo{})
	settingRepo := &supportModelSettingRepo{values: map[string]string{
		domain.SettingKeyStrictSupportModelsRoutingEnabled: map[bool]string{true: "true", false: "false"}[strict],
	}}
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
	systemsettingcache.Invalidate(domain.SettingKeyStrictSupportModelsRoutingEnabled)
	r := NewRouter(routeRepo, providerRepo, strategyRepo, retryRepo, projectRepo, settingRepo)
	if err := r.InitAdapters(); err != nil {
		t.Fatalf("InitAdapters: %v", err)
	}
	return r
}

func TestStrictSupportModelsRoutingSkipsUnsupportedProvider(t *testing.T) {
	r := newSupportModelRoutingTestRouter(t, true,
		[]*domain.Route{
			{ID: 1, TenantID: 1, ProviderID: 101, ClientType: domain.ClientTypeCodex, IsEnabled: true, Position: 1},
			{ID: 2, TenantID: 1, ProviderID: 102, ClientType: domain.ClientTypeCodex, IsEnabled: true, Position: 2},
		},
		[]*domain.Provider{
			{ID: 101, TenantID: 1, Type: wsRouterNativeType, Name: "unsupported", SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}, SupportModels: []string{"other-model"}},
			{ID: 102, TenantID: 1, Type: wsRouterNativeType, Name: "supported", SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}, SupportModels: []string{"codex-target"}},
		},
	)

	result, err := r.Match(&MatchContext{TenantID: 1, ClientType: domain.ClientTypeCodex, RequestModel: "codex-target"})
	if err != nil {
		t.Fatalf("Match() error = %v", err)
	}
	if got := matchedProviderIDs(result); len(got) != 1 || got[0] != 102 {
		t.Fatalf("matched provider IDs = %v, want [102]", got)
	}
}

func TestStrictSupportModelsRoutingDisabledKeepsLegacyCandidates(t *testing.T) {
	r := newSupportModelRoutingTestRouter(t, false,
		[]*domain.Route{
			{ID: 1, TenantID: 1, ProviderID: 101, ClientType: domain.ClientTypeCodex, IsEnabled: true, Position: 1},
			{ID: 2, TenantID: 1, ProviderID: 102, ClientType: domain.ClientTypeCodex, IsEnabled: true, Position: 2},
		},
		[]*domain.Provider{
			{ID: 101, TenantID: 1, Type: wsRouterNativeType, Name: "legacy-first", SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}, SupportModels: []string{"other-model"}},
			{ID: 102, TenantID: 1, Type: wsRouterNativeType, Name: "supported", SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}, SupportModels: []string{"codex-target"}},
		},
	)

	result, err := r.Match(&MatchContext{TenantID: 1, ClientType: domain.ClientTypeCodex, RequestModel: "codex-target"})
	if err != nil {
		t.Fatalf("Match() error = %v", err)
	}
	if got := matchedProviderIDs(result); len(got) != 2 || got[0] != 101 || got[1] != 102 {
		t.Fatalf("matched provider IDs = %v, want [101 102]", got)
	}
}

func TestStrictSupportModelsRoutingReturnsModelUnsupportedWhenAllSkipped(t *testing.T) {
	r := newSupportModelRoutingTestRouter(t, true,
		[]*domain.Route{
			{ID: 1, TenantID: 1, ProviderID: 101, ClientType: domain.ClientTypeCodex, IsEnabled: true, Position: 1},
		},
		[]*domain.Provider{
			{ID: 101, TenantID: 1, Type: wsRouterNativeType, Name: "unsupported", SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex}, SupportModels: []string{"other-model"}},
		},
	)

	_, err := r.Match(&MatchContext{TenantID: 1, ClientType: domain.ClientTypeCodex, RequestModel: "codex-target"})
	if !errors.Is(err, domain.ErrModelNotSupported) {
		t.Fatalf("Match() error = %v, want ErrModelNotSupported", err)
	}
}
