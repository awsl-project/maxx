package e2e_test

import (
	"net/http"
	"testing"
)

// getMemberToken creates a member user and returns their JWT token.
func getMemberToken(t *testing.T, env *TestEnv) string {
	t.Helper()
	// Admin registers a new member user
	resp := env.AdminPost("/api/admin/auth/register", map[string]string{
		"username": "member-user",
		"password": "Member1!",
	})
	AssertStatus(t, resp, http.StatusCreated)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	token, ok := result["token"].(string)
	if !ok || token == "" {
		t.Fatalf("Expected non-empty token for member user, got %v", result["token"])
	}
	return token
}

func TestRBAC_AdminFullAccess(t *testing.T) {
	env := NewTestEnv(t)

	// Admin can list providers (GET)
	resp := env.AdminGet("/api/admin/providers")
	AssertStatus(t, resp, http.StatusOK)

	// Admin can create a provider (POST)
	provider := map[string]any{
		"name": "rbac-test-provider",
		"type": "custom",
		"config": map[string]any{
			"custom": map[string]any{
				"baseURL": "https://api.example.com",
				"apiKey":  "sk-test-key",
			},
		},
		"supportedClientTypes": []string{"claude"},
	}
	resp = env.AdminPost("/api/admin/providers", provider)
	AssertStatus(t, resp, http.StatusCreated)

	// Admin can access dashboard (GET)
	resp = env.AdminGet("/api/admin/dashboard")
	AssertStatus(t, resp, http.StatusOK)

	// Admin can access settings (GET)
	resp = env.AdminGet("/api/admin/settings")
	AssertStatus(t, resp, http.StatusOK)
}

func TestRBAC_MemberReadOnly(t *testing.T) {
	env := NewTestEnv(t)
	memberToken := getMemberToken(t, env)

	// Member can GET allowed resources (dashboard, requests, etc.)
	resp := env.RequestWithToken(http.MethodGet, "/api/admin/dashboard", nil, memberToken)
	AssertStatus(t, resp, http.StatusOK)

	resp = env.RequestWithToken(http.MethodGet, "/api/admin/requests", nil, memberToken)
	AssertStatus(t, resp, http.StatusOK)

	// Member CANNOT GET resources not in the allowed list (providers)
	resp = env.RequestWithToken(http.MethodGet, "/api/admin/providers", nil, memberToken)
	AssertStatus(t, resp, http.StatusForbidden)

	// Member CANNOT POST to any resource
	provider := map[string]any{
		"name": "member-test-provider",
		"type": "custom",
		"config": map[string]any{
			"custom": map[string]any{
				"baseURL": "https://api.example.com",
				"apiKey":  "sk-test-key",
			},
		},
		"supportedClientTypes": []string{"claude"},
	}
	resp = env.RequestWithToken(http.MethodPost, "/api/admin/providers", provider, memberToken)
	AssertStatus(t, resp, http.StatusForbidden)

	// Member CANNOT DELETE
	resp = env.RequestWithToken(http.MethodDelete, "/api/admin/providers/1", nil, memberToken)
	AssertStatus(t, resp, http.StatusForbidden)
}

func TestRBAC_MemberCanGetSessions(t *testing.T) {
	env := NewTestEnv(t)
	memberToken := getMemberToken(t, env)

	resp := env.RequestWithToken(http.MethodGet, "/api/admin/sessions", nil, memberToken)
	AssertStatus(t, resp, http.StatusOK)
}

func TestRBAC_MemberCanGetUsageStats(t *testing.T) {
	env := NewTestEnv(t)
	memberToken := getMemberToken(t, env)

	resp := env.RequestWithToken(http.MethodGet, "/api/admin/usage-stats", nil, memberToken)
	AssertStatus(t, resp, http.StatusOK)
}

func TestRBAC_MemberCanGetProxyStatus(t *testing.T) {
	env := NewTestEnv(t)
	memberToken := getMemberToken(t, env)

	resp := env.RequestWithToken(http.MethodGet, "/api/admin/proxy-status", nil, memberToken)
	AssertStatus(t, resp, http.StatusOK)
}

func TestRBAC_MemberCanGetCooldowns(t *testing.T) {
	env := NewTestEnv(t)
	memberToken := getMemberToken(t, env)

	resp := env.RequestWithToken(http.MethodGet, "/api/admin/cooldowns", nil, memberToken)
	AssertStatus(t, resp, http.StatusOK)
}

func TestRBAC_MemberCannotGetProviders(t *testing.T) {
	env := NewTestEnv(t)
	memberToken := getMemberToken(t, env)

	resp := env.RequestWithToken(http.MethodGet, "/api/admin/providers", nil, memberToken)
	AssertStatus(t, resp, http.StatusForbidden)
}

func TestRBAC_MemberCannotGetSettings(t *testing.T) {
	env := NewTestEnv(t)
	memberToken := getMemberToken(t, env)

	resp := env.RequestWithToken(http.MethodGet, "/api/admin/settings", nil, memberToken)
	AssertStatus(t, resp, http.StatusForbidden)
}

func TestRBAC_MemberCannotGetRoutes(t *testing.T) {
	env := NewTestEnv(t)
	memberToken := getMemberToken(t, env)

	resp := env.RequestWithToken(http.MethodGet, "/api/admin/routes", nil, memberToken)
	AssertStatus(t, resp, http.StatusForbidden)
}

func TestRBAC_MemberCannotPutRequests(t *testing.T) {
	env := NewTestEnv(t)
	memberToken := getMemberToken(t, env)

	resp := env.RequestWithToken(http.MethodPut, "/api/admin/requests", map[string]any{
		"id": 1,
	}, memberToken)
	AssertStatus(t, resp, http.StatusForbidden)
}

func TestRBAC_InvalidToken(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.RequestWithToken(http.MethodGet, "/api/admin/dashboard", nil, "fake-invalid-token-12345")
	AssertStatus(t, resp, http.StatusUnauthorized)
}

func TestRBAC_NoAuthHeader(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.UnauthGet("/api/admin/dashboard")
	AssertStatus(t, resp, http.StatusUnauthorized)
}
