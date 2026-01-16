package executor

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider/antigravity"
	"github.com/awsl-project/maxx/internal/cooldown"
	ctxutil "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/event"
	"github.com/awsl-project/maxx/internal/repository"
	"github.com/awsl-project/maxx/internal/router"
	"github.com/awsl-project/maxx/internal/usage"
	"github.com/awsl-project/maxx/internal/waiter"
)

// Executor handles request execution with retry logic
type Executor struct {
	router           *router.Router
	proxyRequestRepo repository.ProxyRequestRepository
	attemptRepo      repository.ProxyUpstreamAttemptRepository
	retryConfigRepo  repository.RetryConfigRepository
	sessionRepo      repository.SessionRepository
	broadcaster      event.Broadcaster
	projectWaiter    *waiter.ProjectWaiter
	instanceID       string
}

// NewExecutor creates a new executor
func NewExecutor(
	r *router.Router,
	prr repository.ProxyRequestRepository,
	ar repository.ProxyUpstreamAttemptRepository,
	rcr repository.RetryConfigRepository,
	sessionRepo repository.SessionRepository,
	bc event.Broadcaster,
	projectWaiter *waiter.ProjectWaiter,
	instanceID string,
) *Executor {
	return &Executor{
		router:           r,
		proxyRequestRepo: prr,
		attemptRepo:      ar,
		retryConfigRepo:  rcr,
		sessionRepo:      sessionRepo,
		broadcaster:      bc,
		projectWaiter:    projectWaiter,
		instanceID:       instanceID,
	}
}

