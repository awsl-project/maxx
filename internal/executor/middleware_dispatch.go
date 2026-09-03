package executor

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/converter"
	"github.com/awsl-project/maxx/internal/cooldown"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/executor/responsemodifier"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/pricing"
	"github.com/awsl-project/maxx/internal/requestmeta"
	"github.com/awsl-project/maxx/internal/sticky"
)

// maxUpstreamAttemptsPerRequest is a hard safety ceiling on the total number of
// upstream calls a single inbound request may generate across ALL routes,
// providers, model candidates and retries combined. It is a backstop against
// pathological retry/failover amplification — e.g. a provider with
// disableErrorCooldown that (before the retry-classification fix) retried a
// single non-retryable 4xx 151,780 times over 4 hours. Correct classification
// should stop such loops long before this cap; the cap exists so that no
// classification bug or misconfiguration can ever turn one request into tens of
// thousands of upstream calls again. 200 is far above any legitimate
// route×retry fan-out yet orders of magnitude below the observed blowups.
const maxUpstreamAttemptsPerRequest = 200

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
	if state.wsExchange != nil {
		e.dispatchResponsesWebSocket(c)
		return
	}

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

	// totalUpstreamAttempts counts every upstream dispatch this inbound request
	// has made across all routes/providers/candidates/retries. It backs the
	// hard ceiling (maxUpstreamAttemptsPerRequest) that guarantees a single
	// request can never amplify into a runaway upstream-call storm.
	totalUpstreamAttempts := 0

