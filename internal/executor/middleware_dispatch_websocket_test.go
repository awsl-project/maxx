package executor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/cooldown"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/router"
	"github.com/tidwall/gjson"
)

// recordingWSAdapter records whether HTTP Execute or native WS dispatch ran.
// errSequence is drained one error per ExecuteResponsesWebSocket call; once
// exhausted the adapter succeeds.
type recordingWSAdapter struct {
	httpExecuteCalls int
	wsCalls          int
	errSequence      []error
	lastFrame        []byte
	resultModel      string
}

func (a *recordingWSAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeCodex}
}

func (a *recordingWSAdapter) Execute(*flow.Ctx, *domain.Provider) error {
	a.httpExecuteCalls++
	return errors.New("HTTP Execute must not run on native Responses WebSocket path")
}

func (a *recordingWSAdapter) ExecuteResponsesWebSocket(
	_ *flow.Ctx,
	provider *domain.Provider,
	exchange *domain.ResponsesWebSocketExchange,
) (*domain.ResponsesWebSocketResult, error) {
	a.wsCalls++
	if exchange != nil {
		a.lastFrame = append([]byte(nil), exchange.Frame...)
	}
	if exchange != nil && exchange.TryAcquireProviderSlot != nil {
		release, acquired := exchange.AcquireProviderSlot()
		if !acquired {
			proxyErr := domain.NewProxyErrorWithMessage(domain.ErrNoAvailableProviders, true, "provider concurrency limit reached")
			proxyErr.Scope = domain.ScopeProvider
			return nil, &domain.ResponsesWebSocketAttemptError{Err: proxyErr}
		}
		defer release()
	}
	if len(a.errSequence) > 0 {
		err := a.errSequence[0]
		a.errSequence = a.errSequence[1:]
		return nil, err
	}
	model := a.resultModel
	if model == "" {
		model = gjson.GetBytes(exchange.Frame, "model").String()
	}
	return &domain.ResponsesWebSocketResult{
		ProviderID:    provider.ID,
		ResponseModel: model,
	}, nil
}

func newWSDispatchExecutor(proxyRepo *recordingProxyRequestRepo, attemptRepo *recordingAttemptRepo) *Executor {
	return &Executor{
		proxyRequestRepo: proxyRepo,
		attemptRepo:      attemptRepo,
		modelMappingRepo: &stubModelMappingRepo{},
		settingsRepo:     &stubExecutorSettingsRepo{},
	}
}

func newWSDispatchCtx(
	t *testing.T,
	frame []byte,
	exchange *domain.ResponsesWebSocketExchange,
	routes []*router.MatchedRoute,
) (*flow.Ctx, *recordingProxyRequestRepo, *recordingAttemptRepo) {
	t.Helper()
	proxyRepo := &recordingProxyRequestRepo{}
	attemptRepo := &recordingAttemptRepo{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/responses", nil).WithContext(context.Background())
	c := flow.NewCtx(rec, req)
	if exchange == nil {
		exchange = &domain.ResponsesWebSocketExchange{
			ConnectionID: "ws-conn-test",
			Frame:        frame,
		}
	} else if exchange.Frame == nil {
		exchange.Frame = frame
	}
	proxyReq := &domain.ProxyRequest{
		ID:           501,
		TenantID:     domain.DefaultTenantID,
		ClientType:   domain.ClientTypeCodex,
		RequestModel: "gpt-test",
		Status:       "IN_PROGRESS",
		StartTime:    time.Now(),
	}
	state := &execState{
		ctx:          context.Background(),
		proxyReq:     proxyReq,
		tenantID:     domain.DefaultTenantID,
		clientType:   domain.ClientTypeCodex,
		requestModel: "gpt-test",
		isStream:     true,
		requestBody:  frame,
		requestURI:   "/v1/responses",
		routes:       routes,
		wsExchange:   exchange,
	}
	c.Set(flow.KeyExecutorState, state)
	return c, proxyRepo, attemptRepo
}

func wsMatchedRoute(id, providerID uint64, adapter *recordingWSAdapter, native bool) *router.MatchedRoute {
	return &router.MatchedRoute{
		Route: &domain.Route{
			ID:         id,
			TenantID:   domain.DefaultTenantID,
			ProviderID: providerID,
			ClientType: domain.ClientTypeCodex,
			IsNative:   native,
		},
		Provider: &domain.Provider{
			ID:                   providerID,
			TenantID:             domain.DefaultTenantID,
			Type:                 "codex",
			Name:                 "ws-provider",
			SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex},
		},
		ProviderAdapter: adapter,
		RetryConfig:     &domain.RetryConfig{MaxRetries: 0, InitialInterval: 0, BackoffRate: 1, MaxInterval: 0},
	}
}

