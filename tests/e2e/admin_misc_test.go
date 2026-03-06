package e2e_test

import (
	"net/http"
	"testing"
)

func TestProviderStats(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/provider-stats")
	AssertStatus(t, resp, http.StatusOK)

	var stats map[string]any
	DecodeJSON(t, resp, &stats)

	// Provider stats should return a valid JSON object
	if stats == nil {
		t.Fatal("Expected non-nil provider stats response")
	}
}

func TestResponseModels(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/response-models")
	AssertStatus(t, resp, http.StatusOK)

	var models []any
	DecodeJSON(t, resp, &models)

	// Fresh environment should have no response models (or an empty list)
	// Just verify we get a valid JSON array response
	if models == nil {
		t.Fatal("Expected non-nil response models array")
	}
}

func TestPricing(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/pricing")
	AssertStatus(t, resp, http.StatusOK)

	var pricing map[string]any
	DecodeJSON(t, resp, &pricing)

	// Pricing should return a valid JSON object
	if pricing == nil {
		t.Fatal("Expected non-nil pricing response")
	}
}

func TestAdminLogs(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/logs")
	// Logs endpoint may return 200 with empty lines or 500 if log path is not set.
	// In test env the logPath is "", so we accept either a valid response or an error.
	if resp.StatusCode == http.StatusOK {
		var result map[string]any
		DecodeJSON(t, resp, &result)

		// Verify response structure has lines and count
		if _, exists := result["lines"]; !exists {
			t.Fatal("Expected 'lines' field in logs response")
		}
		if _, exists := result["count"]; !exists {
			t.Fatal("Expected 'count' field in logs response")
		}
	} else if resp.StatusCode == http.StatusInternalServerError {
		// Acceptable: log file not configured in test environment
		var result map[string]any
		DecodeJSON(t, resp, &result)

		if result["error"] == nil || result["error"] == "" {
			t.Fatal("Expected error message when log file is not available")
		}
	} else {
		t.Fatalf("Expected status 200 or 500 for logs endpoint, got %d", resp.StatusCode)
	}
}
