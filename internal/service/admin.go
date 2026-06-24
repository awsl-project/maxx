package service

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/event"
	"github.com/awsl-project/maxx/internal/payloadoverride"
	"github.com/awsl-project/maxx/internal/pricing"
	"github.com/awsl-project/maxx/internal/repository"
	"github.com/awsl-project/maxx/internal/version"
)

// ProviderAdapterRefresher is an interface for refreshing provider adapters
// Implemented by Router to receive notifications when providers change
type ProviderAdapterRefresher interface {
	RefreshAdapter(p *domain.Provider) error
	RemoveAdapter(providerID uint64)
	// GetAdapter returns the cached adapter for a provider, if any. Used
	// by admin endpoints that reach into adapter-specific runtime state.
	GetAdapter(providerID uint64) (provider.ProviderAdapter, bool)
}

// GetProviderAdapter exposes the cached adapter for a provider so HTTP
// handlers can call adapter-specific methods (e.g. Bedrock discovery).
// Returns nil,false when no adapter is registered yet or refresher is
// unwired (test setups).
func (s *AdminService) GetProviderAdapter(providerID uint64) (provider.ProviderAdapter, bool) {
	if s.adapterRefresher == nil {
		return nil, false
	}
	return s.adapterRefresher.GetAdapter(providerID)
}

// AdminService provides business logic for admin operations
// Both HTTP handlers and Wails bindings call this service
type AdminService struct {
	providerRepo        repository.ProviderRepository
	routeRepo           repository.RouteRepository
	projectRepo         repository.ProjectRepository
	sessionRepo         repository.SessionRepository
	retryConfigRepo     repository.RetryConfigRepository
	routingStrategyRepo repository.RoutingStrategyRepository
	proxyRequestRepo    repository.ProxyRequestRepository
	attemptRepo         repository.ProxyUpstreamAttemptRepository
	settingRepo         repository.SystemSettingRepository
	apiTokenRepo        repository.APITokenRepository
	inviteCodeRepo      repository.InviteCodeRepository
	inviteCodeUsageRepo repository.InviteCodeUsageRepository
	modelMappingRepo    repository.ModelMappingRepository
	usageStatsRepo      repository.UsageStatsRepository
	responseModelRepo   repository.ResponseModelRepository
	modelPriceRepo      repository.ModelPriceRepository
	serverAddr          string
	adapterRefresher    ProviderAdapterRefresher
	broadcaster         event.Broadcaster
	pprofReloader       PprofReloader
}

// PprofReloader is an interface for reloading pprof configuration
type PprofReloader interface {
	ReloadPprofConfig() error
}

// NewAdminService creates a new admin service
func NewAdminService(
	providerRepo repository.ProviderRepository,
	routeRepo repository.RouteRepository,
	projectRepo repository.ProjectRepository,
	sessionRepo repository.SessionRepository,
	retryConfigRepo repository.RetryConfigRepository,
	routingStrategyRepo repository.RoutingStrategyRepository,
	proxyRequestRepo repository.ProxyRequestRepository,
	attemptRepo repository.ProxyUpstreamAttemptRepository,
	settingRepo repository.SystemSettingRepository,
	apiTokenRepo repository.APITokenRepository,
	inviteCodeRepo repository.InviteCodeRepository,
	inviteCodeUsageRepo repository.InviteCodeUsageRepository,
	modelMappingRepo repository.ModelMappingRepository,
	usageStatsRepo repository.UsageStatsRepository,
	responseModelRepo repository.ResponseModelRepository,
	modelPriceRepo repository.ModelPriceRepository,
	serverAddr string,
	adapterRefresher ProviderAdapterRefresher,
	broadcaster event.Broadcaster,
	pprofReloader PprofReloader,
) *AdminService {
	return &AdminService{
		providerRepo:        providerRepo,
		routeRepo:           routeRepo,
		projectRepo:         projectRepo,
		sessionRepo:         sessionRepo,
		retryConfigRepo:     retryConfigRepo,
		routingStrategyRepo: routingStrategyRepo,
		proxyRequestRepo:    proxyRequestRepo,
		attemptRepo:         attemptRepo,
		settingRepo:         settingRepo,
		apiTokenRepo:        apiTokenRepo,
		inviteCodeRepo:      inviteCodeRepo,
		inviteCodeUsageRepo: inviteCodeUsageRepo,
		modelMappingRepo:    modelMappingRepo,
		usageStatsRepo:      usageStatsRepo,
		responseModelRepo:   responseModelRepo,
		modelPriceRepo:      modelPriceRepo,
		serverAddr:          serverAddr,
		adapterRefresher:    adapterRefresher,
		broadcaster:         broadcaster,
		pprofReloader:       pprofReloader,
	}
}

// ===== Provider API =====

func (s *AdminService) GetProviders(tenantID uint64) ([]*domain.Provider, error) {
	return s.providerRepo.List(tenantID)
}

func (s *AdminService) GetProvider(tenantID uint64, id uint64) (*domain.Provider, error) {
	return s.providerRepo.GetByID(tenantID, id)
}

func (s *AdminService) CreateProvider(tenantID uint64, provider *domain.Provider) error {
	provider.TenantID = tenantID

	// Auto-set SupportedClientTypes based on provider type
	s.autoSetSupportedClientTypes(provider)

	if err := s.providerRepo.Create(provider); err != nil {
		return err
	}
	// Refresh adapter cache for the new provider
	if s.adapterRefresher != nil {
		s.adapterRefresher.RefreshAdapter(provider)
	}
	return nil
}

func (s *AdminService) UpdateProvider(tenantID uint64, provider *domain.Provider) error {
	existing, err := s.providerRepo.GetByID(tenantID, provider.ID)
	if err != nil {
		return err
	}
	preserveExcludedProviderWriteOnlyMode(existing, provider)
	preserveEmptyProviderSecrets(existing, provider)

	// Auto-set SupportedClientTypes based on provider type
	s.autoSetSupportedClientTypes(provider)

	if err := s.providerRepo.Update(provider); err != nil {
		return err
	}
	// Refresh adapter cache for the updated provider
	if s.adapterRefresher != nil {
		s.adapterRefresher.RefreshAdapter(provider)
	}
	return nil
}

func (s *AdminService) DeleteProvider(tenantID uint64, id uint64) error {
	// Delete related routes first
	routes, _ := s.routeRepo.List(tenantID)
	for _, route := range routes {
		if route.ProviderID == id {
			s.routeRepo.Delete(tenantID, route.ID)
		}
	}

	// Delete provider-scoped model mappings so provider deletion does not leave
	// orphaned mapping rules behind.
	mappings, _ := s.modelMappingRepo.List(tenantID)
	for _, mapping := range mappings {
		if mapping.Scope == domain.ModelMappingScopeProvider && mapping.ProviderID == id {
			s.modelMappingRepo.Delete(tenantID, mapping.ID)
		}
	}

	// Remove adapter from cache
	if s.adapterRefresher != nil {
		s.adapterRefresher.RemoveAdapter(id)
	}
	return s.providerRepo.Delete(tenantID, id)
}

