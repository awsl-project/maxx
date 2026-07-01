package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
)

func TestListProviders_Empty(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/providers")
	AssertStatus(t, resp, http.StatusOK)

	var providers []map[string]any
	DecodeJSON(t, resp, &providers)

	if len(providers) != 0 {
		t.Fatalf("Expected 0 providers, got %d", len(providers))
	}
}

func TestCreateProvider(t *testing.T) {
	env := NewTestEnv(t)

	provider := map[string]any{
		"name": "test-provider",
		"type": "custom",
		"config": map[string]any{
			"custom": map[string]any{
				"baseURL": "https://api.example.com",
				"apiKey":  "sk-test-key",
			},
		},
		"supportedClientTypes": []string{"claude"},
	}

	resp := env.AdminPost("/api/admin/providers", provider)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)

	if created["name"] != "test-provider" {
		t.Fatalf("Expected name 'test-provider', got %v", created["name"])
	}
	if created["type"] != "custom" {
		t.Fatalf("Expected type 'custom', got %v", created["type"])
	}

	id, ok := created["id"].(float64)
	if !ok || id == 0 {
		t.Fatalf("Expected non-zero id, got %v", created["id"])
	}

	// Verify it appears in the list
	resp = env.AdminGet("/api/admin/providers")
	AssertStatus(t, resp, http.StatusOK)

	var providers []map[string]any
	DecodeJSON(t, resp, &providers)

	if len(providers) != 1 {
		t.Fatalf("Expected 1 provider, got %d", len(providers))
	}
}

func TestGetProvider_ByID(t *testing.T) {
	env := NewTestEnv(t)

	// Create a provider first
	provider := map[string]any{
		"name": "get-test-provider",
		"type": "custom",
		"config": map[string]any{
			"custom": map[string]any{
				"baseURL": "https://api.example.com",
				"apiKey":  "sk-test-key",
			},
		},
		"supportedClientTypes": []string{"claude"},
	}

	resp := env.AdminPost("/api/admin/providers", provider)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	// Get by ID
	resp = env.AdminGet(fmt.Sprintf("/api/admin/providers/%d", int(id)))
	AssertStatus(t, resp, http.StatusOK)

	var fetched map[string]any
	DecodeJSON(t, resp, &fetched)

	if fetched["name"] != "get-test-provider" {
		t.Fatalf("Expected name 'get-test-provider', got %v", fetched["name"])
	}
}

func TestUpdateProvider(t *testing.T) {
	env := NewTestEnv(t)

	// Create a provider
	provider := map[string]any{
		"name": "update-test-provider",
		"type": "custom",
		"config": map[string]any{
			"custom": map[string]any{
				"baseURL": "https://api.example.com",
				"apiKey":  "sk-test-key",
			},
		},
		"supportedClientTypes": []string{"claude"},
	}

	resp := env.AdminPost("/api/admin/providers", provider)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	// Update the provider
	updated := map[string]any{
		"name": "updated-provider-name",
		"type": "custom",
		"config": map[string]any{
			"custom": map[string]any{
				"baseURL": "https://api.updated.com",
				"apiKey":  "sk-updated-key",
			},
		},
		"supportedClientTypes": []string{"claude", "openai"},
	}

	resp = env.AdminPut(fmt.Sprintf("/api/admin/providers/%d", int(id)), updated)
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["name"] != "updated-provider-name" {
		t.Fatalf("Expected name 'updated-provider-name', got %v", result["name"])
	}
}

func TestDeleteProvider(t *testing.T) {
	env := NewTestEnv(t)

	// Create a provider
	provider := map[string]any{
		"name": "delete-test-provider",
		"type": "custom",
		"config": map[string]any{
			"custom": map[string]any{
				"baseURL": "https://api.example.com",
				"apiKey":  "sk-test-key",
			},
		},
		"supportedClientTypes": []string{"claude"},
	}

	resp := env.AdminPost("/api/admin/providers", provider)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	// Delete the provider
	resp = env.AdminDelete(fmt.Sprintf("/api/admin/providers/%d", int(id)))
	AssertStatus(t, resp, http.StatusNoContent)

	// Verify it is soft-deleted (still returned but with deletedAt set)
	resp = env.AdminGet("/api/admin/providers")
	AssertStatus(t, resp, http.StatusOK)

	var remaining []map[string]any
	DecodeJSON(t, resp, &remaining)

	if len(remaining) != 0 {
		t.Fatalf("Expected 0 providers in list after delete, got %d", len(remaining))
	}
}

