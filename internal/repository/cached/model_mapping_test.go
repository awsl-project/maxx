package cached

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

type modelMappingTestRepo struct {
	mappings []*domain.ModelMapping
}

func (r *modelMappingTestRepo) Create(mapping *domain.ModelMapping) error {
	if mapping.ID == 0 {
		mapping.ID = uint64(len(r.mappings) + 1)
	}
	r.mappings = append(r.mappings, mapping)
	return nil
}

func (r *modelMappingTestRepo) Update(mapping *domain.ModelMapping) error { return nil }
func (r *modelMappingTestRepo) Delete(uint64, uint64) error               { return nil }
func (r *modelMappingTestRepo) GetByID(uint64, uint64) (*domain.ModelMapping, error) {
	return nil, domain.ErrNotFound
}
func (r *modelMappingTestRepo) List(uint64) ([]*domain.ModelMapping, error) {
	return append([]*domain.ModelMapping(nil), r.mappings...), nil
}
func (r *modelMappingTestRepo) ListEnabled(tenantID uint64) ([]*domain.ModelMapping, error) {
	list, err := r.List(tenantID)
	if err != nil {
		return nil, err
	}
	result := make([]*domain.ModelMapping, 0, len(list))
	for _, mapping := range list {
		if mapping.IsEnabled {
			result = append(result, mapping)
		}
	}
	return result, nil
}
func (r *modelMappingTestRepo) ListByClientType(uint64, domain.ClientType) ([]*domain.ModelMapping, error) {
	return nil, nil
}
func (r *modelMappingTestRepo) ListByQuery(uint64, *domain.ModelMappingQuery) ([]*domain.ModelMapping, error) {
	return nil, nil
}
func (r *modelMappingTestRepo) Reorder(uint64, domain.ModelMappingReorderRequest) error { return nil }
func (r *modelMappingTestRepo) Count(uint64) (int, error)                               { return len(r.mappings), nil }
func (r *modelMappingTestRepo) DeleteAll(uint64) error                                  { return nil }
func (r *modelMappingTestRepo) ClearAll(uint64) error                                   { return nil }
func (r *modelMappingTestRepo) SeedDefaults(uint64) error                               { return nil }

func TestModelMappingRepositoryCachedReadMethodsSkipDisabledMappings(t *testing.T) {
	baseRepo := &modelMappingTestRepo{mappings: []*domain.ModelMapping{
		{ID: 1, TenantID: 1, Scope: domain.ModelMappingScopeGlobal, ClientType: domain.ClientTypeClaude, Pattern: "disabled-*", Target: "disabled-target", IsEnabled: false},
		{ID: 2, TenantID: 1, Scope: domain.ModelMappingScopeGlobal, ClientType: domain.ClientTypeClaude, Pattern: "enabled-*", Target: "enabled-target", IsEnabled: true},
		{ID: 3, TenantID: 2, Scope: domain.ModelMappingScopeGlobal, ClientType: domain.ClientTypeClaude, Pattern: "other-*", Target: "other-target", IsEnabled: true},
	}}
	repo := NewModelMappingRepository(baseRepo)
	if err := repo.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	enabled, err := repo.ListEnabled(1)
	if err != nil {
		t.Fatalf("ListEnabled() error = %v", err)
	}
	if len(enabled) != 1 || enabled[0].ID != 2 {
		t.Fatalf("ListEnabled() = %#v, want only enabled tenant mapping", enabled)
	}

	byClient, err := repo.ListByClientType(1, domain.ClientTypeClaude)
	if err != nil {
		t.Fatalf("ListByClientType() error = %v", err)
	}
	if len(byClient) != 1 || byClient[0].ID != 2 {
		t.Fatalf("ListByClientType() = %#v, want only enabled tenant mapping", byClient)
	}

	byQuery, err := repo.ListByQuery(1, &domain.ModelMappingQuery{ClientType: domain.ClientTypeClaude})
	if err != nil {
		t.Fatalf("ListByQuery() error = %v", err)
	}
	if len(byQuery) != 1 || byQuery[0].ID != 2 {
		t.Fatalf("ListByQuery() = %#v, want only enabled tenant mapping", byQuery)
	}
}

func TestWorkBuddyProviderMappingScopeIsolation(t *testing.T) {
	repo := NewModelMappingRepository(&modelMappingTestRepo{mappings: []*domain.ModelMapping{
		{ID: 1, TenantID: 1, Scope: domain.ModelMappingScopeProvider, ProviderType: "workbuddy-ai", ProviderID: 41, Pattern: "alias", Target: "target", IsEnabled: true},
		{ID: 2, TenantID: 1, Scope: domain.ModelMappingScopeRoute, ProviderType: "workbuddy-ai", ProviderID: 41, RouteID: 9, ClientType: domain.ClientTypeOpenAI, Pattern: "alias", Target: "route-target", IsEnabled: true},
	}})
	if err := repo.Load(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		client   domain.ClientType
		provider uint64
		typ      string
		route    uint64
		count    int
	}{
		{"native", domain.ClientTypeOpenAI, 41, "workbuddy-ai", 8, 1},
		{"converted", domain.ClientTypeClaude, 41, "workbuddy-ai", 8, 1},
		{"route override", domain.ClientTypeOpenAI, 41, "workbuddy-ai", 9, 2},
		{"route protocol isolated", domain.ClientTypeClaude, 41, "workbuddy-ai", 9, 1},
		{"other provider", domain.ClientTypeOpenAI, 42, "workbuddy-ai", 8, 0},
		{"other type", domain.ClientTypeOpenAI, 41, "custom", 8, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.ListByQuery(1, &domain.ModelMappingQuery{ClientType: tc.client, ProviderID: tc.provider, ProviderType: tc.typ, RouteID: tc.route})
			if err != nil || len(got) != tc.count {
				t.Fatalf("count=%d want=%d err=%v", len(got), tc.count, err)
			}
		})
	}
}
