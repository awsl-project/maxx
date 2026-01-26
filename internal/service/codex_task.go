package service

import (
	"context"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider/codex"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/event"
	"github.com/awsl-project/maxx/internal/repository"
)

// Default refresh interval for Codex quotas (in minutes)
const defaultCodexQuotaRefreshInterval = 10

// CodexTaskService handles periodic quota refresh and auto-sorting for Codex providers
type CodexTaskService struct {
	providerRepo repository.ProviderRepository
	routeRepo    repository.RouteRepository
	quotaRepo    repository.CodexQuotaRepository
	settingRepo  repository.SystemSettingRepository
	requestRepo  repository.ProxyRequestRepository
	broadcaster  event.Broadcaster
}

// NewCodexTaskService creates a new CodexTaskService
func NewCodexTaskService(
	providerRepo repository.ProviderRepository,
	routeRepo repository.RouteRepository,
	quotaRepo repository.CodexQuotaRepository,
	settingRepo repository.SystemSettingRepository,
	requestRepo repository.ProxyRequestRepository,
	broadcaster event.Broadcaster,
) *CodexTaskService {
	return &CodexTaskService{
		providerRepo: providerRepo,
		routeRepo:    routeRepo,
		quotaRepo:    quotaRepo,
		settingRepo:  settingRepo,
		requestRepo:  requestRepo,
		broadcaster:  broadcaster,
	}
}

// GetRefreshInterval returns the configured refresh interval in minutes (0 = disabled)
func (s *CodexTaskService) GetRefreshInterval() int {
	val, err := s.settingRepo.Get(domain.SettingKeyQuotaRefreshInterval)
	if err != nil || val == "" {
		return defaultCodexQuotaRefreshInterval
	}
	interval, err := strconv.Atoi(val)
	if err != nil {
		return defaultCodexQuotaRefreshInterval
	}
	return interval
}

// RefreshQuotas refreshes all Codex quotas (for periodic auto-refresh)
// Returns true if quotas were refreshed
// Skips refresh if no requests in the last 10 minutes
func (s *CodexTaskService) RefreshQuotas(ctx context.Context) bool {
	// Check if there were any requests in the last 10 minutes
	since := time.Now().Add(-10 * time.Minute)
	hasRecent, err := s.requestRepo.HasRecentRequests(since)
	if err != nil {
		log.Printf("[CodexTask] Failed to check recent requests: %v", err)
	} else if !hasRecent {
		log.Printf("[CodexTask] No requests in the last 10 minutes, skipping quota refresh")
		return false
	}

	refreshed := s.refreshAllQuotas(ctx)
	if refreshed {
		s.broadcaster.BroadcastMessage("codex_quota_updated", nil)

		// Check if auto-sort is enabled
		if s.isAutoSortEnabled() {
			s.autoSortRoutes(ctx)
		}
	}
	return refreshed
}

// ForceRefreshQuotas forces a refresh of all Codex quotas
func (s *CodexTaskService) ForceRefreshQuotas(ctx context.Context) bool {
	refreshed := s.refreshAllQuotas(ctx)
	if refreshed {
		s.broadcaster.BroadcastMessage("codex_quota_updated", nil)

		if s.isAutoSortEnabled() {
			s.autoSortRoutes(ctx)
		}
	}
	return refreshed
}

// SortRoutes manually sorts Codex routes by quota
func (s *CodexTaskService) SortRoutes(ctx context.Context) {
	s.autoSortRoutes(ctx)
}

// isAutoSortEnabled checks if Codex auto-sort is enabled
func (s *CodexTaskService) isAutoSortEnabled() bool {
	val, err := s.settingRepo.Get(domain.SettingKeyAutoSortCodex)
	if err != nil {
		return false
	}
	return val == "true"
}

// refreshAllQuotas refreshes quotas for all Codex providers
func (s *CodexTaskService) refreshAllQuotas(ctx context.Context) bool {
	if s.quotaRepo == nil {
		return false
	}

	providers, err := s.providerRepo.List()
	if err != nil {
		log.Printf("[CodexTask] Failed to list providers: %v", err)
		return false
	}

	refreshedCount := 0
	for _, provider := range providers {
		if provider.Type != "codex" || provider.Config == nil || provider.Config.Codex == nil {
			continue
		}

		config := provider.Config.Codex
		if config.RefreshToken == "" {
			continue
		}

		// Get or refresh access token
		accessToken := config.AccessToken
		if accessToken == "" || s.isTokenExpired(config.ExpiresAt) {
			tokenResp, err := codex.RefreshAccessToken(ctx, config.RefreshToken)
			if err != nil {
				log.Printf("[CodexTask] Failed to refresh token for provider %d: %v", provider.ID, err)
				continue
			}
			accessToken = tokenResp.AccessToken

			// Update provider config
			config.AccessToken = tokenResp.AccessToken
			config.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Format(time.RFC3339)
			if tokenResp.RefreshToken != "" && tokenResp.RefreshToken != config.RefreshToken {
				config.RefreshToken = tokenResp.RefreshToken
			}
			_ = s.providerRepo.Update(provider)
		}

		// Fetch quota
		usage, err := codex.FetchUsage(ctx, accessToken, config.AccountID)
		if err != nil {
			log.Printf("[CodexTask] Failed to fetch usage for provider %d: %v", provider.ID, err)
			// Mark as forbidden if 403 error
			if strings.Contains(err.Error(), "403") {
				s.saveQuotaToDB(config.Email, config.AccountID, config.PlanType, nil, true)
			}
			continue
		}

		// Save to database
		s.saveQuotaToDB(config.Email, config.AccountID, usage.PlanType, usage, false)
		refreshedCount++
	}

	if refreshedCount > 0 {
		log.Printf("[CodexTask] Refreshed quotas for %d providers", refreshedCount)
		return true
	}
	return false
}

