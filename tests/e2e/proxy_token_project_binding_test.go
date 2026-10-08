package e2e_test

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
)

func TestProxyTokenProjectBindingIsolatedWhenAuthDisabled(t *testing.T) {
	for _, explicitSessionID := range []bool{false, true} {
		name := "generated session ID"
		if explicitSessionID {
			name = "explicit session ID"
		}
		t.Run(name, func(t *testing.T) {
			upstream := newMockOpenAIUpstream(t, &capturedRequest{})
			defer upstream.Close()
			env := NewProxyTestEnv(t)
			// Reproduce the default single-tenant, unauthenticated configuration.
			resp := env.AdminPut("/api/admin/settings/ui_multitenant_enabled", map[string]any{"value": "false"})
			AssertStatus(t, resp, http.StatusOK)
			resp.Body.Close()
			providerID := createProvider(t, env, "openai", upstream.URL, []string{"openai"})
			createRoute(t, env, "openai", providerID)

			createProject := func(slug string) uint64 {
				t.Helper()
				resp := env.AdminPost("/api/admin/projects", map[string]any{"name": slug, "slug": slug})
				AssertStatus(t, resp, http.StatusCreated)
				var project domain.Project
				DecodeJSON(t, resp, &project)
				return project.ID
			}
			tokenProjectID := createProject("token-project")
			manualProjectID := createProject("manual-project")
			resp = env.AdminPost("/api/admin/api-tokens", map[string]any{
				"name": "project-token", "projectID": tokenProjectID,
			})
			AssertStatus(t, resp, http.StatusCreated)
			var token domain.APITokenCreateResult
			DecodeJSON(t, resp, &token)
			headers := map[string]string{"Authorization": "Bearer " + token.Token}
			if explicitSessionID {
				headers["X-Session-Id"] = "same-client-session"
			}
			setAuth := func(value string) {
				t.Helper()
				resp := env.AdminPut("/api/admin/settings/api_token_auth_enabled", map[string]any{"value": value})
				AssertStatus(t, resp, http.StatusOK)
				resp.Body.Close()
			}
			requestRepo := sqlite.NewProxyRequestRepository(env.DB)
			proxyRequest := func(path string, wantProjectID, wantTokenID uint64) string {
				t.Helper()
				resp := env.ProxyPost(path, openaiRequest("gpt-4o"), headers)
				AssertStatus(t, resp, http.StatusOK)
				resp.Body.Close()
				requests, err := requestRepo.List(domain.DefaultTenantID, 1, 0)
				if err != nil || len(requests) != 1 {
					t.Fatalf("latest request: requests=%v err=%v", requests, err)
				}
				request := requests[0]
				if request.ProjectID != wantProjectID || request.APITokenID != wantTokenID {
					t.Fatalf("request project=%d token=%d, want project=%d token=%d", request.ProjectID, request.APITokenID, wantProjectID, wantTokenID)
				}
				return request.SessionID
			}

			setAuth("true")
			authSessionID := proxyRequest("/v1/chat/completions", tokenProjectID, token.APIToken.ID)
			setAuth("false")
			noAuthSessionID := proxyRequest("/v1/chat/completions", 0, 0)
			if noAuthSessionID == authSessionID {
				t.Fatal("authenticated and unauthenticated requests must use separate sessions")
			}

			// Only explicit sessions may reuse a manually bound project.
			resp = env.AdminPut("/api/admin/sessions/"+noAuthSessionID+"/project", map[string]any{"projectID": manualProjectID})
			AssertStatus(t, resp, http.StatusOK)
			resp.Body.Close()
			wantNoAuthProjectID := uint64(0)
			if explicitSessionID {
				wantNoAuthProjectID = manualProjectID
			}
			got := proxyRequest("/v1/chat/completions", wantNoAuthProjectID, 0)
			if explicitSessionID && got != noAuthSessionID {
				t.Fatalf("unauthenticated session changed: got %q, want %q", got, noAuthSessionID)
			}
			if !explicitSessionID && got == noAuthSessionID {
				t.Fatal("requests without explicit session IDs must not reuse a project-bound fallback session")
			}
			setAuth("true")
			if got := proxyRequest("/v1/chat/completions", tokenProjectID, token.APIToken.ID); got != authSessionID {
				t.Fatalf("authenticated session changed: got %q, want %q", got, authSessionID)
			}
			if explicitSessionID {
				// A client-selected authenticated ID must not select a record
				// from the server's unauthenticated session namespace.
				headers["X-Session-Id"] = noAuthSessionID
				proxyRequest("/v1/chat/completions", tokenProjectID, token.APIToken.ID)
				headers["X-Session-Id"] = "same-client-session"
			}
			setAuth("false")
			proxyRequest("/v1/chat/completions", wantNoAuthProjectID, 0)

			// Explicit project selection still works for new unauthenticated sessions.
			headers["X-Session-Id"] = "project-url-session"
			proxyRequest("/project/manual-project/v1/chat/completions", manualProjectID, 0)
			headers["X-Session-Id"] = "project-header-session"
			headers["X-Maxx-Project-ID"] = strconv.FormatUint(manualProjectID, 10)
			proxyRequest("/v1/chat/completions", manualProjectID, 0)
		})
	}
}

