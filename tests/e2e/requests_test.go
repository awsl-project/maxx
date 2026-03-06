package e2e_test

import (
	"net/http"
	"testing"
)

func TestListRequests_Empty(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/requests")
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	// Response should contain items (empty list) and pagination info
	items, ok := result["items"].([]any)
	if !ok {
		t.Fatal("Expected response to contain 'items' array")
	}
	if len(items) != 0 {
		t.Fatalf("Expected 0 requests, got %d", len(items))
	}
}

func TestGetActiveRequests(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/requests/active")
	AssertStatus(t, resp, http.StatusOK)

	var requests []any
	DecodeJSON(t, resp, &requests)

	// No active requests in a fresh environment
	if len(requests) != 0 {
		t.Fatalf("Expected 0 active requests, got %d", len(requests))
	}
}

func TestListRequests_WithPagination(t *testing.T) {
	env := NewTestEnv(t)

	// Request with explicit limit parameter
	resp := env.AdminGet("/api/admin/requests?limit=5")
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	items, ok := result["items"].([]any)
	if !ok {
		t.Fatal("Expected response to contain 'items' array")
	}
	// Fresh environment should have no requests
	if len(items) != 0 {
		t.Fatalf("Expected 0 requests with limit=5, got %d", len(items))
	}
}

func TestListRequests_WithFilter_Status(t *testing.T) {
	env := NewTestEnv(t)

	// Request with status filter
	resp := env.AdminGet("/api/admin/requests?status=success")
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	items, ok := result["items"].([]any)
	if !ok {
		t.Fatal("Expected response to contain 'items' array")
	}
	// No requests with status=success in a fresh environment
	if len(items) != 0 {
		t.Fatalf("Expected 0 requests with status filter, got %d", len(items))
	}
}

func TestGetRequestsCount(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/requests/count")
	AssertStatus(t, resp, http.StatusOK)

	var count float64
	DecodeJSON(t, resp, &count)

	// Fresh environment should have 0 requests
	if count != 0 {
		t.Fatalf("Expected request count 0, got %v", count)
	}
}

func TestGetRequest_ByID_NotFound(t *testing.T) {
	env := NewTestEnv(t)

	// Request a non-existent request ID
	resp := env.AdminGet("/api/admin/requests/999999")
	AssertStatus(t, resp, http.StatusNotFound)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["error"] == nil || result["error"] == "" {
		t.Fatal("Expected error message in response")
	}
}
