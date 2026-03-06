package e2e_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestSQLInjection_ProviderName(t *testing.T) {
	env := NewTestEnv(t)

	provider := map[string]any{
		"name": "'; DROP TABLE providers; --",
		"type": "custom",
		"config": map[string]any{
			"custom": map[string]any{
				"baseURL": "https://api.example.com",
				"apiKey":  "sk-test-key",
			},
		},
		"supportedClientTypes": []string{"claude"},
	}
	resp := env.AdminPost("/api/admin/providers", provider)
	// Should not return 500 (internal server error)
	if resp.StatusCode == http.StatusInternalServerError {
		t.Fatalf("SQL injection in provider name caused server error (500)")
	}
	resp.Body.Close()
}

func TestSQLInjection_SettingKey(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/settings/'; DROP TABLE system_settings;--")
	// Should not return 500
	if resp.StatusCode == http.StatusInternalServerError {
		t.Fatalf("SQL injection in setting key caused server error (500)")
	}
	resp.Body.Close()
}

func TestXSS_ProjectName(t *testing.T) {
	env := NewTestEnv(t)

	project := map[string]any{
		"name":        "<script>alert(1)</script>",
		"description": "XSS test project",
	}
	resp := env.AdminPost("/api/admin/projects", project)
	// Should not return 500
	if resp.StatusCode == http.StatusInternalServerError {
		t.Fatalf("XSS payload in project name caused server error (500)")
	}

	// If created successfully, verify the stored value does not execute scripts
	// (i.e., it's stored as-is or sanitized, but doesn't cause errors)
	if resp.StatusCode == http.StatusCreated {
		body := ReadBody(t, resp)
		// The response should contain the name as data, not as executable HTML
		if strings.Contains(body, "<script>") {
			// This is acceptable if the API returns JSON (scripts won't execute in JSON context)
			// Just verify no 500 error occurred
			t.Logf("Note: XSS payload stored as-is in JSON response (safe in API context)")
		}
	} else {
		resp.Body.Close()
	}
}

func TestXSS_ProviderName(t *testing.T) {
	env := NewTestEnv(t)

	provider := map[string]any{
		"name": "<img src=x onerror=alert(1)>",
		"type": "custom",
		"config": map[string]any{
			"custom": map[string]any{
				"baseURL": "https://api.example.com",
				"apiKey":  "sk-test-key",
			},
		},
		"supportedClientTypes": []string{"claude"},
	}
	resp := env.AdminPost("/api/admin/providers", provider)
	// Should not return 500
	if resp.StatusCode == http.StatusInternalServerError {
		t.Fatalf("HTML tag in provider name caused server error (500)")
	}
	resp.Body.Close()
}

func TestOverlongInput_ProviderName(t *testing.T) {
	env := NewTestEnv(t)

	longName := strings.Repeat("A", 10001)
	provider := map[string]any{
		"name": longName,
		"type": "custom",
		"config": map[string]any{
			"custom": map[string]any{
				"baseURL": "https://api.example.com",
				"apiKey":  "sk-test-key",
			},
		},
		"supportedClientTypes": []string{"claude"},
	}
	resp := env.AdminPost("/api/admin/providers", provider)
	// Should not crash (500) - any 4xx is acceptable
	if resp.StatusCode == http.StatusInternalServerError {
		t.Fatalf("Overlong provider name caused server error (500)")
	}
	resp.Body.Close()
}

func TestOverlongInput_Username(t *testing.T) {
	env := NewTestEnv(t)

	longUsername := strings.Repeat("u", 1001)
	resp := env.UnauthPost("/api/admin/auth/login", map[string]string{
		"username": longUsername,
		"password": "some-password",
	})
	// Should not crash - expect 401 (user not found) or 400
	if resp.StatusCode == http.StatusInternalServerError {
		t.Fatalf("Overlong username caused server error (500)")
	}
	resp.Body.Close()
}

func TestUnauthorized_AllEndpoints(t *testing.T) {
	env := NewTestEnv(t)

	// All admin endpoints that require authentication (must match admin.go ServeHTTP dispatch)
	adminEndpoints := []string{
		"/api/admin/dashboard",
		"/api/admin/providers",
		"/api/admin/routes",
		"/api/admin/projects",
		"/api/admin/sessions",
		"/api/admin/requests",
		"/api/admin/settings",
		"/api/admin/proxy-status",
		"/api/admin/cooldowns",
		"/api/admin/usage-stats",
		"/api/admin/api-tokens",
		"/api/admin/model-mappings",
		"/api/admin/model-prices",
		"/api/admin/users",
		"/api/admin/backup",
		"/api/admin/retry-configs",
		"/api/admin/routing-strategies",
		"/api/admin/response-models",
		"/api/admin/pricing",
		"/api/admin/provider-stats",
		"/api/admin/logs",
	}

	for _, endpoint := range adminEndpoints {
		t.Run(endpoint, func(t *testing.T) {
			resp := env.UnauthGet(endpoint)
			AssertStatus(t, resp, http.StatusUnauthorized)
		})
	}
}

func TestExpiredToken(t *testing.T) {
	env := NewTestEnv(t)

	expiredToken := env.GenerateExpiredToken()
	resp := env.RequestWithToken(http.MethodGet, "/api/admin/dashboard", nil, expiredToken)
	AssertStatus(t, resp, http.StatusUnauthorized)
}
