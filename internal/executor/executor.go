package executor

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/awsl-project/maxx/internal/converter"
	"github.com/awsl-project/maxx/internal/cooldown"
	ctxutil "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/event"
	"github.com/awsl-project/maxx/internal/pricing"
	"github.com/awsl-project/maxx/internal/repository"
	"github.com/awsl-project/maxx/internal/router"
	"github.com/awsl-project/maxx/internal/stats"
	"github.com/awsl-project/maxx/internal/usage"
	"github.com/awsl-project/maxx/internal/waiter"
)

// Executor handles request execution with retry logic
type Executor struct {
	router             *router.Router
	proxyRequestRepo   repository.ProxyRequestRepository
	attemptRepo        repository.ProxyUpstreamAttemptRepository
	retryConfigRepo    repository.RetryConfigRepository
	sessionRepo        repository.SessionRepository
	modelMappingRepo   repository.ModelMappingRepository
	settingsRepo       repository.SystemSettingRepository
	broadcaster        event.Broadcaster
	projectWaiter      *waiter.ProjectWaiter
	instanceID         string
	statsAggregator    *stats.StatsAggregator
	converter          *converter.Registry
}

// NewExecutor creates a new executor
func NewExecutor(
	r *router.Router,
	prr repository.ProxyRequestRepository,
	ar repository.ProxyUpstreamAttemptRepository,
	rcr repository.RetryConfigRepository,
	sessionRepo repository.SessionRepository,
	modelMappingRepo repository.ModelMappingRepository,
	settingsRepo repository.SystemSettingRepository,
	bc event.Broadcaster,
	projectWaiter *waiter.ProjectWaiter,
	instanceID string,
	statsAggregator *stats.StatsAggregator,
) *Executor {
	return &Executor{
		router:             r,
		proxyRequestRepo:   prr,
		attemptRepo:        ar,
		retryConfigRepo:    rcr,
		sessionRepo:        sessionRepo,
		modelMappingRepo:   modelMappingRepo,
		settingsRepo:       settingsRepo,
		broadcaster:        bc,
		projectWaiter:      projectWaiter,
		instanceID:         instanceID,
		statsAggregator:    statsAggregator,
		converter:          converter.GetGlobalRegistry(),
	}
}