// isTokenExpired checks if the access token is expired or about to expire
func (s *CodexTaskService) isTokenExpired(expiresAt string) bool {
	if expiresAt == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return true
	}
	return time.Now().After(t.Add(-60 * time.Second))
}

// saveQuotaToDB saves Codex quota to database
func (s *CodexTaskService) saveQuotaToDB(email, accountID, planType string, usage *codex.CodexUsageResponse, isForbidden bool) {
	if s.quotaRepo == nil || email == "" {
		return
	}

	quota := &domain.CodexQuota{
		Email:       email,
		AccountID:   accountID,
		PlanType:    planType,
		IsForbidden: isForbidden,
	}

	if usage != nil {
		if usage.RateLimit != nil {
			quota.PrimaryWindow = convertCodexWindow(usage.RateLimit.PrimaryWindow)
			quota.SecondaryWindow = convertCodexWindow(usage.RateLimit.SecondaryWindow)
		}
		if usage.CodeReviewRateLimit != nil {
			quota.CodeReviewWindow = convertCodexWindow(usage.CodeReviewRateLimit.PrimaryWindow)
		}
	}

	s.quotaRepo.Upsert(quota)
}

// convertCodexWindow converts codex package window to domain window
func convertCodexWindow(w *codex.CodexUsageWindow) *domain.CodexQuotaWindow {
	if w == nil {
		return nil
	}
	return &domain.CodexQuotaWindow{
		UsedPercent:        w.UsedPercent,
		LimitWindowSeconds: w.LimitWindowSeconds,
		ResetAfterSeconds:  w.ResetAfterSeconds,
		ResetAt:            w.ResetAt,
	}
}

// autoSortRoutes sorts Codex routes by quota for all scopes
func (s *CodexTaskService) autoSortRoutes(ctx context.Context) {
	log.Printf("[CodexTask] Starting auto-sort")

	routes, err := s.routeRepo.List()
	if err != nil {
		log.Printf("[CodexTask] Failed to list routes: %v", err)
		return
	}

	providers, err := s.providerRepo.List()
	if err != nil {
		log.Printf("[CodexTask] Failed to list providers: %v", err)
		return
	}

	providerMap := make(map[uint64]*domain.Provider)
	codexCount := 0
	for _, p := range providers {
		providerMap[p.ID] = p
		if p.Type == "codex" {
			codexCount++
		}
	}
	log.Printf("[CodexTask] Found %d Codex providers, %d total routes", codexCount, len(routes))

	if s.quotaRepo == nil {
		log.Printf("[CodexTask] Codex quota repository not initialized")
		return
	}

	quotas, err := s.quotaRepo.List()
	if err != nil {
		log.Printf("[CodexTask] Failed to list quotas: %v", err)
		return
	}
	log.Printf("[CodexTask] Found %d quotas in database", len(quotas))

	quotaByEmail := make(map[string]*domain.CodexQuota)
	for _, q := range quotas {
		quotaByEmail[q.Email] = q
	}

	// Collect all unique scopes
	type scope struct {
		clientType domain.ClientType
		projectID  uint64
	}
	scopes := make(map[scope]bool)
	for _, r := range routes {
		scopes[scope{r.ClientType, r.ProjectID}] = true
	}

	var allUpdates []domain.RoutePositionUpdate
	for sc := range scopes {
		updates := s.sortRoutesForScope(routes, providerMap, quotaByEmail, sc.clientType, sc.projectID)
		allUpdates = append(allUpdates, updates...)
	}

	if len(allUpdates) > 0 {
		if err := s.routeRepo.BatchUpdatePositions(allUpdates); err != nil {
			log.Printf("[CodexTask] Failed to update route positions: %v", err)
			return
		}
		log.Printf("[CodexTask] Auto-sorted %d routes", len(allUpdates))
		s.broadcaster.BroadcastMessage("routes_updated", nil)
	}
}

