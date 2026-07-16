package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"
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

func TestGetModels_OnlyCurrentAvailableModelsAcrossScopes(t *testing.T) {
	env := NewProxyTestEnv(t)

	globalProviderID := createModelListProvider(t, env, "models available global", []string{"gpt-models-global", "gpt-models-cold"})
	disabledProviderID := createModelListProvider(t, env, "models unavailable disabled", []string{"gpt-models-disabled"})
	projectProviderID := createModelListProvider(t, env, "models available project", []string{"gpt-models-project"})

	createModelListRoute(t, env, globalProviderID, 0, true)
	createModelListRoute(t, env, disabledProviderID, 0, false)

	projectResp := env.AdminPost("/api/admin/projects", map[string]any{
		"name":                "models availability project",
		"slug":                "models-availability-project",
		"enabledCustomRoutes": []string{"openai"},
	})
	AssertStatus(t, projectResp, http.StatusCreated)
	var project map[string]any
	DecodeJSON(t, projectResp, &project)
	projectID := int(project["id"].(float64))
	createModelListRoute(t, env, projectProviderID, projectID, true)

	until := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	cooldownResp := env.AdminPut(fmt.Sprintf("/api/admin/cooldowns/%d", globalProviderID), map[string]any{
		"untilTime":  until,
		"clientType": "openai",
		"model":      "gpt-models-cold",
	})
	AssertStatus(t, cooldownResp, http.StatusOK)
	cooldownResp.Body.Close()

	globalIDs := modelIDsFromResponse(t, env.doRequest(http.MethodGet, "/v1/models", nil, env.Token))
	assertModelPresent(t, globalIDs, "gpt-models-global")
	assertModelAbsent(t, globalIDs, "gpt-models-cold")
	assertModelAbsent(t, globalIDs, "gpt-models-disabled")
	assertModelAbsent(t, globalIDs, "gpt-models-project")

	providerIDs := modelIDsFromResponse(t, env.doRequest(http.MethodGet, fmt.Sprintf("/provider/%d/v1/models", globalProviderID), nil, env.Token))
	assertModelPresent(t, providerIDs, "gpt-models-global")
	assertModelAbsent(t, providerIDs, "gpt-models-cold")
	assertModelAbsent(t, providerIDs, "gpt-models-disabled")
	assertModelAbsent(t, providerIDs, "gpt-models-project")

	projectIDs := modelIDsFromResponse(t, env.doRequest(http.MethodGet, "/project/models-availability-project/v1/models", nil, env.Token))
	assertModelPresent(t, projectIDs, "gpt-models-project")
	assertModelAbsent(t, projectIDs, "gpt-models-global")
	assertModelAbsent(t, projectIDs, "gpt-models-disabled")
}

func createModelListProvider(t *testing.T, env *ProxyTestEnv, name string, supportModels []string) int {
	t.Helper()
	resp := env.AdminPost("/api/admin/providers", map[string]any{
		"name": name,
		"type": "custom",
		"config": map[string]any{"custom": map[string]any{
			"baseURL": "https://models-availability.example.test/v1",
			"apiKey":  "sk-models-availability",
		}},
		"supportedClientTypes": []string{"openai"},
		"supportModels":        supportModels,
	})
	AssertStatus(t, resp, http.StatusCreated)
	var provider map[string]any
	DecodeJSON(t, resp, &provider)
	return int(provider["id"].(float64))
}

func createModelListRoute(t *testing.T, env *ProxyTestEnv, providerID int, projectID int, enabled bool) {
	t.Helper()
	resp := env.AdminPost("/api/admin/routes", map[string]any{
		"isEnabled":  enabled,
		"isNative":   true,
		"clientType": "openai",
		"providerID": providerID,
		"projectID":  projectID,
		"position":   1,
	})
	AssertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()
}

func modelIDsFromResponse(t *testing.T, resp *http.Response) map[string]bool {
	t.Helper()
	AssertStatus(t, resp, http.StatusOK)
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	DecodeJSON(t, resp, &payload)
	ids := make(map[string]bool, len(payload.Data))
	for _, item := range payload.Data {
		ids[item.ID] = true
	}
	return ids
}

func assertModelPresent(t *testing.T, ids map[string]bool, model string) {
	t.Helper()
	if !ids[model] {
		t.Fatalf("expected model %q in %v", model, ids)
	}
}

func assertModelAbsent(t *testing.T, ids map[string]bool, model string) {
	t.Helper()
	if ids[model] {
		t.Fatalf("did not expect model %q in %v", model, ids)
	}
}