func preserveExcludedProviderWriteOnlyMode(existing, incoming *domain.Provider) {
	if existing == nil || incoming == nil || !existing.ExcludeFromExport {
		return
	}
	incoming.ExcludeFromExport = true
	preserveEmptyExcludedProviderURLs(existing, incoming)
}

func preserveEmptyExcludedProviderURLs(existing, incoming *domain.Provider) {
	if existing == nil || incoming == nil || existing.Config == nil || existing.Config.Custom == nil {
		return
	}
	effectiveType := incoming.Type
	if effectiveType == "" {
		effectiveType = existing.Type
	}
	if effectiveType != "custom" || existing.Type != "custom" {
		return
	}
	if incoming.Config == nil {
		incoming.Config = &domain.ProviderConfig{}
	}
	if incoming.Config.Custom == nil {
		custom := *existing.Config.Custom
		incoming.Config.Custom = &custom
		return
	}

	if incoming.Config.Custom.BaseURL == "" {
		incoming.Config.Custom.BaseURL = existing.Config.Custom.BaseURL
	}
	if len(incoming.Config.Custom.ClientBaseURL) == 0 {
		incoming.Config.Custom.ClientBaseURL = existing.Config.Custom.ClientBaseURL
		return
	}
	merged := make(map[domain.ClientType]string, len(existing.Config.Custom.ClientBaseURL)+len(incoming.Config.Custom.ClientBaseURL))
	for clientType, url := range existing.Config.Custom.ClientBaseURL {
		merged[clientType] = url
	}
	for clientType, url := range incoming.Config.Custom.ClientBaseURL {
		if url != "" {
			merged[clientType] = url
		}
	}
	incoming.Config.Custom.ClientBaseURL = merged
}

func preserveEmptyProviderSecrets(existing, incoming *domain.Provider) {
	if existing == nil || incoming == nil || existing.Config == nil {
		return
	}
	effectiveType := incoming.Type
	if effectiveType == "" {
		effectiveType = existing.Type
	}
	if effectiveType != existing.Type {
		return
	}
	if incoming.Config == nil {
		incoming.Config = &domain.ProviderConfig{}
	}

	switch effectiveType {
	case "custom":
		if existing.Config.Custom != nil {
			if incoming.Config.Custom == nil {
				custom := *existing.Config.Custom
				incoming.Config.Custom = &custom
			} else if incoming.Config.Custom.APIKey == "" {
				incoming.Config.Custom.APIKey = existing.Config.Custom.APIKey
			}
		}
	case "antigravity":
		if existing.Config.Antigravity != nil {
			if incoming.Config.Antigravity == nil {
				antigravity := *existing.Config.Antigravity
				incoming.Config.Antigravity = &antigravity
			} else if incoming.Config.Antigravity.RefreshToken == "" {
				incoming.Config.Antigravity.RefreshToken = existing.Config.Antigravity.RefreshToken
			}
		}
	case "bedrock":
		if existing.Config.Bedrock != nil {
			if incoming.Config.Bedrock == nil {
				bedrock := *existing.Config.Bedrock
				incoming.Config.Bedrock = &bedrock
			} else if incoming.Config.Bedrock.SecretAccessKey == "" {
				incoming.Config.Bedrock.SecretAccessKey = existing.Config.Bedrock.SecretAccessKey
			}
		}
	case "kiro":
		if existing.Config.Kiro != nil {
			if incoming.Config.Kiro == nil {
				kiro := *existing.Config.Kiro
				incoming.Config.Kiro = &kiro
			} else {
				if incoming.Config.Kiro.RefreshToken == "" {
					incoming.Config.Kiro.RefreshToken = existing.Config.Kiro.RefreshToken
				}
				if incoming.Config.Kiro.ClientSecret == "" {
					incoming.Config.Kiro.ClientSecret = existing.Config.Kiro.ClientSecret
				}
			}
		}
	case "codex":
		if existing.Config.Codex != nil {
			if incoming.Config.Codex == nil {
				codex := *existing.Config.Codex
				incoming.Config.Codex = &codex
			} else {
				if incoming.Config.Codex.RefreshToken == "" {
					incoming.Config.Codex.RefreshToken = existing.Config.Codex.RefreshToken
				}
				if incoming.Config.Codex.AccessToken == "" {
					incoming.Config.Codex.AccessToken = existing.Config.Codex.AccessToken
				}
			}
		}
	case "claude":
		if existing.Config.Claude != nil {
			if incoming.Config.Claude == nil {
				claude := *existing.Config.Claude
				incoming.Config.Claude = &claude
			} else {
				if incoming.Config.Claude.RefreshToken == "" {
					incoming.Config.Claude.RefreshToken = existing.Config.Claude.RefreshToken
				}
				if incoming.Config.Claude.AccessToken == "" {
					incoming.Config.Claude.AccessToken = existing.Config.Claude.AccessToken
				}
			}
		}
	}
}

// ExportProviders exports all providers for backup/transfer
// Returns providers without ID and timestamps for clean import
func (s *AdminService) ExportProviders(tenantID uint64) ([]*domain.Provider, error) {
	providers, err := s.providerRepo.List(tenantID)
	if err != nil {
		return nil, err
	}
	filtered := make([]*domain.Provider, 0, len(providers))
	for _, provider := range providers {
		if provider.ExcludeFromExport {
			continue
		}
		filtered = append(filtered, provider)
	}
	return filtered, nil
}

// ImportProviders imports providers from exported data
// Creates new providers, skipping duplicates by name
func (s *AdminService) ImportProviders(tenantID uint64, providers []*domain.Provider) (*ImportResult, error) {
	result := &ImportResult{
		Imported: 0,
		Skipped:  0,
		Errors:   []string{},
	}

	// Get existing providers for duplicate detection
	existing, err := s.providerRepo.List(tenantID)
	if err != nil {
		return nil, err
	}
	existingNames := make(map[string]bool)
	for _, p := range existing {
		existingNames[p.Name] = true
	}

	for _, provider := range providers {
		// Skip if name already exists
		if existingNames[provider.Name] {
			result.Skipped++
			result.Errors = append(result.Errors, "skipped duplicate: "+provider.Name)
			continue
		}

		// Reset ID and timestamps for new creation
		provider.ID = 0
		provider.DeletedAt = nil

		// Create the provider
		if err := s.CreateProvider(tenantID, provider); err != nil {
			result.Errors = append(result.Errors, "failed to import "+provider.Name+": "+err.Error())
			continue
		}

		result.Imported++
		existingNames[provider.Name] = true
	}

	return result, nil
}

// ImportResult holds the result of an import operation
type ImportResult struct {
	Imported int      `json:"imported"`
	Skipped  int      `json:"skipped"`
	Errors   []string `json:"errors"`
}

// ===== Route API =====

func (s *AdminService) GetRoutes(tenantID uint64) ([]*domain.Route, error) {
	return s.routeRepo.List(tenantID)
}

func (s *AdminService) GetRoute(tenantID uint64, id uint64) (*domain.Route, error) {
	return s.routeRepo.GetByID(tenantID, id)
}

func (s *AdminService) CreateRoute(tenantID uint64, route *domain.Route) error {
	route.TenantID = tenantID
	return s.routeRepo.Create(route)
}

func (s *AdminService) UpdateRoute(tenantID uint64, route *domain.Route) error {
	return s.routeRepo.Update(route)
}

