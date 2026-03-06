package e2e_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestListRetryConfigs_Empty(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/retry-configs")
	AssertStatus(t, resp, http.StatusOK)

	var configs []map[string]any
	DecodeJSON(t, resp, &configs)

	if len(configs) != 0 {
		t.Fatalf("Expected 0 retry configs, got %d", len(configs))
	}
}

func TestCreateRetryConfig(t *testing.T) {
	env := NewTestEnv(t)

	config := map[string]any{
		"name":            "test-retry-config",
		"maxRetries":      3,
		"initialInterval": 1000000000, // 1 second in nanoseconds
		"backoffRate":     2.0,
		"maxInterval":     10000000000, // 10 seconds in nanoseconds
	}

	resp := env.AdminPost("/api/admin/retry-configs", config)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)

	if created["name"] != "test-retry-config" {
		t.Fatalf("Expected name 'test-retry-config', got %v", created["name"])
	}

	id, ok := created["id"].(float64)
	if !ok || id == 0 {
		t.Fatalf("Expected non-zero id, got %v", created["id"])
	}

	// Verify it appears in the list
	resp = env.AdminGet("/api/admin/retry-configs")
	AssertStatus(t, resp, http.StatusOK)

	var configs []map[string]any
	DecodeJSON(t, resp, &configs)

	if len(configs) != 1 {
		t.Fatalf("Expected 1 retry config, got %d", len(configs))
	}
}

func TestGetRetryConfig_ByID(t *testing.T) {
	env := NewTestEnv(t)

	config := map[string]any{
		"name":            "get-test-retry-config",
		"maxRetries":      2,
		"initialInterval": 500000000,
		"backoffRate":     1.5,
		"maxInterval":     5000000000,
	}

	resp := env.AdminPost("/api/admin/retry-configs", config)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	// Get by ID
	resp = env.AdminGet(fmt.Sprintf("/api/admin/retry-configs/%d", int(id)))
	AssertStatus(t, resp, http.StatusOK)

	var fetched map[string]any
	DecodeJSON(t, resp, &fetched)

	if fetched["name"] != "get-test-retry-config" {
		t.Fatalf("Expected name 'get-test-retry-config', got %v", fetched["name"])
	}
}

func TestUpdateRetryConfig(t *testing.T) {
	env := NewTestEnv(t)

	config := map[string]any{
		"name":            "update-test-retry-config",
		"maxRetries":      3,
		"initialInterval": 1000000000,
		"backoffRate":     2.0,
		"maxInterval":     10000000000,
	}

	resp := env.AdminPost("/api/admin/retry-configs", config)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	// Update the retry config
	updated := map[string]any{
		"name":            "updated-retry-config",
		"maxRetries":      5,
		"initialInterval": 2000000000,
		"backoffRate":     3.0,
		"maxInterval":     30000000000,
	}

	resp = env.AdminPut(fmt.Sprintf("/api/admin/retry-configs/%d", int(id)), updated)
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["name"] != "updated-retry-config" {
		t.Fatalf("Expected name 'updated-retry-config', got %v", result["name"])
	}
}

func TestDeleteRetryConfig(t *testing.T) {
	env := NewTestEnv(t)

	config := map[string]any{
		"name":            "delete-test-retry-config",
		"maxRetries":      1,
		"initialInterval": 1000000000,
		"backoffRate":     1.0,
		"maxInterval":     1000000000,
	}

	resp := env.AdminPost("/api/admin/retry-configs", config)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	// Delete the retry config
	resp = env.AdminDelete(fmt.Sprintf("/api/admin/retry-configs/%d", int(id)))
	AssertStatus(t, resp, http.StatusNoContent)

	// Verify it is gone from the list
	resp = env.AdminGet("/api/admin/retry-configs")
	AssertStatus(t, resp, http.StatusOK)

	var remaining []map[string]any
	DecodeJSON(t, resp, &remaining)

	if len(remaining) != 0 {
		t.Fatalf("Expected 0 retry configs in list after delete, got %d", len(remaining))
	}
}

func TestGetRetryConfig_NotFound(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/retry-configs/999999")
	AssertStatus(t, resp, http.StatusNotFound)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["error"] == nil || result["error"] == "" {
		t.Fatal("Expected error message in not-found response")
	}
}

func TestCreateRetryConfig_InvalidJSON(t *testing.T) {
	env := NewTestEnv(t)

	// Send a raw string that is not valid JSON as the body
	req, err := http.NewRequest(http.MethodPost, env.URL("/api/admin/retry-configs"), strings.NewReader("not valid json"))
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
