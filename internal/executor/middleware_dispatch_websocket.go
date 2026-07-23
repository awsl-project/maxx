package executor

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"time"

	provideradapter "github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/cooldown"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/pricing"
	"github.com/awsl-project/maxx/internal/sticky"
	"github.com/tidwall/sjson"
)

func (e *Executor) dispatchResponsesWebSocket(c *flow.Ctx) {
	state, ok := getExecState(c)
	if !ok || state.wsExchange == nil || state.proxyReq == nil {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrInvalidState, false, "responses websocket dispatch state missing")
		proxyErr.Scope = domain.ScopeRequest
		if state != nil {
			state.lastErr = proxyErr
		}
		c.Err = proxyErr
		return
	}

	exchange := state.wsExchange
	proxyReq := state.proxyReq
	ctx := state.ctx
	clearDetail := e.shouldClearRequestDetailFor(state)

	if len(state.routes) == 0 {
		proxyErr := domain.NewProxyErrorWithMessage(
			domain.ErrNoResponsesWebSocketProviders,
			true,
			"no eligible native Codex Responses WebSocket adapter is available",
		)
		proxyErr.Scope = domain.ScopeRequest
		proxyErr.Code = "websocket_transport_unavailable"
		proxyErr.HTTPStatusCode = http.StatusServiceUnavailable
		state.lastErr = proxyErr
		c.Err = proxyErr
		return
	}

	// Execute the first ordered WebSocket candidate. A later candidate is tried
	// only when this unpinned first turn loses the provider-slot race before dial.
	matched := state.routes[0]
	if matched == nil || matched.Route == nil || matched.Provider == nil || matched.ProviderAdapter == nil {
		proxyErr := domain.NewProxyErrorWithMessage(
			domain.ErrNoResponsesWebSocketProviders,
			true,
			"no eligible native Codex Responses WebSocket adapter is available",
		)
		proxyErr.Scope = domain.ScopeRequest
		proxyErr.Code = "websocket_transport_unavailable"
		proxyErr.HTTPStatusCode = http.StatusServiceUnavailable
		state.lastErr = proxyErr
		c.Err = proxyErr
		return
	}

	wsAdapter, ok := matched.ProviderAdapter.(provideradapter.ResponsesWebSocketAdapter)
	if !ok {
		proxyErr := domain.NewProxyErrorWithMessage(
			domain.ErrNoResponsesWebSocketProviders,
			true,
			"no eligible native Codex Responses WebSocket adapter is available",
		)
		proxyErr.Scope = domain.ScopeRequest
		proxyErr.Code = "websocket_transport_unavailable"
		proxyErr.HTTPStatusCode = http.StatusServiceUnavailable
		state.lastErr = proxyErr
		c.Err = proxyErr
		return
	}

	if ctx.Err() != nil {
		state.lastErr = ctx.Err()
		c.Err = state.lastErr
		return
	}

	mappedModel := e.mapModel(
		state.tenantID,
		state.requestModel,
		matched.Route,
		matched.Provider,
		domain.ClientTypeCodex,
		state.projectID,
		state.apiTokenID,
	)
	outboundFrame, err := sjson.SetBytes(bytes.Clone(exchange.Frame), "model", mappedModel)
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrResponsesWebSocketProtocol, false, "failed to map websocket request model")
		proxyErr.Scope = domain.ScopeRequest
		proxyErr.Code = "websocket_protocol_error"
		proxyErr.HTTPStatusCode = http.StatusBadRequest
		state.lastErr = proxyErr
		c.Err = proxyErr
		return
	}
	outboundFrame = e.applyOutboundParamPolicy(outboundFrame, domain.ClientTypeCodex, mappedModel, matched.Provider)
	turnExchange := *exchange
	turnExchange.Frame = outboundFrame
	if e.router != nil {
		turnExchange.TryAcquireProviderSlot = func() (func(), bool) {
			return e.router.TryAcquireProvider(matched.Provider)
		}
	}

	proxyReq.RouteID = matched.Route.ID
	proxyReq.ProviderID = matched.Provider.ID
	proxyReq.MappedModel = mappedModel
	_ = e.proxyRequestRepo.Update(proxyReq)

	attempt := &domain.ProxyUpstreamAttempt{
		TenantID:       proxyReq.TenantID,
		ProxyRequestID: proxyReq.ID,
		RouteID:        matched.Route.ID,
		ProviderID:     matched.Provider.ID,
		IsStream:       true,
		Status:         "IN_PROGRESS",
		StartTime:      time.Now(),
		RequestModel:   state.requestModel,
		MappedModel:    mappedModel,
		RequestInfo:    proxyReq.RequestInfo,
	}
	if attempt.TenantID == 0 {
		attempt.TenantID = state.tenantID
	}
	if err := e.attemptRepo.Create(attempt); err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(err, false, "failed to create websocket upstream attempt")
		proxyErr.Scope = domain.ScopeRequest
		state.lastErr = proxyErr
		c.Err = proxyErr
		return
	}
	if e.broadcaster != nil {
		e.broadcaster.BroadcastProxyUpstreamAttempt(attempt)
	}
	state.currentAttempt = attempt
	proxyReq.ProxyUpstreamAttemptCount++

	eventChan := domain.NewAdapterEventChan()
	c.Set(flow.KeyClientType, domain.ClientTypeCodex)
	c.Set(flow.KeyOriginalClientType, domain.ClientTypeCodex)
	c.Set(flow.KeyMappedModel, mappedModel)
	c.Set(flow.KeyEventChan, eventChan)
	c.Set(flow.KeyProxyRequest, proxyReq)
	c.Set(flow.KeyUpstreamAttempt, attempt)
	c.Set(flow.KeyBroadcaster, e.broadcaster)
	eventDone := make(chan struct{})
	go e.processAdapterEventsRealtime(eventChan, attempt, eventDone, clearDetail)

	result, execErr := wsAdapter.ExecuteResponsesWebSocket(c, matched.Provider, &turnExchange)
	eventChan.Close()
	<-eventDone

	attempt.EndTime = time.Now()
	attempt.Duration = attempt.EndTime.Sub(attempt.StartTime)
	multiplier := getProviderMultiplier(matched.Provider, domain.ClientTypeCodex)

	if execErr == nil {
		attempt.Status = "COMPLETED"
		attempt.Error = ""
		if result != nil && result.ResponseModel != "" {
			attempt.ResponseModel = result.ResponseModel
		}
		pricing.FinalizeAttemptCost(attempt, multiplier)
		if clearDetail {
			attempt.RequestInfo = nil
			attempt.ResponseInfo = nil
		}
		_ = e.attemptRepo.Update(attempt)
		if e.broadcaster != nil {
			e.broadcaster.BroadcastProxyUpstreamAttempt(attempt)
		}
		state.currentAttempt = nil
		cooldown.Default().RecordSuccess(matched.Provider.ID, string(domain.ClientTypeCodex), mappedModel)

		if state.stickyWrite != nil {
			stickyCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			_ = sticky.Default().Set(stickyCtx, state.stickyWrite.Key, matched.Provider.ID, state.stickyWrite.TTL)
			cancel()
		}

		exchange.PinnedProviderID = matched.Provider.ID
		proxyReq.Status = "COMPLETED"
		proxyReq.StatusCode = http.StatusSwitchingProtocols
		proxyReq.EndTime = time.Now()
		proxyReq.Duration = proxyReq.EndTime.Sub(proxyReq.StartTime)
		proxyReq.FinalProxyUpstreamAttemptID = attempt.ID
		proxyReq.ResponseModel = attempt.ResponseModel
		if proxyReq.ResponseModel == "" {
			proxyReq.ResponseModel = mappedModel
		}
		if !clearDetail {
			proxyReq.ResponseInfo = attempt.ResponseInfo
		}
		pricing.MirrorCostToRequest(proxyReq, attempt)
		proxyReq.TTFT = attempt.TTFT
		clearProxyRequestDetail(proxyReq, clearDetail)
		_ = e.proxyRequestRepo.Update(proxyReq)
		if e.broadcaster != nil {
			e.broadcaster.BroadcastProxyRequest(proxyReq)
		}
		state.lastErr = nil
		state.ctx = ctx
		return
	}

	attempt.Status = attemptFailureStatus(ctx, execErr)
	attempt.Error = execErr.Error()
	pricing.FinalizeAttemptCost(attempt, multiplier)
	if clearDetail {
		attempt.RequestInfo = nil
		attempt.ResponseInfo = nil
	}
	_ = e.attemptRepo.Update(attempt)
	if e.broadcaster != nil {
		e.broadcaster.BroadcastProxyUpstreamAttempt(attempt)
	}
	state.currentAttempt = nil
	proxyReq.FinalProxyUpstreamAttemptID = attempt.ID
	pricing.MirrorCostToRequest(proxyReq, attempt)
	proxyReq.TTFT = attempt.TTFT
	_ = e.proxyRequestRepo.Update(proxyReq)

	// Slot acquisition happens before dialing or writing an upstream frame. If
	// it loses the race after routing, an unpinned first turn may safely try the
	// next ordered candidate. All other WebSocket failures remain pinned and do
	// not fall back across providers.
	if errors.Is(execErr, domain.ErrNoAvailableProviders) && exchange.PinnedProviderID == 0 && len(state.routes) > 1 {
		state.routes = state.routes[1:]
		e.dispatchResponsesWebSocket(c)
		return
	}

	var wsErr *domain.ResponsesWebSocketAttemptError
	errors.As(execErr, &wsErr)
	if proxyErr, ok := asProxyError(execErr); ok && (wsErr == nil || !wsErr.CapabilityFailure) &&
		ctx.Err() == nil && !shouldSkipErrorCooldown(matched.Provider) {
		e.handleCooldown(proxyErr, matched.Provider, domain.ClientTypeCodex, mappedModel)
	}
	state.lastErr = execErr
	c.Err = execErr
}
