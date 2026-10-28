package executor

import (
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
)

// enforceRequestPrivacy runs once per logical request (including each WebSocket
// turn), before creating an upstream attempt or entering the retry loop.
func (e *Executor) enforceRequestPrivacy(c *flow.Ctx, state *execState) bool {
	body := state.originalRequestBody
	if len(body) == 0 {
		body = state.requestBody
	}
	enabled, rejection := e.privacy.Check(state.tenantID, state.projectID, body, state.requestHeaders.Get("Content-Type"))
	state.privacyEnabled = enabled
	if state.proxyReq != nil {
		state.proxyReq.PrivacyEnabled = enabled
	}
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
	c.Abort()
	return false
}
