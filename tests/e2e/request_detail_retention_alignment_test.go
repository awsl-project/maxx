package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

func TestRequestDetailRetentionZero_AlignedAcrossProxyPaths(t *testing.T) {
	t.Run("global URL clears request detail", func(t *testing.T) {
		request := runRequestDetailRetentionZeroCase(t, "/v1/messages", false)
		assertRequestDetailCleared(t, request)
		if got := getProjectID(t, request); got != 0 {
			t.Fatalf("expected global request projectID 0, got %d", got)
		}
	})

	t.Run("project URL clears request detail", func(t *testing.T) {
		request := runRequestDetailRetentionZeroCase(t, "/project/issue-526/v1/messages", true)
		assertRequestDetailCleared(t, request)
		if got := getProjectID(t, request); got == 0 {
			t.Fatalf("expected project request to carry non-zero projectID, request=%#v", request)
		}
	})

	t.Run("provider URL clears request detail", func(t *testing.T) {
		request := runRequestDetailRetentionZeroCase(t, "", false)
		assertRequestDetailCleared(t, request)
		if got := getProjectID(t, request); got != 0 {
			t.Fatalf("expected provider request projectID 0, got %d", got)
		}
	})
}

func runRequestDetailRetentionZeroCase(t *testing.T, path string, createProject bool) map[string]any {
	t.Helper()

	env := NewProxyTestEnv(t)
	mock := startMockClaudeProxyServer(t)
	defer mock.Close()

	providerID := createProvider(t, env, "Retention Provider", mock.URL, []string{"claude"})
	createRoute(t, env, "claude", providerID)

	if createProject {
		resp := env.AdminPost("/api/admin/projects", map[string]any{
			"name":                "issue-526",
			"slug":                "issue-526",
			"enabledCustomRoutes": []string{},
		})
		AssertStatus(t, resp, http.StatusCreated)
	}

	resp := env.AdminPut("/api/admin/settings/request_detail_retention_seconds", map[string]any{"value": "0"})
	AssertStatus(t, resp, http.StatusOK)

	if path == "" {
		path = fmt.Sprintf("/provider/%d/v1/messages", providerID)
	}

	proxyResp := env.ProxyPost(path, map[string]any{
		"model":      "claude-sonnet-4-20250514",
		"max_tokens": 64,
		"messages": []map[string]any{{
			"role":    "user",
			"content": "retain nothing",
		}},
	}, map[string]string{
		"anthropic-version": "2023-06-01",
	})
	AssertStatus(t, proxyResp, http.StatusOK)

	requestsResp := env.AdminGet("/api/admin/requests?limit=10")
	AssertStatus(t, requestsResp, http.StatusOK)

	var result struct {
		Items []map[string]any `json:"items"`
	}
	DecodeJSON(t, requestsResp, &result)
	if len(result.Items) != 1 {
		t.Fatalf("expected exactly 1 recorded request, got %d", len(result.Items))
	}

	requestID := int(result.Items[0]["id"].(float64))
	requestResp := env.AdminGet(fmt.Sprintf("/api/admin/requests/%d", requestID))
	AssertStatus(t, requestResp, http.StatusOK)

	var request map[string]any
	DecodeJSON(t, requestResp, &request)
	return request
}

func assertRequestDetailCleared(t *testing.T, request map[string]any) {
	t.Helper()
	if request["requestInfo"] != nil {
		t.Fatalf("expected requestInfo to be nil when retention=0, got %#v", request["requestInfo"])
	}
	if request["responseInfo"] != nil {
		t.Fatalf("expected responseInfo to be nil when retention=0, got %#v", request["responseInfo"])
	}
}

func getProjectID(t *testing.T, request map[string]any) int {
	t.Helper()
	value, ok := request["projectID"]
	if !ok {
		t.Fatalf("projectID missing from request: %#v", request)
	}
	pid, ok := value.(float64)
	if !ok {
		t.Fatalf("projectID missing or wrong type: value=%#v request=%#v", value, request)
	}
	return int(pid)
}