func (s *AdminService) BatchUpdateRoutePositions(tenantID uint64, updates []domain.RoutePositionUpdate) error {
	return s.routeRepo.BatchUpdatePositions(tenantID, updates)
}

func (s *AdminService) DeleteRoute(tenantID uint64, id uint64) error {
	return s.routeRepo.Delete(tenantID, id)
}

func (s *AdminService) BulkDeleteRoutes(tenantID uint64, req domain.RouteBulkDeleteRequest) (*domain.RouteBulkDeleteResult, error) {
	if len(req.IDs) == 0 {
		return nil, fmt.Errorf("ids required")
	}
	if !isValidRouteClientType(req.ClientType) {
		return nil, fmt.Errorf("invalid clientType")
	}
	return s.routeRepo.BulkDelete(tenantID, req)
}

func (s *AdminService) SyncRoutesFromProject(tenantID uint64, req domain.RouteSyncRequest) (*domain.RouteSyncResult, error) {
	if !isValidRouteClientType(req.ClientType) {
		return nil, fmt.Errorf("invalid clientType")
	}
	if req.Mode == "" {
		req.Mode = domain.RouteSyncModeOverwrite
	}
	if req.Mode != domain.RouteSyncModeOverwrite && req.Mode != domain.RouteSyncModeAddMissing {
		return nil, fmt.Errorf("invalid mode")
	}

	if req.SourceProjectID > 0 {
		if _, err := s.projectRepo.GetByID(tenantID, req.SourceProjectID); err != nil {
			return nil, fmt.Errorf("source project not found")
		}
	}

	var targetProject *domain.Project
	if req.TargetProjectID > 0 {
		project, err := s.projectRepo.GetByID(tenantID, req.TargetProjectID)
		if err != nil {
			return nil, fmt.Errorf("target project not found")
		}
		targetProject = project
	}

	effectiveSourceProjectID := req.SourceProjectID
	if req.SourceProjectID > 0 {
		sourceProject, err := s.projectRepo.GetByID(tenantID, req.SourceProjectID)
		if err != nil {
			return nil, fmt.Errorf("source project not found")
		}
		if !projectHasCustomRoutes(sourceProject, req.ClientType) {
			effectiveSourceProjectID = 0
		}
	}

	routes, err := s.routeRepo.List(tenantID)
	if err != nil {
		return nil, err
	}

	sourceRoutes := filterRoutesByScope(routes, effectiveSourceProjectID, req.ClientType)
	targetRoutes := filterRoutesByScope(routes, req.TargetProjectID, req.ClientType)
	targetByProvider := make(map[uint64]*domain.Route, len(targetRoutes))
	for _, route := range targetRoutes {
		targetByProvider[route.ProviderID] = route
	}

	result := &domain.RouteSyncResult{
		SourceProjectID:          req.SourceProjectID,
		EffectiveSourceProjectID: effectiveSourceProjectID,
		TargetProjectID:          req.TargetProjectID,
		ClientType:               req.ClientType,
		Mode:                     req.Mode,
		Routes:                   []*domain.Route{},
	}

	if req.Mode == domain.RouteSyncModeAddMissing {
		maxPosition := 0
		for _, route := range targetRoutes {
			if route.Position > maxPosition {
				maxPosition = route.Position
			}
		}
		for _, source := range sourceRoutes {
			if _, exists := targetByProvider[source.ProviderID]; exists {
				result.SkippedCount++
				continue
			}
			maxPosition++
			created := cloneRouteForTarget(source, tenantID, req.TargetProjectID, maxPosition)
			if err := s.routeRepo.Create(created); err != nil {
				return nil, err
			}
			result.CreatedCount++
			result.Routes = append(result.Routes, created)
		}
	} else {
		sourceProviderIDs := make(map[uint64]struct{}, len(sourceRoutes))
		for index, source := range sourceRoutes {
			sourceProviderIDs[source.ProviderID] = struct{}{}
			position := index + 1
			if target, exists := targetByProvider[source.ProviderID]; exists {
				if copyRouteFields(target, source, position) {
					if err := s.routeRepo.Update(target); err != nil {
						return nil, err
					}
					result.UpdatedCount++
				}
				result.Routes = append(result.Routes, target)
				continue
			}

			created := cloneRouteForTarget(source, tenantID, req.TargetProjectID, position)
			if err := s.routeRepo.Create(created); err != nil {
				return nil, err
			}
			result.CreatedCount++
			result.Routes = append(result.Routes, created)
		}

		for _, target := range targetRoutes {
			if _, keep := sourceProviderIDs[target.ProviderID]; keep {
				continue
			}
			if err := s.routeRepo.Delete(tenantID, target.ID); err != nil {
				return nil, err
			}
			result.DeletedCount++
		}
	}

	if targetProject != nil && !projectHasCustomRoutes(targetProject, req.ClientType) {
		targetProject.EnabledCustomRoutes = append(targetProject.EnabledCustomRoutes, req.ClientType)
		if err := s.projectRepo.Update(targetProject); err != nil {
			return nil, err
		}
		result.EnabledCustomRoutes = true
	}

	return result, nil
}

func filterRoutesByScope(routes []*domain.Route, projectID uint64, clientType domain.ClientType) []*domain.Route {
	filtered := make([]*domain.Route, 0)
	for _, route := range routes {
		if route.ProjectID == projectID && route.ClientType == clientType {
			filtered = append(filtered, route)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Position == filtered[j].Position {
			return filtered[i].ID < filtered[j].ID
		}
		return filtered[i].Position < filtered[j].Position
	})
	return filtered
}

func cloneRouteForTarget(source *domain.Route, tenantID, targetProjectID uint64, position int) *domain.Route {
	return &domain.Route{
		TenantID:      tenantID,
		IsEnabled:     source.IsEnabled,
		IsNative:      source.IsNative,
		ProjectID:     targetProjectID,
		ClientType:    source.ClientType,
		ProviderID:    source.ProviderID,
		Position:      position,
		Weight:        normalizedRouteWeight(source.Weight),
		RetryConfigID: source.RetryConfigID,
	}
}

func copyRouteFields(target, source *domain.Route, position int) bool {
	changed := false
	if target.IsEnabled != source.IsEnabled {
		target.IsEnabled = source.IsEnabled
		changed = true
	}
	if target.IsNative != source.IsNative {
		target.IsNative = source.IsNative
		changed = true
	}
	if target.Position != position {
		target.Position = position
		changed = true
	}
	weight := normalizedRouteWeight(source.Weight)
	if target.Weight != weight {
		target.Weight = weight
		changed = true
	}
	if target.RetryConfigID != source.RetryConfigID {
		target.RetryConfigID = source.RetryConfigID
		changed = true
	}
	return changed
}

func normalizedRouteWeight(weight int) int {
	if weight <= 0 {
		return 1
	}
	return weight
}

func projectHasCustomRoutes(project *domain.Project, clientType domain.ClientType) bool {
	if project == nil {
		return false
	}
	for _, enabled := range project.EnabledCustomRoutes {
		if enabled == clientType {
			return true
		}
	}
	return false
}

