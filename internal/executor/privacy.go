package executor

import (
	"net/http"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/privacy"
)

// enforceRequestPrivacy runs once per logical request (including each WebSocket
// turn), before creating an upstream attempt or entering the retry loop.
func (e *Executor) enforceRequestPrivacy(c *flow.Ctx, state *execState) bool {
	if state.projectID == 0 {
		return true
	}
	enabled, err := e.router.ProjectBlocksSecrets(state.tenantID, state.projectID)
	message, code, status := "Unable to inspect request privacy", "privacy_check_failed", http.StatusServiceUnavailable
	if err == nil && !enabled {
		return true
	}
	if err == nil {
		body := state.originalRequestBody
		if len(body) == 0 {
			body = state.requestBody
		}
		var kind string
		kind, err = privacy.InspectRequest(body, state.requestHeaders.Get("Content-Type"))
		if err == nil && kind == "" {
			return true
		}
		status = http.StatusBadRequest
		if err == nil {
			message, code = "Request blocked: credential detected in message content ("+kind+")", "privacy_blocked"
		}
	}
	proxyErr := domain.NewProxyErrorWithMessage(domain.ErrInvalidInput, false, message)
	proxyErr.Scope = domain.ScopeRequest
	proxyErr.Code = code
	proxyErr.HTTPStatusCode = status
	state.lastErr, c.Err = proxyErr, proxyErr
	req := state.proxyReq
	isNew := req == nil
	if isNew {
		req = e.newProxyRequest(c, state, "REJECTED")
		state.proxyReq = req
	}
	req.Status, req.StatusCode, req.Error = "REJECTED", status, proxyErr.Error()
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
