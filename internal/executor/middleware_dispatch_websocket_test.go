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
		Err:                   errors.New("handshake failed before request frame write"),
		SafeToTryNextProvider: true,
	}
}

func postWriteWSErr(flags domain.ResponsesWebSocketAttemptError) *domain.ResponsesWebSocketAttemptError {
	flags.Err = errors.New("attempt failed after downstream commitment")
	if !flags.SafeToTryNextProvider {
		// Keep the flag true so tests assert the secondary safety gates
		// (frame sent / first event / client write), not SafeToTryNext alone.
		flags.SafeToTryNextProvider = true
	}
	return &flags
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

// Failover is allowed only while the attempt is still pre-write / pre-event.
func TestDispatch_ResponsesWebSocketFailsOverBeforeClientWrite(t *testing.T) {
	first := &recordingWSAdapter{errSequence: []error{preWriteWSErr()}}
	second := &recordingWSAdapter{resultModel: "gpt-test"}
	frame := []byte(`{"type":"response.create","model":"gpt-test","input":[]}`)
	c, proxyRepo, attemptRepo := newWSDispatchCtx(t, frame, nil, []*router.MatchedRoute{
		wsMatchedRoute(1, 21, first, true),
		wsMatchedRoute(2, 22, second, true),
	})
	e := newWSDispatchExecutor(proxyRepo, attemptRepo)

	e.dispatch(c)

	if c.Err != nil {
		t.Fatalf("dispatch error: %v", c.Err)
	}
	if first.httpExecuteCalls != 0 || second.httpExecuteCalls != 0 {
		t.Fatalf("HTTP Execute calls first=%d second=%d, want 0/0", first.httpExecuteCalls, second.httpExecuteCalls)
	}
	if first.wsCalls != 1 || second.wsCalls != 1 {
		t.Fatalf("WS calls first=%d second=%d, want 1/1 (pre-write failover)", first.wsCalls, second.wsCalls)
	}
	if len(attemptRepo.created) != 2 {
		t.Fatalf("created attempts = %d, want 2", len(attemptRepo.created))
	}
	if attemptRepo.created[0].ProviderID != 21 || attemptRepo.created[1].ProviderID != 22 {
		t.Fatalf("attempt providers = [%d %d], want [21 22]",
			attemptRepo.created[0].ProviderID, attemptRepo.created[1].ProviderID)
	}
	state, _ := getExecState(c)
	if state.wsExchange.PinnedProviderID != 22 {
		t.Fatalf("PinnedProviderID = %d, want second provider 22", state.wsExchange.PinnedProviderID)
	}
	last := proxyRepo.updated[len(proxyRepo.updated)-1]
	if last.Status != "COMPLETED" || last.ProviderID != 22 {
		t.Fatalf("final proxy = status=%s provider=%d, want COMPLETED/22", last.Status, last.ProviderID)
	}
}

func TestDispatch_ResponsesWebSocketRecordsCooldownBeforeFailover(t *testing.T) {
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
		Err:                   proxyErr,
		SafeToTryNextProvider: true,
	}}}
	second := &recordingWSAdapter{resultModel: "gpt-test"}
	frame := []byte(`{"type":"response.create","model":"gpt-test","input":[]}`)
	c, proxyRepo, attemptRepo := newWSDispatchCtx(t, frame, nil, []*router.MatchedRoute{
		wsMatchedRoute(1, firstProviderID, first, true),
		wsMatchedRoute(2, firstProviderID+1, second, true),
	})
	e := newWSDispatchExecutor(proxyRepo, attemptRepo)

	e.dispatch(c)

	if c.Err != nil {
		t.Fatalf("dispatch error: %v", c.Err)
	}
	if !cooldown.Default().IsInCooldown(firstProviderID, string(domain.ClientTypeCodex), "") {
		t.Fatal("pre-write provider failure did not enter cooldown before failover")
	}
}

