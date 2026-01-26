package service

import (
	"context"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider/antigravity"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/event"
	"github.com/awsl-project/maxx/internal/repository"
)

const (
	defaultQuotaRefreshInterval = 0 // 默认不自动刷新
)

// AntigravityTaskService handles periodic quota refresh and auto-sorting
type AntigravityTaskService struct {
	providerRepo      repository.ProviderRepository
	routeRepo         repository.RouteRepository
	quotaRepo         repository.AntigravityQuotaRepository
	settingRepo       repository.SystemSettingRepository
	requestRepo       repository.ProxyRequestRepository
	broadcaster       event.Broadcaster
}

// NewAntigravityTaskService creates a new AntigravityTaskService
func NewAntigravityTaskService(
	providerRepo repository.ProviderRepository,
	routeRepo repository.RouteRepository,
	quotaRepo repository.AntigravityQuotaRepository,
	settingRepo repository.SystemSettingRepository,
	requestRepo repository.ProxyRequestRepository,
	broadcaster event.Broadcaster,
) *AntigravityTaskService {
	return &AntigravityTaskService{
		providerRepo:   providerRepo,
		routeRepo:      routeRepo,
		quotaRepo:      quotaRepo,
		settingRepo:    settingRepo,
		requestRepo:    requestRepo,
		broadcaster:    broadcaster,
	}
}

// GetRefreshInterval returns the configured refresh interval in minutes (0 = disabled)
func (s *AntigravityTaskService) GetRefreshInterval() int {
	val, err := s.settingRepo.Get(domain.SettingKeyQuotaRefreshInterval)
	if err != nil || val == "" {
		return defaultQuotaRefreshInterval
	}
	interval, err := strconv.Atoi(val)
	if err != nil {
		return defaultQuotaRefreshInterval
	}
	return interval
}

// RefreshQuotas refreshes all Antigravity quotas (for periodic auto-refresh)
// Returns true if quotas were refreshed
// Skips refresh if no requests in the last 10 minutes
func (s *AntigravityTaskService) RefreshQuotas(ctx context.Context) bool {
	// Check if there were any requests in the last 10 minutes
	since := time.Now().Add(-10 * time.Minute)
	hasRecent, err := s.requestRepo.HasRecentRequests(since)
	if err != nil {
		log.Printf("[AntigravityTask] Failed to check recent requests: %v", err)
		// Continue with refresh on error
	} else if !hasRecent {
		log.Printf("[AntigravityTask] No requests in the last 10 minutes, skipping quota refresh")
		return false
	}

	// Refresh quotas
	refreshed := s.refreshAllQuotas(ctx)
	if refreshed {
		// Broadcast quota updated message
		s.broadcaster.BroadcastMessage("quota_updated", nil)

		// Check if auto-sort is enabled
		autoSortEnabled := s.isAutoSortEnabled()
		log.Printf("[AntigravityTask] Auto-sort enabled: %v", autoSortEnabled)
		if autoSortEnabled {
			s.autoSortAntigravityRoutes(ctx)
		}
	}

	return refreshed
}

// ForceRefreshQuotas forces a refresh of all Antigravity quotas
func (s *AntigravityTaskService) ForceRefreshQuotas(ctx context.Context) bool {
	refreshed := s.refreshAllQuotas(ctx)
	if refreshed {
		// Broadcast quota updated message
		s.broadcaster.BroadcastMessage("quota_updated", nil)

		// Check if auto-sort is enabled
		autoSortEnabled := s.isAutoSortEnabled()
		log.Printf("[AntigravityTask] Auto-sort enabled: %v", autoSortEnabled)
		if autoSortEnabled {
			s.autoSortAntigravityRoutes(ctx)
		}
	}
	return refreshed
}

// SortRoutes manually sorts Antigravity routes by resetTime
func (s *AntigravityTaskService) SortRoutes(ctx context.Context) {
	s.autoSortAntigravityRoutes(ctx)
}

