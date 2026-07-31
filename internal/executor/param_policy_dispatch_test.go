package executor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/converter"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/router"
	"github.com/tidwall/gjson"
)

// paramPolicyDispatchCtx builds a no-conversion OpenAI→OpenAI dispatch (the
// OpenRouter/custom shape) whose sole provider carries the given reasoning
// policy, so the outbound body the adapter observes reflects the executor's
// authoritative param stage.
func paramPolicyDispatchCtx(t *testing.T, requestBody string, policy *domain.ReasoningPolicy, adapter *openAIOnlyConversionAdapter) (*flow.Ctx, *Executor, *recordingProxyRequestRepo) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(requestBody)).WithContext(context.Background())
	c := flow.NewCtx(rec, req)
	proxyReq := &domain.ProxyRequest{
		ID: 301, TenantID: domain.DefaultTenantID, ClientType: domain.ClientTypeOpenAI,
		RequestModel: "gpt-4o", Status: "IN_PROGRESS", StartTime: time.Now(),
	}
	state := &execState{
		ctx:                 context.Background(),
		proxyReq:            proxyReq,
		tenantID:            domain.DefaultTenantID,
		clientType:          domain.ClientTypeOpenAI,
		requestModel:        "gpt-4o",
		isStream:            false,
		requestBody:         []byte(requestBody),
		originalRequestBody: []byte(requestBody),
		requestHeaders:      http.Header{"Content-Type": []string{"application/json"}},
		requestURI:          "/v1/chat/completions",
		routes: []*router.MatchedRoute{
			{
				Route: &domain.Route{ID: 32, TenantID: domain.DefaultTenantID, ProviderID: 42, ClientType: domain.ClientTypeOpenAI},
				Provider: &domain.Provider{
					ID: 42, TenantID: domain.DefaultTenantID, Type: "custom", Name: "openrouter-like",
					SupportedClientTypes: []domain.ClientType{domain.ClientTypeOpenAI},
					Config:               &domain.ProviderConfig{Reasoning: policy},
				},
				ProviderAdapter: adapter,
				RetryConfig:     &domain.RetryConfig{MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
			},
		},
	}
	c.Set(flow.KeyExecutorState, state)
	proxyRepo := &recordingProxyRequestRepo{}
	e := &Executor{
		proxyRequestRepo: proxyRepo,
		attemptRepo:      &recordingAttemptRepo{},
		modelMappingRepo: &staticModelMappingRepo{},
		settingsRepo:     &stubExecutorSettingsRepo{},
		converter:        converter.GetGlobalRegistry(),
	}
	return c, e, proxyRepo
}

// A provider MaxEffort ceiling clamps the outbound reasoning_effort even though
// no adapter (custom/OpenRouter) touches effort — proving the executor param
// stage, not the adapter, is authoritative and covers the passthrough path.
func TestDispatchClampsReasoningEffortCeiling(t *testing.T) {
	adapter := &openAIOnlyConversionAdapter{responseBody: `{"id":"x","object":"chat.completion","choices":[]}`}
	c, e, proxyRepo := paramPolicyDispatchCtx(t,
		`{"model":"gpt-4o","reasoning_effort":"high","messages":[{"role":"user","content":"hi"}],"stream":false}`,
		&domain.ReasoningPolicy{MaxEffort: "medium"}, adapter)

	e.dispatch(c)

	if c.Err != nil {
		t.Fatalf("dispatch returned error: %v", c.Err)
	}
	if adapter.calls != 1 {
		t.Fatalf("adapter calls = %d, want 1", adapter.calls)
	}
	if got := gjson.GetBytes(adapter.seenRequestBody, "reasoning_effort").String(); got != "medium" {
		t.Fatalf("outbound reasoning_effort = %q, want clamped to medium; body=%s", got, adapter.seenRequestBody)
	}
	if len(proxyRepo.updated) == 0 {
		t.Fatal("proxy request was not updated")
	}
	if got := proxyRepo.updated[len(proxyRepo.updated)-1].ReasoningEffort; got != "medium" {
		t.Fatalf("recorded reasoning effort = %q, want medium", got)
	}
}

// DefaultEffort fills an absent effort; a below-ceiling explicit value is kept.
func TestDispatchBroadcastsPolicyReasoningBeforeAdapterExecute(t *testing.T) {
	var updatesBeforeAdapter int
	adapter := &openAIOnlyConversionAdapter{
		responseBody: `{"id":"x","object":"chat.completion","choices":[]}`,
		onExecute: func() {
			updatesBeforeAdapter = 0
		},
	}
	c, e, proxyRepo := paramPolicyDispatchCtx(t,
		`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"stream":false}`,
		&domain.ReasoningPolicy{DefaultEffort: "low"}, adapter)
	adapter.onExecute = func() {
		updatesBeforeAdapter = len(proxyRepo.updated)
	}

	e.dispatch(c)

	if c.Err != nil {
		t.Fatalf("dispatch returned error: %v", c.Err)
	}
	if updatesBeforeAdapter == 0 {
		t.Fatal("reasoning effort was not persisted before adapter execution")
	}
	if got := proxyRepo.updated[updatesBeforeAdapter-1].ReasoningEffort; got != "low" {
		t.Fatalf("pre-adapter reasoning effort = %q, want low", got)
	}
}

func TestDispatchFillsDefaultEffortWhenAbsent(t *testing.T) {
	adapter := &openAIOnlyConversionAdapter{responseBody: `{"id":"x","object":"chat.completion","choices":[]}`}
	c, e, proxyRepo := paramPolicyDispatchCtx(t,
		`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"stream":false}`,
		&domain.ReasoningPolicy{MaxEffort: "high", DefaultEffort: "low"}, adapter)

	e.dispatch(c)

	if c.Err != nil {
		t.Fatalf("dispatch returned error: %v", c.Err)
	}
	if got := gjson.GetBytes(adapter.seenRequestBody, "reasoning_effort").String(); got != "low" {
		t.Fatalf("outbound reasoning_effort = %q, want default low; body=%s", got, adapter.seenRequestBody)
	}
	if len(proxyRepo.updated) == 0 {
		t.Fatal("proxy request was not updated")
	}
	if got := proxyRepo.updated[len(proxyRepo.updated)-1].ReasoningEffort; got != "low" {
		t.Fatalf("recorded reasoning effort = %q, want low", got)
	}
}
