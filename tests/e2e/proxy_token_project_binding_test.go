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
			resp := env.AdminPost("/api/admin/api-tokens", map[string]any{
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

			// Manual binding remains available within the unauthenticated mode.
			resp = env.AdminPut("/api/admin/sessions/"+noAuthSessionID+"/project", map[string]any{"projectID": manualProjectID})
			AssertStatus(t, resp, http.StatusOK)
			resp.Body.Close()
			if got := proxyRequest("/v1/chat/completions", manualProjectID, 0); got != noAuthSessionID {
				t.Fatalf("unauthenticated session changed: got %q, want %q", got, noAuthSessionID)
			}
			setAuth("true")
			if got := proxyRequest("/v1/chat/completions", tokenProjectID, token.APIToken.ID); got != authSessionID {
				t.Fatalf("authenticated session changed: got %q, want %q", got, authSessionID)
			}
			setAuth("false")
			proxyRequest("/v1/chat/completions", manualProjectID, 0)

			// Explicit project selection still works for new unauthenticated sessions.
			headers["X-Session-Id"] = "project-url-session"
			proxyRequest("/project/manual-project/v1/chat/completions", manualProjectID, 0)
			headers["X-Session-Id"] = "project-header-session"
			headers["X-Maxx-Project-ID"] = strconv.FormatUint(manualProjectID, 10)
			proxyRequest("/v1/chat/completions", manualProjectID, 0)
		})
	}
}
