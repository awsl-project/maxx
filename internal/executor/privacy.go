package executor

import (
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/privacy"
)

// enforceRequestPrivacy runs once per logical request (including each WebSocket
// turn), before creating an upstream attempt or entering the retry loop.
func (e *Executor) enforceRequestPrivacy(c *flow.Ctx, state *execState) bool {
	exchange, rejection := e.privacy.Prepare(state.ctx, state.tenantID, state.requestBody, state.originalRequestBody, state.requestHeaders.Get("Content-Type"))
	state.privacyExchange = exchange
	state.privacyEnabled = exchange != nil || rejection != nil
	e.applyPrivacyExchange(c, state)
	return e.rejectPrivacy(c, state, rejection)
}

func (e *Executor) applyPrivacyExchange(c *flow.Ctx, state *execState) {
	exchange := state.privacyExchange
	if exchange != nil {
		state.requestBody = exchange.Body
		state.originalRequestBody = exchange.OriginalBody
		c.Set(flow.KeyRequestBody, state.requestBody)
		c.Set(flow.KeyOriginalRequestBody, state.originalRequestBody)
	}
	if state.proxyReq != nil {
		state.proxyReq.PrivacyEnabled = state.privacyEnabled
		if exchange != nil {
			state.proxyReq.PrivacyFilter = exchange.Archive
		}
	}
}
func (e *Executor) resolvePrivacyPlan(c *flow.Ctx, state *execState) bool {
	ids := make([]uint64, 0, len(state.routes))
	for _, route := range state.routes {
		if route != nil && route.Provider != nil {
			ids = append(ids, route.Provider.ID)
		}
	}
	rejection := state.privacyExchange.ResolvePlan(ids)
	e.applyPrivacyExchange(c, state)
	return e.rejectPrivacy(c, state, rejection)
}
func (e *Executor) rejectPrivacy(c *flow.Ctx, state *execState, rejection *privacy.Rejection) bool {
	if rejection == nil {
		return true
	}
	proxyErr := domain.NewProxyErrorWithMessage(domain.ErrInvalidInput, false, rejection.Message)
	proxyErr.Scope = domain.ScopeRequest
	proxyErr.Code = rejection.Code
	proxyErr.HTTPStatusCode = rejection.HTTPStatusCode
	state.lastErr, c.Err = proxyErr, proxyErr
	req := state.proxyReq
	isNew := req == nil || req.ID == 0
	if req == nil {
		req = e.newProxyRequest(c, state, "REJECTED")
		state.proxyReq = req
	}
	req.Status, req.StatusCode, req.Error = "REJECTED", rejection.HTTPStatusCode, proxyErr.Error()
	req.EndTime = time.Now()
	req.Duration = req.EndTime.Sub(req.StartTime)
	if isNew {
		e.createProxyRequest(req)
	} else {
		_ = e.proxyRequestRepo.Update(req)
		if e.broadcaster != nil {
			e.broadcaster.BroadcastProxyRequest(req)
		}
	}
	state.privacyExchange.BeginRecord(req)
	c.Abort()
	return false
}