func TestGetProvider_NotFound(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/providers/999999")
	AssertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestUpdateProvider_NotFound(t *testing.T) {
	env := NewTestEnv(t)

	body := map[string]any{
		"name": "ghost-provider",
		"type": "custom",
		"config": map[string]any{
			"custom": map[string]any{
				"baseURL": "https://api.example.com",
				"apiKey":  "sk-test-key",
			},
		},
		"supportedClientTypes": []string{"claude"},
	}

	resp := env.AdminPut("/api/admin/providers/999999", body)
	AssertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestCreateProvider_InvalidJSON(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminRawPost("/api/admin/providers", `{invalid json!!!}`)
	AssertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestCreateProvider_EmptyBody(t *testing.T) {
	env := NewTestEnv(t)

	// Empty object - should still succeed (no required field validation at handler level)
	resp := env.AdminPost("/api/admin/providers", map[string]any{})
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)

	id, ok := created["id"].(float64)
	if !ok || id == 0 {
		t.Fatalf("Expected non-zero id, got %v", created["id"])
	}
}

func TestBulkDeleteProvidersEndToEndCleansReferencesAndPreservesScope(t *testing.T) {
	env := NewTestEnv(t)

	projectID := createE2EProject(t, env, "bulk-delete-project", "bulk-delete-project")
	deleteAID := createE2EProvider(t, env, "bulk-delete-a")
	deleteBID := createE2EProvider(t, env, "bulk-delete-b")
	keepID := createE2EProvider(t, env, "bulk-delete-keep")

	createE2ERoute(t, env, deleteAID, 0, "claude")
	createE2ERoute(t, env, deleteAID, projectID, "claude")
	createE2ERoute(t, env, deleteBID, projectID, "openai")
	keepRouteID := createE2ERoute(t, env, keepID, projectID, "claude")

	createE2EProviderModelMapping(t, env, deleteAID, "delete-a-*", "target-a")
	createE2EProviderModelMapping(t, env, deleteBID, "delete-b-*", "target-b")
	keepMappingID := createE2EProviderModelMapping(t, env, keepID, "keep-*", "target-keep")
	globalMappingID := createE2EGlobalModelMapping(t, env, "global-*", "target-global")

	otherTenantID, otherProviderID := seedOtherTenantProviderReferences(t, env)

	resp := env.AdminPost("/api/admin/providers/bulk-delete", map[string]any{
		"ids": []uint64{deleteAID, deleteBID, 999999},
	})
	AssertStatus(t, resp, http.StatusOK)

	var result domain.ProviderBulkDeleteResult
	DecodeJSON(t, resp, &result)
	assertUintSet(t, "deletedIDs", result.DeletedIDs, []uint64{deleteAID, deleteBID})
	assertUintSet(t, "notFoundIDs", result.NotFoundIDs, []uint64{999999})
	if result.DeletedCount != 2 {
		t.Fatalf("expected 2 deleted providers, got %d", result.DeletedCount)
	}
	if result.RouteDeletedCount != 3 {
		t.Fatalf("expected 3 deleted routes, got %d", result.RouteDeletedCount)
	}
	if result.ModelMappingDeletedCount != 2 {
		t.Fatalf("expected 2 deleted provider mappings, got %d", result.ModelMappingDeletedCount)
	}

	resp = env.AdminGet("/api/admin/providers")
	AssertStatus(t, resp, http.StatusOK)
	var providers []map[string]any
	DecodeJSON(t, resp, &providers)
	assertListedIDs(t, "providers after bulk delete", providers, []uint64{keepID})

	resp = env.AdminGet("/api/admin/routes")
	AssertStatus(t, resp, http.StatusOK)
	var routes []map[string]any
	DecodeJSON(t, resp, &routes)
	assertListedIDs(t, "routes after bulk delete", routes, []uint64{keepRouteID})
	if gotProviderID := uint64(routes[0]["providerID"].(float64)); gotProviderID != keepID {
		t.Fatalf("expected remaining route to belong to provider %d, got %d", keepID, gotProviderID)
	}

	resp = env.AdminGet("/api/admin/model-mappings")
	AssertStatus(t, resp, http.StatusOK)
	var mappings []map[string]any
	DecodeJSON(t, resp, &mappings)
	assertListedIDs(t, "model mappings after bulk delete", mappings, []uint64{keepMappingID, globalMappingID})
	for _, mapping := range mappings {
		if providerID, ok := mapping["providerID"].(float64); ok && uint64(providerID) != 0 && uint64(providerID) != keepID {
			t.Fatalf("unexpected provider-scoped mapping remained after bulk delete: %#v", mapping)
		}
	}

	otherProviders, err := sqlite.NewProviderRepository(env.DB).List(otherTenantID)
	if err != nil {
		t.Fatalf("list other tenant providers: %v", err)
	}
	assertDomainProviderIDs(t, "other tenant providers", otherProviders, []uint64{otherProviderID})

	otherRoutes, err := sqlite.NewRouteRepository(env.DB).List(otherTenantID)
	if err != nil {
		t.Fatalf("list other tenant routes: %v", err)
	}
	if len(otherRoutes) != 1 || otherRoutes[0].ProviderID != otherProviderID {
		t.Fatalf("expected other tenant route to remain for provider %d, got %#v", otherProviderID, otherRoutes)
	}

	otherMappings, err := sqlite.NewModelMappingRepository(env.DB).List(otherTenantID)
	if err != nil {
		t.Fatalf("list other tenant mappings: %v", err)
	}
	if len(otherMappings) != 1 || otherMappings[0].ProviderID != otherProviderID {
		t.Fatalf("expected other tenant provider mapping to remain for provider %d, got %#v", otherProviderID, otherMappings)
	}
}

func TestProviders_Unauthorized(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.UnauthGet("/api/admin/providers")
	AssertStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()
}

func TestProvidersExport(t *testing.T) {
	env := NewTestEnv(t)

	// Create a provider first
	provider := map[string]any{
		"name": "export-test-provider",
		"type": "custom",
		"config": map[string]any{
			"custom": map[string]any{
				"baseURL": "https://api.example.com",
				"apiKey":  "sk-test-key",
			},
		},
		"supportedClientTypes": []string{"claude"},
	}
	resp := env.AdminPost("/api/admin/providers", provider)
	AssertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Export providers
	resp = env.AdminGet("/api/admin/providers/export")
	AssertStatus(t, resp, http.StatusOK)

	body := ReadBody(t, resp)
	// Verify it is valid JSON array
	var exported []json.RawMessage
	if err := json.Unmarshal([]byte(body), &exported); err != nil {
		t.Fatalf("Expected valid JSON array from export, got error: %v", err)
	}
	if len(exported) == 0 {
		t.Fatal("Expected at least 1 exported provider")
	}
}

func TestProvidersImport(t *testing.T) {
	env := NewTestEnv(t)

	// Import an array of providers
	providers := []map[string]any{
		{
			"name": "imported-provider-1",
			"type": "custom",
			"config": map[string]any{
				"custom": map[string]any{
					"baseURL": "https://api.import1.com",
					"apiKey":  "sk-import-1",
				},
			},
			"supportedClientTypes": []string{"claude"},
		},
		{
			"name": "imported-provider-2",
			"type": "custom",
			"config": map[string]any{
				"custom": map[string]any{
					"baseURL": "https://api.import2.com",
					"apiKey":  "sk-import-2",
				},
			},
			"supportedClientTypes": []string{"openai"},
		},
	}

	resp := env.AdminPost("/api/admin/providers/import", providers)
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	// Verify the providers were imported by listing
	resp = env.AdminGet("/api/admin/providers")
	AssertStatus(t, resp, http.StatusOK)

	var list []map[string]any
	DecodeJSON(t, resp, &list)

	if len(list) < 2 {
		t.Fatalf("Expected at least 2 providers after import, got %d", len(list))
	}
}

func TestCreateProvider_SQLInjection(t *testing.T) {
	env := NewTestEnv(t)

	provider := map[string]any{
		"name": "'; DROP TABLE providers; --",
		"type": "custom",
		"config": map[string]any{
			"custom": map[string]any{
				"baseURL": "https://api.example.com",
				"apiKey":  "sk-test-key",
			},
		},
		"supportedClientTypes": []string{"claude"},
	}

	resp := env.AdminPost("/api/admin/providers", provider)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)

	// The SQL injection payload should be stored as a literal string
	if created["name"] != "'; DROP TABLE providers; --" {
		t.Fatalf("Expected SQL injection string stored literally, got %v", created["name"])
	}

	// Verify providers table still works
	resp = env.AdminGet("/api/admin/providers")
	AssertStatus(t, resp, http.StatusOK)

	var providers []map[string]any
	DecodeJSON(t, resp, &providers)

	if len(providers) != 1 {
		t.Fatalf("Expected 1 provider (table should not be dropped), got %d", len(providers))
	}

	// Verify we can get it by ID
	id := int(created["id"].(float64))
	resp = env.AdminGet(fmt.Sprintf("/api/admin/providers/%d", id))
	AssertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func createE2EProvider(t *testing.T, env *TestEnv, name string) uint64 {
	t.Helper()
	resp := env.AdminPost("/api/admin/providers", map[string]any{
		"name": name,
		"type": "custom",
		"config": map[string]any{
			"custom": map[string]any{
				"baseURL": "https://api.example.com/" + name,
				"apiKey":  "sk-test-key",
			},
		},
		"supportedClientTypes": []string{"claude", "openai"},
	})
	AssertStatus(t, resp, http.StatusCreated)
	var created map[string]any
	DecodeJSON(t, resp, &created)
	return uint64(created["id"].(float64))
}

func createE2EProject(t *testing.T, env *TestEnv, name, slug string) uint64 {
	t.Helper()
	resp := env.AdminPost("/api/admin/projects", map[string]any{
		"name":                name,
		"slug":                slug,
		"enabledCustomRoutes": []string{"claude", "openai"},
	})
	AssertStatus(t, resp, http.StatusCreated)
	var created map[string]any
	DecodeJSON(t, resp, &created)
	return uint64(created["id"].(float64))
}

func createE2ERoute(t *testing.T, env *TestEnv, providerID, projectID uint64, clientType string) uint64 {
	t.Helper()
	resp := env.AdminPost("/api/admin/routes", map[string]any{
		"isEnabled":  true,
		"isNative":   true,
		"projectID":  projectID,
		"clientType": clientType,
		"providerID": providerID,
		"position":   1,
	})
	AssertStatus(t, resp, http.StatusCreated)
	var created map[string]any
	DecodeJSON(t, resp, &created)
	return uint64(created["id"].(float64))
}

func createE2EProviderModelMapping(t *testing.T, env *TestEnv, providerID uint64, pattern, target string) uint64 {
	t.Helper()
	return createE2EModelMapping(t, env, map[string]any{
		"scope":      "provider",
		"providerID": providerID,
		"pattern":    pattern,
		"target":     target,
		"priority":   10,
	})
}

func createE2EGlobalModelMapping(t *testing.T, env *TestEnv, pattern, target string) uint64 {
	t.Helper()
	return createE2EModelMapping(t, env, map[string]any{
		"scope":    "global",
		"pattern":  pattern,
		"target":   target,
		"priority": 20,
	})
}

func createE2EModelMapping(t *testing.T, env *TestEnv, body map[string]any) uint64 {
	t.Helper()
	resp := env.AdminPost("/api/admin/model-mappings", body)
	AssertStatus(t, resp, http.StatusCreated)
	var created map[string]any
	DecodeJSON(t, resp, &created)
	return uint64(created["id"].(float64))
}

func seedOtherTenantProviderReferences(t *testing.T, env *TestEnv) (uint64, uint64) {
	t.Helper()
	tenantRepo := sqlite.NewTenantRepository(env.DB)
	otherTenant := &domain.Tenant{Name: "Other Tenant", Slug: "other-tenant"}
	if err := tenantRepo.Create(otherTenant); err != nil {
		t.Fatalf("create other tenant: %v", err)
	}

	providerRepo := sqlite.NewProviderRepository(env.DB)
	otherProvider := &domain.Provider{
		TenantID: otherTenant.ID,
		Name:     "other-tenant-provider",
		Type:     "custom",
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL: "https://api.example.com/other",
			APIKey:  "sk-other",
		}},
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeClaude},
	}
	if err := providerRepo.Create(otherProvider); err != nil {
		t.Fatalf("create other tenant provider: %v", err)
	}

	routeRepo := sqlite.NewRouteRepository(env.DB)
	if err := routeRepo.Create(&domain.Route{
		TenantID:   otherTenant.ID,
		IsEnabled:  true,
		IsNative:   true,
		ProjectID:  0,
		ClientType: domain.ClientTypeClaude,
		ProviderID: otherProvider.ID,
		Position:   1,
	}); err != nil {
		t.Fatalf("create other tenant route: %v", err)
	}

	mappingRepo := sqlite.NewModelMappingRepository(env.DB)
	if err := mappingRepo.Create(&domain.ModelMapping{
		TenantID:   otherTenant.ID,
		Scope:      domain.ModelMappingScopeProvider,
		ProviderID: otherProvider.ID,
		Pattern:    "other-*",
		Target:     "other-target",
		Priority:   1,
	}); err != nil {
		t.Fatalf("create other tenant mapping: %v", err)
	}

	return otherTenant.ID, otherProvider.ID
}

func assertListedIDs(t *testing.T, label string, items []map[string]any, want []uint64) {
	t.Helper()
	got := make([]uint64, 0, len(items))
	for _, item := range items {
		got = append(got, uint64(item["id"].(float64)))
	}
	assertUintSet(t, label, got, want)
}

func assertDomainProviderIDs(t *testing.T, label string, items []*domain.Provider, want []uint64) {
	t.Helper()
	got := make([]uint64, 0, len(items))
	for _, item := range items {
		got = append(got, item.ID)
	}
	assertUintSet(t, label, got, want)
}

func assertUintSet(t *testing.T, label string, got []uint64, want []uint64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: expected ids %v, got %v", label, want, got)
	}
	counts := make(map[uint64]int, len(want))
	for _, id := range want {
		counts[id]++
	}
	for _, id := range got {
		counts[id]--
	}
	for id, count := range counts {
		if count != 0 {
			t.Fatalf("%s: expected ids %v, got %v; id %d count delta %d", label, want, got, id, count)
		}
	}
}
