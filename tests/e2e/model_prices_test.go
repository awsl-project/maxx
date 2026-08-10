package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

func TestListModelPrices(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/model-prices")
	AssertStatus(t, resp, http.StatusOK)

	var prices []map[string]any
	DecodeJSON(t, resp, &prices)

	// Initially there may be no custom model prices
	if prices == nil {
		t.Fatal("Expected non-nil response for model prices list")
	}
}

func TestCreateModelPrice(t *testing.T) {
	env := NewTestEnv(t)

	price := map[string]any{
		"modelId":          "test-model-v1",
		"inputPriceMicro":  3000,
		"outputPriceMicro": 15000,
	}

	resp := env.AdminPost("/api/admin/model-prices", price)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)

	if created["modelId"] != "test-model-v1" {
		t.Fatalf("Expected modelId 'test-model-v1', got %v", created["modelId"])
	}

	id, ok := created["id"].(float64)
	if !ok || id == 0 {
		t.Fatalf("Expected non-zero id, got %v", created["id"])
	}

	// Verify it appears in the list
	resp = env.AdminGet("/api/admin/model-prices")
	AssertStatus(t, resp, http.StatusOK)

	var prices []map[string]any
	DecodeJSON(t, resp, &prices)

	found := false
	for _, p := range prices {
		if p["modelId"] == "test-model-v1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Created model price not found in list")
	}
}

func TestGetModelPrice_ByID(t *testing.T) {
	env := NewTestEnv(t)

	// Create a model price first
	price := map[string]any{
		"modelId":          "get-test-model",
		"inputPriceMicro":  5000,
		"outputPriceMicro": 20000,
	}

	resp := env.AdminPost("/api/admin/model-prices", price)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	// Get by ID
	resp = env.AdminGet(fmt.Sprintf("/api/admin/model-prices/%d", int(id)))
	AssertStatus(t, resp, http.StatusOK)

	var fetched map[string]any
	DecodeJSON(t, resp, &fetched)

	if fetched["modelId"] != "get-test-model" {
		t.Fatalf("Expected modelId 'get-test-model', got %v", fetched["modelId"])
	}
}

func TestUpdateModelPrice(t *testing.T) {
	env := NewTestEnv(t)

	// Create a model price
	price := map[string]any{
		"modelId":          "update-test-model",
		"inputPriceMicro":  3000,
		"outputPriceMicro": 15000,
	}

	resp := env.AdminPost("/api/admin/model-prices", price)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	// Update the model price
	updated := map[string]any{
		"modelId":          "update-test-model",
		"inputPriceMicro":  6000,
		"outputPriceMicro": 30000,
	}

	resp = env.AdminPut(fmt.Sprintf("/api/admin/model-prices/%d", int(id)), updated)
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["inputPriceMicro"].(float64) != 6000 {
		t.Fatalf("Expected inputPriceMicro 6000, got %v", result["inputPriceMicro"])
	}
	if result["outputPriceMicro"].(float64) != 30000 {
		t.Fatalf("Expected outputPriceMicro 30000, got %v", result["outputPriceMicro"])
	}
}

func TestDeleteModelPrice(t *testing.T) {
	env := NewTestEnv(t)

	// Create a model price
	price := map[string]any{
		"modelId":          "delete-test-model",
		"inputPriceMicro":  3000,
		"outputPriceMicro": 15000,
	}

	resp := env.AdminPost("/api/admin/model-prices", price)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	// Delete the model price
	resp = env.AdminDelete(fmt.Sprintf("/api/admin/model-prices/%d", int(id)))
	AssertStatus(t, resp, http.StatusNoContent)

	// Verify it no longer exists
	resp = env.AdminGet(fmt.Sprintf("/api/admin/model-prices/%d", int(id)))
	AssertStatus(t, resp, http.StatusNotFound)
}

func TestGetModelPrice_NotFound(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/model-prices/99999")
	AssertStatus(t, resp, http.StatusNotFound)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["error"] != "model price not found" {
		t.Fatalf("Expected error 'model price not found', got %v", result["error"])
	}
}

func TestCreateModelPrice_InvalidJSON(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminRawPost("/api/admin/model-prices", "{not valid json!!!")
	AssertStatus(t, resp, http.StatusBadRequest)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["error"] != "invalid request body" {
		t.Fatalf("Expected error 'invalid request body', got %v", result["error"])
	}
}

func TestModelPricesReset(t *testing.T) {
	env := NewTestEnv(t)

	// Create a custom price first
	price := map[string]any{
		"modelId":          "reset-test-model",
		"inputPriceMicro":  1000,
		"outputPriceMicro": 5000,
	}
	resp := env.AdminPost("/api/admin/model-prices", price)
	AssertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Reset to defaults
	resp = env.AdminPost("/api/admin/model-prices/reset", nil)
	AssertStatus(t, resp, http.StatusOK)

	var prices []map[string]any
	DecodeJSON(t, resp, &prices)

	// After reset, should have default prices (non-nil response)
	if prices == nil {
		t.Fatal("Expected non-nil response after model prices reset")
	}
}
