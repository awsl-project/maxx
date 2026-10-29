package e2e_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
)

func waitPrivacyRequest(t *testing.T, env *ProxyTestEnv) *domain.ProxyRequest {
	t.Helper()
	repo := sqlite.NewProxyRequestRepository(env.DB)
	deadline := time.Now().Add(3 * time.Second)
	for {
		rows, err := repo.List(domain.DefaultTenantID, 1, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) == 1 && rows[0].Status == "COMPLETED" {
			req, err := repo.GetByID(domain.DefaultTenantID, rows[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			return req
		}
		if time.Now().After(deadline) {
			t.Fatalf("request not completed: %+v", rows)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestDisabledPrivacyPreservesHTTPAndSSE(t *testing.T) {
	jsonReply := `{"id":"chat-test","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"` + privacyTestKey + `"},"finish_reason":"stop"}]}`
	sseReply := "data: {\"id\":\"chat-test\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"" + privacyTestKey + "\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), privacyTestKey) || r.Header.Get("X-Service-Secret") != "custom-secret" || r.URL.Query().Get("key") != "query-secret" {
			t.Error("disabled feature changed upstream request")
		}
		w.Header().Set("X-Service-Secret", "response-secret")
		if strings.Contains(string(body), `"stream":true`) {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseReply)
		} else {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, jsonReply)
		}
	}))
	defer upstream.Close()
	env := NewProxyTestEnv(t)
	provider := createProvider(t, env, "off-compatibility", upstream.URL, []string{"openai"})
	createRoute(t, env, "openai", provider)
	resp := env.AdminPost("/api/admin/projects", map[string]any{"name": "default-off", "slug": "default-off"})
	AssertStatus(t, resp, http.StatusCreated)
	var project domain.Project
	DecodeJSON(t, resp, &project)
	for _, path := range []string{"/v1/chat/completions", "/project/default-off/v1/chat/completions", fmt.Sprintf("/provider/%d/v1/chat/completions", provider)} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%v", path, stream), func(t *testing.T) {
				body := openaiRequest("gpt-4o")
				body["stream"] = stream
				body["messages"] = []any{map[string]any{"role": "user", "content": privacyTestKey}}
				headers := map[string]string{"Authorization": "Bearer fixed-placeholder", "X-Service-Secret": "custom-secret"}
				if strings.HasPrefix(path, "/provider/") {
					headers["X-Maxx-Project-ID"] = fmt.Sprint(project.ID)
				}
				resp := env.ProxyPost(path+"?key=query-secret", body, headers)
				AssertStatus(t, resp, http.StatusOK)
				received, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				if err != nil || !strings.Contains(string(received), privacyTestKey) || stream && !strings.Contains(string(received), "[DONE]") {
					t.Fatalf("client response changed: %s %v", received, err)
				}
				stored := waitPrivacyRequest(t, env)
				if stored.RequestInfo == nil || stored.ResponseInfo == nil || !strings.Contains(stored.RequestInfo.Body, privacyTestKey) || !strings.Contains(stored.ResponseInfo.Body, privacyTestKey) || !strings.Contains(stored.RequestInfo.URL, "key=query-secret") || stored.RequestInfo.Headers["X-Service-Secret"] != "custom-secret" {
					t.Fatal("disabled feature changed request archive")
				}
				attempts, err := sqlite.NewProxyUpstreamAttemptRepository(env.DB).ListByProxyRequestID(stored.ID)
				if err != nil || len(attempts) != 1 || attempts[0].Status != "COMPLETED" || attempts[0].RequestInfo == nil || attempts[0].ResponseInfo == nil || !strings.Contains(attempts[0].RequestInfo.Body, privacyTestKey) || (!stream && !strings.Contains(attempts[0].ResponseInfo.Body, privacyTestKey) || stream && attempts[0].ResponseInfo.Body != "[streaming]") {
					dump, _ := json.Marshal(attempts)
					t.Fatalf("disabled feature changed attempt or response archive: err=%v attempts=%s", err, dump)
				}
			})
		}
	}
}

func TestPrivacyPolicyIsFixedForInFlightRequest(t *testing.T) {
	for _, initiallyEnabled := range []bool{false, true} {
		t.Run(fmt.Sprint(initiallyEnabled), func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				<-release
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"id":"chat-test","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"`+privacyTestKey+`"},"finish_reason":"stop"}]}`)
			}))
			defer upstream.Close()
			env := NewProxyTestEnv(t)
			provider := createProvider(t, env, "in-flight-policy", upstream.URL, []string{"openai"})
			createRoute(t, env, "openai", provider)
			createPrivacyTestProject(t, env, "in-flight")
			setGlobalPrivacyBlocking(t, env, initiallyEnabled)
			done := make(chan struct{})
			go func() {
				defer close(done)
				resp := env.ProxyPost("/project/in-flight/v1/chat/completions", openaiRequest("gpt-4o"), nil)
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				close(release)
				t.Fatal("upstream not reached")
			}
			setGlobalPrivacyBlocking(t, env, !initiallyEnabled)
			close(release)
			<-done
			stored := waitPrivacyRequest(t, env)
			if stored.ResponseInfo == nil || strings.Contains(stored.ResponseInfo.Body, privacyTestKey) == initiallyEnabled {
				t.Fatal("mid-request settings change altered archive policy")
			}
		})
	}
}

func TestDeletedProjectReferencesPreserveLegacyRouting(t *testing.T) {
	upstream := newMockOpenAIUpstream(t, &capturedRequest{})
	defer upstream.Close()
	env := NewProxyTestEnv(t)
	provider := createProvider(t, env, "deleted-project", upstream.URL, []string{"openai"})
	createRoute(t, env, "openai", provider)
	project := createPrivacyTestProject(t, env, "deleted-project")
	resp := env.AdminPut("/api/admin/settings/api_token_auth_enabled", map[string]any{"value": "true"})
	AssertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	resp = env.AdminPost("/api/admin/api-tokens", map[string]any{"name": "old-binding", "projectID": project.ID})
	AssertStatus(t, resp, http.StatusCreated)
	var token domain.APITokenCreateResult
	DecodeJSON(t, resp, &token)
	body := openaiRequest("gpt-4o")
	body["messages"] = []any{map[string]any{"role": "user", "content": privacyTestKey}}
	headers := map[string]string{"Authorization": "Bearer " + token.Token, "X-Session-Id": "old-project-session"}
	send := func() {
		t.Helper()
		resp := env.ProxyPost("/v1/chat/completions", body, headers)
		AssertStatus(t, resp, http.StatusOK)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	send() // Warm both the token and session caches with their project binding.
	resp = env.doRequest(http.MethodDelete, fmt.Sprintf("/api/admin/projects/%d", project.ID), nil, env.Token)
	AssertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
	send()
	resp = env.AdminPut("/api/admin/settings/api_token_auth_enabled", map[string]any{"value": "false"})
	AssertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	headers["Authorization"] = "Bearer fixed-placeholder"
	send() // An old explicitly bound session also works with authentication off.
}
