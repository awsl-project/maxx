package executor

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/awsl-project/maxx/internal/converter"
	"github.com/awsl-project/maxx/internal/cooldown"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/pricing"
	"github.com/awsl-project/maxx/internal/usage"
)

func (e *Executor) dispatch(c *flow.Ctx) {
	state, ok := getExecState(c)
	if !ok {
		err := domain.NewProxyErrorWithMessage(domain.ErrInvalidInput, false, "executor state missing")
		c.Err = err
		c.Abort()
		return
	}

	proxyReq := state.proxyReq
	ctx := state.ctx
	clearDetail := e.shouldClearRequestDetailFor(state)

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
			responseCapture := NewResponseCapture(c.Writer)
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

			eventChan.Close()
			<-eventDone

			if err == nil {
				attemptRecord.EndTime = time.Now()
				attemptRecord.Duration = attemptRecord.EndTime.Sub(attemptRecord.StartTime)
				attemptRecord.Status = "COMPLETED"

				if attemptRecord.InputTokenCount > 0 || attemptRecord.OutputTokenCount > 0 {
					metrics := &usage.Metrics{
						InputTokens:          attemptRecord.InputTokenCount,
						OutputTokens:         attemptRecord.OutputTokenCount,
						CacheReadCount:       attemptRecord.CacheReadCount,
						CacheCreationCount:   attemptRecord.CacheWriteCount,
						Cache5mCreationCount: attemptRecord.Cache5mWriteCount,
						Cache1hCreationCount: attemptRecord.Cache1hWriteCount,
					}
					pricingModel := attemptRecord.ResponseModel
					if pricingModel == "" {
						pricingModel = attemptRecord.MappedModel
					}
					multiplier := getProviderMultiplier(matchedRoute.Provider, clientType)
					result := pricing.GlobalCalculator().CalculateWithResult(pricingModel, metrics, multiplier)
					attemptRecord.Cost = result.Cost
					attemptRecord.ModelPriceID = result.ModelPriceID
					attemptRecord.Multiplier = result.Multiplier
				}

				if clearDetail {
					attemptRecord.RequestInfo = nil
					attemptRecord.ResponseInfo = nil
				}

				_ = e.attemptRepo.Update(attemptRecord)
				if e.broadcaster != nil {
					e.broadcaster.BroadcastProxyUpstreamAttempt(attemptRecord)
				}
				state.currentAttempt = nil

				cooldown.Default().RecordSuccess(matchedRoute.Provider.ID, string(currentClientType))

				proxyReq.Status = "COMPLETED"
				proxyReq.EndTime = time.Now()
				proxyReq.Duration = proxyReq.EndTime.Sub(proxyReq.StartTime)
				proxyReq.FinalProxyUpstreamAttemptID = attemptRecord.ID
				proxyReq.ModelPriceID = attemptRecord.ModelPriceID
				proxyReq.Multiplier = attemptRecord.Multiplier
				proxyReq.ResponseModel = mappedModel

				if !clearDetail {
					proxyReq.ResponseInfo = &domain.ResponseInfo{
						Status:  responseCapture.StatusCode(),
						Headers: responseCapture.CapturedHeaders(),
						Body:    responseCapture.Body(),
					}
				}
				proxyReq.StatusCode = responseCapture.StatusCode()

				if metrics := usage.ExtractFromResponse(responseCapture.Body()); metrics != nil {
					proxyReq.InputTokenCount = metrics.InputTokens
					proxyReq.OutputTokenCount = metrics.OutputTokens
					proxyReq.CacheReadCount = metrics.CacheReadCount
					proxyReq.CacheWriteCount = metrics.CacheCreationCount
					proxyReq.Cache5mWriteCount = metrics.Cache5mCreationCount
					proxyReq.Cache1hWriteCount = metrics.Cache1hCreationCount
				}
				proxyReq.Cost = attemptRecord.Cost
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

			if attemptRecord.InputTokenCount > 0 || attemptRecord.OutputTokenCount > 0 {
				metrics := &usage.Metrics{
					InputTokens:          attemptRecord.InputTokenCount,
					OutputTokens:         attemptRecord.OutputTokenCount,
					CacheReadCount:       attemptRecord.CacheReadCount,
					CacheCreationCount:   attemptRecord.CacheWriteCount,
					Cache5mCreationCount: attemptRecord.Cache5mWriteCount,
					Cache1hCreationCount: attemptRecord.Cache1hWriteCount,
				}
				pricingModel := attemptRecord.ResponseModel
				if pricingModel == "" {
					pricingModel = attemptRecord.MappedModel
				}
				multiplier := getProviderMultiplier(matchedRoute.Provider, clientType)
				result := pricing.GlobalCalculator().CalculateWithResult(pricingModel, metrics, multiplier)
				attemptRecord.Cost = result.Cost
				attemptRecord.ModelPriceID = result.ModelPriceID
				attemptRecord.Multiplier = result.Multiplier
			}

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
			proxyReq.ModelPriceID = attemptRecord.ModelPriceID
			proxyReq.Multiplier = attemptRecord.Multiplier

			if responseCapture.Body() != "" {
				proxyReq.StatusCode = responseCapture.StatusCode()
				if !clearDetail {
					proxyReq.ResponseInfo = &domain.ResponseInfo{
						Status:  responseCapture.StatusCode(),
						Headers: responseCapture.CapturedHeaders(),
						Body:    responseCapture.Body(),
					}
				}
				if metrics := usage.ExtractFromResponse(responseCapture.Body()); metrics != nil {
					proxyReq.InputTokenCount = metrics.InputTokens
					proxyReq.OutputTokenCount = metrics.OutputTokens
					proxyReq.CacheReadCount = metrics.CacheReadCount
					proxyReq.CacheWriteCount = metrics.CacheCreationCount
					proxyReq.Cache5mWriteCount = metrics.Cache5mCreationCount
					proxyReq.Cache1hWriteCount = metrics.Cache1hCreationCount
				}
			}
			proxyReq.Cost = attemptRecord.Cost
			proxyReq.TTFT = attemptRecord.TTFT

			clearProxyRequestDetail(proxyReq, clearDetail)

			_ = e.proxyRequestRepo.Update(proxyReq)
			if e.broadcaster != nil {
				e.broadcaster.BroadcastProxyRequest(proxyReq)
			}

			proxyErr, ok := err.(*domain.ProxyError)
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

			if ok && ctx.Err() != context.Canceled {
				log.Printf("[Executor] ProxyError - IsNetworkError: %v, IsServerError: %v, Retryable: %v, Provider: %d",
					proxyErr.IsNetworkError, proxyErr.IsServerError, proxyErr.Retryable, matchedRoute.Provider.ID)
				if !shouldSkipErrorCooldown(matchedRoute.Provider) {
					e.handleCooldown(proxyErr, matchedRoute.Provider, currentClientType, originalClientType)
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
	}
	clearProxyRequestDetail(proxyReq, clearDetail)
	_ = e.proxyRequestRepo.Update(proxyReq)
	if e.broadcaster != nil {
		e.broadcaster.BroadcastProxyRequest(proxyReq)
	}

	if state.lastErr == nil {
		state.lastErr = domain.NewProxyErrorWithMessage(domain.ErrAllRoutesFailed, false, "all routes exhausted")
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