routeLoop:
	for _, matchedRoute := range state.routes {
		if ctx.Err() != nil {
			state.lastErr = ctx.Err()
			c.Err = state.lastErr
			return
		}

		clientType := state.clientType
		modelCandidates := e.mapModelCandidates(state.tenantID, state.requestModel, matchedRoute.Route, matchedRoute.Provider, clientType, state.projectID, state.apiTokenID)
		useSmartMappingRetry := shouldUseSmartMappingRetry(matchedRoute.Provider, len(modelCandidates))
		smartMappingRetryLimit := getSmartMappingRetryLimit(matchedRoute.Provider)
		smartMappingKey := ""
		modelCandidateIndex := 0
		if useSmartMappingRetry {
			smartMappingKey = smartMappingCacheKey(state.tenantID, state.requestModel, matchedRoute.Route, matchedRoute.Provider, clientType, state.projectID, state.apiTokenID, modelCandidates)
			modelCandidateIndex = e.smartMappingStartIndex(smartMappingKey, modelCandidates)
		}
		retryConfig := e.getRetryConfig(state.tenantID, matchedRoute.RetryConfig)

		for attempt := 0; ; {
			if attempt > retryConfig.MaxRetries && !shouldSkipErrorCooldown(matchedRoute.Provider) {
				break
			}
			if ctx.Err() != nil {
				state.lastErr = ctx.Err()
				c.Err = state.lastErr
				return
			}
			if totalUpstreamAttempts >= maxUpstreamAttemptsPerRequest {
				log.Printf("[Executor] Hard attempt ceiling reached: %d upstream attempts for a single request (provider %d); aborting to prevent retry amplification. lastErr=%v",
					totalUpstreamAttempts, matchedRoute.Provider.ID, state.lastErr)
				if state.lastErr == nil {
					proxyErr := domain.NewProxyErrorWithMessage(domain.ErrAllRoutesFailed, false, "upstream attempt ceiling reached")
					proxyErr.Scope = domain.ScopeRequest
					state.lastErr = proxyErr
				}
				break routeLoop
			}
			if modelCandidateIndex >= len(modelCandidates) {
				break
			}

			mappedModel := modelCandidates[modelCandidateIndex]
			proxyReq.RouteID = matchedRoute.Route.ID
			proxyReq.ProviderID = matchedRoute.Provider.ID
			proxyReq.MappedModel = mappedModel
			if quotaErr := e.ensureAPITokenQuota(state, proxyReq, matchedRoute.Provider); quotaErr != nil {
				rejectProxyRequestForQuota(proxyReq, quotaErr)
				clearProxyRequestDetail(proxyReq, clearDetail)
				_ = e.proxyRequestRepo.Update(proxyReq)
				if e.broadcaster != nil {
					e.broadcaster.BroadcastProxyRequest(proxyReq)
				}
				state.lastErr = quotaErr
				c.Err = quotaErr
				return
			}
			_ = e.proxyRequestRepo.Update(proxyReq)
			if e.broadcaster != nil {
				e.broadcaster.BroadcastProxyRequest(proxyReq)
			}

			originalClientType := clientType
			currentClientType := clientType
			needsConversion := false
			convertedBody := []byte(nil)
			var convErr error
			requestBody := state.requestBody
			requestURI := state.requestURI

			supportedTypes := matchedRoute.ProviderAdapter.SupportedClientTypes()
			if bridgeCodexForOpenRouter(e.codexOpenRouterBridgeEnabled(), matchedRoute.Provider, clientType, supportedTypes) {
				currentClientType = domain.ClientTypeOpenAI
				needsConversion = true
				log.Printf("[Executor] OpenRouter-compatible custom provider %s: bridging Codex request through OpenAI Chat Completions",
					matchedRoute.Provider.Name)

				convertedBody, convErr = e.converter.TransformRequest(
					clientType, currentClientType, requestBody, mappedModel, state.isStream)
				if convErr != nil {
					log.Printf("[Executor] OpenRouter Codex->OpenAI conversion failed: %v, proceeding with original format", convErr)
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
			} else if e.converter.NeedConvert(clientType, supportedTypes) {
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
						log.Printf("[Executor] Request conversion failed: %v; refusing to send original %s payload to %s provider %s",
							convErr, clientType, currentClientType, matchedRoute.Provider.Name)
						proxyErr := domain.NewProxyErrorWithMessage(convErr, true, "request format conversion failed")
						proxyErr.Scope = domain.ScopeRequest
						state.lastErr = proxyErr
						break
					}

					requestBody = convertedBody

					originalURI := requestURI
					convertedURI := ConvertRequestURI(requestURI, clientType, currentClientType, mappedModel, state.isStream)
					if convertedURI != originalURI {
						requestURI = convertedURI
						log.Printf("[Executor] URI converted: %s -> %s", originalURI, convertedURI)
					}
				}
			}

			attemptStartTime := time.Now()
			releaseProvider := func() {}
			if e.router != nil {
				var acquired bool
				releaseProvider, acquired = e.router.TryAcquireProvider(matchedRoute.Provider)
				if !acquired {
					log.Printf("[Executor] Provider %d is at its concurrency limit before upstream dispatch; trying the next route", matchedRoute.Provider.ID)
					proxyErr := domain.NewProxyErrorWithMessage(domain.ErrNoAvailableProviders, true, "provider concurrency limit reached before upstream dispatch")
					proxyErr.Scope = domain.ScopeProvider
					proxyErr.Reason = domain.CooldownReasonConcurrentLimit
					proxyErr.HTTPStatusCode = http.StatusTooManyRequests
					state.lastErr = proxyErr
					continue routeLoop
				}
			}
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
			totalUpstreamAttempts++
			if e.broadcaster != nil {
				e.broadcaster.BroadcastProxyRequest(proxyReq)
				e.broadcaster.BroadcastProxyUpstreamAttempt(attemptRecord)
			}

			// Authoritative outbound param stage: apply once per attempt on the
			// final converted body, before it reaches the adapter. Idempotent, so
			// re-running on retry is safe.
			requestBody = e.applyOutboundParamPolicy(requestBody, currentClientType, mappedModel, matchedRoute.Provider)
			if effort := requestmeta.ReasoningEffort(requestBody); effort != "" && effort != proxyReq.ReasoningEffort {
				proxyReq.ReasoningEffort = effort
				if err := e.proxyRequestRepo.Update(proxyReq); err != nil {
					log.Printf("[Executor] Failed to update proxy request reasoning effort: %v", err)
				}
				if e.broadcaster != nil {
					e.broadcaster.BroadcastProxyRequest(proxyReq)
				}
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
			if e.shouldApplyOpenAIChatStreamTimeouts(originalClientType, requestURI) {
				c.Set(flowKeyStreamFirstEventTimeout, e.streamFirstEventTimeout())
				c.Set(flowKeyStreamIdleTimeout, e.streamIdleTimeout())
			}

			err := executeWithProviderSlot(releaseProvider, func() error {
				return matchedRoute.ProviderAdapter.Execute(c, matchedRoute.Provider)
			})
			c.Writer = originalWriter

			if needsConversion && convertingWriter != nil && !state.isStream {
				if finalizeErr := convertingWriter.Finalize(); finalizeErr != nil {
					log.Printf("[Executor] Response conversion finalize failed: %v", finalizeErr)
					proxyErr := domain.NewProxyErrorWithMessage(finalizeErr, true, "response format conversion failed")
					proxyErr.Scope = domain.ScopeProvider
					proxyErr.Reason = domain.CooldownReasonServerError
					err = proxyErr
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
				attemptRecord.Error = ""

				pricing.FinalizeAttemptCost(attemptRecord, multiplier)
				e.deductAPITokenQuota(state, attemptRecord)

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
				if matchedRoute.Provider != nil && matchedRoute.Provider.Config != nil && matchedRoute.Provider.Config.ConsecutiveErrorFreezeEnabled {
					cooldown.Default().ResetFailures(matchedRoute.Provider.ID, "", "")
				}
				if useSmartMappingRetry {
					e.recordSmartMappingSuccess(smartMappingKey, mappedModel)
				}

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

			attemptRecord.Status = attemptFailureStatus(ctx, err)
			attemptRecord.Error = err.Error()

			pricing.FinalizeAttemptCost(attemptRecord, multiplier)
			e.deductAPITokenQuota(state, attemptRecord)

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

			proxyErr, ok := asProxyError(err)
			if ok {
				normalizeUpstreamConnectionError(proxyErr)
				applyDisabledErrorCooldownRetryPolicy(matchedRoute.Provider, proxyErr)
				applyCommittedStreamReadRetryPolicy(proxyErr)
			}
			if responseCapture.WroteToClient() && !shouldRetryCommittedResponseError(proxyErr) {
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
				proxyReq.Status, proxyReq.Error = requestFailureStatusAndError(ctx, err)
				proxyReq.EndTime = time.Now()
				proxyReq.Duration = proxyReq.EndTime.Sub(proxyReq.StartTime)
				clearProxyRequestDetail(proxyReq, clearDetail)
				_ = e.proxyRequestRepo.Update(proxyReq)
				if e.broadcaster != nil {
					e.broadcaster.BroadcastProxyRequest(proxyReq)
				}
				state.lastErr = ctx.Err()
				c.Err = state.lastErr
				return
			}

			if ok && forceRetryUpstreamErrorIfSafe(proxyErr, ctx, responseCapture.WroteToClient(), e.forceRetryUpstreamErrorsEnabled(retryConfig)) {
				log.Printf("[Executor] Force retry upstream errors enabled; retrying provider-side error after provider %d: %v", matchedRoute.Provider.ID, err)
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

			providerFrozenAfterThreshold := false
			if ok && ctx.Err() != context.Canceled {
				log.Printf("[Executor] ProxyError - Scope: %s, Reason: %s, Retryable: %v, Provider: %d",
					proxyErr.Scope, proxyErr.Reason, proxyErr.Retryable, matchedRoute.Provider.ID)
				if shouldUseConsecutiveErrorFreeze(matchedRoute.Provider, proxyErr) {
					failureCount, freezeUntil := cooldown.Default().RecordFailureAfterThreshold(
						matchedRoute.Provider.ID,
						string(currentClientType),
						mappedModel,
						cooldown.CooldownReason(proxyErr.Reason),
						proxyErr.Scope,
						e.rateLimitDefaultCooldownUntil(),
						consecutiveErrorFreezeThreshold(matchedRoute.Provider),
					)
					providerFrozenAfterThreshold = !freezeUntil.IsZero()
					log.Printf("[Executor] Provider %d consecutive upstream failure count=%d threshold=%d frozen=%v until=%s",
						matchedRoute.Provider.ID,
						failureCount,
						consecutiveErrorFreezeThreshold(matchedRoute.Provider),
						providerFrozenAfterThreshold,
						freezeUntil.Format(time.RFC3339),
					)
					if providerFrozenAfterThreshold && e.broadcaster != nil {
						e.broadcaster.BroadcastMessage("cooldown_update", map[string]interface{}{
							"providerID": matchedRoute.Provider.ID,
						})
					}
				} else if !shouldSkipErrorCooldownUpdate(matchedRoute.Provider, proxyErr) && !shouldDeferNetworkErrorCooldown(proxyErr, attempt, retryConfig) {
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

			if providerFrozenAfterThreshold {
				log.Printf("[Executor] Provider %d reached consecutive error freeze threshold; failing over to next provider", matchedRoute.Provider.ID)
				continue routeLoop
			}

			if !ok || !proxyErr.Retryable {
				log.Printf("[Executor] Not retrying provider %d error: proxyError=%v retryable=%v scope=%s reason=%s attempt=%d maxRetries=%d ctxErr=%v responseCommitted=%v err=%v",
					matchedRoute.Provider.ID,
					ok,
					ok && proxyErr.Retryable,
					proxyErrorScopeForLog(proxyErr),
					proxyErrorReasonForLog(proxyErr),
					attempt,
					retryConfig.MaxRetries,
					ctx.Err(),
					responseCapture.WroteToClient(),
					err,
				)
				break
			}

			if useSmartMappingRetry && attempt+1 >= smartMappingRetryLimit {
				modelCandidateIndex++
				attempt = 0
				if modelCandidateIndex >= len(modelCandidates) && shouldSkipErrorCooldown(matchedRoute.Provider) {
					modelCandidateIndex = 0
				}
				if modelCandidateIndex < len(modelCandidates) {
					log.Printf("[Executor] Smart mapping retry switching provider %d mapped model %q -> %q after %d failed attempts",
						matchedRoute.Provider.ID, mappedModel, modelCandidates[modelCandidateIndex], smartMappingRetryLimit)
					continue
				}
				break
			}

			if attempt < retryConfig.MaxRetries || shouldSkipErrorCooldown(matchedRoute.Provider) {
				waitTime := e.calculateBackoff(retryConfig, attempt)
				if proxyErr.RetryAfter > 0 {
					waitTime = proxyErr.RetryAfter
				}
				select {
				case <-ctx.Done():
					proxyReq.Status, proxyReq.Error = requestFailureStatusAndError(ctx, err)
					if proxyReq.Status == "CANCELLED" {
						proxyReq.Error = "client disconnected during retry wait"
					} else if ctx.Err() == context.DeadlineExceeded {
						proxyReq.Error = "request timeout during retry wait"
					}
					proxyReq.EndTime = time.Now()
					proxyReq.Duration = proxyReq.EndTime.Sub(proxyReq.StartTime)
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
			attempt++
		}
	}

	proxyReq.Status = "FAILED"
	proxyReq.EndTime = time.Now()
	proxyReq.Duration = proxyReq.EndTime.Sub(proxyReq.StartTime)
	if state.lastErr != nil {
		proxyReq.Error = state.lastErr.Error()
		if proxyErr, ok := asProxyError(state.lastErr); ok && proxyErr.HTTPStatusCode >= 400 && proxyErr.HTTPStatusCode < 600 {
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

func executeWithProviderSlot(release func(), execute func() error) error {
	defer release()
	return execute()
}

func shouldDeferNetworkErrorCooldown(proxyErr *domain.ProxyError, attempt int, retryConfig *domain.RetryConfig) bool {
	if proxyErr == nil || !proxyErr.Retryable || proxyErr.Reason != domain.CooldownReasonNetworkError {
		return false
	}
	if proxyErr.CooldownUntil != nil || proxyErr.RetryAfter > 0 {
		return false
	}
	maxRetries := 0
	if retryConfig != nil {
		maxRetries = retryConfig.MaxRetries
	}
	return attempt < maxRetries
}

func clearProxyRequestDetail(req *domain.ProxyRequest, clearDetail bool) {
	if !clearDetail || req == nil {
		return
	}
	req.RequestInfo = nil
	req.ResponseInfo = nil
}

func asProxyError(err error) (*domain.ProxyError, bool) {
	if err == nil {
		return nil, false
	}
	var proxyErr *domain.ProxyError
	if errors.As(err, &proxyErr) {
		return proxyErr, true
	}
	return nil, false
}

func normalizeUpstreamConnectionError(proxyErr *domain.ProxyError) {
	if proxyErr == nil || !errors.Is(proxyErr.Err, domain.ErrUpstreamError) {
		return
	}
	if !strings.HasPrefix(proxyErr.Message, "failed to connect to upstream") {
		return
	}
	proxyErr.Scope = domain.ScopeProvider
	proxyErr.Reason = domain.CooldownReasonNetworkError
	proxyErr.Retryable = true
}

func shouldUseSmartMappingRetry(provider *domain.Provider, candidateCount int) bool {
	if provider == nil || provider.Config == nil {
		return false
	}
	return provider.Config.DisableErrorCooldown && provider.Config.SmartMappingRetryEnabled && candidateCount > 1
}

func getSmartMappingRetryLimit(provider *domain.Provider) int {
	if provider == nil || provider.Config == nil || provider.Config.SmartMappingRetryLimit <= 0 {
		return 1
	}
	return provider.Config.SmartMappingRetryLimit
}
