package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

func TestListModelMappings(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/model-mappings")
	AssertStatus(t, resp, http.StatusOK)

	var mappings []map[string]any
	DecodeJSON(t, resp, &mappings)

	// May or may not have default mappings; just verify the endpoint works
	if mappings == nil {
		t.Fatalf("Expected non-nil mappings list")
	}
}

func TestCreateModelMapping(t *testing.T) {
	env := NewTestEnv(t)

	mapping := map[string]any{
		"scope":    "global",
		"pattern":  "claude-3-opus*",
		"target":   "claude-sonnet-4-20250514",
		"priority": 10,
	}

	resp := env.AdminPost("/api/admin/model-mappings", mapping)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)

	if created["pattern"] != "claude-3-opus*" {
		t.Fatalf("Expected pattern 'claude-3-opus*', got %v", created["pattern"])
	}
	if created["target"] != "claude-sonnet-4-20250514" {
		t.Fatalf("Expected target 'claude-sonnet-4-20250514', got %v", created["target"])
	}

	id, ok := created["id"].(float64)
	if !ok || id == 0 {
		t.Fatalf("Expected non-zero id, got %v", created["id"])
	}
}

func TestUpdateModelMapping(t *testing.T) {
	env := NewTestEnv(t)

	mapping := map[string]any{
		"scope":    "global",
		"pattern":  "gpt-4*",
		"target":   "claude-sonnet-4-20250514",
		"priority": 20,
	}

	resp := env.AdminPost("/api/admin/model-mappings", mapping)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	// Update the mapping
	updateBody := map[string]any{
		"target":   "claude-opus-4-20250514",
		"priority": 5,
	}

	resp = env.AdminPut(fmt.Sprintf("/api/admin/model-mappings/%d", int(id)), updateBody)
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["target"] != "claude-opus-4-20250514" {
		t.Fatalf("Expected target 'claude-opus-4-20250514', got %v", result["target"])
	}
}

func TestDeleteModelMapping(t *testing.T) {
	env := NewTestEnv(t)

	mapping := map[string]any{
		"scope":    "global",
		"pattern":  "delete-test-*",
		"target":   "some-model",
		"priority": 100,
	}

	resp := env.AdminPost("/api/admin/model-mappings", mapping)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	// Delete the mapping
	resp = env.AdminDelete(fmt.Sprintf("/api/admin/model-mappings/%d", int(id)))
	AssertStatus(t, resp, http.StatusNoContent)
}

func TestGetModelMapping_ByID(t *testing.T) {
	env := NewTestEnv(t)

	// Create a mapping first
	mapping := map[string]any{
		"scope":    "global",
		"pattern":  "get-by-id-*",
		"target":   "claude-sonnet-4-20250514",
		"priority": 15,
	}

	resp := env.AdminPost("/api/admin/model-mappings", mapping)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	// Get by ID
	resp = env.AdminGet(fmt.Sprintf("/api/admin/model-mappings/%d", int(id)))
	AssertStatus(t, resp, http.StatusOK)

	var fetched map[string]any
	DecodeJSON(t, resp, &fetched)

	if fetched["pattern"] != "get-by-id-*" {
		t.Fatalf("Expected pattern 'get-by-id-*', got %v", fetched["pattern"])
	}
	if fetched["target"] != "claude-sonnet-4-20250514" {
		t.Fatalf("Expected target 'claude-sonnet-4-20250514', got %v", fetched["target"])
	}
}

func TestGetModelMapping_NotFound(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/model-mappings/99999")
	AssertStatus(t, resp, http.StatusNotFound)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["error"] != "mapping not found" {
		t.Fatalf("Expected error 'mapping not found', got %v", result["error"])
	}
}

func TestUpdateModelMapping_NotFound(t *testing.T) {
	env := NewTestEnv(t)

	updateBody := map[string]any{
		"target":   "some-model",
		"priority": 1,
	}

	resp := env.AdminPut("/api/admin/model-mappings/99999", updateBody)
	AssertStatus(t, resp, http.StatusNotFound)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["error"] != "mapping not found" {
		t.Fatalf("Expected error 'mapping not found', got %v", result["error"])
	}
}

func TestClearAllModelMappings(t *testing.T) {
	env := NewTestEnv(t)

	// Create a mapping first
	mapping := map[string]any{
		"scope":    "global",
		"pattern":  "clear-all-test-*",
		"target":   "some-model",
		"priority": 50,
	}
	resp := env.AdminPost("/api/admin/model-mappings", mapping)
	AssertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Clear all mappings
	resp = env.AdminDelete("/api/admin/model-mappings/clear-all")
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["message"] != "all mappings cleared" {
		t.Fatalf("Expected message 'all mappings cleared', got %v", result["message"])
	}

	// Verify all mappings are gone
	resp = env.AdminGet("/api/admin/model-mappings")
	AssertStatus(t, resp, http.StatusOK)

	var mappings []map[string]any
	DecodeJSON(t, resp, &mappings)

	if len(mappings) != 0 {
		t.Fatalf("Expected 0 mappings after clear-all, got %d", len(mappings))
	}
}

func TestResetModelMappingsToDefaults(t *testing.T) {
	env := NewTestEnv(t)

	// First clear all mappings
	resp := env.AdminDelete("/api/admin/model-mappings/clear-all")
	AssertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// Reset to defaults
	resp = env.AdminPost("/api/admin/model-mappings/reset-defaults", nil)
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["message"] != "mappings reset to defaults" {
		t.Fatalf("Expected message 'mappings reset to defaults', got %v", result["message"])
	}
}
