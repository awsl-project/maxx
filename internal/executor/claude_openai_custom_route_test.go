package executor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider/custom"
	"github.com/awsl-project/maxx/internal/converter"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/router"
)

func TestDispatchClaudeRouteThroughRealCustomOpenAIProvider(t *testing.T) {
	var upstreamPath string
	var upstreamBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("upstream path = %s, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Fatalf("Authorization = %q, want provider bearer key", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Fatalf("decode upstream body: %v", err)
		}
		if _, ok := upstreamBody["messages"].([]any); !ok {
			t.Fatalf("upstream body is not OpenAI chat format: %#v", upstreamBody)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-real-custom",
			"object":"chat.completion",
			"created":1700000000,
			"model":"gpt-4o-mini",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok from openai custom"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}
		}`))
	}))
	defer upstream.Close()

	provider := &domain.Provider{
		ID:                   40,
		TenantID:             domain.DefaultTenantID,
		Type:                 "custom",
		Name:                 "openai-format-custom",
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeOpenAI},
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL: upstream.URL + "/v1",
			APIKey:  "sk-test",
		}},
	}
	adapter, err := custom.NewAdapter(provider)
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	requestBody := `{"model":"claude-3-5-sonnet","max_tokens":64,"messages":[{"role":"user","content":"hello"}],"stream":false}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(requestBody)).WithContext(context.Background())
	c := flow.NewCtx(rec, req)
	proxyReq := &domain.ProxyRequest{
		ID:         200,
		TenantID:   domain.DefaultTenantID,
		ClientType: domain.ClientTypeClaude,
		Status:     "IN_PROGRESS",
		StartTime:  time.Now(),
	}
	c.Set(flow.KeyExecutorState, &execState{
		ctx:                 context.Background(),
		proxyReq:            proxyReq,
		tenantID:            domain.DefaultTenantID,
		clientType:          domain.ClientTypeClaude,
		requestModel:        "claude-3-5-sonnet",
		isStream:            false,
		requestBody:         []byte(requestBody),
		originalRequestBody: []byte(requestBody),
		requestHeaders:      http.Header{"Content-Type": []string{"application/json"}},
		requestURI:          "/v1/messages",
		routes: []*router.MatchedRoute{{
			Route:           &domain.Route{ID: 30, TenantID: domain.DefaultTenantID, ProviderID: provider.ID, ClientType: domain.ClientTypeClaude, IsEnabled: true},
			Provider:        provider,
			ProviderAdapter: adapter,
			RetryConfig:     &domain.RetryConfig{MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
		}},
	})

	e := &Executor{
		proxyRequestRepo: &recordingProxyRequestRepo{},
		attemptRepo:      &recordingAttemptRepo{},
		modelMappingRepo: &stubModelMappingRepo{},
		settingsRepo:     &stubExecutorSettingsRepo{},
		converter:        converter.GetGlobalRegistry(),
	}

	e.dispatch(c)

	if c.Err != nil {
		t.Fatalf("dispatch returned error: %v", c.Err)
	}
	if upstreamPath != "/v1/chat/completions" {
		t.Fatalf("upstream was not called as OpenAI chat completions, path=%q", upstreamPath)
	}
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `"type":"message"`) || !strings.Contains(body, `"text":"ok from openai custom"`) {
		t.Fatalf("client did not receive converted Claude response: status=%d body=%s", rec.Code, body)
	}
}