// refreshAllQuotas refreshes quotas for all Antigravity providers
func (s *AntigravityTaskService) refreshAllQuotas(ctx context.Context) bool {
	providers, err := s.providerRepo.List()
	if err != nil {
		log.Printf("[AntigravityTask] Failed to list providers: %v", err)
		return false
	}

	refreshedCount := 0
	for _, provider := range providers {
		if provider.Type != "antigravity" || provider.Config == nil || provider.Config.Antigravity == nil {
			continue
		}

		config := provider.Config.Antigravity
		if config.RefreshToken == "" {
			continue
		}

		// Fetch quota from API
		quota, err := antigravity.FetchQuotaForProvider(ctx, config.RefreshToken, config.ProjectID)
		if err != nil {
			log.Printf("[AntigravityTask] Failed to fetch quota for provider %d: %v", provider.ID, err)
			continue
		}

		// Save to database
		s.saveQuotaToDB(config.Email, config.ProjectID, quota)
		refreshedCount++
	}

	if refreshedCount > 0 {
		log.Printf("[AntigravityTask] Refreshed quotas for %d providers", refreshedCount)
		return true
	}

	return false
}

// saveQuotaToDB saves quota to database
func (s *AntigravityTaskService) saveQuotaToDB(email, projectID string, quota *antigravity.QuotaData) {
	if s.quotaRepo == nil || email == "" {
		return
	}

	var models []domain.AntigravityModelQuota
	var subscriptionTier string
	var isForbidden bool

	if quota != nil {
		models = make([]domain.AntigravityModelQuota, len(quota.Models))
		for i, m := range quota.Models {
			models[i] = domain.AntigravityModelQuota{
				Name:       m.Name,
				Percentage: m.Percentage,
				ResetTime:  m.ResetTime,
			}
		}
		subscriptionTier = quota.SubscriptionTier
		isForbidden = quota.IsForbidden
	}

	// Try to preserve existing user info
	var name, picture string
	if existing, _ := s.quotaRepo.GetByEmail(email); existing != nil {
		name = existing.Name
		picture = existing.Picture
	}

	domainQuota := &domain.AntigravityQuota{
		Email:            email,
		Name:             name,
		Picture:          picture,
		GCPProjectID:     projectID,
		SubscriptionTier: subscriptionTier,
		IsForbidden:      isForbidden,
		Models:           models,
	}

	s.quotaRepo.Upsert(domainQuota)
}

// isAutoSortEnabled checks if auto-sort is enabled in settings
func (s *AntigravityTaskService) isAutoSortEnabled() bool {
	val, err := s.settingRepo.Get(domain.SettingKeyAutoSortAntigravity)
	if err != nil {
		return false
	}
	return val == "true"
}

// autoSortAntigravityRoutes sorts Antigravity routes by resetTime for all scopes
func (s *AntigravityTaskService) autoSortAntigravityRoutes(ctx context.Context) {
	log.Printf("[AntigravityTask] Starting auto-sort")

	routes, err := s.routeRepo.List()
	if err != nil {
		log.Printf("[AntigravityTask] Failed to list routes: %v", err)
		return
	}

	providers, err := s.providerRepo.List()
	if err != nil {
		log.Printf("[AntigravityTask] Failed to list providers: %v", err)
		return
	}

	// Build provider map
	providerMap := make(map[uint64]*domain.Provider)
	antigravityCount := 0
	for _, p := range providers {
		providerMap[p.ID] = p
		if p.Type == "antigravity" {
			antigravityCount++
		}
	}
	log.Printf("[AntigravityTask] Found %d Antigravity providers, %d total routes", antigravityCount, len(routes))

	// Get all quotas
	quotas, err := s.quotaRepo.List()
	if err != nil {
		log.Printf("[AntigravityTask] Failed to list quotas: %v", err)
		return
	}
	log.Printf("[AntigravityTask] Found %d quotas in database", len(quotas))

	// Build email to quota map
	quotaByEmail := make(map[string]*domain.AntigravityQuota)
	for _, q := range quotas {
		quotaByEmail[q.Email] = q
	}

	// Collect all unique (clientType, projectID) combinations
	type scope struct {
		clientType domain.ClientType
		projectID  uint64
	}
	scopes := make(map[scope]bool)
	for _, r := range routes {
		scopes[scope{r.ClientType, r.ProjectID}] = true
	}

	// Process each scope
	var allUpdates []domain.RoutePositionUpdate
	for sc := range scopes {
		updates := s.sortAntigravityRoutesForScope(routes, providerMap, quotaByEmail, sc.clientType, sc.projectID)
		allUpdates = append(allUpdates, updates...)
	}

	if len(allUpdates) > 0 {
		if err := s.routeRepo.BatchUpdatePositions(allUpdates); err != nil {
			log.Printf("[AntigravityTask] Failed to update route positions: %v", err)
			return
		}
		log.Printf("[AntigravityTask] Auto-sorted %d routes", len(allUpdates))

		// Broadcast routes updated
		s.broadcaster.BroadcastMessage("routes_updated", nil)
	}
}

