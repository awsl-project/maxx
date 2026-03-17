package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

func TestBackupExport(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/backup/export")
	AssertStatus(t, resp, http.StatusOK)

	var backup map[string]any
	DecodeJSON(t, resp, &backup)

	if backup["version"] == nil {
		t.Fatal("Expected backup to contain 'version' field")
	}
	if backup["exportedAt"] == nil {
		t.Fatal("Expected backup to contain 'exportedAt' field")
	}
	_, ok := backup["data"].(map[string]any)
	if !ok {
		t.Fatal("Expected backup to contain 'data' object")
	}
}

func TestBackupImport_DryRun(t *testing.T) {
	env := NewTestEnv(t)

	// First create a provider so there's data to export
	provider := map[string]any{
		"name": "backup-test-provider",
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

	// Export
	resp = env.AdminGet("/api/admin/backup/export")
	AssertStatus(t, resp, http.StatusOK)

	var backup map[string]any
	DecodeJSON(t, resp, &backup)

	// Import with dryRun=true
	resp = env.AdminPost("/api/admin/backup/import?dryRun=true&conflictStrategy=skip", backup)
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["success"] != true {
		t.Fatalf("Expected import dry run to succeed, got %v", result["success"])
	}
}

func TestBackupExportImport_RoundTrip(t *testing.T) {
	env := NewTestEnv(t)

	// Create a provider
	provider := map[string]any{
		"name": "roundtrip-provider",
		"type": "custom",
		"config": map[string]any{
			"custom": map[string]any{
				"baseURL": "https://api.roundtrip.com",
				"apiKey":  "sk-roundtrip-key",
			},
		},
		"supportedClientTypes": []string{"claude"},
	}
	resp := env.AdminPost("/api/admin/providers", provider)
	AssertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Export current state
	resp = env.AdminGet("/api/admin/backup/export")
	AssertStatus(t, resp, http.StatusOK)

	var backup map[string]any
	DecodeJSON(t, resp, &backup)

	// Get the provider ID, then delete it
	resp = env.AdminGet("/api/admin/providers")
	AssertStatus(t, resp, http.StatusOK)

	var providers []map[string]any
	DecodeJSON(t, resp, &providers)

	if len(providers) != 1 {
		t.Fatalf("Expected 1 provider before delete, got %d", len(providers))
	}

	providerID := fmt.Sprintf("%.0f", providers[0]["id"].(float64))
	resp = env.AdminDelete("/api/admin/providers/" + providerID)
	resp.Body.Close()

	// Verify provider is deleted
	resp = env.AdminGet("/api/admin/providers")
	AssertStatus(t, resp, http.StatusOK)
	var emptyProviders []map[string]any
	DecodeJSON(t, resp, &emptyProviders)
	if len(emptyProviders) != 0 {
		t.Fatalf("Expected 0 providers after delete, got %d", len(emptyProviders))
	}

	// Import the backup to restore the deleted provider
	resp = env.AdminPost("/api/admin/backup/import?conflictStrategy=overwrite", backup)
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["success"] != true {
		t.Fatalf("Expected import to succeed, got %v", result["success"])
	}

	// Verify provider was restored
	resp = env.AdminGet("/api/admin/providers")
	AssertStatus(t, resp, http.StatusOK)

	var afterProviders []map[string]any
	DecodeJSON(t, resp, &afterProviders)

	if len(afterProviders) < 1 {
		t.Fatal("Expected at least 1 provider after round-trip import")
	}

	found := false
	for _, p := range afterProviders {
		if p["name"] == "roundtrip-provider" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Expected 'roundtrip-provider' to exist after round-trip import")
	}
}

func TestBackupImport_InvalidJSON(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminRawPost("/api/admin/backup/import?conflictStrategy=skip", "{not valid json!!!")
	AssertStatus(t, resp, http.StatusBadRequest)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	errMsg, ok := result["error"].(string)
	if !ok || errMsg == "" {
		t.Fatal("Expected non-empty error message for invalid JSON import")
	}
}

func TestBackupImport_SkipStrategy(t *testing.T) {
	env := NewTestEnv(t)

	// Create a provider
	provider := map[string]any{
		"name": "skip-strategy-provider",
		"type": "custom",
		"config": map[string]any{
			"custom": map[string]any{
				"baseURL": "https://api.skip.com",
				"apiKey":  "sk-skip-key",
			},
		},
		"supportedClientTypes": []string{"claude"},
	}
	resp := env.AdminPost("/api/admin/providers", provider)
	AssertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Export
	resp = env.AdminGet("/api/admin/backup/export")
	AssertStatus(t, resp, http.StatusOK)

	var backup map[string]any
	DecodeJSON(t, resp, &backup)

	// Import with skip strategy (provider already exists, should be skipped)
	resp = env.AdminPost("/api/admin/backup/import?conflictStrategy=skip", backup)
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["success"] != true {
		t.Fatalf("Expected import with skip strategy to succeed, got %v", result["success"])
	}
}

func TestBackupExport_ExcludedProviderIsOmitted(t *testing.T) {
	env := NewTestEnv(t)

	provider := map[string]any{
		"name":              "excluded-provider",
		"type":              "custom",
		"excludeFromExport": true,
		"config": map[string]any{
			"custom": map[string]any{
				"baseURL": "https://api.example.com",
				"apiKey":  "sk-hidden-key",
			},
		},
		"supportedClientTypes": []string{"claude"},
	}
	resp := env.AdminPost("/api/admin/providers", provider)
	AssertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = env.AdminGet("/api/admin/backup/export")
	AssertStatus(t, resp, http.StatusOK)

	var backup map[string]any
	DecodeJSON(t, resp, &backup)

	data := backup["data"].(map[string]any)
	providers, _ := data["providers"].([]any)
	if len(providers) != 0 {
		t.Fatalf("expected excluded providers to be omitted from backup export, got %d entries", len(providers))
	}
}
