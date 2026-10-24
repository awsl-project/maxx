package e2e_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
	"github.com/gorilla/websocket"
)

const privacyTestKey = "sk-proj-testtesttesttesttesttesttesttest"

func createPrivacyProject(t *testing.T, env *ProxyTestEnv, slug string, enabled bool) domain.Project {
	t.Helper()
	resp := env.AdminPost("/api/admin/projects", map[string]any{"name": slug, "slug": slug, "blockSecrets": enabled})
	AssertStatus(t, resp, http.StatusCreated)
	var project domain.Project
	DecodeJSON(t, resp, &project)
	if project.BlockSecrets != enabled {
		t.Fatal("project policy not saved")
	}
	return project
}

func TestRequestPrivacyProjectPolicy(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("Authorization") != "" && r.Header.Get("Authorization") != "Bearer sk-mock-test-key" {
			t.Error("upstream authorization changed")
		}
		if !strings.Contains(strings.ToLower(string(body)), "hello") && !strings.Contains(string(body), privacyTestKey) {
			t.Error("message changed")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"chat-test","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer upstream.Close()
	env := NewProxyTestEnv(t)
	provider := createProvider(t, env, "privacy-upstream", upstream.URL, []string{"openai"})
	createRoute(t, env, "openai", provider)
	protected := createPrivacyProject(t, env, "protected", true)
	createPrivacyProject(t, env, "unprotected", false)
	requestRepo := sqlite.NewProxyRequestRepository(env.DB)
	blocked := func(path string, body any) {
		t.Helper()
		before := calls.Load()
		resp := env.ProxyPost(path, body, map[string]string{"Authorization": "Bearer " + privacyTestKey, "X-Maxx-Project-ID": fmt.Sprint(protected.ID)})
		AssertStatus(t, resp, http.StatusBadRequest)
		result, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if !strings.Contains(string(result), "privacy_blocked") || strings.Contains(string(result), privacyTestKey) {
			t.Fatalf("unsafe/wrong error: %s", result)
		}
		if calls.Load() != before {
			t.Fatal("blocked content reached upstream")
		}
		requests, err := requestRepo.List(domain.DefaultTenantID, 1, 0)
		if err != nil || len(requests) != 1 || requests[0].Status != "REJECTED" || requests[0].ProjectID != protected.ID {
			t.Fatalf("rejection not recorded: %+v %v", requests, err)
		}
		var count int64
		env.DB.GormDB().Model(&sqlite.ProxyUpstreamAttempt{}).Where("proxy_request_id = ?", requests[0].ID).Count(&count)
		if count != 0 {
			t.Fatal("privacy rejection created an upstream attempt")
		}
	}
	secretBody := openaiRequest("gpt-4o")
	secretBody["messages"] = []any{map[string]any{"role": "user", "content": privacyTestKey}}
	blocked("/project/protected/v1/chat/completions", secretBody)
	blocked(fmt.Sprintf("/provider/%d/v1/chat/completions", provider), secretBody)
	toolBody := openaiRequest("gpt-4o")
	toolBody["messages"] = []any{map[string]any{"role": "tool", "tool_call_id": "call-1", "content": `{"password":"tiny"}`}}
	blocked("/project/protected/v1/chat/completions", toolBody)

	// Client auth keys are not message content; safe messages still pass.
	resp := env.ProxyPost("/project/protected/v1/chat/completions", openaiRequest("gpt-4o"), map[string]string{"Authorization": "Bearer " + privacyTestKey})
	AssertStatus(t, resp, http.StatusOK)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	// A different project's disabled policy must preserve the secret upstream.
	resp = env.ProxyPost("/project/unprotected/v1/chat/completions", secretBody, nil)
	AssertStatus(t, resp, http.StatusOK)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// Existing route editors omit this field. Such saves must not disable it.
	resp = env.AdminPut(fmt.Sprintf("/api/admin/projects/%d", protected.ID), map[string]any{"name": "protected", "slug": "protected", "enabledCustomRoutes": []string{}})
	AssertStatus(t, resp, http.StatusOK)
	var updated domain.Project
	DecodeJSON(t, resp, &updated)
	if !updated.BlockSecrets {
		t.Fatal("omitted field disabled protection")
	}
	blocked("/project/protected/v1/chat/completions", secretBody)
	resp = env.AdminPut(fmt.Sprintf("/api/admin/projects/%d/privacy", protected.ID), map[string]any{"blockSecrets": false})
	AssertStatus(t, resp, http.StatusOK)
	DecodeJSON(t, resp, &updated)
	if updated.BlockSecrets {
		t.Fatal("cannot disable protection")
	}
	resp = env.ProxyPost("/project/protected/v1/chat/completions", secretBody, nil)
	AssertStatus(t, resp, http.StatusOK)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// Both rejected and forwarded requests, and all attempts, are scrubbed in DB.
	var records []sqlite.ProxyRequest
	if err := env.DB.GormDB().Find(&records).Error; err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if strings.Contains(string(record.RequestInfo), privacyTestKey) || strings.Contains(string(record.RequestInfo), "tiny") {
			t.Fatalf("request archive leaked: %s", record.RequestInfo)
		}
	}
	var attempts []sqlite.ProxyUpstreamAttempt
	if err := env.DB.GormDB().Find(&attempts).Error; err != nil {
		t.Fatal(err)
	}
	for _, attempt := range attempts {
		if strings.Contains(string(attempt.RequestInfo), privacyTestKey) {
			t.Fatal("attempt archive leaked")
		}
	}
}

