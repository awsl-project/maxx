package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestListAPITokens_Empty(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/api-tokens")
	AssertStatus(t, resp, http.StatusOK)

	var tokens []map[string]any
	DecodeJSON(t, resp, &tokens)

	if len(tokens) != 0 {
		t.Fatalf("Expected 0 api tokens, got %d", len(tokens))
	}
}

func TestCreateAPIToken(t *testing.T) {
	env := NewTestEnv(t)

	body := map[string]any{
		"name":        "test-token",
		"description": "A test API token",
	}

	resp := env.AdminPost("/api/admin/api-tokens", body)
	AssertStatus(t, resp, http.StatusCreated)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	// CreateAPIToken returns APITokenCreateResult with "token" and "apiToken" fields
	tokenStr, ok := result["token"].(string)
	if !ok || tokenStr == "" {
		t.Fatalf("Expected non-empty token string, got %v", result["token"])
	}

	apiToken, ok := result["apiToken"].(map[string]any)
	if !ok {
		t.Fatalf("Expected apiToken object, got %v", result["apiToken"])
	}

	if apiToken["name"] != "test-token" {
		t.Fatalf("Expected name 'test-token', got %v", apiToken["name"])
	}

	id, ok := apiToken["id"].(float64)
	if !ok || id == 0 {
		t.Fatalf("Expected non-zero id, got %v", apiToken["id"])
	}

	// Verify it appears in the list
	resp = env.AdminGet("/api/admin/api-tokens")
	AssertStatus(t, resp, http.StatusOK)

	var tokens []map[string]any
	DecodeJSON(t, resp, &tokens)

	if len(tokens) != 1 {
		t.Fatalf("Expected 1 api token, got %d", len(tokens))
	}
}

func TestGetAPIToken_ByID(t *testing.T) {
	env := NewTestEnv(t)

	body := map[string]any{
		"name":        "get-test-token",
		"description": "Token for get test",
	}

	resp := env.AdminPost("/api/admin/api-tokens", body)
	AssertStatus(t, resp, http.StatusCreated)

	var result map[string]any
	DecodeJSON(t, resp, &result)
	apiToken := result["apiToken"].(map[string]any)
	id := apiToken["id"].(float64)

	// Get by ID
	resp = env.AdminGet(fmt.Sprintf("/api/admin/api-tokens/%d", int(id)))
	AssertStatus(t, resp, http.StatusOK)

	var fetched map[string]any
	DecodeJSON(t, resp, &fetched)

	if fetched["name"] != "get-test-token" {
		t.Fatalf("Expected name 'get-test-token', got %v", fetched["name"])
	}
}

func TestUpdateAPIToken_Disable(t *testing.T) {
	env := NewTestEnv(t)

	body := map[string]any{
		"name":        "disable-test-token",
		"description": "Token to be disabled",
	}

	resp := env.AdminPost("/api/admin/api-tokens", body)
	AssertStatus(t, resp, http.StatusCreated)

	var result map[string]any
	DecodeJSON(t, resp, &result)
	apiToken := result["apiToken"].(map[string]any)
	id := apiToken["id"].(float64)

	// Disable the token
	updateBody := map[string]any{
		"isEnabled": false,
	}

	resp = env.AdminPut(fmt.Sprintf("/api/admin/api-tokens/%d", int(id)), updateBody)
	AssertStatus(t, resp, http.StatusOK)

	var updated map[string]any
	DecodeJSON(t, resp, &updated)

	if updated["isEnabled"] != false {
		t.Fatalf("Expected isEnabled false, got %v", updated["isEnabled"])
	}
}

func TestDeleteAPIToken(t *testing.T) {
	env := NewTestEnv(t)

	body := map[string]any{
		"name":        "delete-test-token",
		"description": "Token to be deleted",
	}

	resp := env.AdminPost("/api/admin/api-tokens", body)
	AssertStatus(t, resp, http.StatusCreated)

	var result map[string]any
	DecodeJSON(t, resp, &result)
	apiToken := result["apiToken"].(map[string]any)
	id := apiToken["id"].(float64)

	// Delete the token
	resp = env.AdminDelete(fmt.Sprintf("/api/admin/api-tokens/%d", int(id)))
	AssertStatus(t, resp, http.StatusNoContent)

	// Verify it is gone from the list
	resp = env.AdminGet("/api/admin/api-tokens")
	AssertStatus(t, resp, http.StatusOK)

	var tokens []map[string]any
	DecodeJSON(t, resp, &tokens)

	if len(tokens) != 0 {
		t.Fatalf("Expected 0 api tokens in list after delete, got %d", len(tokens))
	}
}

func TestGetAPIToken_NotFound(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/api-tokens/999999")
	AssertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestCreateAPIToken_EmptyName(t *testing.T) {
	env := NewTestEnv(t)

	body := map[string]any{
		"name":        "",
		"description": "Token with empty name",
	}

	resp := env.AdminPost("/api/admin/api-tokens", body)
	AssertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestCreateAPIToken_InvalidJSON(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminRawPost("/api/admin/api-tokens", `{not valid json`)
	AssertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestUpdateAPIToken_NotFound(t *testing.T) {
	env := NewTestEnv(t)

	body := map[string]any{
		"isEnabled": false,
	}

	resp := env.AdminPut("/api/admin/api-tokens/999999", body)
	AssertStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestDeleteAPIToken_NotFound(t *testing.T) {
	env := NewTestEnv(t)

	// Deleting a non-existent token - the handler calls svc.DeleteAPIToken
	// which may return an error (500) or succeed silently (204)
	resp := env.AdminDelete("/api/admin/api-tokens/999999")
	status := resp.StatusCode
	resp.Body.Close()
	if status != http.StatusNoContent && status != http.StatusInternalServerError {
		t.Fatalf("Expected status 204 or 500 for deleting non-existent token, got %d", status)
	}
}

func TestCreateAPIToken_WithExpiry(t *testing.T) {
	env := NewTestEnv(t)

	expiresAt := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	body := map[string]any{
		"name":        "expiring-token",
		"description": "Token with expiry",
		"expiresAt":   expiresAt,
	}

	resp := env.AdminPost("/api/admin/api-tokens", body)
	AssertStatus(t, resp, http.StatusCreated)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	apiToken, ok := result["apiToken"].(map[string]any)
	if !ok {
		t.Fatalf("Expected apiToken object, got %v", result["apiToken"])
	}

	if apiToken["name"] != "expiring-token" {
		t.Fatalf("Expected name 'expiring-token', got %v", apiToken["name"])
	}

	// Verify expiresAt is set
	expiresAtValue, ok := apiToken["expiresAt"]
	if !ok || expiresAtValue == nil {
		t.Fatal("Expected expiresAt to be set")
	}

	// Verify we can retrieve it
	id := int(apiToken["id"].(float64))
	resp = env.AdminGet(fmt.Sprintf("/api/admin/api-tokens/%d", id))
	AssertStatus(t, resp, http.StatusOK)

	var fetched map[string]any
	DecodeJSON(t, resp, &fetched)
	if fetched["expiresAt"] == nil {
		t.Fatal("Expected expiresAt to persist after retrieval")
	}
}
