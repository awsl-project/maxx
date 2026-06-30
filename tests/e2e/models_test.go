package e2e_test

import (
	"net/http"
	"testing"
)

func TestGetModels(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.UnauthGet("/v1/models")
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	// OpenAI-style response should have "object" and "data" fields
	if result["object"] != "list" {
		t.Fatalf("Expected object 'list', got %v", result["object"])
	}

	data, ok := result["data"].([]any)
	if !ok {
		t.Fatal("Expected 'data' to be an array")
	}

	// In a fresh environment with no providers, the model list comes from
	// the default pricing table, so it should not be empty
	if len(data) == 0 {
		t.Fatal("Expected at least some models from default pricing table")
	}

	// Verify each model entry has expected fields
	for i, item := range data {
		model, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("Expected model entry %d to be an object", i)
		}
		if model["id"] == nil || model["id"] == "" {
			t.Fatalf("Expected model entry %d to have non-empty 'id'", i)
		}
		if model["object"] != "model" {
			t.Fatalf("Expected model entry %d object to be 'model', got %v", i, model["object"])
		}
	}
}

func TestGetModels_ResponseFormat(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.UnauthGet("/v1/models")
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	// Verify OpenAI-compatible top-level fields
	if result["object"] != "list" {
		t.Fatalf("Expected top-level 'object' to be 'list', got %v", result["object"])
	}

	data, ok := result["data"].([]any)
	if !ok {
		t.Fatal("Expected 'data' to be an array")
	}

	if len(data) == 0 {
		t.Fatal("Expected at least one model in data array")
	}

	// Verify OpenAI-compatible fields on each model entry
	model, ok := data[0].(map[string]any)
	if !ok {
		t.Fatal("Expected first model entry to be an object")
	}

	requiredFields := []string{"id", "object", "created", "owned_by"}
	for _, field := range requiredFields {
		if _, exists := model[field]; !exists {
			t.Fatalf("Expected model entry to contain OpenAI-compatible field '%s'", field)
		}
	}

	if model["object"] != "model" {
		t.Fatalf("Expected model entry 'object' to be 'model', got %v", model["object"])
	}
}

func TestGetModels_APITokenAuthAndModelSources(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminPost("/api/admin/providers", map[string]any{
		"name": "models-source-provider",
		"type": "custom",
		"config": map[string]any{
			"custom": map[string]any{
				"baseURL": "https://models-source.example.test",
				"apiKey":  "sk-models-source",
			},
		},
		"supportedClientTypes": []string{"openai"},
		"supportModels": []string{
			"gpt-models-e2e-provider",
			"gpt-models-e2e-duplicate",
		},
	})
	AssertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = env.AdminPost("/api/admin/model-mappings", map[string]any{
		"scope":    "global",
		"pattern":  "gpt-models-e2e-pattern",
		"target":   "gpt-models-e2e-duplicate",
		"priority": 10,
	})
	AssertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = env.AdminPut("/api/admin/settings/api_token_auth_enabled", map[string]any{"value": "true"})
	AssertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = env.UnauthGet("/v1/models")
	AssertStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()

	resp = env.RequestWithToken(http.MethodGet, "/v1/models", nil, "maxx_wrong")
	AssertStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()

	resp = env.AdminPost("/api/admin/api-tokens", map[string]any{
		"name":        "models-list-token",
		"description": "Token for /v1/models e2e coverage",
	})
	AssertStatus(t, resp, http.StatusCreated)
	var created map[string]any
	DecodeJSON(t, resp, &created)
	tokenStr, ok := created["token"].(string)
	if !ok || tokenStr == "" {
		t.Fatalf("Expected token string, got %v", created["token"])
	}

	resp = env.RequestWithToken(http.MethodGet, "/v1/models", nil, tokenStr)
	AssertStatus(t, resp, http.StatusOK)
	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["object"] != "list" {
		t.Fatalf("Expected object 'list', got %v", result["object"])
	}
	data, ok := result["data"].([]any)
	if !ok {
		t.Fatal("Expected 'data' to be an array")
	}

	ids := make(map[string]int)
	for i, item := range data {
		model, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("Expected model entry %d to be an object", i)
		}
		id, ok := model["id"].(string)
		if !ok || id == "" {
			t.Fatalf("Expected model entry %d to have non-empty string id", i)
		}
		ids[id]++
		if model["object"] != "model" {
			t.Fatalf("Expected model entry %d object to be 'model', got %v", i, model["object"])
		}
		if _, exists := model["created"]; !exists {
			t.Fatalf("Expected model entry %d to contain created", i)
		}
		if model["owned_by"] != "maxx" {
			t.Fatalf("Expected model entry %d owned_by to be maxx, got %v", i, model["owned_by"])
		}
	}

	for _, want := range []string{"gpt-models-e2e-provider", "gpt-models-e2e-pattern", "gpt-models-e2e-duplicate"} {
		if ids[want] != 1 {
			t.Fatalf("model %q occurrence count = %d, want 1", want, ids[want])
		}
	}
}

func TestGetModels_ProjectAndProviderScopedRoutesRequireAPIToken(t *testing.T) {
	env := NewProxyTestEnv(t)

	projectResp := env.AdminPost("/api/admin/projects", map[string]any{
		"name":                "models scoped project",
		"slug":                "models-scoped-project",
		"enabledCustomRoutes": []string{},
	})
	AssertStatus(t, projectResp, http.StatusCreated)
	projectResp.Body.Close()

	providerResp := env.AdminPost("/api/admin/providers", map[string]any{
		"name": "models scoped provider",
		"type": "custom",
		"config": map[string]any{
			"custom": map[string]any{
				"baseURL": "https://models-scoped-provider.example.test",
				"apiKey":  "sk-models-scoped-provider",
			},
		},
		"supportedClientTypes": []string{"openai"},
		"supportModels":        []string{"gpt-models-scoped-provider"},
	})
	AssertStatus(t, providerResp, http.StatusCreated)
	var provider map[string]any
	DecodeJSON(t, providerResp, &provider)
	providerID := int(provider["id"].(float64))

	resp := env.AdminPut("/api/admin/settings/api_token_auth_enabled", map[string]any{"value": "true"})
	AssertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = env.AdminPost("/api/admin/api-tokens", map[string]any{
		"name":        "scoped-models-list-token",
		"description": "Token for project/provider scoped model-list e2e coverage",
	})
	AssertStatus(t, resp, http.StatusCreated)
	var created map[string]any
	DecodeJSON(t, resp, &created)
	tokenStr, ok := created["token"].(string)
	if !ok || tokenStr == "" {
		t.Fatalf("Expected token string, got %v", created["token"])
	}

	for _, tc := range []struct {
		name string
		path string
	}{
		{name: "project scoped", path: "/project/models-scoped-project/v1/models"},
		{name: "provider scoped", path: "/provider/" + itoa(uint64(providerID)) + "/v1/models"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := env.doRequest(http.MethodGet, tc.path, nil, "")
			AssertStatus(t, resp, http.StatusUnauthorized)
			resp.Body.Close()

			resp = env.doRequest(http.MethodGet, tc.path, nil, "maxx_wrong")
			AssertStatus(t, resp, http.StatusUnauthorized)
			resp.Body.Close()

			resp = env.doRequest(http.MethodGet, tc.path, nil, tokenStr)
			AssertStatus(t, resp, http.StatusOK)
			var result map[string]any
			DecodeJSON(t, resp, &result)
			if result["object"] != "list" {
				t.Fatalf("Expected object 'list', got %v", result["object"])
			}
		})
	}
}
