package executor

import (
	"log"
	"time"

	provideradapter "github.com/awsl-project/maxx/internal/adapter/provider"
	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/pricing"
)

// ExecuteOnce wraps a single adapter.Execute call with the per-attempt
// bookkeeping the retry-driven dispatch middleware performs each iteration:
// attempt-row creation, adapter-event collection, and cost finalization.
//
// It is intended for paths that bypass route selection / retry (currently the
// /provider/<id>/... direct-dispatch handler) but still need a fully recorded
// + billed attempt. Without this, the bypass path persisted a proxy request
// with proxyUpstreamAttemptCount=0 / cost=0 / multiplier=0 and no attempt
// rows at all — silently zero-billing every request.
//
// Caller contract:
//   - c.Writer must already be configured (capture / modifier / conversion
//     wrappers) before this is called; ExecuteOnce does not touch the writer.
//   - The caller owns the ProxyRequest's end-of-life fields beyond what's
//     mirrored from the attempt (Status, EndTime, Duration, ResponseInfo,
//     StatusCode) and persists it via its repository.
//
// ExecuteOnce takes care of:
//   - Inserting a ProxyUpstreamAttempt row before adapter.Execute.
//   - Setting flow.KeyUpstreamAttempt + flow.KeyEventChan so the adapter can
//     emit token / first-token / response-info events.
//   - Spawning processAdapterEventsRealtime to drain those events into the
//     attempt and broadcast progress.
//   - Calling pricing.FinalizeAttemptCost (success or failure) and updating
//     the attempt row.
//   - Bumping proxyReq.ProxyUpstreamAttemptCount and, on success, setting
//     FinalProxyUpstreamAttemptID + mirroring cost via pricing.MirrorCostToRequest
//     and copying TTFT.
//
// Returns the persisted attempt and the adapter's error (nil on success).
func (e *Executor) ExecuteOnce(
	c *flow.Ctx,
	proxyReq *domain.ProxyRequest,
	route *domain.Route,
	provider *domain.Provider,
	adapter provideradapter.ProviderAdapter,
	clientType domain.ClientType,
	requestModel, mappedModel string,
	isStream bool,
	clearDetail bool,
) (*domain.ProxyUpstreamAttempt, error) {
	attempt := &domain.ProxyUpstreamAttempt{
		TenantID:       proxyReq.TenantID,
		ProxyRequestID: proxyReq.ID,
		RouteID:        route.ID,
		ProviderID:     provider.ID,
		IsStream:       isStream,
		Status:         "IN_PROGRESS",
		StartTime:      time.Now(),
		RequestModel:   requestModel,
		MappedModel:    mappedModel,
		RequestInfo:    proxyReq.RequestInfo,
	}
	if attempt.TenantID == 0 {
		attempt.TenantID = maxxctx.GetTenantID(c.Request.Context())
	}
	if err := e.attemptRepo.Create(attempt); err != nil {
		log.Printf("[Executor] ExecuteOnce: failed to create attempt record: %v", err)
	}

	proxyReq.ProxyUpstreamAttemptCount++
	if e.broadcaster != nil {
		e.broadcaster.BroadcastProxyRequest(proxyReq)
		e.broadcaster.BroadcastProxyUpstreamAttempt(attempt)
	}

	eventChan := domain.NewAdapterEventChan()
	c.Set(flow.KeyUpstreamAttempt, attempt)
	c.Set(flow.KeyEventChan, eventChan)
	c.Set(flow.KeyBroadcaster, e.broadcaster)

	eventDone := make(chan struct{})
	go e.processAdapterEventsRealtime(eventChan, attempt, eventDone, clearDetail)

	err := adapter.Execute(c, provider)

	// Closing the channel signals the processor to drain remaining events
	// and exit; waiting on eventDone guarantees all metric writes have
	// landed on the attempt before we finalize cost.
	eventChan.Close()
	<-eventDone

	attempt.EndTime = time.Now()
	attempt.Duration = attempt.EndTime.Sub(attempt.StartTime)
	switch {
	case err == nil:
		attempt.Status = "COMPLETED"
	case c.Request.Context().Err() != nil:
		attempt.Status = "CANCELLED"
	default:
		attempt.Status = "FAILED"
	}

	multiplier := getProviderMultiplier(provider, clientType)
	pricing.FinalizeAttemptCost(attempt, multiplier)

	if clearDetail {
		attempt.RequestInfo = nil
		attempt.ResponseInfo = nil
	}
	_ = e.attemptRepo.Update(attempt)
	if e.broadcaster != nil {
		e.broadcaster.BroadcastProxyUpstreamAttempt(attempt)
	}

	if err == nil {
		proxyReq.FinalProxyUpstreamAttemptID = attempt.ID
		pricing.MirrorCostToRequest(proxyReq, attempt)
		proxyReq.TTFT = attempt.TTFT
	}

	return attempt, err
}