func TestRequestPrivacyStreamArchive(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, part := range []string{"sk-proj-testtest", "testtesttesttesttesttest"} {
			fmt.Fprintf(w, "data: {\"id\":\"chat-test\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":%q},\"finish_reason\":null}]}\n\n", part)
			w.(http.Flusher).Flush()
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer upstream.Close()
	env := NewProxyTestEnv(t)
	provider := createProvider(t, env, "privacy-stream", upstream.URL, []string{"openai"})
	createRoute(t, env, "openai", provider)
	project := createPrivacyProject(t, env, "stream-protected", true)
	body := openaiRequest("gpt-4o")
	body["stream"] = true
	resp := env.ProxyPost("/project/stream-protected/v1/chat/completions", body, nil)
	AssertStatus(t, resp, http.StatusOK)
	received, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(received), "sk-proj-testtest") || !strings.Contains(string(received), "testtesttesttesttesttest") || !strings.Contains(string(received), "[DONE]") {
		t.Fatalf("client stream modified: %s", received)
	}
	repo := sqlite.NewProxyRequestRepository(env.DB)
	deadline := time.Now().Add(3 * time.Second)
	for {
		rows, err := repo.List(domain.DefaultTenantID, 1, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) > 0 && rows[0].Status == "COMPLETED" {
			stored, err := repo.GetByID(domain.DefaultTenantID, rows[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.ProjectID != project.ID || stored.ResponseInfo == nil || strings.Contains(stored.ResponseInfo.Body, "sk-proj-testtest") || !strings.Contains(stored.ResponseInfo.Body, "REDACTED") {
				t.Fatalf("unsafe stored stream: %+v", stored.ResponseInfo)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("stream not finalized")
		}
		time.Sleep(10 * time.Millisecond)
	}
	var attempts []sqlite.ProxyUpstreamAttempt
	env.DB.GormDB().Find(&attempts)
	for _, a := range attempts {
		if strings.Contains(string(a.ResponseInfo), "sk-proj-testtest") {
			t.Fatal("attempt stream leaked")
		}
	}
}

func TestRequestPrivacyWebSocketTurns(t *testing.T) {
	var frames atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Upgrade(w, r, nil, 4096, 4096)
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
			frames.Add(1)
			conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"id":"resp_private","model":"gpt-test","output":[]}}`))
		}
	}))
	defer upstream.Close()
	env := NewProxyTestEnv(t)
	createPrivacyProject(t, env, "ws-protected", true)
	resp := env.AdminPost("/api/admin/providers", map[string]any{"name": "privacy-ws", "type": "custom", "config": map[string]any{"custom": map[string]any{"baseURL": upstream.URL + "/v1", "apiKey": "sk-mock-test-key", "responsesWebSocket": true}}, "supportedClientTypes": []string{"codex"}})
	AssertStatus(t, resp, http.StatusCreated)
	var provider domain.Provider
	DecodeJSON(t, resp, &provider)
	createNativeRoute(t, env, "codex", provider.ID, 1)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(env.Server.URL, "http")+"/project/ws-protected/v1/responses", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-test","input":[{"role":"user","content":"hello"}]}`))
	_, event, err := conn.ReadMessage()
	if err != nil || !strings.Contains(string(event), "response.completed") {
		t.Fatalf("safe turn: %s %v", event, err)
	}
	conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-test","input":[{"role":"user","content":"`+privacyTestKey+`"}]}`))
	_, event, err = conn.ReadMessage()
	if err != nil || !strings.Contains(string(event), "privacy_blocked") || strings.Contains(string(event), privacyTestKey) {
		t.Fatalf("blocked turn: %s %v", event, err)
	}
	if frames.Load() != 1 {
		t.Fatalf("secret turn reached upstream: %d", frames.Load())
	}
}

