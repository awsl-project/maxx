package e2e_test

import (
	"net/http"
	"testing"
)

func TestGetModels(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.UnauthGet("/v1/models")
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	// OpenAI-style response should have "object" and "data" fields
	if result["object"] != "list" {
		t.Fatalf("Expected object 'list', got %v", result["object"])
	}

	data, ok := result["data"].([]any)
	if !ok {
		t.Fatal("Expected 'data' to be an array")
	}

	// In a fresh environment with no providers, the model list comes from
	// the default pricing table, so it should not be empty
	if len(data) == 0 {
		t.Fatal("Expected at least some models from default pricing table")
	}

	// Verify each model entry has expected fields
	for i, item := range data {
		model, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("Expected model entry %d to be an object", i)
		}
		if model["id"] == nil || model["id"] == "" {
			t.Fatalf("Expected model entry %d to have non-empty 'id'", i)
		}
		if model["object"] != "model" {
			t.Fatalf("Expected model entry %d object to be 'model', got %v", i, model["object"])
		}
	}
}

func TestGetModels_ResponseFormat(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.UnauthGet("/v1/models")
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	// Verify OpenAI-compatible top-level fields
	if result["object"] != "list" {
		t.Fatalf("Expected top-level 'object' to be 'list', got %v", result["object"])
	}

	data, ok := result["data"].([]any)
	if !ok {
		t.Fatal("Expected 'data' to be an array")
	}

	if len(data) == 0 {
		t.Fatal("Expected at least one model in data array")
	}

	// Verify OpenAI-compatible fields on each model entry
	model, ok := data[0].(map[string]any)
	if !ok {
		t.Fatal("Expected first model entry to be an object")
	}

	requiredFields := []string{"id", "object", "created", "owned_by"}
	for _, field := range requiredFields {
		if _, exists := model[field]; !exists {
			t.Fatalf("Expected model entry to contain OpenAI-compatible field '%s'", field)
		}
	}

	if model["object"] != "model" {
		t.Fatalf("Expected model entry 'object' to be 'model', got %v", model["object"])
	}
}
