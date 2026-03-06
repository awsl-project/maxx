package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

// createTestProvider is a helper that creates a provider and returns its ID.
func createTestProvider(t *testing.T, env *TestEnv) float64 {
	t.Helper()
	provider := map[string]any{
		"name": "route-test-provider",
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
	return created["id"].(float64)
}

func TestListRoutes_Empty(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/routes")
	AssertStatus(t, resp, http.StatusOK)

	var routes []map[string]any
	DecodeJSON(t, resp, &routes)

	if len(routes) != 0 {
		t.Fatalf("Expected 0 routes, got %d", len(routes))
	}
}

func TestCreateRoute(t *testing.T) {
	env := NewTestEnv(t)
	providerID := createTestProvider(t, env)

	route := map[string]any{
		"isEnabled":  true,
		"isNative":   true,
		"clientType": "claude",
		"providerID": providerID,
		"position":   1,
	}

	resp := env.AdminPost("/api/admin/routes", route)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)

	if created["clientType"] != "claude" {
		t.Fatalf("Expected clientType 'claude', got %v", created["clientType"])
	}
	if created["isEnabled"] != true {
		t.Fatalf("Expected isEnabled=true, got %v", created["isEnabled"])
	}

	id, ok := created["id"].(float64)
	if !ok || id == 0 {
		t.Fatalf("Expected non-zero id, got %v", created["id"])
	}

	// Verify it appears in the list
	resp = env.AdminGet("/api/admin/routes")
	AssertStatus(t, resp, http.StatusOK)

	var routes []map[string]any
	DecodeJSON(t, resp, &routes)

	if len(routes) != 1 {
		t.Fatalf("Expected 1 route, got %d", len(routes))
	}
}

func TestGetRoute_ByID(t *testing.T) {
	env := NewTestEnv(t)
	providerID := createTestProvider(t, env)

	route := map[string]any{
		"isEnabled":  true,
		"isNative":   true,
		"clientType": "claude",
		"providerID": providerID,
		"position":   1,
	}

	resp := env.AdminPost("/api/admin/routes", route)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	// Get by ID
	resp = env.AdminGet(fmt.Sprintf("/api/admin/routes/%d", int(id)))
	AssertStatus(t, resp, http.StatusOK)

	var fetched map[string]any
	DecodeJSON(t, resp, &fetched)

	if fetched["clientType"] != "claude" {
		t.Fatalf("Expected clientType 'claude', got %v", fetched["clientType"])
	}
}

func TestUpdateRoute(t *testing.T) {
	env := NewTestEnv(t)
	providerID := createTestProvider(t, env)

	route := map[string]any{
		"isEnabled":  true,
		"isNative":   true,
		"clientType": "claude",
		"providerID": providerID,
		"position":   1,
	}

	resp := env.AdminPost("/api/admin/routes", route)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	// Update the route (disable it)
	updated := map[string]any{
		"isEnabled": false,
	}

	resp = env.AdminPut(fmt.Sprintf("/api/admin/routes/%d", int(id)), updated)
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["isEnabled"] != false {
		t.Fatalf("Expected isEnabled=false, got %v", result["isEnabled"])
	}
}

func TestDeleteRoute(t *testing.T) {
	env := NewTestEnv(t)
	providerID := createTestProvider(t, env)

	route := map[string]any{
		"isEnabled":  true,
		"isNative":   true,
		"clientType": "claude",
		"providerID": providerID,
		"position":   1,
	}

	resp := env.AdminPost("/api/admin/routes", route)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	// Delete the route
	resp = env.AdminDelete(fmt.Sprintf("/api/admin/routes/%d", int(id)))
	AssertStatus(t, resp, http.StatusNoContent)

	// Verify it is soft-deleted (no longer appears in list)
	resp = env.AdminGet("/api/admin/routes")
	AssertStatus(t, resp, http.StatusOK)

	var remaining []map[string]any
	DecodeJSON(t, resp, &remaining)

	if len(remaining) != 0 {
		t.Fatalf("Expected 0 routes in list after delete, got %d", len(remaining))
	}
}

func TestGetRoute_NotFound(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/routes/999999")
	AssertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestUpdateRoute_NotFound(t *testing.T) {
	env := NewTestEnv(t)

	body := map[string]any{
		"isEnabled": false,
	}

	resp := env.AdminPut("/api/admin/routes/999999", body)
	AssertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestCreateRoute_InvalidJSON(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminRawPost("/api/admin/routes", `{not valid json`)
	AssertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestCreateRoute_InvalidProviderID(t *testing.T) {
	env := NewTestEnv(t)

	route := map[string]any{
		"isEnabled":  true,
		"isNative":   true,
		"clientType": "claude",
		"providerID": float64(999999),
		"position":   1,
	}

	// The route should be created (handler does not validate providerID existence)
	// or return an error depending on service logic. Either way it should not panic.
	resp := env.AdminPost("/api/admin/routes", route)
	// Accept either 201 (created with dangling ref) or 500 (FK constraint)
	status := resp.StatusCode
	resp.Body.Close()
	if status != http.StatusCreated && status != http.StatusInternalServerError {
		t.Fatalf("Expected status 201 or 500 for invalid providerID, got %d", status)
	}
}

func TestBatchUpdateRoutePositions_Success(t *testing.T) {
	env := NewTestEnv(t)
	providerID := createTestProvider(t, env)

	// Create two routes
	route1 := map[string]any{
		"isEnabled":  true,
		"isNative":   true,
		"clientType": "claude",
		"providerID": providerID,
		"position":   1,
	}
	resp := env.AdminPost("/api/admin/routes", route1)
	AssertStatus(t, resp, http.StatusCreated)
	var created1 map[string]any
	DecodeJSON(t, resp, &created1)
	id1 := created1["id"].(float64)

	route2 := map[string]any{
		"isEnabled":  true,
		"isNative":   true,
		"clientType": "openai",
		"providerID": providerID,
		"position":   2,
	}
	resp = env.AdminPost("/api/admin/routes", route2)
	AssertStatus(t, resp, http.StatusCreated)
	var created2 map[string]any
	DecodeJSON(t, resp, &created2)
	id2 := created2["id"].(float64)

	// Batch update positions (swap them)
	positions := []map[string]any{
		{"id": id1, "position": 2},
		{"id": id2, "position": 1},
	}

	resp = env.AdminPut("/api/admin/routes/batch-positions", positions)
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["message"] != "positions updated successfully" {
		t.Fatalf("Expected success message, got %v", result["message"])
	}

	// Verify positions were updated
	resp = env.AdminGet(fmt.Sprintf("/api/admin/routes/%d", int(id1)))
	AssertStatus(t, resp, http.StatusOK)
	var fetched1 map[string]any
	DecodeJSON(t, resp, &fetched1)
	if int(fetched1["position"].(float64)) != 2 {
		t.Fatalf("Expected route1 position=2, got %v", fetched1["position"])
	}

	resp = env.AdminGet(fmt.Sprintf("/api/admin/routes/%d", int(id2)))
	AssertStatus(t, resp, http.StatusOK)
	var fetched2 map[string]any
	DecodeJSON(t, resp, &fetched2)
	if int(fetched2["position"].(float64)) != 1 {
		t.Fatalf("Expected route2 position=1, got %v", fetched2["position"])
	}
}
