package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
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
