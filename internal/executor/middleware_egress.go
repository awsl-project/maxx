package executor

import (
	"time"

	"github.com/awsl-project/maxx/internal/flow"
)

func (e *Executor) egress(c *flow.Ctx) {
	state, ok := getExecState(c)
	if !ok {
		c.Next()
		return
	}

	c.Next()

	proxyReq := state.proxyReq
	if proxyReq != nil && proxyReq.Status == "IN_PROGRESS" {
		proxyReq.EndTime = time.Now()
		proxyReq.Duration = proxyReq.EndTime.Sub(proxyReq.StartTime)
		proxyReq.Status, proxyReq.Error = requestFailureStatusAndError(state.ctx, state.lastErr)
		_ = e.proxyRequestRepo.Update(proxyReq)
		if e.broadcaster != nil {
			e.broadcaster.BroadcastProxyRequest(proxyReq)
		}
	}

	if state.currentAttempt != nil && state.currentAttempt.Status == "IN_PROGRESS" {
		state.currentAttempt.EndTime = time.Now()
		state.currentAttempt.Duration = state.currentAttempt.EndTime.Sub(state.currentAttempt.StartTime)
		state.currentAttempt.Status = attemptFailureStatus(state.ctx, state.lastErr)
		_ = e.attemptRepo.Update(state.currentAttempt)
		if e.broadcaster != nil {
			e.broadcaster.BroadcastProxyUpstreamAttempt(state.currentAttempt)
		}
	}

	_ = state.lastErr
}
