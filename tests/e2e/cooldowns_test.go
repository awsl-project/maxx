package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestListCooldowns_Empty(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/cooldowns")
	AssertStatus(t, resp, http.StatusOK)

	// Response should be null or empty array when no cooldowns are set
	body := ReadBody(t, resp)
	if body != "null\n" && body != "[]\n" {
		t.Fatalf("Expected null or empty array for cooldowns, got %s", body)
	}
}

func TestSetCooldown(t *testing.T) {
	env := NewTestEnv(t)

	// Create a provider first
	provider := map[string]any{
		"name": "cooldown-test-provider",
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
	providerID := int(created["id"].(float64))

	// Set cooldown for this provider (1 hour from now)
	untilTime := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
	cooldownReq := map[string]any{
		"untilTime": untilTime,
	}

	resp = env.AdminPut(fmt.Sprintf("/api/admin/cooldowns/%d", providerID), cooldownReq)
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["message"] != "cooldown set" {
		t.Fatalf("Expected message 'cooldown set', got %v", result["message"])
	}

	// Verify cooldown appears in list
	resp = env.AdminGet("/api/admin/cooldowns")
	AssertStatus(t, resp, http.StatusOK)

	var cooldowns []map[string]any
	DecodeJSON(t, resp, &cooldowns)

	if len(cooldowns) == 0 {
		t.Fatal("Expected at least 1 cooldown after setting one")
	}
}

func TestClearCooldown(t *testing.T) {
	env := NewTestEnv(t)

	// Create a provider
	provider := map[string]any{
		"name": "clear-cooldown-provider",
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
	providerID := int(created["id"].(float64))

	// Set a cooldown
	untilTime := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
	cooldownReq := map[string]any{
		"untilTime": untilTime,
	}
	resp = env.AdminPut(fmt.Sprintf("/api/admin/cooldowns/%d", providerID), cooldownReq)
	AssertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	// Clear the cooldown
	resp = env.AdminDelete(fmt.Sprintf("/api/admin/cooldowns/%d", providerID))
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["message"] != "cooldown cleared" {
		t.Fatalf("Expected message 'cooldown cleared', got %v", result["message"])
	}
}

func TestSetCooldown_InvalidProvider(t *testing.T) {
	env := NewTestEnv(t)

	// Try to set cooldown for a non-existent provider
	untilTime := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
	cooldownReq := map[string]any{
		"untilTime": untilTime,
	}

	resp := env.AdminPut("/api/admin/cooldowns/99999", cooldownReq)
	AssertStatus(t, resp, http.StatusNotFound)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["error"] != "provider not found" {
		t.Fatalf("Expected error 'provider not found', got %v", result["error"])
	}
}

func TestClearCooldown_InvalidProvider(t *testing.T) {
	env := NewTestEnv(t)

	// Try to clear cooldown for a non-existent provider
	resp := env.AdminDelete("/api/admin/cooldowns/99999")
	AssertStatus(t, resp, http.StatusNotFound)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["error"] != "provider not found" {
		t.Fatalf("Expected error 'provider not found', got %v", result["error"])
	}
}

func TestSetCooldown_WithClientType(t *testing.T) {
	env := NewTestEnv(t)

	// Create a provider first
	provider := map[string]any{
		"name": "clienttype-cooldown-provider",
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
	providerID := int(created["id"].(float64))

	// Set cooldown with clientType
	untilTime := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
	cooldownReq := map[string]any{
		"untilTime":  untilTime,
		"clientType": "claude",
	}

	resp = env.AdminPut(fmt.Sprintf("/api/admin/cooldowns/%d", providerID), cooldownReq)
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["message"] != "cooldown set" {
		t.Fatalf("Expected message 'cooldown set', got %v", result["message"])
	}

	// Verify cooldown appears in list
	resp = env.AdminGet("/api/admin/cooldowns")
	AssertStatus(t, resp, http.StatusOK)

	var cooldowns []map[string]any
	DecodeJSON(t, resp, &cooldowns)

	if len(cooldowns) == 0 {
		t.Fatal("Expected at least 1 cooldown after setting one with clientType")
	}
}