func isValidRouteClientType(clientType domain.ClientType) bool {
	switch clientType {
	case domain.ClientTypeClaude, domain.ClientTypeOpenAI, domain.ClientTypeCodex, domain.ClientTypeGemini:
		return true
	default:
		return false
	}
}

// ===== Project API =====

const projectUsageRecentWindow = 30 * 24 * time.Hour

func (s *AdminService) GetProjects(tenantID uint64) ([]*domain.Project, error) {
	projects, err := s.projectRepo.List(tenantID)
	if err != nil {
		return nil, err
	}
	if err := s.attachProjectUsage(tenantID, projects); err != nil {
		log.Printf("warn: attach project usage summaries failed: %v", err)
	}
	return projects, nil
}

func (s *AdminService) GetProject(tenantID uint64, id uint64) (*domain.Project, error) {
	project, err := s.projectRepo.GetByID(tenantID, id)
	if err != nil {
		return nil, err
	}
	if err := s.attachProjectUsage(tenantID, []*domain.Project{project}); err != nil {
		log.Printf("warn: attach project usage summary failed for project %d: %v", id, err)
	}
	return project, nil
}

func (s *AdminService) GetProjectBySlug(tenantID uint64, slug string) (*domain.Project, error) {
	project, err := s.projectRepo.GetBySlug(tenantID, slug)
	if err != nil {
		return nil, err
	}
	if err := s.attachProjectUsage(tenantID, []*domain.Project{project}); err != nil {
		log.Printf("warn: attach project usage summary failed for project slug %q: %v", slug, err)
	}
	return project, nil
}

func (s *AdminService) attachProjectUsage(tenantID uint64, projects []*domain.Project) error {
	if s.proxyRequestRepo == nil || len(projects) == 0 {
		return nil
	}
	projectIDs := make([]uint64, 0, len(projects))
	for _, project := range projects {
		if project == nil {
			continue
		}
		projectIDs = append(projectIDs, project.ID)
	}
	if len(projectIDs) == 0 {
		return nil
	}
	summaries, err := s.proxyRequestRepo.GetProjectUsageSummaries(tenantID, time.Now().Add(-projectUsageRecentWindow), projectIDs...)
	if err != nil {
		return err
	}
	for _, project := range projects {
		if project == nil {
			continue
		}
		summary, ok := summaries[project.ID]
		if !ok {
			continue
		}
		project.LastRequestAt = summary.LastRequestAt
		project.LastSuccessfulRequestAt = summary.LastSuccessfulRequestAt
		project.RequestCount30d = summary.RequestCount30d
		project.SuccessfulRequestCount30d = summary.SuccessfulRequestCount30d
		project.TotalRequestCount = summary.TotalRequestCount
	}
	return nil
}

func (s *AdminService) CreateProject(tenantID uint64, project *domain.Project) error {
	project.TenantID = tenantID
	return s.projectRepo.Create(project)
}

func (s *AdminService) UpdateProject(tenantID uint64, project *domain.Project) error {
	return s.projectRepo.Update(project)
}

func (s *AdminService) DeleteProject(tenantID uint64, id uint64) error {
	return s.projectRepo.Delete(tenantID, id)
}

// ===== Session API =====

func (s *AdminService) GetSessions(tenantID uint64) ([]*domain.Session, error) {
	return s.sessionRepo.List(tenantID)
}

// UpdateSessionProjectResult holds the result of updating session project
type UpdateSessionProjectResult struct {
	Session         *domain.Session `json:"session"`
	UpdatedRequests int64           `json:"updatedRequests"`
}

// UpdateSessionProject updates the session's projectID and all related requests
func (s *AdminService) UpdateSessionProject(tenantID uint64, sessionID string, projectID uint64) (*UpdateSessionProjectResult, error) {
	// Get the session first
	session, err := s.sessionRepo.GetBySessionID(tenantID, sessionID)
	if err != nil {
		return nil, err
	}

	// Update session's projectID
	session.ProjectID = projectID
	if err := s.sessionRepo.Update(session); err != nil {
		return nil, err
	}

	// Batch update all requests with this sessionID
	updatedCount, err := s.proxyRequestRepo.UpdateProjectIDBySessionID(tenantID, sessionID, projectID)
	if err != nil {
		return nil, err
	}

	return &UpdateSessionProjectResult{
		Session:         session,
		UpdatedRequests: updatedCount,
	}, nil
}

// RejectSession marks a session as rejected with current timestamp
func (s *AdminService) RejectSession(tenantID uint64, sessionID string) (*domain.Session, error) {
	// Get the session first
	session, err := s.sessionRepo.GetBySessionID(tenantID, sessionID)
	if err != nil {
		return nil, err
	}

	// Mark as rejected with timestamp
	now := time.Now()
	session.RejectedAt = &now
	if err := s.sessionRepo.Update(session); err != nil {
		return nil, err
	}

	return session, nil
}

// ===== RetryConfig API =====

func (s *AdminService) GetRetryConfigs(tenantID uint64) ([]*domain.RetryConfig, error) {
	return s.retryConfigRepo.List(tenantID)
}

func (s *AdminService) GetRetryConfig(tenantID uint64, id uint64) (*domain.RetryConfig, error) {
	return s.retryConfigRepo.GetByID(tenantID, id)
}

func (s *AdminService) CreateRetryConfig(tenantID uint64, config *domain.RetryConfig) error {
	config.TenantID = tenantID
	return s.retryConfigRepo.Create(config)
}

func (s *AdminService) UpdateRetryConfig(tenantID uint64, config *domain.RetryConfig) error {
	return s.retryConfigRepo.Update(config)
}

func (s *AdminService) DeleteRetryConfig(tenantID uint64, id uint64) error {
	return s.retryConfigRepo.Delete(tenantID, id)
}

// ===== RoutingStrategy API =====

func (s *AdminService) GetRoutingStrategies(tenantID uint64) ([]*domain.RoutingStrategy, error) {
	return s.routingStrategyRepo.List(tenantID)
}

func (s *AdminService) GetRoutingStrategy(tenantID uint64, id uint64) (*domain.RoutingStrategy, error) {
	return s.routingStrategyRepo.GetByID(tenantID, id)
}

func (s *AdminService) CreateRoutingStrategy(tenantID uint64, strategy *domain.RoutingStrategy) error {
	strategy.TenantID = tenantID
	return s.routingStrategyRepo.Create(strategy)
}

func (s *AdminService) UpdateRoutingStrategy(tenantID uint64, strategy *domain.RoutingStrategy) error {
	return s.routingStrategyRepo.Update(strategy)
}

func (s *AdminService) DeleteRoutingStrategy(tenantID uint64, id uint64) error {
	return s.routingStrategyRepo.Delete(tenantID, id)
}

// ===== ProxyRequest API =====

func (s *AdminService) GetProxyRequests(tenantID uint64, limit, offset int) ([]*domain.ProxyRequest, error) {
	return s.proxyRequestRepo.List(tenantID, limit, offset)
}

// CursorPaginationResult 游标分页结果
type CursorPaginationResult struct {
	Items   []*domain.ProxyRequest `json:"items"`
	HasMore bool                   `json:"hasMore"`
	FirstID uint64                 `json:"firstId,omitempty"`
	LastID  uint64                 `json:"lastId,omitempty"`
}

