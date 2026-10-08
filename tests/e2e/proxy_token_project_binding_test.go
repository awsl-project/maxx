package e2e_test

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
)

func TestProjectURLDoesNotAutomaticallyBindAnonymousSession(t *testing.T) {
	cases := []struct {
		name, header, userID, sessionID string
	}{
		{name: "bearer placeholder", header: "Authorization"},
		{name: "api key placeholder", header: "x-api-key"},
		{name: "google key placeholder", header: "x-goog-api-key"},
		{name: "user identity", header: "Authorization", userID: "shared-user"},
		{name: "explicit session", header: "Authorization", sessionID: "client-session"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			upstream := newMockOpenAIUpstream(t, &capturedRequest{})
			defer upstream.Close()
			env := NewProxyTestEnv(t)
			providerID := createProvider(t, env, "upstream", upstream.URL, []string{"openai"})
			createRoute(t, env, "openai", providerID)
			createProject := func(slug string) uint64 {
				t.Helper()
				resp := env.AdminPost("/api/admin/projects", map[string]any{"name": slug, "slug": slug})
				AssertStatus(t, resp, http.StatusCreated)
				var project domain.Project
				DecodeJSON(t, resp, &project)
				return project.ID
			}
			a, b := createProject("project-a"), createProject("project-b")
			headers := map[string]string{tc.header: "arbitrary-placeholder"}
			if tc.header == "Authorization" {
				headers[tc.header] = "Bearer arbitrary-placeholder"
			}
			if tc.sessionID != "" {
				headers["X-Session-Id"] = tc.sessionID
			}
			body := openaiRequest("gpt-4o")
			if tc.userID != "" {
				body["metadata"] = map[string]any{"user_id": tc.userID}
			}
			requestRepo := sqlite.NewProxyRequestRepository(env.DB)
			request := func(path string, wantProject uint64) string {
				t.Helper()
				resp := env.ProxyPost(path, body, headers)
				AssertStatus(t, resp, http.StatusOK)
				resp.Body.Close()
				requests, err := requestRepo.List(domain.DefaultTenantID, 1, 0)
				if err != nil || len(requests) != 1 {
					t.Fatalf("latest request: %v, %v", requests, err)
				}
				if requests[0].ProjectID != wantProject || requests[0].APITokenID != 0 {
					t.Fatalf("request project=%d token=%d, want project=%d token=0", requests[0].ProjectID, requests[0].APITokenID, wantProject)
				}
				return requests[0].SessionID
			}
			assertBinding := func(sessionID string, wantProject uint64) {
				t.Helper()
				session, err := sqlite.NewSessionRepository(env.DB).GetBySessionID(domain.DefaultTenantID, sessionID)
				if err != nil || session == nil || session.ProjectID != wantProject {
					t.Fatalf("session=%+v err=%v, want binding=%d", session, err, wantProject)
				}
			}

			// A project URL routes this request without creating a persistent binding.
			sid := request("/project/project-a/v1/chat/completions", a)
			assertBinding(sid, 0)
			if got := request("/project/project-b/v1/chat/completions", b); got != sid {
				t.Fatal("project selection unexpectedly changed session identity")
			}
			assertBinding(sid, 0)
			request("/v1/chat/completions", 0)
			headers["X-Maxx-Project-ID"] = strconv.FormatUint(b, 10)
			request("/v1/chat/completions", b)
			assertBinding(sid, 0)
			delete(headers, "X-Maxx-Project-ID")

			// Existing bindings remain usable, but cannot override a project URL.
			resp := env.AdminPut("/api/admin/sessions/"+sid+"/project", map[string]any{"projectID": a})
			AssertStatus(t, resp, http.StatusOK)
			resp.Body.Close()
			request("/v1/chat/completions", a)
			request("/project/project-b/v1/chat/completions", b)
			assertBinding(sid, a)
		})
	}
}