func TestRequestPrivacyReviewRegressions(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"chat-test","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}]}`)
	}))
	defer upstream.Close()
	env := NewProxyTestEnv(t)
	provider := createProvider(t, env, "review-regressions", upstream.URL, []string{"openai"})
	createRoute(t, env, "openai", provider)
	createPrivacyProject(t, env, "review-regressions", true)
	safeMessages := []string{"Explain basic internationalization", `password = os.getenv("X")`, `api_key = config.api_key`, `API_KEY=[REDACTED]`, `{"access_token":"[REDACTED]"}`}
	for _, text := range safeMessages {
		body := openaiRequest("gpt-4o")
		body["messages"] = []any{map[string]any{"role": "user", "content": text}}
		resp := env.ProxyPost("/project/review-regressions/v1/chat/completions", body, nil)
		AssertStatus(t, resp, http.StatusOK)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	if calls.Load() != int32(len(safeMessages)) {
		t.Fatal("ordinary messages were not forwarded")
	}
	for _, tc := range []struct{ path, body string }{
		{"/v1/messages", `{"model":"claude-test","max_tokens":1024,"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"tool-1","name":"run","input":{"data":"` + privacyTestKey + `"}}]}]}`},
		{"/v1beta/models/gemini-test:generateContent", `{"contents":[{"parts":[{"functionCall":{"name":"run","args":{"id":"` + privacyTestKey + `"}}}]}]}`},
		{"/v1beta/models/gemini-test:generateContent", `{"contents":[{"parts":[{"functionResponse":{"name":"run","response":{"type":"` + privacyTestKey + `"}}}]}]}`},
	} {
		resp := env.ProxyPost("/project/review-regressions"+tc.path, json.RawMessage(tc.body), nil)
		AssertStatus(t, resp, http.StatusBadRequest)
		result, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if !strings.Contains(string(result), "privacy_blocked") {
			t.Fatalf("unexpected rejection: %s", result)
		}
	}
	if calls.Load() != int32(len(safeMessages)) {
		t.Fatal("tool credential reached upstream")
	}
}

func TestRequestPrivacyMagpieCoverage(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("X-Service-Secret") != "request-header-secret" || r.URL.Query().Get("key") != "query-secret" {
			t.Error("transport metadata changed")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Service-Secret", "response-header-secret")
		fmt.Fprint(w, `{"id":"chat-test","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"password=\"abc;Example456\""},"finish_reason":"stop"}]}`)
	}))
	defer upstream.Close()
	env := NewProxyTestEnv(t)
	provider := createProvider(t, env, "magpie-coverage", upstream.URL, []string{"openai"})
	createRoute(t, env, "openai", provider)
	createPrivacyProject(t, env, "magpie-coverage", true)
	for _, text := range []string{`password="abc;Example456"`, `password=Abcd$efg123`, "hf_" + strings.Repeat("test", 8), `{"aws_secret_access_key":"tiny"}`} {
		body := openaiRequest("gpt-4o")
		body["messages"] = []any{map[string]any{"role": "user", "content": text}}
		resp := env.ProxyPost("/project/magpie-coverage/v1/chat/completions", body, nil)
		AssertStatus(t, resp, http.StatusBadRequest)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	if calls.Load() != 0 {
		t.Fatal("credential reached upstream")
	}
	resp := env.ProxyPost("/project/magpie-coverage/v1/chat/completions?key=query-secret&safe=hello", openaiRequest("gpt-4o"), map[string]string{"Authorization": "Bearer placeholder", "X-Service-Secret": "request-header-secret"})
	AssertStatus(t, resp, http.StatusOK)
	result, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(result), "Example456") {
		t.Fatal("client response changed")
	}
	if calls.Load() != 1 {
		t.Fatal("safe request not forwarded")
	}
	var requests []sqlite.ProxyRequest
	if err := env.DB.GormDB().Find(&requests).Error; err != nil {
		t.Fatal(err)
	}
	var attempts []sqlite.ProxyUpstreamAttempt
	if err := env.DB.GormDB().Find(&attempts).Error; err != nil {
		t.Fatal(err)
	}
	snapshots := make([]string, 0, len(requests)+len(attempts))
	for _, r := range requests {
		snapshots = append(snapshots, string(r.RequestInfo)+string(r.ResponseInfo))
	}
	for _, a := range attempts {
		snapshots = append(snapshots, string(a.RequestInfo)+string(a.ResponseInfo))
	}
	for _, snapshot := range snapshots {
		for _, secret := range []string{"Example456", "Abcd$efg123", "query-secret", "request-header-secret", "response-header-secret"} {
			if strings.Contains(snapshot, secret) {
				t.Errorf("archive contains credential: %s", secret)
			}
		}
	}
}
