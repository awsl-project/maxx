package sqlite

import (
	"errors"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestModelMappingRepositoryReorderUpdatesPrioritiesAtomically(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("NewDBWithDSN() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo := NewModelMappingRepository(db)
	mappings := []*domain.ModelMapping{
		{TenantID: 1, Scope: domain.ModelMappingScopeProvider, ProviderID: 101, Pattern: "a", Target: "target-a", Priority: 0},
		{TenantID: 1, Scope: domain.ModelMappingScopeProvider, ProviderID: 101, Pattern: "b", Target: "target-b", Priority: 10},
		{TenantID: 1, Scope: domain.ModelMappingScopeProvider, ProviderID: 101, Pattern: "c", Target: "target-c", Priority: 20},
	}
	for _, mapping := range mappings {
		if err := repo.Create(mapping); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	if err := repo.Reorder(1, domain.ModelMappingReorderRequest{
		Scope:      domain.ModelMappingScopeProvider,
		ProviderID: 101,
		OrderedIDs: []uint64{mappings[2].ID, mappings[0].ID, mappings[1].ID},
	}); err != nil {
		t.Fatalf("Reorder() error = %v", err)
	}

	got, err := repo.ListByQuery(1, &domain.ModelMappingQuery{ProviderID: 101})
	if err != nil {
		t.Fatalf("ListByQuery() error = %v", err)
	}
	wantIDs := []uint64{mappings[2].ID, mappings[0].ID, mappings[1].ID}
	for i, wantID := range wantIDs {
		if got[i].ID != wantID {
			t.Fatalf("order[%d] = %d, want %d (all: %#v)", i, got[i].ID, wantID, got)
		}
		if got[i].Priority != i*10 {
			t.Fatalf("priority[%d] = %d, want %d", i, got[i].Priority, i*10)
		}
	}
}

func TestModelMappingRepositoryReorderRejectsIncompleteProviderOrder(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("NewDBWithDSN() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo := NewModelMappingRepository(db)
	mappings := []*domain.ModelMapping{
		{TenantID: 1, Scope: domain.ModelMappingScopeProvider, ProviderID: 101, Pattern: "a", Target: "target-a", Priority: 0},
		{TenantID: 1, Scope: domain.ModelMappingScopeProvider, ProviderID: 101, Pattern: "b", Target: "target-b", Priority: 10},
	}
	for _, mapping := range mappings {
		if err := repo.Create(mapping); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	if err := repo.Reorder(1, domain.ModelMappingReorderRequest{
		Scope:      domain.ModelMappingScopeProvider,
		ProviderID: 101,
		OrderedIDs: []uint64{mappings[1].ID},
	}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("Reorder() incomplete order error = %v, want ErrInvalidInput", err)
	}

	got, err := repo.ListByQuery(1, &domain.ModelMappingQuery{ProviderID: 101})
	if err != nil {
		t.Fatalf("ListByQuery() error = %v", err)
	}
	if got[0].ID != mappings[0].ID || got[0].Priority != 0 || got[1].ID != mappings[1].ID || got[1].Priority != 10 {
		t.Fatalf("incomplete reorder should not mutate priorities: %#v", got)
	}
}

func TestModelMappingRepositoryReorderRejectsMissingProviderID(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("NewDBWithDSN() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo := NewModelMappingRepository(db)
	mapping := &domain.ModelMapping{TenantID: 1, Scope: domain.ModelMappingScopeProvider, ProviderID: 101, Pattern: "a", Target: "target-a", Priority: 0}
	if err := repo.Create(mapping); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := repo.Reorder(1, domain.ModelMappingReorderRequest{
		Scope:      domain.ModelMappingScopeProvider,
		OrderedIDs: []uint64{mapping.ID},
	}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("Reorder() missing providerID error = %v, want ErrInvalidInput", err)
	}
}

func TestModelMappingRepositoryReorderRejectsScopeOrProviderMismatch(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("NewDBWithDSN() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo := NewModelMappingRepository(db)
	providerMapping := &domain.ModelMapping{TenantID: 1, Scope: domain.ModelMappingScopeProvider, ProviderID: 101, Pattern: "a", Target: "target-a", Priority: 0}
	otherProviderMapping := &domain.ModelMapping{TenantID: 1, Scope: domain.ModelMappingScopeProvider, ProviderID: 202, Pattern: "b", Target: "target-b", Priority: 10}
	globalMapping := &domain.ModelMapping{TenantID: 1, Scope: domain.ModelMappingScopeGlobal, Pattern: "c", Target: "target-c", Priority: 20}
	for _, mapping := range []*domain.ModelMapping{providerMapping, otherProviderMapping, globalMapping} {
		if err := repo.Create(mapping); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	if err := repo.Reorder(1, domain.ModelMappingReorderRequest{
		Scope:      domain.ModelMappingScopeProvider,
		ProviderID: 101,
		OrderedIDs: []uint64{providerMapping.ID, otherProviderMapping.ID},
	}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("Reorder() provider mismatch error = %v, want ErrInvalidInput", err)
	}
	if err := repo.Reorder(1, domain.ModelMappingReorderRequest{
		Scope:      domain.ModelMappingScopeProvider,
		ProviderID: 101,
		OrderedIDs: []uint64{providerMapping.ID, globalMapping.ID},
	}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("Reorder() scope mismatch error = %v, want ErrInvalidInput", err)
	}
}
