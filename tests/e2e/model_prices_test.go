package e2e_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestModelPricesExternalFetchThenApplySelected(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "sync-model": {
    "litellm_provider": "openai",
    "mode": "chat",
    "input_cost_per_token": 0.000003,
    "output_cost_per_token": 0.000015,
    "cache_read_input_token_cost": 0.0000003,
    "cache_creation_input_token_cost": 0.00000375,
    "cache_creation_input_token_cost_above_1hr": 0.000006,
    "input_cost_per_token_above_200k_tokens": 0.000006,
    "output_cost_per_token_above_200k_tokens": 0.0000225
  },
  "sync-new-model": {
    "litellm_provider": "openai",
    "mode": "chat",
    "input_cost_per_token": 0.000001,
    "output_cost_per_token": 0.000002
  },
  "sample_spec": {
    "input_cost_per_token": 0.000001,
    "output_cost_per_token": 0.000002
  }
}`))
	}))
	defer source.Close()
	t.Setenv("MAXX_MODEL_PRICE_SYNC_SOURCE_URL", source.URL)

	env := NewTestEnv(t)

	custom := map[string]any{
		"modelId":          "sync-custom-model",
		"inputPriceMicro":  1234,
		"outputPriceMicro": 5678,
	}
	resp := env.AdminPost("/api/admin/model-prices", custom)
	AssertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	existing := map[string]any{
		"modelId":          "sync-model",
		"inputPriceMicro":  1,
		"outputPriceMicro": 15000000,
	}
	resp = env.AdminPost("/api/admin/model-prices", existing)
	AssertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = env.AdminPost("/api/admin/model-prices/upstream/prices", map[string]any{"source": "litellm"})
	AssertStatus(t, resp, http.StatusOK)

	var upstream map[string]any
	DecodeJSON(t, resp, &upstream)
	if upstream["source"] != "litellm" {
		t.Fatalf("expected upstream source litellm, got %+v", upstream["source"])
	}
	if upstream["total"].(float64) != 2 {
		t.Fatalf("expected two formatted upstream prices, got %+v", upstream)
	}
	upstreamPrices, ok := upstream["prices"].([]any)
	if !ok || len(upstreamPrices) != 2 {
		t.Fatalf("expected formatted upstream prices, got %+v", upstream["prices"])
	}
	if _, hasChanges := upstream["changes"]; hasChanges {
		t.Fatalf("upstream prices endpoint should not include changes, got %+v", upstream["changes"])
	}

	// Fetching upstream prices must not mutate the existing row.
	resp = env.AdminGet("/api/admin/model-prices")
	AssertStatus(t, resp, http.StatusOK)
	var beforeApply []map[string]any
	DecodeJSON(t, resp, &beforeApply)
	currentByModelID := make(map[string]map[string]any, len(beforeApply))
	for _, price := range beforeApply {
		currentByModelID[price["modelId"].(string)] = price
		if price["modelId"] == "sync-model" && price["inputPriceMicro"].(float64) != 1 {
			t.Fatalf("fetch mutated sync-model: %+v", price)
		}
	}

	for _, item := range upstreamPrices {
		price := item.(map[string]any)
		current := currentByModelID[price["modelId"].(string)]
		if current == nil {
			resp = env.AdminPost("/api/admin/model-prices", price)
			AssertStatus(t, resp, http.StatusCreated)
			resp.Body.Close()
			continue
		}
		id := int(current["id"].(float64))
		resp = env.AdminPut(fmt.Sprintf("/api/admin/model-prices/%d", id), price)
		AssertStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	}

	resp = env.AdminGet("/api/admin/model-prices")
	AssertStatus(t, resp, http.StatusOK)
	var resultPrices []map[string]any
	DecodeJSON(t, resp, &resultPrices)

	foundCustom := false
	foundSyncedExisting := false
	foundCreated := false
	for _, price := range resultPrices {
		switch price["modelId"] {
		case "sync-custom-model":
			foundCustom = true
		case "sync-model":
			foundSyncedExisting = price["inputPriceMicro"].(float64) == 3000000
		case "sync-new-model":
			foundCreated = price["inputPriceMicro"].(float64) == 1000000
		}
	}

	if !foundCustom {
		t.Fatal("sync should preserve custom model prices")
	}
	if !foundSyncedExisting {
		t.Fatal("sync should update existing model price from external source")
	}
	if !foundCreated {
		t.Fatal("sync should create new model price from external source")
	}
}