func preWriteWSErr() *domain.ResponsesWebSocketAttemptError {
	return &domain.ResponsesWebSocketAttemptError{
		Err: errors.New("handshake failed before request frame write"),
	}
}

// dispatch must take the native WS path and never call ProviderAdapter.Execute.
func TestDispatch_ResponsesWebSocketBypassesHTTPExecute(t *testing.T) {
	adapter := &recordingWSAdapter{resultModel: "gpt-test"}
	frame := []byte(`{"type":"response.create","model":"gpt-test","input":[]}`)
	c, proxyRepo, attemptRepo := newWSDispatchCtx(t, frame, nil, []*router.MatchedRoute{
		wsMatchedRoute(1, 11, adapter, true),
	})
	e := newWSDispatchExecutor(proxyRepo, attemptRepo)

	e.dispatch(c)

	if c.Err != nil {
		t.Fatalf("dispatch error: %v", c.Err)
	}
	if adapter.httpExecuteCalls != 0 {
		t.Fatalf("HTTP Execute calls = %d, want 0", adapter.httpExecuteCalls)
	}
	if adapter.wsCalls != 1 {
		t.Fatalf("ExecuteResponsesWebSocket calls = %d, want 1", adapter.wsCalls)
	}
	if len(attemptRepo.created) != 1 {
		t.Fatalf("created attempts = %d, want 1", len(attemptRepo.created))
	}
	if len(proxyRepo.updated) == 0 {
		t.Fatal("expected proxy request updates")
	}
	last := proxyRepo.updated[len(proxyRepo.updated)-1]
	if last.Status != "COMPLETED" {
		t.Fatalf("proxy status = %q, want COMPLETED", last.Status)
	}
	if last.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("proxy status code = %d, want 101", last.StatusCode)
	}
	state, ok := getExecState(c)
	if !ok || state.wsExchange == nil {
		t.Fatal("missing exec state / ws exchange")
	}
	if state.wsExchange.PinnedProviderID != 11 {
		t.Fatalf("PinnedProviderID = %d, want 11 after success", state.wsExchange.PinnedProviderID)
	}
}

// One WebSocket turn executes only the first matched provider; second is never tried.
func TestDispatch_ResponsesWebSocketExecutesFirstProviderOnly(t *testing.T) {
	first := &recordingWSAdapter{errSequence: []error{preWriteWSErr()}}
	second := &recordingWSAdapter{resultModel: "gpt-test"}
	frame := []byte(`{"type":"response.create","model":"gpt-test","input":[]}`)
	c, proxyRepo, attemptRepo := newWSDispatchCtx(t, frame, nil, []*router.MatchedRoute{
		wsMatchedRoute(1, 21, first, true),
		wsMatchedRoute(2, 22, second, true),
	})
	e := newWSDispatchExecutor(proxyRepo, attemptRepo)

	e.dispatch(c)

	if c.Err == nil {
		t.Fatal("expected dispatch error from first provider")
	}
	if first.httpExecuteCalls != 0 || second.httpExecuteCalls != 0 {
		t.Fatalf("HTTP Execute calls first=%d second=%d, want 0/0", first.httpExecuteCalls, second.httpExecuteCalls)
	}
	if first.wsCalls != 1 || second.wsCalls != 0 {
		t.Fatalf("WS calls first=%d second=%d, want 1/0 (no cross-provider fallback)", first.wsCalls, second.wsCalls)
	}
	if len(attemptRepo.created) != 1 {
		t.Fatalf("created attempts = %d, want 1", len(attemptRepo.created))
	}
	if attemptRepo.created[0].ProviderID != 21 {
		t.Fatalf("attempt provider = %d, want 21", attemptRepo.created[0].ProviderID)
	}
	_ = proxyRepo
}