// After a downstream frame/client write (or first event), failover is forbidden.
func TestDispatch_ResponsesWebSocketNoFailoverAfterClientWrite(t *testing.T) {
	cases := []struct {
		name string
		err  *domain.ResponsesWebSocketAttemptError
	}{
		{
			name: "client_event_sent",
			err:  postWriteWSErr(domain.ResponsesWebSocketAttemptError{ClientEventSent: true}),
		},
		{
			name: "first_event_received",
			err:  postWriteWSErr(domain.ResponsesWebSocketAttemptError{FirstEventReceived: true}),
		},
		{
			name: "request_frame_may_have_been_sent",
			err:  postWriteWSErr(domain.ResponsesWebSocketAttemptError{RequestFrameMayHaveBeenSent: true}),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			first := &recordingWSAdapter{errSequence: []error{tc.err}}
			second := &recordingWSAdapter{resultModel: "gpt-test"}
			frame := []byte(`{"type":"response.create","model":"gpt-test","input":[]}`)
			c, proxyRepo, attemptRepo := newWSDispatchCtx(t, frame, nil, []*router.MatchedRoute{
				wsMatchedRoute(1, 31, first, true),
				wsMatchedRoute(2, 32, second, true),
			})
			e := newWSDispatchExecutor(proxyRepo, attemptRepo)

			e.dispatch(c)

			if c.Err == nil {
				t.Fatal("expected dispatch error without failover")
			}
			if !errors.Is(c.Err, tc.err) {
				t.Fatalf("c.Err = %v, want %v", c.Err, tc.err)
			}
			if first.wsCalls != 1 {
				t.Fatalf("first WS calls = %d, want 1", first.wsCalls)
			}
			if second.wsCalls != 0 {
				t.Fatalf("second WS calls = %d, want 0 (no failover after client/write commitment)", second.wsCalls)
			}
			if first.httpExecuteCalls != 0 || second.httpExecuteCalls != 0 {
				t.Fatalf("HTTP Execute must stay unused; first=%d second=%d", first.httpExecuteCalls, second.httpExecuteCalls)
			}
			if len(attemptRepo.created) != 1 {
				t.Fatalf("created attempts = %d, want 1", len(attemptRepo.created))
			}
			_ = proxyRepo
		})
	}
}

// A pinned session must not fail over even when the error is still "safe".
func TestDispatch_ResponsesWebSocketNoFailoverWhenPinned(t *testing.T) {
	first := &recordingWSAdapter{errSequence: []error{preWriteWSErr()}}
	second := &recordingWSAdapter{resultModel: "gpt-test"}
	frame := []byte(`{"type":"response.create","model":"gpt-test","input":[]}`)
	exchange := &domain.ResponsesWebSocketExchange{
		ConnectionID:     "ws-conn-pinned",
		Frame:            frame,
		PinnedProviderID: 41,
	}
	c, _, attemptRepo := newWSDispatchCtx(t, frame, exchange, []*router.MatchedRoute{
		wsMatchedRoute(1, 41, first, true),
		wsMatchedRoute(2, 42, second, true),
	})
	e := newWSDispatchExecutor(&recordingProxyRequestRepo{}, attemptRepo)

	e.dispatch(c)

	if c.Err == nil {
		t.Fatal("expected error when pinned provider fails")
	}
	if first.wsCalls != 1 || second.wsCalls != 0 {
		t.Fatalf("WS calls first=%d second=%d, want 1/0 with pin", first.wsCalls, second.wsCalls)
	}
	if len(attemptRepo.created) != 1 || attemptRepo.created[0].ProviderID != 41 {
		t.Fatalf("attempts = %#v, want single attempt on pinned provider 41", attemptRepo.created)
	}
}

// Non-native routes are skipped even if the adapter implements the WS interface.
func TestDispatch_ResponsesWebSocketSkipsNonNativeRoutes(t *testing.T) {
	nonNative := &recordingWSAdapter{resultModel: "should-not-run"}
	native := &recordingWSAdapter{resultModel: "gpt-test"}
	frame := []byte(`{"type":"response.create","model":"gpt-test","input":[]}`)
	c, _, attemptRepo := newWSDispatchCtx(t, frame, nil, []*router.MatchedRoute{
		wsMatchedRoute(1, 51, nonNative, false),
		wsMatchedRoute(2, 52, native, true),
	})
	e := newWSDispatchExecutor(&recordingProxyRequestRepo{}, attemptRepo)

	e.dispatch(c)

	if c.Err != nil {
		t.Fatalf("dispatch error: %v", c.Err)
	}
	if nonNative.wsCalls != 0 {
		t.Fatalf("non-native WS calls = %d, want 0", nonNative.wsCalls)
	}
	if native.wsCalls != 1 {
		t.Fatalf("native WS calls = %d, want 1", native.wsCalls)
	}
	if len(attemptRepo.created) != 1 || attemptRepo.created[0].ProviderID != 52 {
		t.Fatalf("attempts = %#v, want provider 52 only", attemptRepo.created)
	}
}