func TestProxyPlaceholderTokenDoesNotBindProjectWithoutExplicitSession(t *testing.T) {
	cases := []struct {
		header string
		userID string
	}{
		{header: "Authorization"},
		{header: "x-api-key"},
		{header: "x-goog-api-key"},
		{header: "Authorization", userID: "shared-user"},
	}
	for _, tc := range cases {
		name := tc.header
		if tc.userID != "" {
			name += "/bare metadata user ID"
		}
		t.Run(name, func(t *testing.T) {
			upstream := newMockOpenAIUpstream(t, &capturedRequest{})
			defer upstream.Close()
			env := NewProxyTestEnv(t)
			providerID := createProvider(t, env, "openai", upstream.URL, []string{"openai"})
			createRoute(t, env, "openai", providerID)
			resp := env.AdminPost("/api/admin/projects", map[string]any{
				"name": "selected-project", "slug": "selected-project",
			})
			AssertStatus(t, resp, http.StatusCreated)
			var project domain.Project
			DecodeJSON(t, resp, &project)

			value := "arbitrary-placeholder"
			if tc.header == "Authorization" {
				value = "Bearer " + value
			}
			headers := map[string]string{tc.header: value}
			body := openaiRequest("gpt-4o")
			if tc.userID != "" {
				body["metadata"] = map[string]any{"user_id": tc.userID}
			}
			requestRepo := sqlite.NewProxyRequestRepository(env.DB)
			proxyRequest := func(path string, wantProjectID uint64) string {
				t.Helper()
				resp := env.ProxyPost(path, body, headers)
				AssertStatus(t, resp, http.StatusOK)
				resp.Body.Close()
				requests, err := requestRepo.List(domain.DefaultTenantID, 1, 0)
				if err != nil || len(requests) != 1 {
					t.Fatalf("latest request: requests=%v err=%v", requests, err)
				}
				request := requests[0]
				if request.ProjectID != wantProjectID || request.APITokenID != 0 {
					t.Fatalf("request project=%d token=%d, want project=%d token=0", request.ProjectID, request.APITokenID, wantProjectID)
				}
				return request.SessionID
			}

			projectSessionID := proxyRequest("/project/selected-project/v1/chat/completions", project.ID)
			globalSessionID := proxyRequest("/v1/chat/completions", 0)
			if projectSessionID == globalSessionID {
				t.Fatal("a placeholder token must not create a shared persistent session")
			}

			// A manually assigned fallback session must not bind future requests
			// that happen to carry the same placeholder token either.
			resp = env.AdminPut("/api/admin/sessions/"+globalSessionID+"/project", map[string]any{"projectID": project.ID})
			AssertStatus(t, resp, http.StatusOK)
			resp.Body.Close()
			proxyRequest("/v1/chat/completions", 0)

			// A real client session still supports project binding even when
			// the API key is an arbitrary placeholder.
			headers["X-Session-Id"] = "explicit-placeholder-session"
			explicitSessionID := proxyRequest("/project/selected-project/v1/chat/completions", project.ID)
			if got := proxyRequest("/v1/chat/completions", project.ID); got != explicitSessionID {
				t.Fatalf("explicit session changed: got %q, want %q", got, explicitSessionID)
			}
		})
	}
}
