package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func startMockClaudeProxyServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}

		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		model, _ := body["model"].(string)
		if model == "" {
			model = "claude-sonnet-4-20250514"
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "msg_provider_scope_test",
			"type":  "message",
			"role":  "assistant",
			"model": model,
			"content": []map[string]any{{
				"type": "text",
				"text": "hello from provider-scoped route",
			}},
			"stop_reason":   "end_turn",
			"stop_sequence": nil,
			"usage": map[string]any{
				"input_tokens":                12,
				"output_tokens":               6,
				"cache_creation_input_tokens": 0,
				"cache_read_input_tokens":     0,
			},
		})
	}))
}

func TestProviderScopedProxyRoute(t *testing.T) {
	env := NewProxyTestEnv(t)
	mock := startMockClaudeProxyServer(t)
	defer mock.Close()

	providerResp := env.AdminPost("/api/admin/providers", map[string]any{
		"name": "Provider Scoped Claude",
		"type": "custom",
		"config": map[string]any{
			"custom": map[string]any{
				"baseURL": mock.URL,
				"apiKey":  "mock-key",
			},
		},
		"supportedClientTypes": []string{"claude"},
		"supportModels":        []string{"*"},
	})
	AssertStatus(t, providerResp, http.StatusCreated)

	var provider map[string]any
	DecodeJSON(t, providerResp, &provider)
	providerID := int(provider["id"].(float64))

	routeResp := env.AdminPost("/api/admin/routes", map[string]any{
		"isEnabled":  true,
		"isNative":   false,
		"clientType": "claude",
		"providerID": providerID,
		"projectID":  0,
		"position":   1,
	})
	AssertStatus(t, routeResp, http.StatusCreated)

	model := fmt.Sprintf("claude-sonnet-4-20250514-provider-%d", providerID)
	resp := env.ProxyPost(fmt.Sprintf("/provider/%d/v1/messages", providerID), map[string]any{
		"model":      model,
		"max_tokens": 64,
		"messages": []map[string]any{{
			"role":    "user",
			"content": "hello provider route",
		}},
	}, map[string]string{
		"anthropic-version": "2023-06-01",
	})
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)
	if result["model"] != model {
		t.Fatalf("expected model %q, got %v", model, result["model"])
	}

	requestsResp := env.doRequest(http.MethodGet, fmt.Sprintf("/api/admin/requests?limit=20&providerId=%d", providerID), nil, env.Token)
	AssertStatus(t, requestsResp, http.StatusOK)

	var requests struct {
		Items []map[string]any `json:"items"`
	}
	DecodeJSON(t, requestsResp, &requests)
	if len(requests.Items) == 0 {
		t.Fatalf("expected at least one request for provider %d", providerID)
	}
	if got := int(requests.Items[0]["providerID"].(float64)); got != providerID {
		t.Fatalf("expected providerID %d in recorded request, got %d", providerID, got)
	}
}
