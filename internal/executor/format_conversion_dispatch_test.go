package executor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/converter"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/router"
)

type openAIOnlyConversionAdapter struct {
	calls            int
	seenClientType   domain.ClientType
	seenRequestURI   string
	seenRequestBody  []byte
	responseBody     string
	responseStatus   int
	responseStreamed bool
}

func (a *openAIOnlyConversionAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeOpenAI}
}

func (a *openAIOnlyConversionAdapter) Execute(c *flow.Ctx, _ *domain.Provider) error {
	a.calls++
	a.seenClientType = flow.GetClientType(c)
	a.seenRequestURI = flow.GetRequestURI(c)
	a.seenRequestBody = append([]byte(nil), flow.GetRequestBody(c)...)

	status := a.responseStatus
	if status == 0 {
		status = http.StatusOK
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(status)
	_, err := c.Writer.Write([]byte(a.responseBody))
	return err
}

func TestDispatchConvertsClaudeRouteThroughOpenAIProvider(t *testing.T) {
	adapter := &openAIOnlyConversionAdapter{responseBody: `{
		"id":"chatcmpl-claude-route",
		"object":"chat.completion",
		"created":1700000000,
		"model":"gpt-4o-mini",
		"choices":[{"index":0,"message":{"role":"assistant","content":"converted back to claude"},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}
	}`}
	c, proxyRepo, attemptRepo := newClaudeOpenAIConversionDispatchCtx(t, `{
		"model":"claude-3-5-sonnet",
		"max_tokens":64,
		"messages":[{"role":"user","content":"hello"}],
		"stream":false
	}`, adapter)
	e := newClaudeOpenAIConversionTestExecutor(proxyRepo, attemptRepo)

	e.dispatch(c)

	if c.Err != nil {
		t.Fatalf("dispatch returned error: %v", c.Err)
	}
	if adapter.calls != 1 {
		t.Fatalf("adapter calls = %d, want 1", adapter.calls)
	}
	if adapter.seenClientType != domain.ClientTypeOpenAI {
		t.Fatalf("adapter client type = %s, want openai", adapter.seenClientType)
	}
	if adapter.seenRequestURI != "/v1/chat/completions" {
		t.Fatalf("request URI = %q, want /v1/chat/completions", adapter.seenRequestURI)
	}

	var openaiReq map[string]any
	if err := json.Unmarshal(adapter.seenRequestBody, &openaiReq); err != nil {
		t.Fatalf("adapter received invalid OpenAI body: %v\n%s", err, adapter.seenRequestBody)
	}
	if got := openaiReq["model"]; got != "claude-3-5-sonnet" {
		t.Fatalf("converted model = %v, want claude-3-5-sonnet", got)
	}
	if got, ok := openaiReq["stream"]; ok && got != false {
		t.Fatalf("converted stream = %v, want absent or false", got)
	}
	messages, ok := openaiReq["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("converted messages = %#v, want one OpenAI message", openaiReq["messages"])
	}
	first, _ := messages[0].(map[string]any)
	if first["role"] != "user" {
		t.Fatalf("converted role = %v, want user", first["role"])
	}

	body := c.Writer.(*httptest.ResponseRecorder).Body.String()
	if !strings.Contains(body, `"type":"message"`) || !strings.Contains(body, `"text":"converted back to claude"`) {
		t.Fatalf("client body was not converted back to Claude format: %s", body)
	}
	if len(attemptRepo.updated) == 0 || attemptRepo.updated[len(attemptRepo.updated)-1].Status != "COMPLETED" {
		t.Fatalf("expected completed attempt, got %#v", attemptRepo.updated)
	}
	if len(proxyRepo.updated) == 0 || proxyRepo.updated[len(proxyRepo.updated)-1].Status != "COMPLETED" {
		t.Fatalf("expected completed proxy request, got %#v", proxyRepo.updated)
	}
}

func TestDispatchFailsClosedWhenClaudeToOpenAIRequestConversionFails(t *testing.T) {
	adapter := &openAIOnlyConversionAdapter{responseBody: `{}`}
	c, proxyRepo, attemptRepo := newClaudeOpenAIConversionDispatchCtx(t, `{not-json`, adapter)
	e := newClaudeOpenAIConversionTestExecutor(proxyRepo, attemptRepo)

	e.dispatch(c)

	if c.Err == nil {
		t.Fatal("expected request conversion failure")
	}
	if adapter.calls != 0 {
		t.Fatalf("adapter calls = %d, want 0 so malformed Claude body is not sent as OpenAI", adapter.calls)
	}
	if len(attemptRepo.created) != 0 {
		t.Fatalf("created attempts = %d, want 0 before upstream dispatch", len(attemptRepo.created))
	}
	if got := c.Writer.(*httptest.ResponseRecorder).Body.String(); got != "" {
		t.Fatalf("client body = %q, want empty fail-closed body", got)
	}
	if len(proxyRepo.updated) == 0 || proxyRepo.updated[len(proxyRepo.updated)-1].Status != "FAILED" {
		t.Fatalf("expected failed proxy request update, got %#v", proxyRepo.updated)
	}
}

func TestDispatchFailsClosedWhenOpenAIToClaudeResponseConversionFails(t *testing.T) {
	adapter := &openAIOnlyConversionAdapter{responseBody: `{not-openai-json`}
	c, proxyRepo, attemptRepo := newClaudeOpenAIConversionDispatchCtx(t, `{
		"model":"claude-3-5-sonnet",
		"max_tokens":64,
		"messages":[{"role":"user","content":"hello"}],
		"stream":false
	}`, adapter)
	e := newClaudeOpenAIConversionTestExecutor(proxyRepo, attemptRepo)

	e.dispatch(c)

	if c.Err == nil {
		t.Fatal("expected response conversion failure")
	}
	if adapter.calls != 1 {
		t.Fatalf("adapter calls = %d, want 1", adapter.calls)
	}
	if got := c.Writer.(*httptest.ResponseRecorder).Body.String(); got != "" {
		t.Fatalf("client body = %q, want empty body instead of raw OpenAI payload", got)
	}
	if len(attemptRepo.updated) == 0 || attemptRepo.updated[len(attemptRepo.updated)-1].Status != "FAILED" {
		t.Fatalf("expected failed attempt, got %#v", attemptRepo.updated)
	}
	if len(proxyRepo.updated) == 0 || proxyRepo.updated[len(proxyRepo.updated)-1].Status != "FAILED" {
		t.Fatalf("expected failed proxy request update, got %#v", proxyRepo.updated)
	}
}

func newClaudeOpenAIConversionTestExecutor(proxyRepo *codexGuardProxyRequestRepo, attemptRepo *recordingAttemptRepo) *Executor {
	return &Executor{
		proxyRequestRepo: proxyRepo,
		attemptRepo:      attemptRepo,
		modelMappingRepo: &codexGuardModelMappingRepo{},
		settingsRepo:     &codexGuardSettingsRepo{},
		converter:        converter.GetGlobalRegistry(),
	}
}

func newClaudeOpenAIConversionDispatchCtx(t *testing.T, requestBody string, adapter *openAIOnlyConversionAdapter) (*flow.Ctx, *codexGuardProxyRequestRepo, *recordingAttemptRepo) {
	t.Helper()
	proxyRepo := &codexGuardProxyRequestRepo{}
	attemptRepo := &recordingAttemptRepo{}
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
	state := &execState{
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
		routes: []*router.MatchedRoute{
			{
				Route: &domain.Route{ID: 30, TenantID: domain.DefaultTenantID, ProviderID: 40, ClientType: domain.ClientTypeClaude},
				Provider: &domain.Provider{
					ID:                   40,
					TenantID:             domain.DefaultTenantID,
					Type:                 "custom",
					Name:                 "openai-only-custom",
					SupportedClientTypes: []domain.ClientType{domain.ClientTypeOpenAI},
				},
				ProviderAdapter: adapter,
				RetryConfig:     &domain.RetryConfig{MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
			},
		},
	}
	c.Set(flow.KeyExecutorState, state)
	return c, proxyRepo, attemptRepo
}