// sortRoutesForScope sorts Codex routes within a scope
// Sorts by: 1) resetTime ascending (earliest reset = highest priority)
// If no resetTime, uses remaining percentage (higher remaining = higher priority)
func (s *CodexTaskService) sortRoutesForScope(
	routes []*domain.Route,
	providerMap map[uint64]*domain.Provider,
	quotaByEmail map[string]*domain.CodexQuota,
	clientType domain.ClientType,
	projectID uint64,
) []domain.RoutePositionUpdate {
	// Filter routes for this scope
	var scopeRoutes []*domain.Route
	for _, r := range routes {
		if r.ClientType == clientType && r.ProjectID == projectID {
			scopeRoutes = append(scopeRoutes, r)
		}
	}

	if len(scopeRoutes) == 0 {
		return nil
	}

	// Sort by current position
	sort.Slice(scopeRoutes, func(i, j int) bool {
		return scopeRoutes[i].Position < scopeRoutes[j].Position
	})

	// Collect Codex routes and their sort keys
	type codexRoute struct {
		route            *domain.Route
		index            int
		resetTime        *time.Time
		remainingPercent *float64
	}
	var codexRoutes []codexRoute

	for i, r := range scopeRoutes {
		provider := providerMap[r.ProviderID]
		if provider == nil || provider.Type != "codex" {
			continue
		}

		var resetTime *time.Time
		var remainingPercent *float64

		if provider.Config != nil && provider.Config.Codex != nil {
			email := provider.Config.Codex.Email
			if quota := quotaByEmail[email]; quota != nil && !quota.IsForbidden {
				resetTime, remainingPercent = s.getSortKey(quota)
			}
		}

		codexRoutes = append(codexRoutes, codexRoute{
			route:            r,
			index:            i,
			resetTime:        resetTime,
			remainingPercent: remainingPercent,
		})
	}

	if len(codexRoutes) <= 1 {
		return nil
	}

	// Save original order
	originalOrder := make([]uint64, len(codexRoutes))
	for i, cr := range codexRoutes {
		originalOrder[i] = cr.route.ID
	}

	// Sort Codex routes:
	// 1. Routes with resetTime: sort by resetTime ascending (earlier reset = higher priority)
	// 2. Routes without resetTime: sort by remaining percentage descending (higher remaining = higher priority)
	// 3. Routes with forbidden/no quota: go to end
	sort.Slice(codexRoutes, func(i, j int) bool {
		a, b := codexRoutes[i], codexRoutes[j]

		// Both have resetTime - sort by time (earlier = higher priority)
		if a.resetTime != nil && b.resetTime != nil {
			return a.resetTime.Before(*b.resetTime)
		}
		// Only a has resetTime - a has higher priority
		if a.resetTime != nil && b.resetTime == nil {
			return true
		}
		// Only b has resetTime - b has higher priority
		if a.resetTime == nil && b.resetTime != nil {
			return false
		}

		// Neither has resetTime - sort by remaining percentage
		// Higher remaining = higher priority
		if a.remainingPercent != nil && b.remainingPercent != nil {
			return *a.remainingPercent > *b.remainingPercent
		}
		if a.remainingPercent != nil && b.remainingPercent == nil {
			return true
		}
		if a.remainingPercent == nil && b.remainingPercent != nil {
			return false
		}

		return false
	})

	// Check if order changed
	needsReorder := false
	for i, cr := range codexRoutes {
		if cr.route.ID != originalOrder[i] {
			needsReorder = true
			break
		}
	}

	if !needsReorder {
		return nil
	}

	// Build new route order
	newScopeRoutes := make([]*domain.Route, len(scopeRoutes))
	copy(newScopeRoutes, scopeRoutes)

	originalIndices := make([]int, len(codexRoutes))
	for i, cr := range codexRoutes {
		originalIndices[i] = cr.index
	}
	sort.Ints(originalIndices)

	// Place sorted routes into original positions
	for i, idx := range originalIndices {
		newScopeRoutes[idx] = codexRoutes[i].route
	}

	// Generate position updates
	var updates []domain.RoutePositionUpdate
	for i, r := range newScopeRoutes {
		newPosition := i + 1
		if r.Position != newPosition {
			updates = append(updates, domain.RoutePositionUpdate{
				ID:       r.ID,
				Position: newPosition,
			})
		}
	}

	return updates
}

// getSortKey extracts sort key from Codex quota
// Returns (resetTime, remainingPercent)
func (s *CodexTaskService) getSortKey(quota *domain.CodexQuota) (*time.Time, *float64) {
	if quota == nil || quota.IsForbidden {
		return nil, nil
	}

	// Use primary window (5h limit) for sorting
	if quota.PrimaryWindow == nil {
		return nil, nil
	}

	var resetTime *time.Time
	var remainingPercent *float64

	// Calculate reset time
	if quota.PrimaryWindow.ResetAt != nil && *quota.PrimaryWindow.ResetAt > 0 {
		t := time.Unix(*quota.PrimaryWindow.ResetAt, 0)
		resetTime = &t
	} else if quota.PrimaryWindow.ResetAfterSeconds != nil && *quota.PrimaryWindow.ResetAfterSeconds > 0 {
		t := time.Now().Add(time.Duration(*quota.PrimaryWindow.ResetAfterSeconds) * time.Second)
		resetTime = &t
	}

	// Calculate remaining percentage
	if quota.PrimaryWindow.UsedPercent != nil {
		remaining := 100.0 - *quota.PrimaryWindow.UsedPercent
		if remaining < 0 {
			remaining = 0
		}
		remainingPercent = &remaining
	}

	return resetTime, remainingPercent
}
