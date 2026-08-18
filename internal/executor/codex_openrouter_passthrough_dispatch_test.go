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
	"github.com/awsl-project/maxx/internal/systemsettingcache"
)

// codexOpenRouterRecordingAdapter stands in for a native OpenRouter provider that
// speaks claude/openai/codex. It records the client type, URI, and body it is
// handed so the test can prove whether the Codex request passed through natively
// (/responses) or was bridged to OpenAI Chat Completions.
type codexOpenRouterRecordingAdapter struct {
	calls           int
	seenClientType  domain.ClientType
	seenRequestURI  string
	seenRequestBody []byte
	responseBody    string
}

func (a *codexOpenRouterRecordingAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeClaude, domain.ClientTypeOpenAI, domain.ClientTypeCodex}
}

func (a *codexOpenRouterRecordingAdapter) Execute(c *flow.Ctx, _ *domain.Provider) error {
	a.calls++
	a.seenClientType = flow.GetClientType(c)
	a.seenRequestURI = flow.GetRequestURI(c)
	a.seenRequestBody = append([]byte(nil), flow.GetRequestBody(c)...)
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	_, err := c.Writer.Write([]byte(a.responseBody))
	return err
}

// codexCustomToolRequest is a minimal Codex Responses request carrying a custom
// (code-mode) tool — the shape that the legacy chat bridge silently corrupted.
const codexCustomToolRequest = `{
	"model":"gpt-5.6-sol",
	"instructions":"You are a coding agent.",
	"tools":[{"type":"custom","name":"exec","description":"Run a shell command"}],
	"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"run echo hi via exec"}]}],
	"stream":false
}`

