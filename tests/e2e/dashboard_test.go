package e2e_test

import (
	"net/http"
	"testing"
)

func TestGetDashboard_Empty(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/dashboard")
	AssertStatus(t, resp, http.StatusOK)

	var dashboard map[string]any
	DecodeJSON(t, resp, &dashboard)

	// Verify dashboard contains expected top-level keys
	expectedKeys := []string{"today", "yesterday", "allTime", "heatmap", "topModels", "trend24h"}
	for _, key := range expectedKeys {
		if _, exists := dashboard[key]; !exists {
			t.Fatalf("Expected dashboard to contain '%s' key", key)
		}
	}

	// Verify today summary has zero values for fresh environment
	today, ok := dashboard["today"].(map[string]any)
	if !ok {
		t.Fatal("Expected 'today' to be an object")
	}
	if today["requests"].(float64) != 0 {
		t.Fatalf("Expected 0 requests today, got %v", today["requests"])
	}
}

func TestGetDashboard_ResponseStructure(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/dashboard")
	AssertStatus(t, resp, http.StatusOK)

	var dashboard map[string]any
	DecodeJSON(t, resp, &dashboard)

	// Verify all expected top-level keys exist
	expectedKeys := []string{"today", "yesterday", "allTime", "heatmap", "topModels", "trend24h"}
	for _, key := range expectedKeys {
		if _, exists := dashboard[key]; !exists {
			t.Fatalf("Expected dashboard to contain '%s' key", key)
		}
	}

	// Verify today summary structure has expected fields
	today, ok := dashboard["today"].(map[string]any)
	if !ok {
		t.Fatal("Expected 'today' to be an object")
	}
	todayFields := []string{"requests", "tokens", "cost"}
	for _, field := range todayFields {
		if _, exists := today[field]; !exists {
			t.Fatalf("Expected 'today' to contain '%s' field", field)
		}
	}

	// Verify yesterday summary structure
	yesterday, ok := dashboard["yesterday"].(map[string]any)
	if !ok {
		t.Fatal("Expected 'yesterday' to be an object")
	}
	for _, field := range todayFields {
		if _, exists := yesterday[field]; !exists {
			t.Fatalf("Expected 'yesterday' to contain '%s' field", field)
		}
	}

	// Verify allTime summary structure
	allTime, ok := dashboard["allTime"].(map[string]any)
	if !ok {
		t.Fatal("Expected 'allTime' to be an object")
	}
	for _, field := range todayFields {
		if _, exists := allTime[field]; !exists {
			t.Fatalf("Expected 'allTime' to contain '%s' field", field)
		}
	}

	// Verify heatmap is an array
	if _, ok := dashboard["heatmap"].([]any); !ok {
		t.Fatal("Expected 'heatmap' to be an array")
	}

	// Verify topModels is an array
	if _, ok := dashboard["topModels"].([]any); !ok {
		t.Fatal("Expected 'topModels' to be an array")
	}

	// Verify trend24h is an array
	if _, ok := dashboard["trend24h"].([]any); !ok {
		t.Fatal("Expected 'trend24h' to be an array")
	}
}
