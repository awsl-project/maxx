package executor

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/converter"
	"github.com/awsl-project/maxx/internal/cooldown"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/executor/responsemodifier"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/pricing"
	"github.com/awsl-project/maxx/internal/sticky"
)

func (e *Executor) dispatch(c *flow.Ctx) {
	state, ok := getExecState(c)
	if !ok {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrInvalidInput, false, "executor state missing")
		proxyErr.Scope = domain.ScopeRequest
		c.Err = proxyErr
		c.Abort()
		return
	}

	proxyReq := state.proxyReq
	ctx := state.ctx
	clearDetail := e.shouldClearRequestDetailFor(state)

	// Pre-warm tokens for all matched routes in parallel.
	// This avoids serial token refresh delays when failing over between providers.
	if len(state.routes) > 1 {
		type tokenWarmer interface {
			WarmToken(ctx context.Context) error
		}
		var wg sync.WaitGroup
		for _, mr := range state.routes {
			if warmer, ok := mr.ProviderAdapter.(tokenWarmer); ok {
				wg.Add(1)
				go func(w tokenWarmer) {
					defer wg.Done()
					_ = w.WarmToken(ctx)
				}(warmer)
			}
		}
		wg.Wait()
	}

	for _, matchedRoute := range state.routes {
		if ctx.Err() != nil {
			state.lastErr = ctx.Err()
			c.Err = state.lastErr
			return
		}

		proxyReq.RouteID = matchedRoute.Route.ID
		proxyReq.ProviderID = matchedRoute.Provider.ID
		_ = e.proxyRequestRepo.Update(proxyReq)
		if e.broadcaster != nil {
			e.broadcaster.BroadcastProxyRequest(proxyReq)
		}

		clientType := state.clientType
		mappedModel := e.mapModel(state.tenantID, state.requestModel, matchedRoute.Route, matchedRoute.Provider, clientType, state.projectID, state.apiTokenID)

		originalClientType := clientType
		currentClientType := clientType
		needsConversion := false
		convertedBody := []byte(nil)
		var convErr error
		requestBody := state.requestBody
		requestURI := state.requestURI

		supportedTypes := matchedRoute.ProviderAdapter.SupportedClientTypes()
		if e.converter.NeedConvert(clientType, supportedTypes) {
			currentClientType = GetPreferredTargetType(supportedTypes, clientType, matchedRoute.Provider.Type)
			if currentClientType != clientType {
				needsConversion = true
				log.Printf("[Executor] Format conversion needed: %s -> %s for provider %s",
					clientType, currentClientType, matchedRoute.Provider.Name)

				if currentClientType == domain.ClientTypeCodex {
					if headers := state.requestHeaders; headers != nil {
						requestBody = converter.InjectCodexUserAgent(requestBody, headers.Get("User-Agent"))
					}
				}
				convertedBody, convErr = e.converter.TransformRequest(
					clientType, currentClientType, requestBody, mappedModel, state.isStream)
				if convErr != nil {
					log.Printf("[Executor] Request conversion failed: %v, proceeding with original format", convErr)
					needsConversion = false
					currentClientType = clientType
				} else {
					requestBody = convertedBody

					originalURI := requestURI
					convertedURI := ConvertRequestURI(requestURI, clientType, currentClientType, mappedModel, state.isStream)
					if convertedURI != originalURI {
						requestURI = convertedURI
						log.Printf("[Executor] URI converted: %s -> %s", originalURI, convertedURI)
					}
				}
			}
		}

		retryConfig := e.getRetryConfig(state.tenantID, matchedRoute.RetryConfig)

		for attempt := 0; attempt <= retryConfig.MaxRetries; attempt++ {
			if ctx.Err() != nil {
				state.lastErr = ctx.Err()
				c.Err = state.lastErr
				return
			}

			attemptStartTime := time.Now()
			attemptRecord := &domain.ProxyUpstreamAttempt{
				TenantID:       proxyReq.TenantID,
				ProxyRequestID: proxyReq.ID,
				RouteID:        matchedRoute.Route.ID,
				ProviderID:     matchedRoute.Provider.ID,
				IsStream:       state.isStream,
				Status:         "IN_PROGRESS",
				StartTime:      attemptStartTime,
				RequestModel:   state.requestModel,
				MappedModel:    mappedModel,
				RequestInfo:    proxyReq.RequestInfo,
			}
			if attemptRecord.TenantID == 0 {
				attemptRecord.TenantID = state.tenantID
			}
			if err := e.attemptRepo.Create(attemptRecord); err != nil {
				log.Printf("[Executor] Failed to create attempt record: %v", err)
			}
			state.currentAttempt = attemptRecord

			proxyReq.ProxyUpstreamAttemptCount++
			if e.broadcaster != nil {
				e.broadcaster.BroadcastProxyRequest(proxyReq)
				e.broadcaster.BroadcastProxyUpstreamAttempt(attemptRecord)
			}

			eventChan := domain.NewAdapterEventChan()
			c.Set(flow.KeyClientType, currentClientType)
			c.Set(flow.KeyOriginalClientType, originalClientType)
			c.Set(flow.KeyMappedModel, mappedModel)
			c.Set(flow.KeyRequestBody, requestBody)
			c.Set(flow.KeyRequestURI, requestURI)
			c.Set(flow.KeyRequestHeaders, state.requestHeaders)
			c.Set(flow.KeyProxyRequest, state.proxyReq)
			c.Set(flow.KeyUpstreamAttempt, attemptRecord)
			c.Set(flow.KeyEventChan, eventChan)
			c.Set(flow.KeyBroadcaster, e.broadcaster)
			eventDone := make(chan struct{})
			go e.processAdapterEventsRealtime(eventChan, attemptRecord, eventDone, clearDetail)

			var responseWriter http.ResponseWriter
			var convertingWriter *ConvertingResponseWriter
			modifierWriter := responsemodifier.NewResponseModifierWriter(c.Writer, matchedRoute.Provider, originalClientType, state.isStream)
			captureWriter := http.ResponseWriter(c.Writer)
			if modifierWriter != nil {
				captureWriter = modifierWriter
			}
			// Keep capture before modifier so stored response details remain upstream-visible,
			// while only the client-facing writer receives response modifications.
			responseCapture := NewResponseCapture(captureWriter)
			if needsConversion {
				convertingWriter = NewConvertingResponseWriter(
					responseCapture, e.converter, originalClientType, currentClientType, state.isStream, state.originalRequestBody)
				responseWriter = convertingWriter
			} else {
				responseWriter = responseCapture
			}

			originalWriter := c.Writer
			c.Writer = responseWriter
			err := matchedRoute.ProviderAdapter.Execute(c, matchedRoute.Provider)
			c.Writer = originalWriter

			if needsConversion && convertingWriter != nil && !state.isStream {
				if finalizeErr := convertingWriter.Finalize(); finalizeErr != nil {
					log.Printf("[Executor] Response conversion finalize failed: %v", finalizeErr)
				}
			}
			if err == nil && modifierWriter != nil {
				if finalizeErr := modifierWriter.Finalize(); finalizeErr != nil {
					log.Printf("[Executor] Response modifier finalize failed: %v", finalizeErr)
				}
			}

			eventChan.Close()
			<-eventDone

			multiplier := getProviderMultiplier(matchedRoute.Provider, clientType)

			if err == nil {
				attemptRecord.EndTime = time.Now()
				attemptRecord.Duration = attemptRecord.EndTime.Sub(attemptRecord.StartTime)
				attemptRecord.Status = "COMPLETED"

				pricing.FinalizeAttemptCost(attemptRecord, multiplier)

				if clearDetail {
					attemptRecord.RequestInfo = nil
					attemptRecord.ResponseInfo = nil
				}

				_ = e.attemptRepo.Update(attemptRecord)
				if e.broadcaster != nil {
					e.broadcaster.BroadcastProxyUpstreamAttempt(attemptRecord)
				}
				state.currentAttempt = nil

				cooldown.Default().RecordSuccess(matchedRoute.Provider.ID, string(currentClientType), mappedModel)

				// Sticky write-back: bind this session to the provider that
				// just succeeded. Overwrites any previous binding (e.g. when
				// we failed over from A → B, sticky now points at B for the
				// next request). Errors are non-fatal — affinity is best-effort,
				// the next call would just re-roll via weighted_random.
				//
				// Use a fresh background context with a tight timeout: by the
				// time we get here the request ctx may already be Done (for
				// streaming responses the client has disconnected just before
				// this hook fires), which would turn every Set into a silent
				// failure under load. 500ms is a deliberate budget — the
				// write is on the response tail-latency path, so a slow
				// Redis must not stall the request; affinity is best-effort
				// and the next request will re-roll if the write timed out.
				if state.stickyWrite != nil {
					stickyCtx, stickyCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
					if err := sticky.Default().Set(stickyCtx, state.stickyWrite.Key, matchedRoute.Provider.ID, state.stickyWrite.TTL); err != nil {
						log.Printf("[Executor] sticky set failed (non-fatal): %v", err)
					}
					stickyCancel()
				}

				proxyReq.Status = "COMPLETED"
				proxyReq.EndTime = time.Now()
				proxyReq.Duration = proxyReq.EndTime.Sub(proxyReq.StartTime)
				proxyReq.FinalProxyUpstreamAttemptID = attemptRecord.ID
				proxyReq.ResponseModel = mappedModel

				if !clearDetail {
					proxyReq.ResponseInfo = &domain.ResponseInfo{
						Status:  responseCapture.StatusCode(),
						Headers: responseCapture.CapturedHeaders(),
						Body:    responseCapture.Body(),
					}
				}
				proxyReq.StatusCode = responseCapture.StatusCode()

				pricing.MirrorCostToRequest(proxyReq, attemptRecord)
				proxyReq.TTFT = attemptRecord.TTFT

				clearProxyRequestDetail(proxyReq, clearDetail)

				_ = e.proxyRequestRepo.Update(proxyReq)
				if e.broadcaster != nil {
					e.broadcaster.BroadcastProxyRequest(proxyReq)
				}

				state.lastErr = nil
				state.ctx = ctx
				return
			}

			attemptRecord.EndTime = time.Now()
			attemptRecord.Duration = attemptRecord.EndTime.Sub(attemptRecord.StartTime)
			state.lastErr = err

			if ctx.Err() != nil {
				attemptRecord.Status = "CANCELLED"
			} else {
				attemptRecord.Status = "FAILED"
			}

			pricing.FinalizeAttemptCost(attemptRecord, multiplier)

			if clearDetail {
				attemptRecord.RequestInfo = nil
				attemptRecord.ResponseInfo = nil
			}

			_ = e.attemptRepo.Update(attemptRecord)
			if e.broadcaster != nil {
				e.broadcaster.BroadcastProxyUpstreamAttempt(attemptRecord)
			}
			state.currentAttempt = nil

			proxyReq.FinalProxyUpstreamAttemptID = attemptRecord.ID

			if responseCapture.Body() != "" {
				proxyReq.StatusCode = responseCapture.StatusCode()
				if !clearDetail {
					proxyReq.ResponseInfo = &domain.ResponseInfo{
						Status:  responseCapture.StatusCode(),
						Headers: responseCapture.CapturedHeaders(),
						Body:    responseCapture.Body(),
					}
				}
			}
			pricing.MirrorCostToRequest(proxyReq, attemptRecord)
			proxyReq.TTFT = attemptRecord.TTFT

			clearProxyRequestDetail(proxyReq, clearDetail)

			_ = e.proxyRequestRepo.Update(proxyReq)
			if e.broadcaster != nil {
				e.broadcaster.BroadcastProxyRequest(proxyReq)
			}

			proxyErr, ok := err.(*domain.ProxyError)
			if responseCapture.WroteToClient() {
				log.Printf("[Executor] Response already committed; not failing over after provider %d error: %v", matchedRoute.Provider.ID, err)
				proxyReq.Status = "FAILED"
				proxyReq.EndTime = time.Now()
				proxyReq.Duration = proxyReq.EndTime.Sub(proxyReq.StartTime)
				proxyReq.Error = err.Error()
				proxyReq.StatusCode = responseCapture.StatusCode()
				if !clearDetail {
					proxyReq.ResponseInfo = &domain.ResponseInfo{
						Status:  responseCapture.StatusCode(),
						Headers: responseCapture.CapturedHeaders(),
						Body:    responseCapture.Body(),
					}
				}
				clearProxyRequestDetail(proxyReq, clearDetail)
				_ = e.proxyRequestRepo.Update(proxyReq)
				if e.broadcaster != nil {
					e.broadcaster.BroadcastProxyRequest(proxyReq)
				}
				state.lastErr = err
				c.Err = err
				return
			}
			if ok && ctx.Err() != nil {
				proxyReq.Status = "CANCELLED"
				proxyReq.EndTime = time.Now()
				proxyReq.Duration = proxyReq.EndTime.Sub(proxyReq.StartTime)
				if ctx.Err() == context.Canceled {
					proxyReq.Error = "client disconnected"
				} else if ctx.Err() == context.DeadlineExceeded {
					proxyReq.Error = "request timeout"
				} else {
					proxyReq.Error = ctx.Err().Error()
				}
				clearProxyRequestDetail(proxyReq, clearDetail)
				_ = e.proxyRequestRepo.Update(proxyReq)
				if e.broadcaster != nil {
					e.broadcaster.BroadcastProxyRequest(proxyReq)
				}
				state.lastErr = ctx.Err()
				c.Err = state.lastErr
				return
			}

			if ok && proxyErr.Scope == domain.ScopeRequest && !proxyErr.Retryable {
				log.Printf("[Executor] Request-scoped non-retryable error; not failing over after provider %d: %v", matchedRoute.Provider.ID, err)
				proxyReq.Status = "FAILED"
				proxyReq.EndTime = time.Now()
				proxyReq.Duration = proxyReq.EndTime.Sub(proxyReq.StartTime)
				proxyReq.Error = err.Error()
				if proxyErr.HTTPStatusCode >= 400 && proxyErr.HTTPStatusCode < 600 {
					proxyReq.StatusCode = proxyErr.HTTPStatusCode
				}
				clearProxyRequestDetail(proxyReq, clearDetail)
				_ = e.proxyRequestRepo.Update(proxyReq)
				if e.broadcaster != nil {
					e.broadcaster.BroadcastProxyRequest(proxyReq)
				}
				state.lastErr = err
				c.Err = err
				return
			}

			if ok && ctx.Err() != context.Canceled {
				log.Printf("[Executor] ProxyError - Scope: %s, Reason: %s, Retryable: %v, Provider: %d",
					proxyErr.Scope, proxyErr.Reason, proxyErr.Retryable, matchedRoute.Provider.ID)
				if !shouldSkipErrorCooldown(matchedRoute.Provider) {
					e.handleCooldown(proxyErr, matchedRoute.Provider, currentClientType, mappedModel)
					if e.broadcaster != nil {
						e.broadcaster.BroadcastMessage("cooldown_update", map[string]interface{}{
							"providerID": matchedRoute.Provider.ID,
						})
					}
				}
			} else if ok && ctx.Err() == context.Canceled {
				log.Printf("[Executor] Client disconnected, skipping cooldown for Provider: %d", matchedRoute.Provider.ID)
			} else if !ok {
				log.Printf("[Executor] Error is not ProxyError, type: %T, error: %v", err, err)
			}

			if !ok || !proxyErr.Retryable {
				break
			}

			if attempt < retryConfig.MaxRetries {
				waitTime := e.calculateBackoff(retryConfig, attempt)
				if proxyErr.RetryAfter > 0 {
					waitTime = proxyErr.RetryAfter
				}
				select {
				case <-ctx.Done():
					proxyReq.Status = "CANCELLED"
					proxyReq.EndTime = time.Now()
					proxyReq.Duration = proxyReq.EndTime.Sub(proxyReq.StartTime)
					if ctx.Err() == context.Canceled {
						proxyReq.Error = "client disconnected during retry wait"
					} else if ctx.Err() == context.DeadlineExceeded {
						proxyReq.Error = "request timeout during retry wait"
					} else {
						proxyReq.Error = ctx.Err().Error()
					}
					clearProxyRequestDetail(proxyReq, clearDetail)
					_ = e.proxyRequestRepo.Update(proxyReq)
					if e.broadcaster != nil {
						e.broadcaster.BroadcastProxyRequest(proxyReq)
					}
					state.lastErr = ctx.Err()
					c.Err = state.lastErr
					return
				case <-time.After(waitTime):
				}
			}
		}
	}

	proxyReq.Status = "FAILED"
	proxyReq.EndTime = time.Now()
	proxyReq.Duration = proxyReq.EndTime.Sub(proxyReq.StartTime)
	if state.lastErr != nil {
		proxyReq.Error = state.lastErr.Error()
		if proxyErr, ok := state.lastErr.(*domain.ProxyError); ok && proxyErr.HTTPStatusCode >= 400 && proxyErr.HTTPStatusCode < 600 {
			proxyReq.StatusCode = proxyErr.HTTPStatusCode
		}
	}
	clearProxyRequestDetail(proxyReq, clearDetail)
	_ = e.proxyRequestRepo.Update(proxyReq)
	if e.broadcaster != nil {
		e.broadcaster.BroadcastProxyRequest(proxyReq)
	}

	if state.lastErr == nil {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrAllRoutesFailed, false, "all routes exhausted")
		proxyErr.Scope = domain.ScopeRequest
		state.lastErr = proxyErr
	}
	state.ctx = ctx
	c.Err = state.lastErr
}

func clearProxyRequestDetail(req *domain.ProxyRequest, clearDetail bool) {
	if !clearDetail || req == nil {
		return
	}
	req.RequestInfo = nil
	req.ResponseInfo = nil
}
