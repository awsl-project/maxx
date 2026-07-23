package sqlite

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestRouteRepositoryBulkDeleteScopesByTenantClientAndProject(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("NewDBWithDSN() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo := NewRouteRepository(db)
	routes := []*domain.Route{
		{TenantID: 1, ProjectID: 0, ClientType: domain.ClientTypeClaude, ProviderID: 101, IsEnabled: true, IsNative: true, Position: 1, Weight: 1},
		{TenantID: 1, ProjectID: 0, ClientType: domain.ClientTypeClaude, ProviderID: 102, IsEnabled: true, IsNative: false, Position: 2, Weight: 1},
		{TenantID: 1, ProjectID: 7, ClientType: domain.ClientTypeClaude, ProviderID: 103, IsEnabled: true, IsNative: true, Position: 3, Weight: 1},
		{TenantID: 1, ProjectID: 0, ClientType: domain.ClientTypeOpenAI, ProviderID: 104, IsEnabled: true, IsNative: true, Position: 4, Weight: 1},
		{TenantID: 2, ProjectID: 0, ClientType: domain.ClientTypeClaude, ProviderID: 105, IsEnabled: true, IsNative: true, Position: 5, Weight: 1},
	}
	for _, route := range routes {
		if err := repo.Create(route); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	result, err := repo.BulkDelete(1, domain.RouteBulkDeleteRequest{
		IDs: []uint64{
			routes[0].ID,
			routes[1].ID,
			routes[2].ID,
			routes[3].ID,
			routes[4].ID,
			999999,
			routes[0].ID,
		},
		ClientType: domain.ClientTypeClaude,
		ProjectID:  0,
	})
	if err != nil {
		t.Fatalf("BulkDelete() error = %v", err)
	}

	if result.DeletedCount != 2 {
		t.Fatalf("DeletedCount = %d, want 2 (result: %#v)", result.DeletedCount, result)
	}
	assertContainsRouteID(t, result.DeletedIDs, routes[0].ID)
	assertContainsRouteID(t, result.DeletedIDs, routes[1].ID)
	assertContainsRouteID(t, result.SkippedIDs, routes[2].ID)
	assertContainsRouteID(t, result.SkippedIDs, routes[3].ID)
	assertContainsRouteID(t, result.NotFoundIDs, routes[4].ID)
	assertContainsRouteID(t, result.NotFoundIDs, 999999)

	remaining, err := repo.List(1)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("remaining tenant 1 routes = %d, want 2", len(remaining))
	}
	for _, route := range remaining {
		if route.ProjectID == 0 && route.ClientType == domain.ClientTypeClaude {
			t.Fatalf("deleted route still visible after BulkDelete: %#v", route)
		}
	}

	otherTenantRoutes, err := repo.List(2)
	if err != nil {
		t.Fatalf("List(other tenant) error = %v", err)
	}
	if len(otherTenantRoutes) != 1 || otherTenantRoutes[0].ID != routes[4].ID {
		t.Fatalf("other tenant route was affected: %#v", otherTenantRoutes)
	}
}

func assertContainsRouteID(t *testing.T, ids []uint64, want uint64) {
	t.Helper()
	for _, id := range ids {
		if id == want {
			return
		}
	}
	t.Fatalf("ids %v does not contain %d", ids, want)
}
