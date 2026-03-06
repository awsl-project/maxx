package e2e_test

import (
	"net/http"
	"testing"
)

func TestGetAllSettings(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/settings")
	AssertStatus(t, resp, http.StatusOK)

	var settings map[string]any
	DecodeJSON(t, resp, &settings)

	// At minimum, the JWT secret setting should exist (set during NewTestEnv)
	if len(settings) == 0 {
		t.Fatalf("Expected at least 1 setting, got 0")
	}
}

func TestSetSetting(t *testing.T) {
	env := NewTestEnv(t)

	body := map[string]any{
		"value": "Asia/Tokyo",
	}

	resp := env.AdminPut("/api/admin/settings/timezone", body)
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["key"] != "timezone" {
		t.Fatalf("Expected key 'timezone', got %v", result["key"])
	}
	if result["value"] != "Asia/Tokyo" {
		t.Fatalf("Expected value 'Asia/Tokyo', got %v", result["value"])
	}
}

func TestGetSetting_ByKey(t *testing.T) {
	env := NewTestEnv(t)

	// Set a setting first
	body := map[string]any{
		"value": "9090",
	}
	resp := env.AdminPut("/api/admin/settings/proxy_port", body)
	AssertStatus(t, resp, http.StatusOK)

	// Get the setting by key
	resp = env.AdminGet("/api/admin/settings/proxy_port")
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["key"] != "proxy_port" {
		t.Fatalf("Expected key 'proxy_port', got %v", result["key"])
	}
	if result["value"] != "9090" {
		t.Fatalf("Expected value '9090', got %v", result["value"])
	}
}

func TestDeleteSetting(t *testing.T) {
	env := NewTestEnv(t)

	// Set a setting first
	body := map[string]any{
		"value": "120",
	}
	resp := env.AdminPut("/api/admin/settings/request_retention_hours", body)
	AssertStatus(t, resp, http.StatusOK)

	// Delete the setting
	resp = env.AdminDelete("/api/admin/settings/request_retention_hours")
	AssertStatus(t, resp, http.StatusNoContent)
}

func TestSetSetting_InvalidJSON(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminRawPut("/api/admin/settings/timezone", "{not valid json!!!")
	AssertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestGetSetting_NonExistent(t *testing.T) {
	env := NewTestEnv(t)

	// Getting a non-existent key returns 200 with empty value (SQLite repo returns "" without error)
	resp := env.AdminGet("/api/admin/settings/nonexistent_key_12345")
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["key"] != "nonexistent_key_12345" {
		t.Fatalf("Expected key 'nonexistent_key_12345', got %v", result["key"])
	}
	if result["value"] != "" {
		t.Fatalf("Expected empty value for non-existent setting, got %q", result["value"])
	}
}