func (s *AdminService) GetProxyRequestsCursor(tenantID uint64, limit int, before, after uint64, filter *repository.ProxyRequestFilter) (*CursorPaginationResult, error) {
	items, err := s.proxyRequestRepo.ListCursor(tenantID, limit+1, before, after, filter)
	if err != nil {
		return nil, err
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	result := &CursorPaginationResult{
		Items:   items,
		HasMore: hasMore,
	}

	if len(items) > 0 {
		result.FirstID = items[0].ID
		result.LastID = items[len(items)-1].ID
	}

	return result, nil
}

func (s *AdminService) GetProxyRequestsCount(tenantID uint64) (int64, error) {
	return s.proxyRequestRepo.Count(tenantID)
}

func (s *AdminService) GetProxyRequestsCountWithFilter(tenantID uint64, filter *repository.ProxyRequestFilter) (int64, error) {
	return s.proxyRequestRepo.CountWithFilter(tenantID, filter)
}

func (s *AdminService) GetProxyRequestErrorStats(tenantID uint64, filter *repository.ProxyRequestFilter) (*repository.ProxyRequestErrorStats, error) {
	return s.proxyRequestRepo.GetErrorStats(tenantID, filter)
}

func (s *AdminService) GetProxyRequest(tenantID uint64, id uint64) (*domain.ProxyRequest, error) {
	return s.proxyRequestRepo.GetByID(tenantID, id)
}

func (s *AdminService) GetActiveProxyRequests(tenantID uint64) ([]*domain.ProxyRequest, error) {
	return s.proxyRequestRepo.ListActive(tenantID)
}

func (s *AdminService) GetProxyUpstreamAttempts(tenantID uint64, proxyRequestID uint64) ([]*domain.ProxyUpstreamAttempt, error) {
	return s.attemptRepo.ListByProxyRequestID(proxyRequestID)
}

func (s *AdminService) GetProviderStats(tenantID uint64, clientType string, projectID uint64) (map[uint64]*domain.ProviderStats, error) {
	return s.usageStatsRepo.GetProviderStats(tenantID, clientType, projectID)
}

// ===== Settings API =====

func (s *AdminService) GetSettings() (map[string]string, error) {
	settings, err := s.settingRepo.GetAll()
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for _, setting := range settings {
		result[setting.Key] = setting.Value
	}
	return result, nil
}

func (s *AdminService) GetSetting(key string) (string, error) {
	return s.settingRepo.Get(key)
}

func (s *AdminService) UpdateSetting(key, value string) error {
	if err := validateSystemSettingValue(key, value); err != nil {
		return err
	}
	if err := s.settingRepo.Set(key, value); err != nil {
		return err
	}
	if key == domain.SettingKeyPayloadOverrideRules {
		payloadoverride.InvalidateGlobalSettingsCache()
	}

	// 如果更新的是 pprof 相关设置，触发重载
	switch key {
	case domain.SettingKeyEnablePprof, domain.SettingKeyPprofPort, domain.SettingKeyPprofPassword:
		if s.pprofReloader != nil {
			if err := s.pprofReloader.ReloadPprofConfig(); err != nil {
				return fmt.Errorf("设置已保存，但重载 pprof 失败: %w", err)
			}
		}
	}

	return nil
}

func (s *AdminService) DeleteSetting(key string) error {
	if err := s.settingRepo.Delete(key); err != nil {
		return err
	}
	if key == domain.SettingKeyPayloadOverrideRules {
		payloadoverride.InvalidateGlobalSettingsCache()
	}

	// 如果删除的是 pprof 相关设置，触发重载
	switch key {
	case domain.SettingKeyEnablePprof, domain.SettingKeyPprofPort, domain.SettingKeyPprofPassword:
		if s.pprofReloader != nil {
			if err := s.pprofReloader.ReloadPprofConfig(); err != nil {
				return fmt.Errorf("设置已删除，但重载 pprof 失败: %w", err)
			}
		}
	}

	return nil
}

// ===== Proxy Status API =====

type ProxyStatus struct {
	Running bool   `json:"running"`
	Address string `json:"address"`
	Port    int    `json:"port"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

func (s *AdminService) GetProxyStatus(r *http.Request) *ProxyStatus {
	// 获取真实的访问地址
	// 优先使用 X-Forwarded-Host (反向代理场景)，否则使用 r.Host
	// r.Host 已经包含了正确的 host:port 格式（标准端口不带端口号）
	displayAddr := r.Header.Get("X-Forwarded-Host")
	if displayAddr == "" {
		displayAddr = r.Host
	}
	// X-Forwarded-Host 可能包含多个值（逗号分隔），取第一个
	displayAddr = strings.TrimSpace(strings.Split(displayAddr, ",")[0])

	// 如果获取不到，回退到 localhost 和服务器监听端口
	if displayAddr == "" {
		addr := s.serverAddr
		port := 9880 // default
		if idx := strings.LastIndex(addr, ":"); idx >= 0 {
			if p, err := strconv.Atoi(addr[idx+1:]); err == nil {
				port = p
			}
		}
		displayAddr = "localhost:" + strconv.Itoa(port)
	}

	// 从 displayAddr 中解析端口（用于 Port 字段）
	port := 80 // 默认 HTTP 端口
	if _, portStr, err := net.SplitHostPort(displayAddr); err == nil {
		// 地址包含端口
		if p, err := strconv.Atoi(portStr); err == nil {
			port = p
		}
		// displayAddr 保持 host:port 格式不变
	}
	// else: 地址不包含端口，说明是标准端口 80，displayAddr 保持原样

	return &ProxyStatus{
		Running: true,
		Address: displayAddr,
		Port:    port,
		Version: version.Version,
		Commit:  version.Commit,
	}
}

// ===== Logs API =====

type LogsResult struct {
	Lines []string `json:"lines"`
	Count int      `json:"count"`
}

// GetLogs is a placeholder - actual implementation needs log reader
// The log reading logic is in handler package, will be refactored later
func (s *AdminService) GetLogs(limit int) (*LogsResult, error) {
	// This will be implemented by injecting a log reader
	return &LogsResult{Lines: []string{}, Count: 0}, nil
}

// ===== Private helpers =====

// autoSetSupportedClientTypes sets SupportedClientTypes based on provider type
func (s *AdminService) autoSetSupportedClientTypes(provider *domain.Provider) {
	switch provider.Type {
	case "antigravity":
		// Antigravity natively supports Claude and Gemini.
		// Conversion preference is Gemini-first.
		provider.SupportedClientTypes = []domain.ClientType{
			domain.ClientTypeGemini,
			domain.ClientTypeClaude,
		}
	case "kiro":
		// Kiro natively supports Claude protocol only
		provider.SupportedClientTypes = []domain.ClientType{
			domain.ClientTypeClaude,
		}
	case "codex":
		// Codex natively supports Codex protocol only
		provider.SupportedClientTypes = []domain.ClientType{
			domain.ClientTypeCodex,
		}
	case "claude":
		// Claude natively supports Claude protocol only
		provider.SupportedClientTypes = []domain.ClientType{
			domain.ClientTypeClaude,
		}
	case "custom":
		// Custom providers use their configured SupportedClientTypes
		// If not set, default to OpenAI
		if len(provider.SupportedClientTypes) == 0 {
			provider.SupportedClientTypes = []domain.ClientType{domain.ClientTypeOpenAI}
		}
	}
}

// ===== API Token API =====

func (s *AdminService) GetAPITokens(tenantID uint64) ([]*domain.APIToken, error) {
	return s.apiTokenRepo.List(tenantID)
}

func (s *AdminService) GetAPIToken(tenantID uint64, id uint64) (*domain.APIToken, error) {
	return s.apiTokenRepo.GetByID(tenantID, id)
}

// CreateAPIToken creates a new API token and returns the plain token (only shown once)
func (s *AdminService) CreateAPIToken(tenantID uint64, name, description string, projectID uint64, expiresAt *time.Time) (*domain.APITokenCreateResult, error) {
	// Generate token
	plain, prefix, err := generateAPIToken()
	if err != nil {
		return nil, err
	}

	token := &domain.APIToken{
		TenantID:    tenantID,
		Token:       plain,
		TokenPrefix: prefix,
		Name:        name,
		Description: description,
		ProjectID:   projectID,
		IsEnabled:   true,
		ExpiresAt:   expiresAt,
	}

	if err := s.apiTokenRepo.Create(token); err != nil {
		return nil, err
	}

	return &domain.APITokenCreateResult{
		Token:    plain,
		APIToken: token,
	}, nil
}

func (s *AdminService) UpdateAPIToken(tenantID uint64, token *domain.APIToken) error {
	return s.apiTokenRepo.Update(token)
}

func (s *AdminService) DeleteAPIToken(tenantID uint64, id uint64) error {
	return s.apiTokenRepo.Delete(tenantID, id)
}

func (s *AdminService) CleanupExpiredAPITokens(tenantID uint64, now time.Time) (*domain.APITokenCleanupResult, error) {
	deletedTokens, err := s.apiTokenRepo.DeleteExpired(tenantID, now, domain.APITokenInactiveExpiry)
	if err != nil {
		return nil, err
	}

	items := make([]domain.APITokenCleanupItem, 0, len(deletedTokens))
	for _, token := range deletedTokens {
		items = append(items, domain.APITokenCleanupItem{
			ID:          token.ID,
			Name:        token.Name,
			TokenPrefix: token.TokenPrefix,
		})
	}

	return &domain.APITokenCleanupResult{
		DeletedCount: len(items),
		Tokens:       items,
	}, nil
}

// ===== Invite Code API =====

func (s *AdminService) GetInviteCodes(tenantID uint64) ([]*domain.InviteCode, error) {
	if s.inviteCodeRepo == nil {
		return nil, fmt.Errorf("invite code repository not configured")
	}
	return s.inviteCodeRepo.List(tenantID)
}

func (s *AdminService) GetInviteCode(tenantID uint64, id uint64) (*domain.InviteCode, error) {
	if s.inviteCodeRepo == nil {
		return nil, fmt.Errorf("invite code repository not configured")
	}
	return s.inviteCodeRepo.GetByID(tenantID, id)
}

func (s *AdminService) CreateInviteCodes(
	tenantID uint64,
	createdByUserID uint64,
	count int,
	maxUses uint64,
	expiresAt *time.Time,
	note string,
) (*domain.InviteCodeCreateResult, error) {
	if s.inviteCodeRepo == nil {
		return nil, fmt.Errorf("invite code repository not configured")
	}
	if count <= 0 {
		count = 1
	}
	if count > 100 {
		return nil, fmt.Errorf("count too large (max 100)")
	}

	result := &domain.InviteCodeCreateResult{
		Items: make([]domain.InviteCodeCreateItem, 0, count),
	}
	createdIDs := make([]uint64, 0, count)

	for i := 0; i < count; i++ {
		var lastErr error
		for attempt := 0; attempt < 5; attempt++ {
			plain, hash, prefix, err := generateInviteCode()
			if err != nil {
				return nil, err
			}
			code := &domain.InviteCode{
				TenantID:        tenantID,
				CodeHash:        hash,
				CodePrefix:      prefix,
				Status:          domain.InviteCodeStatusActive,
				MaxUses:         maxUses,
				UsedCount:       0,
				ExpiresAt:       expiresAt,
				CreatedByUserID: createdByUserID,
				Note:            note,
			}
			if err := s.inviteCodeRepo.Create(code); err != nil {
				lastErr = err
				continue
			}
			createdIDs = append(createdIDs, code.ID)
			result.Items = append(result.Items, domain.InviteCodeCreateItem{
				Code:       plain,
				InviteCode: code,
			})
			lastErr = nil
			break
		}
		if lastErr != nil {
			for _, id := range createdIDs {
				if err := s.inviteCodeRepo.Delete(tenantID, id); err != nil && err != domain.ErrNotFound {
					log.Printf("[Admin] Failed to cleanup invite code %d after create error: %v", id, err)
				}
			}
			return nil, lastErr
		}
	}
	return result, nil
}

func (s *AdminService) UpdateInviteCode(tenantID uint64, code *domain.InviteCode) error {
	if s.inviteCodeRepo == nil {
		return fmt.Errorf("invite code repository not configured")
	}
	if code.TenantID != 0 && code.TenantID != tenantID {
		return domain.ErrNotFound
	}
	current, err := s.inviteCodeRepo.GetByID(tenantID, code.ID)
	if err != nil {
		return err
	}
	if current == nil {
		return domain.ErrNotFound
	}
	if code.Status != "" && code.Status != current.Status {
		if code.Status == domain.InviteCodeStatusActive {
			desiredMax := code.MaxUses
			if desiredMax > 0 && current.UsedCount >= desiredMax {
				return domain.ErrInvalidState
			}
		}
	}
	return s.inviteCodeRepo.Update(tenantID, code)
}

func (s *AdminService) DeleteInviteCode(tenantID uint64, id uint64) error {
	if s.inviteCodeRepo == nil {
		return fmt.Errorf("invite code repository not configured")
	}
	return s.inviteCodeRepo.Delete(tenantID, id)
}

func (s *AdminService) ListInviteCodeUsages(tenantID uint64, codeID uint64) ([]*domain.InviteCodeUsage, error) {
	if s.inviteCodeUsageRepo == nil {
		return nil, fmt.Errorf("invite code usage repository not configured")
	}
	return s.inviteCodeUsageRepo.ListByCodeID(tenantID, codeID)
}

// generateAPIToken creates a new random token
// Returns: plain token, prefix for display, error if generation fails
func generateAPIToken() (plain string, prefix string, err error) {
	const tokenPrefix = "maxx_"
	const tokenPrefixDisplayLen = 24

	// Generate 32 random bytes (64 hex chars)
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", fmt.Errorf("failed to generate random token: %w", err)
	}

	plain = tokenPrefix + hex.EncodeToString(bytes)

	// Create display prefix (e.g., "maxx_abc12345...")
	if len(plain) > tokenPrefixDisplayLen {
		prefix = plain[:tokenPrefixDisplayLen] + "..."
	} else {
		prefix = plain
	}

	return plain, prefix, nil
}

func generateInviteCode() (plain string, hash string, prefix string, err error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", "", fmt.Errorf("failed to generate invite code: %w", err)
	}
	plain = strings.ToUpper(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(bytes))
	hash = domain.HashInviteCode(plain)
	prefix = domain.InviteCodePrefix(plain)
	return plain, hash, prefix, nil
}

// ===== Model Mapping API =====

// GetModelMappings returns all model mappings
func (s *AdminService) GetModelMappings(tenantID uint64) ([]*domain.ModelMapping, error) {
	return s.modelMappingRepo.List(tenantID)
}

// GetModelMapping returns a model mapping by ID
func (s *AdminService) GetModelMapping(tenantID uint64, id uint64) (*domain.ModelMapping, error) {
	return s.modelMappingRepo.GetByID(tenantID, id)
}

// CreateModelMapping creates a new model mapping
func (s *AdminService) CreateModelMapping(tenantID uint64, mapping *domain.ModelMapping) error {
	mapping.TenantID = tenantID
	return s.modelMappingRepo.Create(mapping)
}

// UpdateModelMapping updates an existing model mapping
func (s *AdminService) UpdateModelMapping(tenantID uint64, mapping *domain.ModelMapping) error {
	return s.modelMappingRepo.Update(mapping)
}

// DeleteModelMapping deletes a model mapping by ID
func (s *AdminService) DeleteModelMapping(tenantID uint64, id uint64) error {
	return s.modelMappingRepo.Delete(tenantID, id)
}

// ClearAllModelMappings deletes all model mappings (both builtin and non-builtin)
func (s *AdminService) ClearAllModelMappings(tenantID uint64) error {
	return s.modelMappingRepo.ClearAll(tenantID)
}

// ===== Response Model API =====

// GetResponseModelNames returns all unique response model names
func (s *AdminService) GetResponseModelNames() ([]string, error) {
	return s.responseModelRepo.ListNames()
}

// ResetModelMappingsToDefaults re-seeds default builtin mappings
func (s *AdminService) ResetModelMappingsToDefaults(tenantID uint64) error {
	return s.modelMappingRepo.SeedDefaults(tenantID)
}

// GetAvailableClientTypes returns all available client types for model mapping
func (s *AdminService) GetAvailableClientTypes() []domain.ClientType {
	return []domain.ClientType{
		"", // Empty means applies to all
		domain.ClientTypeClaude,
		domain.ClientTypeOpenAI,
		domain.ClientTypeGemini,
	}
}

// ===== Usage Stats API =====

// GetUsageStats queries usage statistics with optional filters
func (s *AdminService) GetUsageStats(tenantID uint64, filter repository.UsageStatsFilter) ([]*domain.UsageStats, error) {
	return s.usageStatsRepo.Query(tenantID, filter)
}

// GetDashboardData returns all dashboard data in a single query
func (s *AdminService) GetDashboardData(tenantID uint64) (*domain.DashboardData, error) {
	return s.usageStatsRepo.QueryDashboardData(tenantID)
}

// RecalculateUsageStatsProgress represents progress update for usage stats recalculation
type RecalculateUsageStatsProgress struct {
	Phase      string `json:"phase"`      // "clearing", "aggregating", "rollup", "completed"
	Current    int    `json:"current"`    // Current step being processed
	Total      int    `json:"total"`      // Total steps to process
	Percentage int    `json:"percentage"` // 0-100
	Message    string `json:"message"`    // Human-readable message
}

// RecalculateUsageStats clears all usage stats and recalculates from raw data
// This only re-aggregates usage stats, it does NOT recalculate costs
func (s *AdminService) RecalculateUsageStats() error {
	// Create progress channel
	progressChan := make(chan domain.Progress, 10)

	// Start goroutine to listen to progress and broadcast via WebSocket
	go func() {
		for progress := range progressChan {
			if s.broadcaster != nil {
				s.broadcaster.BroadcastMessage("recalculate_stats_progress", RecalculateUsageStatsProgress{
					Phase:      progress.Phase,
					Current:    progress.Current,
					Total:      progress.Total,
					Percentage: progress.Percentage,
					Message:    progress.Message,
				})
			}
		}
	}()

	// Call repository method with progress channel
	err := s.usageStatsRepo.ClearAndRecalculateWithProgress(0, progressChan)

	// Close channel when done
	close(progressChan)

	return err
}

// RecalculateCostsResult holds the result of cost recalculation
type RecalculateCostsResult struct {
	TotalAttempts   int    `json:"totalAttempts"`
	UpdatedAttempts int    `json:"updatedAttempts"`
	UpdatedRequests int    `json:"updatedRequests"`
	Message         string `json:"message"`
}

// RecalculateCostsProgress represents progress update for cost recalculation
type RecalculateCostsProgress struct {
	Phase      string `json:"phase"`      // "calculating", "updating_attempts", "updating_requests", "aggregating_stats", "completed"
	Current    int    `json:"current"`    // Current item being processed
	Total      int    `json:"total"`      // Total items to process
	Percentage int    `json:"percentage"` // 0-100
	Message    string `json:"message"`    // Human-readable message
}

// RecalculateCosts recalculates cost for all attempts using the current price table
// and updates the parent requests' cost accordingly (with streaming batch processing)
func (s *AdminService) RecalculateCosts() (*RecalculateCostsResult, error) {
	result := &RecalculateCostsResult{}

	// Helper to broadcast progress
	broadcastProgress := func(phase string, current, total int, message string) {
		if s.broadcaster == nil {
			return
		}
		percentage := 0
		if total > 0 {
			percentage = current * 100 / total
		}
		s.broadcaster.BroadcastMessage("recalculate_costs_progress", RecalculateCostsProgress{
			Phase:      phase,
			Current:    current,
			Total:      total,
			Percentage: percentage,
			Message:    message,
		})
	}

	// 1. Get total count first
	broadcastProgress("calculating", 0, 0, "Counting attempts...")
	totalCount, err := s.attemptRepo.CountAll()
	if err != nil {
		return nil, fmt.Errorf("failed to count attempts: %w", err)
	}
	result.TotalAttempts = int(totalCount)

	if totalCount == 0 {
		result.Message = "No attempts to recalculate"
		broadcastProgress("completed", 0, 0, result.Message)
		return result, nil
	}

	broadcastProgress("calculating", 0, int(totalCount), fmt.Sprintf("Processing %d attempts...", totalCount))

	processedCount := 0
	const batchSize = 100

	// 2. Stream through attempts, process and update each batch immediately
	err = s.attemptRepo.StreamForCostCalc(batchSize, func(batch []*domain.AttemptCostData) error {
		attemptUpdates := make(map[uint64]domain.AttemptCostUpdate, len(batch))

		for _, attempt := range batch {
			// RecalcFromCostData 内部已经处理:历史 multiplier 保留、Cost / ModelPriceID
			// 任一不一致都触发刷新(后者用于"金额相同但价格被替换成等额新版本"的审计链刷新)。
			if _, update, changed := pricing.RecalcFromCostData(attempt); changed {
				attemptUpdates[attempt.ID] = update
			}
			processedCount++
		}

		if len(attemptUpdates) > 0 {
			// 把写入失败传播出去:吞掉的话父请求 cost 会基于内存重算值更新,
			// 跟数据库里实际 attempt 行不一致,后续审计很难发现。
			if err := s.attemptRepo.BatchUpdateCosts(attemptUpdates); err != nil {
				return fmt.Errorf("batch update attempts: %w", err)
			}
			result.UpdatedAttempts += len(attemptUpdates)
		}

		broadcastProgress("calculating", processedCount, int(totalCount),
			fmt.Sprintf("Processed %d/%d attempts", processedCount, totalCount))

		// 给 UI 一点时间消费 WebSocket 消息;否则进度条会一段段跳。
		time.Sleep(50 * time.Millisecond)

		return nil
	})

	if err != nil {
		// Step 2 failed (attempt-cost rewrite). 把 phase 改成 failed 让 UI 不会卡在
		// 最后一次 "calculating N/M";否则进度条会永远停在那里直到超时。
		broadcastProgress("failed", processedCount, int(totalCount),
			fmt.Sprintf("failed to stream attempts: %v", err))
		return nil, fmt.Errorf("failed to stream attempts: %w", err)
	}

	// 3. Recalculate request costs from attempts (with progress via channel).
	// 记一下最后一次见到的 progress,失败时用 request 单位重放(避免 step-2 attempt 单位串台)。
	progressChan := make(chan domain.Progress, 10)
	var lastReqProgress domain.Progress
	var progressMu sync.Mutex
	progressDone := make(chan struct{})
	go func() {
		defer close(progressDone)
		for progress := range progressChan {
			progressMu.Lock()
			lastReqProgress = progress
			progressMu.Unlock()
			broadcastProgress(progress.Phase, progress.Current, progress.Total, progress.Message)
		}
	}()

	updatedRequests, err := s.proxyRequestRepo.RecalculateCostsFromAttemptsWithProgress(progressChan)
	close(progressChan)
	<-progressDone // 确保 goroutine drain 完成,后面读 lastReqProgress 不竞争

	if err != nil {
		// Step 3 失败时不能继续 broadcast "completed":attempts 行已改、父 request 部分
		// 未同步,UI 报成功会掩盖审计偏差。统一传播错误 + 发 failed phase 让运维知道。
		// counter 单位用 step-3 自己看到的最后一次进度(request 单位),不是 step-2 的 attempt 单位 —
		// 后者会让进度条在失败时跳回到一个无意义的位置。
		broadcastProgress("failed", lastReqProgress.Current, lastReqProgress.Total,
			fmt.Sprintf("failed to recalculate request costs: %v", err))
		return nil, fmt.Errorf("failed to recalculate request costs: %w", err)
	}
	result.UpdatedRequests = int(updatedRequests)

	broadcastProgress("updating_requests", result.UpdatedRequests, result.UpdatedRequests,
		fmt.Sprintf("Updated %d requests", result.UpdatedRequests))

	result.Message = fmt.Sprintf("Recalculated %d attempts, updated %d attempts and %d requests",
		result.TotalAttempts, result.UpdatedAttempts, result.UpdatedRequests)

	broadcastProgress("completed", 100, 100, result.Message)

	log.Printf("[RecalculateCosts] %s", result.Message)
	return result, nil
}

// RecalculateRequestCostResult holds the result of single request cost recalculation
type RecalculateRequestCostResult struct {
	RequestID       uint64 `json:"requestId"`
	OldCost         uint64 `json:"oldCost"`
	NewCost         uint64 `json:"newCost"`
	UpdatedAttempts int    `json:"updatedAttempts"`
	Message         string `json:"message"`
}

// RecalculateRequestCost recalculates cost for a single request and its attempts
func (s *AdminService) RecalculateRequestCost(tenantID uint64, requestID uint64) (*RecalculateRequestCostResult, error) {
	result := &RecalculateRequestCostResult{RequestID: requestID}

	// 1. Get the request
	request, err := s.proxyRequestRepo.GetByID(tenantID, requestID)
	if err != nil {
		return nil, fmt.Errorf("failed to get request: %w", err)
	}
	result.OldCost = request.Cost

	// 2. Get all attempts for this request
	attempts, err := s.attemptRepo.ListByProxyRequestID(requestID)
	if err != nil {
		return nil, fmt.Errorf("failed to list attempts: %w", err)
	}

	var totalCost uint64
	updates := make(map[uint64]domain.AttemptCostUpdate, len(attempts))

	// 3. Recalculate cost for each attempt
	for _, attempt := range attempts {
		// RecalcFromAttempt 同时检查 Cost 和 ModelPriceID,保证价格记录被替换成等额新版本时
		// 旧 ID 也会刷到当前匹配行(否则审计链会指向已删除/旧版价格)。
		newCost, update, changed := pricing.RecalcFromAttempt(attempt)
		totalCost += newCost
		if changed {
			updates[attempt.ID] = update
		}
	}

	// 4. Atomic write:attempt updates + request cost 在一个事务里写,
	// 避免 BatchUpdate 成功后 UpdateCost 失败留下"子刷父没刷"的反向 partial-state window。
	result.NewCost = totalCost
	if err := s.proxyRequestRepo.UpdateCostAtomically(requestID, totalCost, updates); err != nil {
		return nil, fmt.Errorf("failed to update request and attempt costs atomically: %w", err)
	}
	result.UpdatedAttempts = len(updates)

	result.Message = fmt.Sprintf("Recalculated request %d: %d -> %d (updated %d attempts)",
		requestID, result.OldCost, result.NewCost, result.UpdatedAttempts)

	log.Printf("[RecalculateRequestCost] %s", result.Message)
	return result, nil
}

// ===== Model Price API =====

// GetModelPrices returns all current model prices
func (s *AdminService) GetModelPrices() ([]*domain.ModelPrice, error) {
	return s.modelPriceRepo.ListCurrentPrices()
}

// GetModelPrice returns a single model price by ID
func (s *AdminService) GetModelPrice(id uint64) (*domain.ModelPrice, error) {
	return s.modelPriceRepo.GetByID(id)
}

// CreateModelPrice creates a new model price record
func (s *AdminService) CreateModelPrice(price *domain.ModelPrice) error {
	return s.modelPriceRepo.Create(price)
}

// UpdateModelPrice updates an existing model price (creates a new version)
// In practice, this creates a new price record for the same model
func (s *AdminService) UpdateModelPrice(price *domain.ModelPrice) error {
	// For versioned pricing, we create a new record instead of updating
	// Clear the ID so GORM generates a new one
	price.ID = 0
	price.CreatedAt = time.Time{}
	return s.modelPriceRepo.Create(price)
}

// DeleteModelPrice deletes a model price record
func (s *AdminService) DeleteModelPrice(id uint64) error {
	return s.modelPriceRepo.Delete(id)
}

// GetModelPriceHistory returns all price records for a model
func (s *AdminService) GetModelPriceHistory(modelID string) ([]*domain.ModelPrice, error) {
	return s.modelPriceRepo.ListByModelID(modelID)
}

// ResetModelPricesToDefaults resets all model prices to defaults (soft deletes existing)
func (s *AdminService) ResetModelPricesToDefaults() ([]*domain.ModelPrice, error) {
	return s.modelPriceRepo.ResetToDefaults()
}
