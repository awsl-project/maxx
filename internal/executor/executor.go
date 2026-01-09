package executor

import (
	"context"
	"log"
	"net/http"
	"time"

	ctxutil "github.com/Bowl42/maxx-next/internal/context"
	"github.com/Bowl42/maxx-next/internal/domain"
	"github.com/Bowl42/maxx-next/internal/event"
	"github.com/Bowl42/maxx-next/internal/repository"
	"github.com/Bowl42/maxx-next/internal/router"
)

// Executor handles request execution with retry logic
type Executor struct {
	router           *router.Router
	proxyRequestRepo repository.ProxyRequestRepository
	attemptRepo      repository.ProxyUpstreamAttemptRepository
	retryConfigRepo  repository.RetryConfigRepository
	broadcaster      event.Broadcaster
	instanceID       string
}

// NewExecutor creates a new executor
func NewExecutor(
	r *router.Router,
	prr repository.ProxyRequestRepository,
	ar repository.ProxyUpstreamAttemptRepository,
	rcr repository.RetryConfigRepository,
	bc event.Broadcaster,
	instanceID string,
) *Executor {
	return &Executor{
		router:           r,
		proxyRequestRepo: prr,
		attemptRepo:      ar,
		retryConfigRepo:  rcr,
		broadcaster:      bc,
		instanceID:       instanceID,
	}
}

// Execute handles the proxy request with routing and retry logic
func (e *Executor) Execute(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
	clientType := ctxutil.GetClientType(ctx)
	projectID := ctxutil.GetProjectID(ctx)
	requestModel := ctxutil.GetRequestModel(ctx)

	log.Printf("[Executor] clientType=%s, projectID=%d, model=%s", clientType, projectID, requestModel)

	// Match routes
	routes, err := e.router.Match(clientType, projectID)
	if err != nil {
		log.Printf("[Executor] Route match error: %v", err)
		return domain.NewProxyErrorWithMessage(domain.ErrNoRoutes, false, "no routes available")
	}

	log.Printf("[Executor] Matched %d routes", len(routes))

	if len(routes) == 0 {
		log.Printf("[Executor] No routes configured for clientType=%s, projectID=%d", clientType, projectID)
		return domain.NewProxyErrorWithMessage(domain.ErrNoRoutes, false, "no routes configured")
	}

	// Create proxy request record
	proxyReq := &domain.ProxyRequest{
		InstanceID:   e.instanceID,
		RequestID:    generateRequestID(),
		SessionID:    ctxutil.GetSessionID(ctx),
		ClientType:   clientType,
		RequestModel: requestModel,
		StartTime:    time.Now(),
		Status:       "IN_PROGRESS",
	}
	if err := e.proxyRequestRepo.Create(proxyReq); err != nil {
		// Log but continue
	}
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

			// Create attempt record
			attemptRecord := &domain.ProxyUpstreamAttempt{
				ProxyRequestID: proxyReq.ID,
				RouteID:        matchedRoute.Route.ID,
				ProviderID:     matchedRoute.Provider.ID,
				Status:         "IN_PROGRESS",
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

			// Execute request
			log.Printf("[Executor] Route %d, attempt %d: executing...", routeIdx+1, attempt+1)
			err := matchedRoute.ProviderAdapter.Execute(attemptCtx, w, req, matchedRoute.Provider)
			if err == nil {
				// Success
				log.Printf("[Executor] Route %d, attempt %d: SUCCESS", routeIdx+1, attempt+1)
				attemptRecord.Status = "COMPLETED"
				_ = e.attemptRepo.Update(attemptRecord)
				if e.broadcaster != nil {
					e.broadcaster.BroadcastProxyUpstreamAttempt(attemptRecord)
				}
				currentAttempt = nil // Clear so defer doesn't update

				proxyReq.Status = "COMPLETED"
				proxyReq.EndTime = time.Now()
				proxyReq.Duration = proxyReq.EndTime.Sub(proxyReq.StartTime)
				proxyReq.FinalProxyUpstreamAttemptID = attemptRecord.ID

				// Copy token usage from successful attempt to request
				proxyReq.InputTokenCount = attemptRecord.InputTokenCount
				proxyReq.OutputTokenCount = attemptRecord.OutputTokenCount
				proxyReq.CacheReadCount = attemptRecord.CacheReadCount
				proxyReq.CacheWriteCount = attemptRecord.CacheWriteCount
				proxyReq.Cache5mWriteCount = attemptRecord.Cache5mWriteCount
				proxyReq.Cache1hWriteCount = attemptRecord.Cache1hWriteCount
				proxyReq.Cost = attemptRecord.Cost

				// Copy response info if not already set
				if proxyReq.ResponseInfo == nil && attemptRecord.ResponseInfo != nil {
					proxyReq.ResponseInfo = attemptRecord.ResponseInfo
				}

				_ = e.proxyRequestRepo.Update(proxyReq)

				// Broadcast to WebSocket clients
				if e.broadcaster != nil {
					e.broadcaster.BroadcastProxyRequest(proxyReq)
				}

				return nil
			}

			// Handle error
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
			proxyReq.InputTokenCount = attemptRecord.InputTokenCount
			proxyReq.OutputTokenCount = attemptRecord.OutputTokenCount
			proxyReq.CacheReadCount = attemptRecord.CacheReadCount
			proxyReq.CacheWriteCount = attemptRecord.CacheWriteCount
			proxyReq.Cache5mWriteCount = attemptRecord.Cache5mWriteCount
			proxyReq.Cache1hWriteCount = attemptRecord.Cache1hWriteCount
			proxyReq.Cost = attemptRecord.Cost
			if attemptRecord.ResponseInfo != nil {
				proxyReq.ResponseInfo = attemptRecord.ResponseInfo
			}
			_ = e.proxyRequestRepo.Update(proxyReq)
			if e.broadcaster != nil {
				e.broadcaster.BroadcastProxyRequest(proxyReq)
			}

			// Check if it's a context cancellation (client disconnect)
			if ctx.Err() != nil {
				log.Printf("[Executor] Context cancelled, stopping")
				return ctx.Err()
			}

			// Check if retryable
			proxyErr, ok := err.(*domain.ProxyError)
			if !ok {
				log.Printf("[Executor] Error is not ProxyError (type=%T), moving to next route", err)
				break // Move to next route
			}
			if !proxyErr.Retryable {
				log.Printf("[Executor] Error is not retryable, moving to next route")
				break // Move to next route
			}
			log.Printf("[Executor] Error is retryable, will retry on same route")

			// Wait before retry (unless last attempt)
			if attempt < retryConfig.MaxRetries {
				waitTime := e.calculateBackoff(retryConfig, attempt)
				log.Printf("[Executor] Waiting %v before retry", waitTime)
				select {
				case <-ctx.Done():
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
	// Route mapping takes precedence
	if route.ModelMapping != nil {
		if mapped, ok := route.ModelMapping[requestModel]; ok {
			return mapped
		}
	}

	// Provider mapping
	if provider.Config != nil {
		if provider.Config.Custom != nil && provider.Config.Custom.ModelMapping != nil {
			if mapped, ok := provider.Config.Custom.ModelMapping[requestModel]; ok {
				return mapped
			}
		}
		if provider.Config.Antigravity != nil && provider.Config.Antigravity.ModelMapping != nil {
			if mapped, ok := provider.Config.Antigravity.ModelMapping[requestModel]; ok {
				return mapped
			}
		}
	}

	// No mapping, use original
	return requestModel
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
