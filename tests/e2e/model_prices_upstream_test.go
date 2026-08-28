package e2e_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestModelPricesExternalFetchThenApplySelected(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "upstream-model": {
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
  "upstream-new-model": {
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
		"modelId":          "upstream-custom-model",
		"inputPriceMicro":  1234,
		"outputPriceMicro": 5678,
	}
	resp := env.AdminPost("/api/admin/model-prices", custom)
	AssertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	existing := map[string]any{
		"modelId":          "upstream-model",
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
	total, ok := upstream["total"].(float64)
	if !ok || total != 2 {
		t.Fatalf("expected two formatted upstream prices, got total=%+v upstream=%+v", upstream["total"], upstream)
	}
	upstreamPrices, ok := upstream["prices"].([]any)
	if !ok || len(upstreamPrices) != 2 {
		t.Fatalf("expected formatted upstream prices, got %+v", upstream["prices"])
	}
	for _, item := range upstreamPrices {
		price, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("expected upstream price object, got %+v", item)
		}
		if price["modelId"] == "sample_spec" {
			t.Fatalf("sample_spec should be filtered from upstream prices: %+v", upstreamPrices)
		}
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
		if price["modelId"] == "upstream-model" && price["inputPriceMicro"].(float64) != 1 {
			t.Fatalf("fetch mutated upstream-model: %+v", price)
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
		case "upstream-custom-model":
			foundCustom = true
		case "upstream-model":
			foundSyncedExisting = price["inputPriceMicro"].(float64) == 3000000
		case "upstream-new-model":
			foundCreated = price["inputPriceMicro"].(float64) == 1000000
		}
	}

	if !foundCustom {
		t.Fatal("upstream import should preserve custom model prices")
	}
	if !foundSyncedExisting {
		t.Fatal("upstream import should update existing model price from external source")
	}
	if !foundCreated {
		t.Fatal("upstream import should create new model price from external source")
	}
}
