package e2e_test

import (
	"net/http"
	"testing"
)

func TestGetProxyStatus(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/proxy-status")
	AssertStatus(t, resp, http.StatusOK)

	var status map[string]any
	DecodeJSON(t, resp, &status)

	// Proxy status should return a valid JSON object with at least some fields
	if status == nil {
		t.Fatal("Expected non-nil proxy status response")
	}
}