// Execute handles the proxy request with routing and retry logic
func (e *Executor) Execute(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	clientType := ctxutil.GetClientType(ctx)
	projectID := ctxutil.GetProjectID(ctx)
	sessionID := ctxutil.GetSessionID(ctx)
	requestModel := ctxutil.GetRequestModel(ctx)
	isStream := ctxutil.GetIsStream(ctx)

	log.Printf("[Executor] clientType=%s, projectID=%d, model=%s, isStream=%v", clientType, projectID, requestModel, isStream)

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

	// Capture client's original request info
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
	routes, err := e.router.Match(clientType, projectID)
	if err != nil {
		log.Printf("[Executor] Route match error: %v", err)
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

	log.Printf("[Executor] Matched %d routes", len(routes))

	if len(routes) == 0 {
		log.Printf("[Executor] No routes configured for clientType=%s, projectID=%d", clientType, projectID)
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
				proxyReq.Error = "client disconnected"
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
	for routeIdx, matchedRoute := range routes {
		log.Printf("[Executor] Trying route %d/%d: routeID=%d, providerID=%d, provider=%s",
			routeIdx+1, len(routes), matchedRoute.Route.ID, matchedRoute.Provider.ID, matchedRoute.Provider.Name)

		// Check context before starting new route
		if ctx.Err() != nil {
			log.Printf("[Executor] Context cancelled before route %d", routeIdx+1)
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
		mappedModel := e.mapModel(requestModel, matchedRoute.Route, matchedRoute.Provider)
		ctx = ctxutil.WithMappedModel(ctx, mappedModel)

		// Get retry config
		retryConfig := e.getRetryConfig(matchedRoute.RetryConfig)

		// Execute with retries
		for attempt := 0; attempt <= retryConfig.MaxRetries; attempt++ {
			// Check context before each attempt
			if ctx.Err() != nil {
				return ctx.Err()
			}

			// Create attempt record with start time
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
			}
			log.Printf("[Executor] Creating attempt for route %d, attempt %d (proxyRequestID=%d, routeID=%d, providerID=%d)",
				routeIdx+1, attempt+1, proxyReq.ID, matchedRoute.Route.ID, matchedRoute.Provider.ID)
			if err := e.attemptRepo.Create(attemptRecord); err != nil {
				log.Printf("[Executor] Failed to create attempt record: %v", err)
			} else {
				log.Printf("[Executor] Created attempt record with ID=%d", attemptRecord.ID)
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

			// Wrap ResponseWriter to capture actual client response
			responseCapture := NewResponseCapture(w)

			// Execute request
			log.Printf("[Executor] Route %d, attempt %d: executing...", routeIdx+1, attempt+1)
			err := matchedRoute.ProviderAdapter.Execute(attemptCtx, responseCapture, req, matchedRoute.Provider)
			if err == nil {
				// Success - set end time and duration
				attemptRecord.EndTime = time.Now()
				attemptRecord.Duration = attemptRecord.EndTime.Sub(attemptRecord.StartTime)
				log.Printf("[Executor] Route %d, attempt %d: SUCCESS", routeIdx+1, attempt+1)
				attemptRecord.Status = "COMPLETED"
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
				proxyReq.ResponseModel = mappedModel // Record the actual model used

				// Capture actual client response (what was sent to client, e.g. Claude format)
				// This is different from attemptRecord.ResponseInfo which is upstream response (Gemini format)
				proxyReq.ResponseInfo = &domain.ResponseInfo{
					Status:  responseCapture.StatusCode(),
					Headers: responseCapture.CapturedHeaders(),
					Body:    responseCapture.Body(),
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
			log.Printf("[Executor] Route %d, attempt %d: FAILED - %v", routeIdx+1, attempt+1, err)
			lastErr = err

			// Update attempt status first (before checking context)
			if ctx.Err() != nil {
				attemptRecord.Status = "CANCELLED"
			} else {
				attemptRecord.Status = "FAILED"
			}
			_ = e.attemptRepo.Update(attemptRecord)
			if e.broadcaster != nil {
				e.broadcaster.BroadcastProxyUpstreamAttempt(attemptRecord)
			}
			currentAttempt = nil // Clear so defer doesn't double update

			// Update proxyReq with latest attempt info (even on failure)
			proxyReq.FinalProxyUpstreamAttemptID = attemptRecord.ID

			// Capture actual client response (even on failure, if any response was sent)
			if responseCapture.Body() != "" {
				proxyReq.ResponseInfo = &domain.ResponseInfo{
					Status:  responseCapture.StatusCode(),
					Headers: responseCapture.CapturedHeaders(),
					Body:    responseCapture.Body(),
				}
				proxyReq.StatusCode = responseCapture.StatusCode()

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

			_ = e.proxyRequestRepo.Update(proxyReq)
			if e.broadcaster != nil {
				e.broadcaster.BroadcastProxyRequest(proxyReq)
			}

			// Check if it's a context cancellation (client disconnect)
			if ctx.Err() != nil {
				// Set final status before returning to ensure it's persisted
				// (defer block also handles this, but we want to be explicit and broadcast immediately)
				proxyReq.Status = "CANCELLED"
				proxyReq.EndTime = time.Now()
				proxyReq.Duration = proxyReq.EndTime.Sub(proxyReq.StartTime)
				proxyReq.Error = "client disconnected"
				_ = e.proxyRequestRepo.Update(proxyReq)
				if e.broadcaster != nil {
					e.broadcaster.BroadcastProxyRequest(proxyReq)
				}
				log.Printf("[Executor] Context cancelled, stopping")
				return ctx.Err()
			}

			// Check if retryable
			proxyErr, ok := err.(*domain.ProxyError)
			if !ok {
				log.Printf("[Executor] Error is not ProxyError (type=%T), moving to next route", err)
				break // Move to next route
			}

			// Handle cooldown (unified cooldown logic for all providers)
			e.handleCooldown(attemptCtx, proxyErr, matchedRoute.Provider)

			if !proxyErr.Retryable {
				log.Printf("[Executor] Error is not retryable, moving to next route")
				break // Move to next route
			}
			log.Printf("[Executor] Error is retryable, will retry on same route")

			// Wait before retry (unless last attempt)
			if attempt < retryConfig.MaxRetries {
				waitTime := e.calculateBackoff(retryConfig, attempt)
				if proxyErr.RetryAfter > 0 {
					waitTime = proxyErr.RetryAfter
				}
				log.Printf("[Executor] Waiting %v before retry", waitTime)
				select {
				case <-ctx.Done():
					// Set final status before returning
					proxyReq.Status = "CANCELLED"
					proxyReq.EndTime = time.Now()
					proxyReq.Duration = proxyReq.EndTime.Sub(proxyReq.StartTime)
					proxyReq.Error = "client disconnected during retry wait"
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
		log.Printf("[Executor] Route %d exhausted (retries: %d), moving to next route if available",
			routeIdx+1, retryConfig.MaxRetries)
	}

	log.Printf("[Executor] All %d routes exhausted, request failed", len(routes))
	// All routes failed
	proxyReq.Status = "FAILED"
	proxyReq.EndTime = time.Now()
	proxyReq.Duration = proxyReq.EndTime.Sub(proxyReq.StartTime)
	if lastErr != nil {
		proxyReq.Error = lastErr.Error()
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

func (e *Executor) mapModel(requestModel string, route *domain.Route, provider *domain.Provider) string {
	log.Printf("[ModelMapping] Input: requestModel=%q, routeID=%d, providerID=%d", requestModel, route.ID, provider.ID)

	// Route mapping takes precedence (supports wildcard patterns)
	if route.ModelMapping != nil {
		log.Printf("[ModelMapping] Route has mapping: %v", route.ModelMapping)
		if mapped := matchModelMapping(requestModel, route.ModelMapping); mapped != "" {
			log.Printf("[ModelMapping] Route mapping matched: %q -> %q", requestModel, mapped)
			return mapped
		}
		log.Printf("[ModelMapping] Route mapping: no match")
	} else {
		log.Printf("[ModelMapping] Route has no mapping")
	}

	// Provider mapping (supports wildcard patterns)
	if provider.Config != nil {
		if provider.Config.Custom != nil && provider.Config.Custom.ModelMapping != nil {
			log.Printf("[ModelMapping] Provider Custom has mapping: %v", provider.Config.Custom.ModelMapping)
			if mapped := matchModelMapping(requestModel, provider.Config.Custom.ModelMapping); mapped != "" {
				log.Printf("[ModelMapping] Provider Custom mapping matched: %q -> %q", requestModel, mapped)
				return mapped
			}
			log.Printf("[ModelMapping] Provider Custom mapping: no match")
		}
		if provider.Config.Antigravity != nil && provider.Config.Antigravity.ModelMapping != nil {
			log.Printf("[ModelMapping] Provider Antigravity has mapping: %v", provider.Config.Antigravity.ModelMapping)
			if mapped := matchModelMapping(requestModel, provider.Config.Antigravity.ModelMapping); mapped != "" {
				log.Printf("[ModelMapping] Provider Antigravity mapping matched: %q -> %q", requestModel, mapped)
				return mapped
			}
			log.Printf("[ModelMapping] Provider Antigravity mapping: no match")
		}
	} else {
		log.Printf("[ModelMapping] Provider has no config")
	}

	// Global Antigravity model mapping rules (lowest priority fallback)
	// This applies the global settings configured in Settings page
	if globalSettings := antigravity.GetGlobalSettings(); globalSettings != nil {
		log.Printf("[ModelMapping] Global settings found, rules count: %d", len(globalSettings.ModelMappingRules))
		if len(globalSettings.ModelMappingRules) > 0 {
			for i, rule := range globalSettings.ModelMappingRules {
				log.Printf("[ModelMapping] Global rule[%d]: pattern=%q, target=%q", i, rule.Pattern, rule.Target)
			}
			if mapped := antigravity.MatchRulesInOrder(requestModel, globalSettings.ModelMappingRules); mapped != "" {
				log.Printf("[ModelMapping] Global mapping matched: %q -> %q", requestModel, mapped)
				return mapped
			}
			log.Printf("[ModelMapping] Global mapping: no match")
		}
	} else {
		log.Printf("[ModelMapping] No global settings found")
	}

	// Fallback to default model mapping rules
	defaultRules := antigravity.GetDefaultModelMappingRules()
	log.Printf("[ModelMapping] Trying default rules, count: %d", len(defaultRules))
	if mapped := antigravity.MatchRulesInOrder(requestModel, defaultRules); mapped != "" {
		log.Printf("[ModelMapping] Default mapping matched: %q -> %q", requestModel, mapped)
		return mapped
	}

	// No mapping, use original
	log.Printf("[ModelMapping] No mapping found, using original: %q", requestModel)
	return requestModel
}

// matchModelMapping matches requestModel against mapping rules with wildcard support
// Returns the mapped model or empty string if no match
func matchModelMapping(requestModel string, mapping map[string]string) string {
	// First try exact match (fast path)
	if mapped, ok := mapping[requestModel]; ok {
		log.Printf("[matchModelMapping] Exact match: %q -> %q", requestModel, mapped)
		return mapped
	}

	// Then try wildcard patterns
	for pattern, target := range mapping {
		if strings.Contains(pattern, "*") {
			matched := matchWildcard(pattern, requestModel)
			log.Printf("[matchModelMapping] Wildcard check: pattern=%q, input=%q, matched=%v", pattern, requestModel, matched)
			if matched {
				return target
			}
		}
	}

	return ""
}

// matchWildcard checks if input matches a wildcard pattern
// Supports * as wildcard matching any characters
// Examples:
//   - "*sonnet*" matches "claude-sonnet-4-20250514"
//   - "gpt-4*" matches "gpt-4-turbo"
//   - "*-20241022" matches "claude-3-5-sonnet-20241022"
func matchWildcard(pattern, input string) bool {
	// Simple cases
	if pattern == "*" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return pattern == input
	}

	parts := strings.Split(pattern, "*")

	// Handle prefix-only pattern: "prefix*"
	if len(parts) == 2 && parts[1] == "" {
		return strings.HasPrefix(input, parts[0])
	}

	// Handle suffix-only pattern: "*suffix"
	if len(parts) == 2 && parts[0] == "" {
		return strings.HasSuffix(input, parts[1])
	}

	// Handle patterns with multiple wildcards
	pos := 0
	for i, part := range parts {
		if part == "" {
			continue
		}

		idx := strings.Index(input[pos:], part)
		if idx < 0 {
			return false
		}

		// First part must be at the beginning if pattern doesn't start with *
		if i == 0 && idx != 0 {
			return false
		}

		pos += idx + len(part)
	}

	// Last part must be at the end if pattern doesn't end with *
	if parts[len(parts)-1] != "" && !strings.HasSuffix(input, parts[len(parts)-1]) {
		return false
	}

	return true
}

func (e *Executor) getRetryConfig(config *domain.RetryConfig) *domain.RetryConfig {
	if config != nil {
		log.Printf("[Executor] Using provided retry config: MaxRetries=%d", config.MaxRetries)
		return config
	}

	// Get default config
	defaultConfig, err := e.retryConfigRepo.GetDefault()
	if err == nil && defaultConfig != nil {
		log.Printf("[Executor] Using default retry config: MaxRetries=%d", defaultConfig.MaxRetries)
		return defaultConfig
	}

	log.Printf("[Executor] No retry config found (err=%v), using MaxRetries=0", err)
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
	// Fallback to current request's clientType if not specified
	if clientType == "" {
		clientType = string(ctxutil.GetClientType(ctx))
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
	until := cooldown.Default().RecordFailure(provider.ID, clientType, reason, explicitUntil)

	// If there's an async update channel, listen for updates
	if proxyErr.CooldownUpdateChan != nil {
		go e.handleAsyncCooldownUpdate(proxyErr.CooldownUpdateChan, provider, clientType, reason)
	}

	clientTypeDesc := clientType
	if clientTypeDesc == "" {
		clientTypeDesc = "all types"
	}

	explicitStr := "policy-based"
	if explicitUntil != nil {
		explicitStr = "explicit from API"
	}

	log.Printf("[Executor] Provider %d (%s): Cooldown until %s for clientType=%s (reason=%s, source=%s)",
		provider.ID, provider.Name, until.Format("2006-01-02 15:04:05"), clientTypeDesc, reason, explicitStr)
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
func (e *Executor) handleAsyncCooldownUpdate(updateChan chan time.Time, provider *domain.Provider, clientType string, reason cooldown.CooldownReason) {
	select {
	case newCooldownTime := <-updateChan:
		if !newCooldownTime.IsZero() {
			cooldown.Default().UpdateCooldown(provider.ID, clientType, newCooldownTime)
			clientTypeDesc := clientType
			if clientTypeDesc == "" {
				clientTypeDesc = "all types"
			}
			log.Printf("[Executor] Provider %d (%s): Updated cooldown to %s for clientType=%s (async update, reason=%s)",
				provider.ID, provider.Name, newCooldownTime.Format("2006-01-02 15:04:05"), clientTypeDesc, reason)
		}
	case <-time.After(15 * time.Second):
		// Timeout waiting for update
		log.Printf("[Executor] Provider %d (%s): Async cooldown update timed out", provider.ID, provider.Name)
	}
}