func TestDispatch_ResponsesWebSocketTriesNextProviderAfterSlotRace(t *testing.T) {
	first := &recordingWSAdapter{resultModel: "gpt-test"}
	second := &recordingWSAdapter{resultModel: "gpt-test"}
	firstRoute := wsMatchedRoute(1, 31, first, true)
	firstRoute.Provider.MaxConcurrency = 1
	secondRoute := wsMatchedRoute(2, 32, second, true)
	secondRoute.Provider.MaxConcurrency = 1
	frame := []byte(`{"type":"response.create","model":"gpt-test","input":[]}`)
	c, proxyRepo, attemptRepo := newWSDispatchCtx(t, frame, nil, []*router.MatchedRoute{firstRoute, secondRoute})
	r := router.NewRouter(nil, nil, nil, nil, nil)
	release, acquired := r.TryAcquireProvider(firstRoute.Provider)
	if !acquired {
		t.Fatal("failed to occupy first provider slot")
	}
	defer release()
	e := newWSDispatchExecutor(proxyRepo, attemptRepo)
	e.router = r

	e.dispatch(c)

	if c.Err != nil {
		t.Fatalf("dispatch error: %v", c.Err)
	}
	if first.wsCalls != 1 || second.wsCalls != 1 {
		t.Fatalf("WS calls first=%d second=%d, want 1/1", first.wsCalls, second.wsCalls)
	}
	if len(attemptRepo.created) != 2 {
		t.Fatalf("created attempts = %d, want 2", len(attemptRepo.created))
	}
	state, ok := getExecState(c)
	if !ok || state.wsExchange.PinnedProviderID != secondRoute.Provider.ID {
		t.Fatalf("pinned provider = %d, want %d", state.wsExchange.PinnedProviderID, secondRoute.Provider.ID)
	}
}

func TestDispatch_ResponsesWebSocketRecordsCooldownWithoutSecondProvider(t *testing.T) {
	const firstProviderID = uint64(920021)
	cooldown.Default().ClearCooldown(firstProviderID, string(domain.ClientTypeCodex), "")
	t.Cleanup(func() {
		cooldown.Default().ClearCooldown(firstProviderID, string(domain.ClientTypeCodex), "")
	})
	proxyErr := domain.NewProxyErrorWithMessage(errors.New("dial failed"), true, "dial failed")
	proxyErr.Scope = domain.ScopeProvider
	proxyErr.Reason = domain.CooldownReasonNetworkError
	proxyErr.HTTPStatusCode = http.StatusBadGateway
	first := &recordingWSAdapter{errSequence: []error{&domain.ResponsesWebSocketAttemptError{
		Err: proxyErr,
	}}}
	second := &recordingWSAdapter{resultModel: "gpt-test"}
	frame := []byte(`{"type":"response.create","model":"gpt-test","input":[]}`)
	c, proxyRepo, attemptRepo := newWSDispatchCtx(t, frame, nil, []*router.MatchedRoute{
		wsMatchedRoute(1, firstProviderID, first, true),
		wsMatchedRoute(2, firstProviderID+1, second, true),
	})
	e := newWSDispatchExecutor(proxyRepo, attemptRepo)

	e.dispatch(c)

	if c.Err == nil {
		t.Fatal("expected error without second provider")
	}
	if second.wsCalls != 0 {
		t.Fatalf("second WS calls = %d, want 0", second.wsCalls)
	}
	if !cooldown.Default().IsInCooldown(firstProviderID, string(domain.ClientTypeCodex), "") {
		t.Fatal("provider failure did not enter cooldown")
	}
	_ = proxyRepo
	_ = attemptRepo
}