func newCodexOpenRouterDispatchCtx(t *testing.T, adapter *codexOpenRouterRecordingAdapter) (*flow.Ctx, *recordingProxyRequestRepo, *recordingAttemptRepo) {
	t.Helper()
	proxyRepo := &recordingProxyRequestRepo{}
	attemptRepo := &recordingAttemptRepo{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/responses", strings.NewReader(codexCustomToolRequest)).WithContext(context.Background())
	c := flow.NewCtx(rec, req)
	proxyReq := &domain.ProxyRequest{
		ID:           400,
		TenantID:     domain.DefaultTenantID,
		ClientType:   domain.ClientTypeCodex,
		RequestModel: "gpt-5.6-sol",
		Status:       "IN_PROGRESS",
		StartTime:    time.Now(),
	}
	state := &execState{
		ctx:                 context.Background(),
		proxyReq:            proxyReq,
		tenantID:            domain.DefaultTenantID,
		clientType:          domain.ClientTypeCodex,
		requestModel:        "gpt-5.6-sol",
		isStream:            false,
		requestBody:         []byte(codexCustomToolRequest),
		originalRequestBody: []byte(codexCustomToolRequest),
		requestHeaders:      http.Header{"Content-Type": []string{"application/json"}},
		requestURI:          "/responses",
		routes: []*router.MatchedRoute{
			{
				Route: &domain.Route{ID: 60, TenantID: domain.DefaultTenantID, ProviderID: 70, ClientType: domain.ClientTypeCodex},
				Provider: &domain.Provider{
					ID:                   70,
					TenantID:             domain.DefaultTenantID,
					Type:                 "openrouter",
					Name:                 "zz-openrouter",
					SupportedClientTypes: []domain.ClientType{domain.ClientTypeClaude, domain.ClientTypeOpenAI, domain.ClientTypeCodex},
				},
				ProviderAdapter: adapter,
				RetryConfig:     &domain.RetryConfig{MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
			},
		},
	}
	c.Set(flow.KeyExecutorState, state)
	return c, proxyRepo, attemptRepo
}

func newCodexOpenRouterTestExecutor(proxyRepo *recordingProxyRequestRepo, attemptRepo *recordingAttemptRepo, bridgeEnabled bool) *Executor {
	values := map[string]string{}
	if bridgeEnabled {
		values[domain.SettingKeyCodexOpenRouterBridgeEnabled] = "true"
	}
	// Clear any cached value so this executor reads its own setting deterministically.
	systemsettingcache.Invalidate(domain.SettingKeyCodexOpenRouterBridgeEnabled)
	return &Executor{
		proxyRequestRepo: proxyRepo,
		attemptRepo:      attemptRepo,
		modelMappingRepo: &stubModelMappingRepo{},
		settingsRepo:     &stubExecutorSettingsRepo{values: values},
		converter:        converter.GetGlobalRegistry(),
	}
}

// TestDispatchCodexOpenRouterPassthroughByDefault proves that, with the bridge
// kill switch off (default), a Codex request to a native OpenRouter provider is
// handed to the adapter unchanged: still Codex, still /responses, still carrying
// the custom (code-mode) tool — never rewritten into OpenAI Chat Completions.
func TestDispatchCodexOpenRouterPassthroughByDefault(t *testing.T) {
	adapter := &codexOpenRouterRecordingAdapter{responseBody: `{"id":"resp_test","object":"response","status":"completed","output":[{"type":"custom_tool_call","id":"ctc_1","call_id":"call_1","name":"exec","input":"echo hi"}],"usage":{"input_tokens":5,"output_tokens":3,"total_tokens":8}}`}
	c, proxyRepo, attemptRepo := newCodexOpenRouterDispatchCtx(t, adapter)
	e := newCodexOpenRouterTestExecutor(proxyRepo, attemptRepo, false)

	e.dispatch(c)

	if c.Err != nil {
		t.Fatalf("dispatch returned error: %v", c.Err)
	}
	if adapter.calls != 1 {
		t.Fatalf("adapter calls = %d, want 1", adapter.calls)
	}
	if adapter.seenClientType != domain.ClientTypeCodex {
		t.Fatalf("adapter client type = %s, want codex (passthrough)", adapter.seenClientType)
	}
	if adapter.seenRequestURI != "/responses" {
		t.Fatalf("request URI = %q, want /responses (no bridge)", adapter.seenRequestURI)
	}
	body := string(adapter.seenRequestBody)
	if !strings.Contains(body, `"type":"custom"`) || !strings.Contains(body, `"name":"exec"`) {
		t.Fatalf("passthrough body lost the custom tool: %s", body)
	}
	if strings.Contains(body, `"messages"`) {
		t.Fatalf("passthrough body was converted to Chat Completions (has messages): %s", body)
	}
}

// TestDispatchCodexOpenRouterBridgesWhenKillSwitchEnabled proves the escape hatch
// still works: enabling SettingKeyCodexOpenRouterBridgeEnabled restores the legacy
// chat bridge, so the adapter is handed an OpenAI Chat Completions request.
func TestDispatchCodexOpenRouterBridgesWhenKillSwitchEnabled(t *testing.T) {
	adapter := &codexOpenRouterRecordingAdapter{responseBody: `{"id":"chatcmpl_test","object":"chat.completion","created":1700000000,"model":"gpt-5.6-sol","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`}
	c, proxyRepo, attemptRepo := newCodexOpenRouterDispatchCtx(t, adapter)
	e := newCodexOpenRouterTestExecutor(proxyRepo, attemptRepo, true)

	e.dispatch(c)

	if c.Err != nil {
		t.Fatalf("dispatch returned error: %v", c.Err)
	}
	if adapter.calls != 1 {
		t.Fatalf("adapter calls = %d, want 1", adapter.calls)
	}
	if adapter.seenClientType != domain.ClientTypeOpenAI {
		t.Fatalf("adapter client type = %s, want openai (bridged)", adapter.seenClientType)
	}
	if adapter.seenRequestURI != "/v1/chat/completions" {
		t.Fatalf("request URI = %q, want /v1/chat/completions (bridged)", adapter.seenRequestURI)
	}
	if body := string(adapter.seenRequestBody); !strings.Contains(body, `"messages"`) {
		t.Fatalf("bridged body was not converted to Chat Completions (no messages): %s", body)
	}
}

// TestCodexOpenRouterBridgeEnabledReadsSetting guards the setting-key wiring in
// isolation: default off, on when the setting is "true".
func TestCodexOpenRouterBridgeEnabledReadsSetting(t *testing.T) {
	systemsettingcache.Invalidate(domain.SettingKeyCodexOpenRouterBridgeEnabled)
	off := &Executor{settingsRepo: &stubExecutorSettingsRepo{}}
	if off.codexOpenRouterBridgeEnabled() {
		t.Fatal("default should be false (passthrough)")
	}

	systemsettingcache.Invalidate(domain.SettingKeyCodexOpenRouterBridgeEnabled)
	on := &Executor{settingsRepo: &stubExecutorSettingsRepo{values: map[string]string{
		domain.SettingKeyCodexOpenRouterBridgeEnabled: "true",
	}}}
	if !on.codexOpenRouterBridgeEnabled() {
		t.Fatal("setting=true should enable the bridge kill switch")
	}
}
