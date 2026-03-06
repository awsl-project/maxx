package e2e_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestListRoutingStrategies_Empty(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/routing-strategies")
	AssertStatus(t, resp, http.StatusOK)

	var strategies []map[string]any
	DecodeJSON(t, resp, &strategies)

	if len(strategies) != 0 {
		t.Fatalf("Expected 0 routing strategies, got %d", len(strategies))
	}
}

func TestCreateRoutingStrategy(t *testing.T) {
	env := NewTestEnv(t)

	strategy := map[string]any{
		"type":   "priority",
		"config": map[string]any{},
	}

	resp := env.AdminPost("/api/admin/routing-strategies", strategy)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)

	if created["type"] != "priority" {
		t.Fatalf("Expected type 'priority', got %v", created["type"])
	}

	id, ok := created["id"].(float64)
	if !ok || id == 0 {
		t.Fatalf("Expected non-zero id, got %v", created["id"])
	}

	// Verify it appears in the list
	resp = env.AdminGet("/api/admin/routing-strategies")
	AssertStatus(t, resp, http.StatusOK)

	var strategies []map[string]any
	DecodeJSON(t, resp, &strategies)

	if len(strategies) != 1 {
		t.Fatalf("Expected 1 routing strategy, got %d", len(strategies))
	}
}

func TestGetRoutingStrategy_ByID(t *testing.T) {
	env := NewTestEnv(t)

	// Create a project first to use as the lookup key
	project := map[string]any{
		"name":                "strategy-project",
		"slug":                "strategy-project",
		"enabledCustomRoutes": []string{},
	}
	resp := env.AdminPost("/api/admin/projects", project)
	AssertStatus(t, resp, http.StatusCreated)
	var createdProject map[string]any
	DecodeJSON(t, resp, &createdProject)
	projectID := createdProject["id"].(float64)

	strategy := map[string]any{
		"type":      "weighted_random",
		"projectID": projectID,
		"config":    map[string]any{},
	}

	resp = env.AdminPost("/api/admin/routing-strategies", strategy)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)

	// GetRoutingStrategy looks up by projectID, not strategy ID
	resp = env.AdminGet(fmt.Sprintf("/api/admin/routing-strategies/%d", int(projectID)))
	AssertStatus(t, resp, http.StatusOK)

	var fetched map[string]any
	DecodeJSON(t, resp, &fetched)

	if fetched["type"] != "weighted_random" {
		t.Fatalf("Expected type 'weighted_random', got %v", fetched["type"])
	}
}

func TestUpdateRoutingStrategy(t *testing.T) {
	env := NewTestEnv(t)

	// Create a project first to use as the lookup key
	project := map[string]any{
		"name":                "update-strategy-project",
		"slug":                "update-strategy-project",
		"enabledCustomRoutes": []string{},
	}
	resp := env.AdminPost("/api/admin/projects", project)
	AssertStatus(t, resp, http.StatusCreated)
	var createdProject map[string]any
	DecodeJSON(t, resp, &createdProject)
	projectID := createdProject["id"].(float64)

	strategy := map[string]any{
		"type":      "priority",
		"projectID": projectID,
		"config":    map[string]any{},
	}

	resp = env.AdminPost("/api/admin/routing-strategies", strategy)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)

	// Update the routing strategy (lookup by projectID)
	updated := map[string]any{
		"type":   "weighted_random",
		"config": map[string]any{},
	}

	resp = env.AdminPut(fmt.Sprintf("/api/admin/routing-strategies/%d", int(projectID)), updated)
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["type"] != "weighted_random" {
		t.Fatalf("Expected type 'weighted_random', got %v", result["type"])
	}
}

func TestDeleteRoutingStrategy(t *testing.T) {
	env := NewTestEnv(t)

	strategy := map[string]any{
		"type":   "priority",
		"config": map[string]any{},
	}

	resp := env.AdminPost("/api/admin/routing-strategies", strategy)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	// Delete the routing strategy
	resp = env.AdminDelete(fmt.Sprintf("/api/admin/routing-strategies/%d", int(id)))
	AssertStatus(t, resp, http.StatusNoContent)

	// Verify it is gone from the list
	resp = env.AdminGet("/api/admin/routing-strategies")
	AssertStatus(t, resp, http.StatusOK)

	var remaining []map[string]any
	DecodeJSON(t, resp, &remaining)

	if len(remaining) != 0 {
		t.Fatalf("Expected 0 routing strategies in list after delete, got %d", len(remaining))
	}
}

func TestGetRoutingStrategy_NotFound(t *testing.T) {
	env := NewTestEnv(t)

	// Use a non-existent project ID to look up a routing strategy
	resp := env.AdminGet("/api/admin/routing-strategies/999999")
	AssertStatus(t, resp, http.StatusNotFound)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["error"] == nil || result["error"] == "" {
		t.Fatal("Expected error message in not-found response")
	}
}

func TestCreateRoutingStrategy_InvalidJSON(t *testing.T) {
	env := NewTestEnv(t)

	req, err := http.NewRequest(http.MethodPost, env.URL("/api/admin/routing-strategies"), strings.NewReader("invalid json"))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+env.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	AssertStatus(t, resp, http.StatusBadRequest)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["error"] == nil || result["error"] == "" {
		t.Fatal("Expected error message for invalid JSON")
	}
}