// Dispatcher no longer re-checks Route.IsNative — a stale false snapshot still executes.
func TestDispatch_ResponsesWebSocketDoesNotRecheckStoredNativeFlag(t *testing.T) {
	adapter := &recordingWSAdapter{resultModel: "gpt-test"}
	frame := []byte(`{"type":"response.create","model":"gpt-test","input":[]}`)
	c, _, attemptRepo := newWSDispatchCtx(t, frame, nil, []*router.MatchedRoute{
		wsMatchedRoute(1, 51, adapter, false),
	})
	e := newWSDispatchExecutor(&recordingProxyRequestRepo{}, attemptRepo)

	e.dispatch(c)

	if c.Err != nil {
		t.Fatalf("dispatch error: %v", c.Err)
	}
	if adapter.wsCalls != 1 {
		t.Fatalf("WS calls = %d, want 1 despite IsNative=false snapshot", adapter.wsCalls)
	}
	if len(attemptRepo.created) != 1 || attemptRepo.created[0].ProviderID != 51 {
		t.Fatalf("attempts = %#v, want provider 51", attemptRepo.created)
	}
}

func TestDispatch_ResponsesWebSocketPreservesProtocolFields(t *testing.T) {
	adapter := &recordingWSAdapter{}
	frame := []byte(`{"type":"response.create","model":"gpt-test","stream":true,"store":true,"generate":false,"previous_response_id":"resp_1","stream_options":{"include_obfuscation":true},"client_metadata":{"x":1},"prompt_cache_key":"pc","input":[],"tools":[{"type":"function"}],"tool_choice":"auto","parallel_tool_calls":true,"unknown_field":"keep"}`)
	c, _, _ := newWSDispatchCtx(t, frame, nil, []*router.MatchedRoute{
		wsMatchedRoute(1, 61, adapter, true),
	})
	e := newWSDispatchExecutor(&recordingProxyRequestRepo{}, &recordingAttemptRepo{})

	e.dispatch(c)

	if c.Err != nil {
		t.Fatalf("dispatch error: %v", c.Err)
	}
	for _, path := range []string{
		"type", "stream", "store", "generate", "previous_response_id",
		"stream_options", "client_metadata", "prompt_cache_key",
		"input", "tools", "tool_choice", "parallel_tool_calls", "unknown_field",
	} {
		if !gjson.GetBytes(adapter.lastFrame, path).Exists() {
			t.Fatalf("outbound frame lost field %q: %s", path, adapter.lastFrame)
		}
	}
	if gjson.GetBytes(adapter.lastFrame, "type").String() != "response.create" {
		t.Fatalf("type mutated: %s", adapter.lastFrame)
	}
}

func TestDispatch_ResponsesWebSocketUnavailableWhenNoAdapter(t *testing.T) {
	frame := []byte(`{"type":"response.create","model":"gpt-test","input":[]}`)
	c, _, _ := newWSDispatchCtx(t, frame, nil, []*router.MatchedRoute{})
	e := newWSDispatchExecutor(&recordingProxyRequestRepo{}, &recordingAttemptRepo{})

	e.dispatch(c)

	if c.Err == nil {
		t.Fatal("expected unavailable error")
	}
	proxyErr, ok := asProxyError(c.Err)
	if !ok || proxyErr.Code != "websocket_transport_unavailable" {
		t.Fatalf("error = %#v, want websocket_transport_unavailable", c.Err)
	}
}