// Execute handles the proxy request with routing and retry logic
func (e *Executor) Execute(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	clientType := ctxutil.GetClientType(ctx)
	projectID := ctxutil.GetProjectID(ctx)
	sessionID := ctxutil.GetSessionID(ctx)
	requestModel := ctxutil.GetRequestModel(ctx)
	isStream := ctxutil.GetIsStream(ctx)

	// Get API Token ID from context
	apiTokenID := ctxutil.GetAPITokenID(ctx)

	// Create proxy request record immediately (PENDING status)
	proxyReq := &domain.ProxyRequest{
		InstanceID:   e.instanceID,
		RequestID:    generateRequestID(),
		SessionID:    sessionID,
		ClientType:   clientType,
		ProjectID:    projectID,
		RequestModel: requestModel,
		StartTime:    time.Now(),
		IsStream:     isStream,
		Status:       "PENDING",
		APITokenID:   apiTokenID,
	}

	// Capture client's original request info unless detail retention is disabled.
	if !e.shouldClearRequestDetail() {
		requestURI := ctxutil.GetRequestURI(ctx)
		requestHeaders := ctxutil.GetRequestHeaders(ctx)
		requestBody := ctxutil.GetRequestBody(ctx)
		headers := flattenHeaders(requestHeaders)
		// Go stores Host separately from headers, add it explicitly
		if req.Host != "" {
			if headers == nil {
				headers = make(map[string]string)
			}
			headers["Host"] = req.Host
		}
		proxyReq.RequestInfo = &domain.RequestInfo{
			Method:  req.Method,
			URL:     requestURI,
			Headers: headers,
			Body:    string(requestBody),
		}
	}

	if err := e.proxyRequestRepo.Create(proxyReq); err != nil {
		log.Printf("[Executor] Failed to create proxy request: %v", err)
	}

	// Broadcast the new request immediately
	if e.broadcaster != nil {
		e.broadcaster.BroadcastProxyRequest(proxyReq)
	}

	ctx = ctxutil.WithProxyRequest(ctx, proxyReq)

	// Check for project binding if required
	if projectID == 0 && e.projectWaiter != nil {
		// Get session for project waiter
		session, _ := e.sessionRepo.GetBySessionID(sessionID)
		if session == nil {
			session = &domain.Session{
				SessionID:  sessionID,
				ClientType: clientType,
				ProjectID:  0,
			}
		}

		if err := e.projectWaiter.WaitForProject(ctx, session); err != nil {
			// Determine status based on error type
			status := "REJECTED"
			errorMsg := "project binding timeout: " + err.Error()
			if err == context.Canceled {
				status = "CANCELLED"
				errorMsg = "client cancelled: " + err.Error()
				// Notify frontend to close the dialog
				if e.broadcaster != nil {
					e.broadcaster.BroadcastMessage("session_pending_cancelled", map[string]interface{}{
						"sessionID": sessionID,
					})
				}
			}

			// Update request record with final status
			proxyReq.Status = status
			proxyReq.Error = errorMsg
			proxyReq.EndTime = time.Now()
			proxyReq.Duration = proxyReq.EndTime.Sub(proxyReq.StartTime)
			_ = e.proxyRequestRepo.Update(proxyReq)

			// Broadcast the updated request
			if e.broadcaster != nil {
				e.broadcaster.BroadcastProxyRequest(proxyReq)
			}

			return domain.NewProxyErrorWithMessage(err, false, "project binding required: "+err.Error())
		}

		// Update projectID from the now-bound session
		projectID = session.ProjectID
		proxyReq.ProjectID = projectID
		ctx = ctxutil.WithProjectID(ctx, projectID)
	}

	// Match routes
	routes, err := e.router.Match(&router.MatchContext{
		ClientType:   clientType,
		ProjectID:    projectID,
		RequestModel: requestModel,
		APITokenID:   apiTokenID,
	})
	if err != nil {
		proxyReq.Status = "FAILED"
		proxyReq.Error = "no routes available"
		proxyReq.EndTime = time.Now()
		proxyReq.Duration = proxyReq.EndTime.Sub(proxyReq.StartTime)
		_ = e.proxyRequestRepo.Update(proxyReq)
		if e.broadcaster != nil {
			e.broadcaster.BroadcastProxyRequest(proxyReq)
		}
		return domain.NewProxyErrorWithMessage(domain.ErrNoRoutes, false, "no routes available")
	}

	if len(routes) == 0 {
		proxyReq.Status = "FAILED"
		proxyReq.Error = "no routes configured"
		proxyReq.EndTime = time.Now()
		proxyReq.Duration = proxyReq.EndTime.Sub(proxyReq.StartTime)
		_ = e.proxyRequestRepo.Update(proxyReq)
		if e.broadcaster != nil {
			e.broadcaster.BroadcastProxyRequest(proxyReq)
		}
		return domain.NewProxyErrorWithMessage(domain.ErrNoRoutes, false, "no routes configured")
	}

	// Update status to IN_PROGRESS
	proxyReq.Status = "IN_PROGRESS"
	_ = e.proxyRequestRepo.Update(proxyReq)
	ctx = ctxutil.WithProxyRequest(ctx, proxyReq)

	// Add broadcaster to context so adapters can send updates
	if e.broadcaster != nil {
		ctx = ctxutil.WithBroadcaster(ctx, e.broadcaster)
	}

	// Broadcast new request immediately so frontend sees it
	if e.broadcaster != nil {
		e.broadcaster.BroadcastProxyRequest(proxyReq)
	}

	// Track current attempt for cleanup
	var currentAttempt *domain.ProxyUpstreamAttempt

	// Ensure final state is always updated
	defer func() {
		// If still IN_PROGRESS, mark as cancelled/failed
		if proxyReq.Status == "IN_PROGRESS" {
			proxyReq.EndTime = time.Now()
			proxyReq.Duration = proxyReq.EndTime.Sub(proxyReq.StartTime)
			if ctx.Err() != nil {
				proxyReq.Status = "CANCELLED"
				if ctx.Err() == context.Canceled {
					proxyReq.Error = "client disconnected"
				} else if ctx.Err() == context.DeadlineExceeded {
					proxyReq.Error = "request timeout"
				} else {
					proxyReq.Error = ctx.Err().Error()
				}
			} else {
				proxyReq.Status = "FAILED"
			}
			_ = e.proxyRequestRepo.Update(proxyReq)
			if e.broadcaster != nil {
				e.broadcaster.BroadcastProxyRequest(proxyReq)
			}
		}

		// If current attempt is still IN_PROGRESS, mark as cancelled/failed
		if currentAttempt != nil && currentAttempt.Status == "IN_PROGRESS" {
			currentAttempt.EndTime = time.Now()
			currentAttempt.Duration = currentAttempt.EndTime.Sub(currentAttempt.StartTime)
			if ctx.Err() != nil {
				currentAttempt.Status = "CANCELLED"
			} else {
				currentAttempt.Status = "FAILED"
			}
			_ = e.attemptRepo.Update(currentAttempt)
			if e.broadcaster != nil {
				e.broadcaster.BroadcastProxyUpstreamAttempt(currentAttempt)
			}
		}
	}()

	// Try routes in order with retry logic
	var lastErr error
	for _, matchedRoute := range routes {
		// Check context before starting new route
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Update proxyReq with current route/provider for real-time tracking
		proxyReq.RouteID = matchedRoute.Route.ID
		proxyReq.ProviderID = matchedRoute.Provider.ID
		_ = e.proxyRequestRepo.Update(proxyReq)
		if e.broadcaster != nil {
			e.broadcaster.BroadcastProxyRequest(proxyReq)
		}

		// Determine model mapping
		// Model mapping is done in Executor after Router has filtered by SupportModels
		clientType := ctxutil.GetClientType(ctx)
		mappedModel := e.mapModel(requestModel, matchedRoute.Route, matchedRoute.Provider, clientType, projectID, apiTokenID)
		ctx = ctxutil.WithMappedModel(ctx, mappedModel)

		// Format conversion: check if client type is supported by provider
		// If not, convert request to a supported format
		originalClientType := clientType
		targetClientType := clientType
		needsConversion := false

		supportedTypes := matchedRoute.ProviderAdapter.SupportedClientTypes()
		if e.converter.NeedConvert(clientType, supportedTypes) {
			targetClientType = GetPreferredTargetType(supportedTypes, clientType)
			if targetClientType != clientType {
				needsConversion = true
				log.Printf("[Executor] Format conversion needed: %s -> %s for provider %s",
					clientType, targetClientType, matchedRoute.Provider.Name)

				// Convert request body
				requestBody := ctxutil.GetRequestBody(ctx)
				convertedBody, convErr := e.converter.TransformRequest(
					clientType, targetClientType, requestBody, mappedModel, isStream)
				if convErr != nil {
					log.Printf("[Executor] Request conversion failed: %v, proceeding with original format", convErr)
					needsConversion = false
				} else {
					// Update context with converted body and new client type
					ctx = ctxutil.WithRequestBody(ctx, convertedBody)
					ctx = ctxutil.WithClientType(ctx, targetClientType)
					ctx = ctxutil.WithOriginalClientType(ctx, originalClientType)

					// Convert request URI to match the target client type
					originalURI := ctxutil.GetRequestURI(ctx)
					convertedURI := ConvertRequestURI(originalURI, clientType, targetClientType)
					if convertedURI != originalURI {
						ctx = ctxutil.WithRequestURI(ctx, convertedURI)
						log.Printf("[Executor] URI converted: %s -> %s", originalURI, convertedURI)
					}
				}
			}
		}

		// Get retry config
		retryConfig := e.getRetryConfig(matchedRoute.RetryConfig)

		// Execute with retries
		for attempt := 0; attempt <= retryConfig.MaxRetries; attempt++ {
			// Check context before each attempt
			if ctx.Err() != nil {
				return ctx.Err()
			}

			// Create attempt record with start time and request info
			attemptStartTime := time.Now()
			attemptRecord := &domain.ProxyUpstreamAttempt{
				ProxyRequestID: proxyReq.ID,
				RouteID:        matchedRoute.Route.ID,
				ProviderID:     matchedRoute.Provider.ID,
				IsStream:       isStream,
				Status:         "IN_PROGRESS",
				StartTime:      attemptStartTime,
				RequestModel:   requestModel,
				MappedModel:    mappedModel,
				RequestInfo:    proxyReq.RequestInfo, // Use original request info initially
			}
			if err := e.attemptRepo.Create(attemptRecord); err != nil {
				log.Printf("[Executor] Failed to create attempt record: %v", err)
			}
			currentAttempt = attemptRecord

			// Increment attempt count when creating a new attempt
			proxyReq.ProxyUpstreamAttemptCount++

			// Broadcast updated request with new attempt count
			if e.broadcaster != nil {
				e.broadcaster.BroadcastProxyRequest(proxyReq)
			}

			// Broadcast new attempt immediately
			if e.broadcaster != nil {
				e.broadcaster.BroadcastProxyUpstreamAttempt(attemptRecord)
			}

			// Put attempt into context so adapter can populate request/response info
			attemptCtx := ctxutil.WithUpstreamAttempt(ctx, attemptRecord)

			// Create event channel for adapter to send events
			eventChan := domain.NewAdapterEventChan()
			attemptCtx = ctxutil.WithEventChan(attemptCtx, eventChan)

			// Start real-time event processing goroutine
			// This ensures RequestInfo is broadcast as soon as adapter sends it
			eventDone := make(chan struct{})
			go e.processAdapterEventsRealtime(eventChan, attemptRecord, eventDone)

			// Wrap ResponseWriter to capture actual client response
			// If format conversion is needed, use ConvertingResponseWriter
			var responseWriter http.ResponseWriter
			var convertingWriter *ConvertingResponseWriter
			responseCapture := NewResponseCapture(w)

			if needsConversion {
				// Use ConvertingResponseWriter to transform response from targetType back to originalType
				convertingWriter = NewConvertingResponseWriter(
					responseCapture, e.converter, originalClientType, targetClientType, isStream)
				responseWriter = convertingWriter
			} else {
				responseWriter = responseCapture
			}

			// Execute request
			err := matchedRoute.ProviderAdapter.Execute(attemptCtx, responseWriter, req, matchedRoute.Provider)

			// For non-streaming responses with conversion, finalize the conversion
			if needsConversion && convertingWriter != nil && !isStream {
				if finalizeErr := convertingWriter.Finalize(); finalizeErr != nil {
					log.Printf("[Executor] Response conversion finalize failed: %v", finalizeErr)
				}
			}

			// Close event channel and wait for processing goroutine to finish
			eventChan.Close()
			<-eventDone

			if err == nil {
				// Success - set end time and duration
				attemptRecord.EndTime = time.Now()
				attemptRecord.Duration = attemptRecord.EndTime.Sub(attemptRecord.StartTime)
				attemptRecord.Status = "COMPLETED"

				// Calculate cost in executor (unified for all adapters)
				// Adapter only needs to set token counts, executor handles pricing
				if attemptRecord.InputTokenCount > 0 || attemptRecord.OutputTokenCount > 0 {
					metrics := &usage.Metrics{
						InputTokens:          attemptRecord.InputTokenCount,
						OutputTokens:         attemptRecord.OutputTokenCount,
						CacheReadCount:       attemptRecord.CacheReadCount,
						CacheCreationCount:   attemptRecord.CacheWriteCount,
						Cache5mCreationCount: attemptRecord.Cache5mWriteCount,
						Cache1hCreationCount: attemptRecord.Cache1hWriteCount,
					}
					// Use ResponseModel for pricing (actual model from API response), fallback to MappedModel
					pricingModel := attemptRecord.ResponseModel
					if pricingModel == "" {
						pricingModel = attemptRecord.MappedModel
					}
					// Get multiplier from provider config
					multiplier := getProviderMultiplier(matchedRoute.Provider, clientType)
					result := pricing.GlobalCalculator().CalculateWithResult(pricingModel, metrics, multiplier)
					attemptRecord.Cost = result.Cost
					attemptRecord.ModelPriceID = result.ModelPriceID
					attemptRecord.Multiplier = result.Multiplier
				}

				// 检查是否需要立即清理 attempt 详情（设置为 0 时不保存）
				if e.shouldClearRequestDetail() {
					attemptRecord.RequestInfo = nil
					attemptRecord.ResponseInfo = nil
				}

				_ = e.attemptRepo.Update(attemptRecord)
				if e.broadcaster != nil {
					e.broadcaster.BroadcastProxyUpstreamAttempt(attemptRecord)
				}
				currentAttempt = nil // Clear so defer doesn't update

				// Reset failure counts on success
				clientType := string(ctxutil.GetClientType(attemptCtx))
				cooldown.Default().RecordSuccess(matchedRoute.Provider.ID, clientType)

				proxyReq.Status = "COMPLETED"
				proxyReq.EndTime = time.Now()
				proxyReq.Duration = proxyReq.EndTime.Sub(proxyReq.StartTime)
				proxyReq.FinalProxyUpstreamAttemptID = attemptRecord.ID
				proxyReq.ModelPriceID = attemptRecord.ModelPriceID
				proxyReq.Multiplier = attemptRecord.Multiplier
				proxyReq.ResponseModel = mappedModel // Record the actual model used

				// Capture actual client response (what was sent to client, e.g. Claude format)
				// This is different from attemptRecord.ResponseInfo which is upstream response (Gemini format)
				if !e.shouldClearRequestDetail() {
					proxyReq.ResponseInfo = &domain.ResponseInfo{
						Status:  responseCapture.StatusCode(),
						Headers: responseCapture.CapturedHeaders(),
						Body:    responseCapture.Body(),
					}
				}
				proxyReq.StatusCode = responseCapture.StatusCode()

				// Extract token usage from final client response (not from upstream attempt)
				// This ensures we use the correct format (Claude/OpenAI/Gemini) for the client type
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

				// 检查是否需要立即清理 proxyReq 详情（设置为 0 时不保存）
				if e.shouldClearRequestDetail() {
					proxyReq.RequestInfo = nil
					proxyReq.ResponseInfo = nil
				}

				_ = e.proxyRequestRepo.Update(proxyReq)

				// Broadcast to WebSocket clients
				if e.broadcaster != nil {
					e.broadcaster.BroadcastProxyRequest(proxyReq)
				}

				return nil
			}

			// Handle error - set end time and duration
			attemptRecord.EndTime = time.Now()
			attemptRecord.Duration = attemptRecord.EndTime.Sub(attemptRecord.StartTime)
			lastErr = err

			// Update attempt status first (before checking context)
			if ctx.Err() != nil {
				attemptRecord.Status = "CANCELLED"
			} else {
				attemptRecord.Status = "FAILED"
			}

			// Calculate cost in executor even for failed attempts (may have partial token usage)
			if attemptRecord.InputTokenCount > 0 || attemptRecord.OutputTokenCount > 0 {
				metrics := &usage.Metrics{
					InputTokens:          attemptRecord.InputTokenCount,
					OutputTokens:         attemptRecord.OutputTokenCount,
					CacheReadCount:       attemptRecord.CacheReadCount,
					CacheCreationCount:   attemptRecord.CacheWriteCount,
					Cache5mCreationCount: attemptRecord.Cache5mWriteCount,
					Cache1hCreationCount: attemptRecord.Cache1hWriteCount,
				}
				// Use ResponseModel for pricing (actual model from API response), fallback to MappedModel
				pricingModel := attemptRecord.ResponseModel
				if pricingModel == "" {
					pricingModel = attemptRecord.MappedModel
				}
				// Get multiplier from provider config
				multiplier := getProviderMultiplier(matchedRoute.Provider, clientType)
				result := pricing.GlobalCalculator().CalculateWithResult(pricingModel, metrics, multiplier)
				attemptRecord.Cost = result.Cost
				attemptRecord.ModelPriceID = result.ModelPriceID
				attemptRecord.Multiplier = result.Multiplier
			}

			// 检查是否需要立即清理 attempt 详情（设置为 0 时不保存）
			if e.shouldClearRequestDetail() {
				attemptRecord.RequestInfo = nil
				attemptRecord.ResponseInfo = nil
			}

			_ = e.attemptRepo.Update(attemptRecord)
			if e.broadcaster != nil {
				e.broadcaster.BroadcastProxyUpstreamAttempt(attemptRecord)
			}
			currentAttempt = nil // Clear so defer doesn't double update

			// Update proxyReq with latest attempt info (even on failure)
			proxyReq.FinalProxyUpstreamAttemptID = attemptRecord.ID
			proxyReq.ModelPriceID = attemptRecord.ModelPriceID
			proxyReq.Multiplier = attemptRecord.Multiplier

			// Capture actual client response (even on failure, if any response was sent)
			if responseCapture.Body() != "" {
				proxyReq.StatusCode = responseCapture.StatusCode()
				if !e.shouldClearRequestDetail() {
					proxyReq.ResponseInfo = &domain.ResponseInfo{
						Status:  responseCapture.StatusCode(),
						Headers: responseCapture.CapturedHeaders(),
						Body:    responseCapture.Body(),
					}
				}

				// Extract token usage from final client response
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

			_ = e.proxyRequestRepo.Update(proxyReq)
			if e.broadcaster != nil {
				e.broadcaster.BroadcastProxyRequest(proxyReq)
			}

			// Handle cooldown only for real server/network errors, NOT client-side cancellations
			proxyErr, ok := err.(*domain.ProxyError)
			if ok && ctx.Err() != context.Canceled {
				log.Printf("[Executor] ProxyError - IsNetworkError: %v, IsServerError: %v, Retryable: %v, Provider: %d",
					proxyErr.IsNetworkError, proxyErr.IsServerError, proxyErr.Retryable, matchedRoute.Provider.ID)
				// Handle cooldown (unified cooldown logic for all providers)
				e.handleCooldown(attemptCtx, proxyErr, matchedRoute.Provider)
				// Broadcast cooldown update event to frontend
				if e.broadcaster != nil {
					e.broadcaster.BroadcastMessage("cooldown_update", map[string]interface{}{
						"providerID": matchedRoute.Provider.ID,
					})
				}
			} else if ok && ctx.Err() == context.Canceled {
				log.Printf("[Executor] Client disconnected, skipping cooldown for Provider: %d", matchedRoute.Provider.ID)
			} else if !ok {
				log.Printf("[Executor] Error is not ProxyError, type: %T, error: %v", err, err)
			}

			// Check if context was cancelled or timed out
			if ctx.Err() != nil {
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
				_ = e.proxyRequestRepo.Update(proxyReq)
				if e.broadcaster != nil {
					e.broadcaster.BroadcastProxyRequest(proxyReq)
				}
				return ctx.Err()
			}

			// Check if retryable
			if !ok {
				break // Move to next route
			}

			if !proxyErr.Retryable {
				break // Move to next route
			}

			// Wait before retry (unless last attempt)
			if attempt < retryConfig.MaxRetries {
				waitTime := e.calculateBackoff(retryConfig, attempt)
				if proxyErr.RetryAfter > 0 {
					waitTime = proxyErr.RetryAfter
				}
				select {
				case <-ctx.Done():
					// Set final status before returning
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
					_ = e.proxyRequestRepo.Update(proxyReq)
					if e.broadcaster != nil {
						e.broadcaster.BroadcastProxyRequest(proxyReq)
					}
					return ctx.Err()
				case <-time.After(waitTime):
				}
			}
		}
		// Inner loop ended, will try next route if available
	}

	// All routes failed
	proxyReq.Status = "FAILED"
	proxyReq.EndTime = time.Now()
	proxyReq.Duration = proxyReq.EndTime.Sub(proxyReq.StartTime)
	if lastErr != nil {
		proxyReq.Error = lastErr.Error()
	}

	// 检查是否需要立即清理详情（设置为 0 时不保存）
	if e.shouldClearRequestDetail() {
		proxyReq.RequestInfo = nil
		proxyReq.ResponseInfo = nil
	}

	_ = e.proxyRequestRepo.Update(proxyReq)

	// Broadcast to WebSocket clients
	if e.broadcaster != nil {
		e.broadcaster.BroadcastProxyRequest(proxyReq)
	}

	if lastErr != nil {
		return lastErr
	}
	return domain.NewProxyErrorWithMessage(domain.ErrAllRoutesFailed, false, "all routes exhausted")
}

func (e *Executor) mapModel(requestModel string, route *domain.Route, provider *domain.Provider, clientType domain.ClientType, projectID uint64, apiTokenID uint64) string {
	// Database model mapping with full query conditions
	query := &domain.ModelMappingQuery{
		ClientType:   clientType,
		ProviderType: provider.Type,
		ProviderID:   provider.ID,
		ProjectID:    projectID,
		RouteID:      route.ID,
		APITokenID:   apiTokenID,
	}
	mappings, _ := e.modelMappingRepo.ListByQuery(query)
	for _, m := range mappings {
		if domain.MatchWildcard(m.Pattern, requestModel) {
			return m.Target
		}
	}

	// No mapping, use original
	return requestModel
}

func (e *Executor) getRetryConfig(config *domain.RetryConfig) *domain.RetryConfig {
	if config != nil {
		return config
	}

	// Get default config
	defaultConfig, err := e.retryConfigRepo.GetDefault()
	if err == nil && defaultConfig != nil {
		return defaultConfig
	}

	// No default config means no retry
	return &domain.RetryConfig{
		MaxRetries:      0,
		InitialInterval: 0,
		BackoffRate:     1.0,
		MaxInterval:     0,
	}
}

func (e *Executor) calculateBackoff(config *domain.RetryConfig, attempt int) time.Duration {
	wait := float64(config.InitialInterval)
	for i := 0; i < attempt; i++ {
		wait *= config.BackoffRate
	}
	if time.Duration(wait) > config.MaxInterval {
		return config.MaxInterval
	}
	return time.Duration(wait)
}

func generateRequestID() string {
	return time.Now().Format("20060102150405.000000")
}

// flattenHeaders converts http.Header to map[string]string (taking first value)
func flattenHeaders(h http.Header) map[string]string {
	if h == nil {
		return nil
	}
	result := make(map[string]string)
	for key, values := range h {
		if len(values) > 0 {
			result[key] = values[0]
		}
	}
	return result
}

// handleCooldown processes cooldown information from ProxyError and sets provider cooldown
// Priority: 1) Explicit time from API, 2) Policy-based calculation based on failure reason
func (e *Executor) handleCooldown(ctx context.Context, proxyErr *domain.ProxyError, provider *domain.Provider) {
	// Determine which client type to apply cooldown to
	clientType := proxyErr.CooldownClientType
	if proxyErr.RateLimitInfo != nil && proxyErr.RateLimitInfo.ClientType != "" {
		clientType = proxyErr.RateLimitInfo.ClientType
	}
	// Fallback to original client type (before format conversion) if not specified
	if clientType == "" {
		// Prefer original client type over converted type
		if origCT := ctxutil.GetOriginalClientType(ctx); origCT != "" {
			clientType = string(origCT)
		} else {
			clientType = string(ctxutil.GetClientType(ctx))
		}
	}

	// Determine cooldown reason and explicit time
	var reason cooldown.CooldownReason
	var explicitUntil *time.Time

	// Priority 1: Check for explicit cooldown time from API
	if proxyErr.CooldownUntil != nil {
		// Has explicit time from API (e.g., from CooldownUntil field)
		explicitUntil = proxyErr.CooldownUntil
		reason = cooldown.ReasonQuotaExhausted // Default, may be overridden below
		if proxyErr.RateLimitInfo != nil {
			reason = mapRateLimitTypeToReason(proxyErr.RateLimitInfo.Type)
		}
	} else if proxyErr.RateLimitInfo != nil && !proxyErr.RateLimitInfo.QuotaResetTime.IsZero() {
		// Has explicit quota reset time from API
		explicitUntil = &proxyErr.RateLimitInfo.QuotaResetTime
		reason = mapRateLimitTypeToReason(proxyErr.RateLimitInfo.Type)
	} else if proxyErr.RetryAfter > 0 {
		// Has Retry-After duration from API
		untilTime := time.Now().Add(proxyErr.RetryAfter)
		explicitUntil = &untilTime
		reason = cooldown.ReasonRateLimit
	} else if proxyErr.IsServerError {
		// Server error (5xx) - no explicit time, use policy
		reason = cooldown.ReasonServerError
		explicitUntil = nil
	} else if proxyErr.IsNetworkError {
		// Network error - no explicit time, use policy
		reason = cooldown.ReasonNetworkError
		explicitUntil = nil
	} else {
		// Unknown error type - use policy
		reason = cooldown.ReasonUnknown
		explicitUntil = nil
	}

	// Record failure and apply cooldown
	// If explicitUntil is not nil, it will be used directly
	// Otherwise, cooldown duration is calculated based on policy and failure count
	cooldown.Default().RecordFailure(provider.ID, clientType, reason, explicitUntil)

	// If there's an async update channel, listen for updates
	if proxyErr.CooldownUpdateChan != nil {
		go e.handleAsyncCooldownUpdate(proxyErr.CooldownUpdateChan, provider, clientType)
	}
}

// mapRateLimitTypeToReason maps RateLimitInfo.Type to CooldownReason
func mapRateLimitTypeToReason(rateLimitType string) cooldown.CooldownReason {
	switch rateLimitType {
	case "quota_exhausted":
		return cooldown.ReasonQuotaExhausted
	case "rate_limit_exceeded":
		return cooldown.ReasonRateLimit
	case "concurrent_limit":
		return cooldown.ReasonConcurrentLimit
	default:
		return cooldown.ReasonRateLimit // Default to rate limit
	}
}

// handleAsyncCooldownUpdate listens for async cooldown updates from providers
func (e *Executor) handleAsyncCooldownUpdate(updateChan chan time.Time, provider *domain.Provider, clientType string) {
	select {
	case newCooldownTime := <-updateChan:
		if !newCooldownTime.IsZero() {
			cooldown.Default().UpdateCooldown(provider.ID, clientType, newCooldownTime)
		}
	case <-time.After(15 * time.Second):
		// Timeout waiting for update
	}
}

// processAdapterEvents drains the event channel and updates attempt record
func (e *Executor) processAdapterEvents(eventChan domain.AdapterEventChan, attempt *domain.ProxyUpstreamAttempt) {
	if eventChan == nil || attempt == nil {
		return
	}

	// Drain all events from channel (non-blocking)
	for {
		select {
		case event, ok := <-eventChan:
			if !ok {
				return // Channel closed
			}
			if event == nil {
				continue
			}

			switch event.Type {
			case domain.EventRequestInfo:
				if event.RequestInfo != nil {
					attempt.RequestInfo = event.RequestInfo
				}
			case domain.EventResponseInfo:
				if event.ResponseInfo != nil {
					attempt.ResponseInfo = event.ResponseInfo
				}
			case domain.EventMetrics:
				if event.Metrics != nil {
					attempt.InputTokenCount = event.Metrics.InputTokens
					attempt.OutputTokenCount = event.Metrics.OutputTokens
					attempt.CacheReadCount = event.Metrics.CacheReadCount
					attempt.CacheWriteCount = event.Metrics.CacheCreationCount
					attempt.Cache5mWriteCount = event.Metrics.Cache5mCreationCount
					attempt.Cache1hWriteCount = event.Metrics.Cache1hCreationCount
				}
			case domain.EventResponseModel:
				if event.ResponseModel != "" {
					attempt.ResponseModel = event.ResponseModel
				}
			case domain.EventFirstToken:
				if event.FirstTokenTime > 0 {
					firstTokenTime := time.UnixMilli(event.FirstTokenTime)
					attempt.TTFT = firstTokenTime.Sub(attempt.StartTime)
				}
			}
		default:
			// No more events
			return
		}
	}
}

// processAdapterEventsRealtime processes events in real-time during adapter execution
// It broadcasts updates immediately when RequestInfo/ResponseInfo are received
func (e *Executor) processAdapterEventsRealtime(eventChan domain.AdapterEventChan, attempt *domain.ProxyUpstreamAttempt, done chan struct{}) {
	defer close(done)

	if eventChan == nil || attempt == nil {
		return
	}

	for event := range eventChan {
		if event == nil {
			continue
		}

		needsBroadcast := false

		switch event.Type {
		case domain.EventRequestInfo:
			if !e.shouldClearRequestDetail() && event.RequestInfo != nil {
				attempt.RequestInfo = event.RequestInfo
				needsBroadcast = true
			}
		case domain.EventResponseInfo:
			if !e.shouldClearRequestDetail() && event.ResponseInfo != nil {
				attempt.ResponseInfo = event.ResponseInfo
				needsBroadcast = true
			}
		case domain.EventMetrics:
			if event.Metrics != nil {
				attempt.InputTokenCount = event.Metrics.InputTokens
				attempt.OutputTokenCount = event.Metrics.OutputTokens
				attempt.CacheReadCount = event.Metrics.CacheReadCount
				attempt.CacheWriteCount = event.Metrics.CacheCreationCount
				attempt.Cache5mWriteCount = event.Metrics.Cache5mCreationCount
				attempt.Cache1hWriteCount = event.Metrics.Cache1hCreationCount
				needsBroadcast = true
			}
		case domain.EventResponseModel:
			if event.ResponseModel != "" {
				attempt.ResponseModel = event.ResponseModel
				needsBroadcast = true
			}
		case domain.EventFirstToken:
			if event.FirstTokenTime > 0 {
				// Calculate TTFT as duration from start time to first token time
				firstTokenTime := time.UnixMilli(event.FirstTokenTime)
				attempt.TTFT = firstTokenTime.Sub(attempt.StartTime)
				needsBroadcast = true
			}
		}

		// Broadcast update immediately for real-time visibility
		if needsBroadcast && e.broadcaster != nil {
			e.broadcaster.BroadcastProxyUpstreamAttempt(attempt)
		}
	}
}

// getRequestDetailRetentionSeconds 获取请求详情保留秒数
// 返回值：-1=永久保存，0=不保存，>0=保留秒数
func (e *Executor) getRequestDetailRetentionSeconds() int {
	if e.settingsRepo == nil {
		return -1 // 默认永久保存
	}
	val, err := e.settingsRepo.Get(domain.SettingKeyRequestDetailRetentionSeconds)
	if err != nil || val == "" {
		return -1 // 默认永久保存
	}
	seconds, err := strconv.Atoi(val)
	if err != nil {
		return -1
	}
	return seconds
}

// shouldClearRequestDetail 检查是否应该立即清理请求详情
// 当设置为 0 时返回 true
func (e *Executor) shouldClearRequestDetail() bool {
	return e.getRequestDetailRetentionSeconds() == 0
}

// getProviderMultiplier 获取 Provider 针对特定 ClientType 的倍率
// 返回 10000 表示 1 倍，15000 表示 1.5 倍
func getProviderMultiplier(provider *domain.Provider, clientType domain.ClientType) uint64 {
	if provider == nil || provider.Config == nil || provider.Config.Custom == nil {
		return 10000 // 默认 1 倍
	}
	if provider.Config.Custom.ClientMultiplier == nil {
		return 10000
	}
	if multiplier, ok := provider.Config.Custom.ClientMultiplier[clientType]; ok && multiplier > 0 {
		return multiplier
	}
	return 10000
}