// sortAntigravityRoutesForScope sorts Antigravity routes within a scope
func (s *AntigravityTaskService) sortAntigravityRoutesForScope(
	routes []*domain.Route,
	providerMap map[uint64]*domain.Provider,
	quotaByEmail map[string]*domain.AntigravityQuota,
	clientType domain.ClientType,
	projectID uint64,
) []domain.RoutePositionUpdate {
	// Filter routes for this scope and sort by position
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

	// Collect Antigravity routes and their indices
	type antigravityRoute struct {
		route     *domain.Route
		index     int
		resetTime *time.Time
	}
	var antigravityRoutes []antigravityRoute

	for i, r := range scopeRoutes {
		provider := providerMap[r.ProviderID]
		if provider == nil || provider.Type != "antigravity" {
			continue
		}

		// Get resetTime from quota
		var resetTime *time.Time
		if provider.Config != nil && provider.Config.Antigravity != nil {
			email := provider.Config.Antigravity.Email
			if quota := quotaByEmail[email]; quota != nil {
				resetTime = s.getClaudeResetTime(quota)
			}
		}

		antigravityRoutes = append(antigravityRoutes, antigravityRoute{
			route:     r,
			index:     i,
			resetTime: resetTime,
		})
	}

	if len(antigravityRoutes) <= 1 {
		return nil
	}

	// Save original order before sorting
	originalOrder := make([]uint64, len(antigravityRoutes))
	for i, ar := range antigravityRoutes {
		originalOrder[i] = ar.route.ID
	}

	// Sort Antigravity routes by resetTime (earliest first)
	sort.Slice(antigravityRoutes, func(i, j int) bool {
		a, b := antigravityRoutes[i].resetTime, antigravityRoutes[j].resetTime
		if a == nil && b == nil {
			return false
		}
		if a == nil {
			return false // nil goes to end
		}
		if b == nil {
			return true
		}
		return a.Before(*b)
	})

	// Check if order changed
	needsReorder := false
	for i, ar := range antigravityRoutes {
		if ar.route.ID != originalOrder[i] {
			needsReorder = true
			break
		}
	}

	if !needsReorder {
		return nil
	}

	// Build new route order: place sorted Antigravity routes back into their original positions
	newScopeRoutes := make([]*domain.Route, len(scopeRoutes))
	copy(newScopeRoutes, scopeRoutes)

	// Get original Antigravity indices
	originalIndices := make([]int, len(antigravityRoutes))
	for i, ar := range antigravityRoutes {
		originalIndices[i] = ar.index
	}
	sort.Ints(originalIndices)

	// Place sorted routes into original positions
	for i, idx := range originalIndices {
		newScopeRoutes[idx] = antigravityRoutes[i].route
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

// getClaudeResetTime extracts Claude model reset time from quota
func (s *AntigravityTaskService) getClaudeResetTime(quota *domain.AntigravityQuota) *time.Time {
	if quota == nil || quota.IsForbidden || len(quota.Models) == 0 {
		return nil
	}

	for _, m := range quota.Models {
		// Use case-insensitive matching for model name
		if strings.Contains(strings.ToLower(m.Name), "claude") {
			t, err := time.Parse(time.RFC3339, m.ResetTime)
			if err == nil {
				return &t
			}
		}
	}
	return nil
}

