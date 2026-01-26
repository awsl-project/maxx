package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/event"
	"github.com/awsl-project/maxx/internal/pricing"
	"github.com/awsl-project/maxx/internal/repository"
	"github.com/awsl-project/maxx/internal/usage"
	"github.com/awsl-project/maxx/internal/version"
)

// ProviderAdapterRefresher is an interface for refreshing provider adapters
// Implemented by Router to receive notifications when providers change
type ProviderAdapterRefresher interface {
	RefreshAdapter(p *domain.Provider) error
	RemoveAdapter(providerID uint64)
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

func (s *AdminService) GetProviders() ([]*domain.Provider, error) {
	return s.providerRepo.List()
}

func (s *AdminService) GetProvider(id uint64) (*domain.Provider, error) {
	return s.providerRepo.GetByID(id)
}

func (s *AdminService) CreateProvider(provider *domain.Provider) error {
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

func (s *AdminService) UpdateProvider(provider *domain.Provider) error {
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

func (s *AdminService) DeleteProvider(id uint64) error {
	// Delete related routes first
	routes, _ := s.routeRepo.List()
	for _, route := range routes {
		if route.ProviderID == id {
			s.routeRepo.Delete(route.ID)
		}
	}
	// Remove adapter from cache
	if s.adapterRefresher != nil {
		s.adapterRefresher.RemoveAdapter(id)
	}
	return s.providerRepo.Delete(id)
}

// ExportProviders exports all providers for backup/transfer
// Returns providers without ID and timestamps for clean import
func (s *AdminService) ExportProviders() ([]*domain.Provider, error) {
	providers, err := s.providerRepo.List()
	if err != nil {
		return nil, err
	}
	// Return as-is, the handler will handle JSON serialization
	return providers, nil
}

// ImportProviders imports providers from exported data
// Creates new providers, skipping duplicates by name
func (s *AdminService) ImportProviders(providers []*domain.Provider) (*ImportResult, error) {
	result := &ImportResult{
		Imported: 0,
		Skipped:  0,
		Errors:   []string{},
	}

	// Get existing providers for duplicate detection
	existing, err := s.providerRepo.List()
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
		if err := s.CreateProvider(provider); err != nil {
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

func (s *AdminService) GetRoutes() ([]*domain.Route, error) {
	return s.routeRepo.List()
}

func (s *AdminService) GetRoute(id uint64) (*domain.Route, error) {
	return s.routeRepo.GetByID(id)
}

func (s *AdminService) CreateRoute(route *domain.Route) error {
	return s.routeRepo.Create(route)
}

func (s *AdminService) UpdateRoute(route *domain.Route) error {
	return s.routeRepo.Update(route)
}

func (s *AdminService) BatchUpdateRoutePositions(updates []domain.RoutePositionUpdate) error {
	return s.routeRepo.BatchUpdatePositions(updates)
}

func (s *AdminService) DeleteRoute(id uint64) error {
	return s.routeRepo.Delete(id)
}

// ===== Project API =====

func (s *AdminService) GetProjects() ([]*domain.Project, error) {
	return s.projectRepo.List()
}

func (s *AdminService) GetProject(id uint64) (*domain.Project, error) {
	return s.projectRepo.GetByID(id)
}

func (s *AdminService) GetProjectBySlug(slug string) (*domain.Project, error) {
	return s.projectRepo.GetBySlug(slug)
}

func (s *AdminService) CreateProject(project *domain.Project) error {
	return s.projectRepo.Create(project)
}

func (s *AdminService) UpdateProject(project *domain.Project) error {
	return s.projectRepo.Update(project)
}

func (s *AdminService) DeleteProject(id uint64) error {
	return s.projectRepo.Delete(id)
}

// ===== Session API =====

func (s *AdminService) GetSessions() ([]*domain.Session, error) {
	return s.sessionRepo.List()
}

// UpdateSessionProjectResult holds the result of updating session project
type UpdateSessionProjectResult struct {
	Session         *domain.Session `json:"session"`
	UpdatedRequests int64           `json:"updatedRequests"`
}

// UpdateSessionProject updates the session's projectID and all related requests
func (s *AdminService) UpdateSessionProject(sessionID string, projectID uint64) (*UpdateSessionProjectResult, error) {
	// Get the session first
	session, err := s.sessionRepo.GetBySessionID(sessionID)
	if err != nil {
		return nil, err
	}

	// Update session's projectID
	session.ProjectID = projectID
	if err := s.sessionRepo.Update(session); err != nil {
		return nil, err
	}

	// Batch update all requests with this sessionID
	updatedCount, err := s.proxyRequestRepo.UpdateProjectIDBySessionID(sessionID, projectID)
	if err != nil {
		return nil, err
	}

	return &UpdateSessionProjectResult{
		Session:         session,
		UpdatedRequests: updatedCount,
	}, nil
}

// RejectSession marks a session as rejected with current timestamp
func (s *AdminService) RejectSession(sessionID string) (*domain.Session, error) {
	// Get the session first
	session, err := s.sessionRepo.GetBySessionID(sessionID)
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

func (s *AdminService) GetRetryConfigs() ([]*domain.RetryConfig, error) {
	return s.retryConfigRepo.List()
}

func (s *AdminService) GetRetryConfig(id uint64) (*domain.RetryConfig, error) {
	return s.retryConfigRepo.GetByID(id)
}

func (s *AdminService) CreateRetryConfig(config *domain.RetryConfig) error {
	return s.retryConfigRepo.Create(config)
}

func (s *AdminService) UpdateRetryConfig(config *domain.RetryConfig) error {
	return s.retryConfigRepo.Update(config)
}

func (s *AdminService) DeleteRetryConfig(id uint64) error {
	return s.retryConfigRepo.Delete(id)
}

// ===== RoutingStrategy API =====

func (s *AdminService) GetRoutingStrategies() ([]*domain.RoutingStrategy, error) {
	return s.routingStrategyRepo.List()
}

func (s *AdminService) GetRoutingStrategy(id uint64) (*domain.RoutingStrategy, error) {
	return s.routingStrategyRepo.GetByProjectID(id)
}

func (s *AdminService) CreateRoutingStrategy(strategy *domain.RoutingStrategy) error {
	return s.routingStrategyRepo.Create(strategy)
}

func (s *AdminService) UpdateRoutingStrategy(strategy *domain.RoutingStrategy) error {
	return s.routingStrategyRepo.Update(strategy)
}

func (s *AdminService) DeleteRoutingStrategy(id uint64) error {
	return s.routingStrategyRepo.Delete(id)
}

// ===== ProxyRequest API =====

func (s *AdminService) GetProxyRequests(limit, offset int) ([]*domain.ProxyRequest, error) {
	return s.proxyRequestRepo.List(limit, offset)
}

// CursorPaginationResult 游标分页结果
type CursorPaginationResult struct {
	Items   []*domain.ProxyRequest `json:"items"`
	HasMore bool                   `json:"hasMore"`
	FirstID uint64                 `json:"firstId,omitempty"`
	LastID  uint64                 `json:"lastId,omitempty"`
}

func (s *AdminService) GetProxyRequestsCursor(limit int, before, after uint64, filter *repository.ProxyRequestFilter) (*CursorPaginationResult, error) {
	items, err := s.proxyRequestRepo.ListCursor(limit+1, before, after, filter)
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

func (s *AdminService) GetProxyRequestsCount() (int64, error) {
	return s.proxyRequestRepo.Count()
}

func (s *AdminService) GetProxyRequestsCountWithFilter(filter *repository.ProxyRequestFilter) (int64, error) {
	return s.proxyRequestRepo.CountWithFilter(filter)
}

func (s *AdminService) GetProxyRequest(id uint64) (*domain.ProxyRequest, error) {
	return s.proxyRequestRepo.GetByID(id)
}

func (s *AdminService) GetActiveProxyRequests() ([]*domain.ProxyRequest, error) {
	return s.proxyRequestRepo.ListActive()
}

func (s *AdminService) GetProxyUpstreamAttempts(proxyRequestID uint64) ([]*domain.ProxyUpstreamAttempt, error) {
	return s.attemptRepo.ListByProxyRequestID(proxyRequestID)
}

func (s *AdminService) GetProviderStats(clientType string, projectID uint64) (map[uint64]*domain.ProviderStats, error) {
	return s.usageStatsRepo.GetProviderStats(clientType, projectID)
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
	if err := s.settingRepo.Set(key, value); err != nil {
		return err
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
		// Antigravity natively supports Claude and Gemini
		// OpenAI requests will be converted to Claude format by Executor
		provider.SupportedClientTypes = []domain.ClientType{
			domain.ClientTypeClaude,
			domain.ClientTypeGemini,
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
	case "custom":
		// Custom providers use their configured SupportedClientTypes
		// If not set, default to OpenAI
		if len(provider.SupportedClientTypes) == 0 {
			provider.SupportedClientTypes = []domain.ClientType{domain.ClientTypeOpenAI}
		}
	}
}

// ===== API Token API =====

func (s *AdminService) GetAPITokens() ([]*domain.APIToken, error) {
	return s.apiTokenRepo.List()
}

func (s *AdminService) GetAPIToken(id uint64) (*domain.APIToken, error) {
	return s.apiTokenRepo.GetByID(id)
}

// CreateAPIToken creates a new API token and returns the plain token (only shown once)
func (s *AdminService) CreateAPIToken(name, description string, projectID uint64, expiresAt *time.Time) (*domain.APITokenCreateResult, error) {
	// Generate token
	plain, prefix, err := generateAPIToken()
	if err != nil {
		return nil, err
	}

	token := &domain.APIToken{
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

func (s *AdminService) UpdateAPIToken(token *domain.APIToken) error {
	return s.apiTokenRepo.Update(token)
}

func (s *AdminService) DeleteAPIToken(id uint64) error {
	return s.apiTokenRepo.Delete(id)
}

// generateAPIToken creates a new random token
// Returns: plain token, prefix for display, error if generation fails
func generateAPIToken() (plain string, prefix string, err error) {
	const tokenPrefix = "maxx_"
	const tokenPrefixDisplayLen = 12

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

// ===== Model Mapping API =====

// GetModelMappings returns all model mappings
func (s *AdminService) GetModelMappings() ([]*domain.ModelMapping, error) {
	return s.modelMappingRepo.List()
}

// GetModelMapping returns a model mapping by ID
func (s *AdminService) GetModelMapping(id uint64) (*domain.ModelMapping, error) {
	return s.modelMappingRepo.GetByID(id)
}

// CreateModelMapping creates a new model mapping
func (s *AdminService) CreateModelMapping(mapping *domain.ModelMapping) error {
	return s.modelMappingRepo.Create(mapping)
}

// UpdateModelMapping updates an existing model mapping
func (s *AdminService) UpdateModelMapping(mapping *domain.ModelMapping) error {
	return s.modelMappingRepo.Update(mapping)
}

// DeleteModelMapping deletes a model mapping by ID
func (s *AdminService) DeleteModelMapping(id uint64) error {
	return s.modelMappingRepo.Delete(id)
}

// ClearAllModelMappings deletes all model mappings (both builtin and non-builtin)
func (s *AdminService) ClearAllModelMappings() error {
	return s.modelMappingRepo.ClearAll()
}

// ===== Response Model API =====

// GetResponseModelNames returns all unique response model names
func (s *AdminService) GetResponseModelNames() ([]string, error) {
	return s.responseModelRepo.ListNames()
}

// ResetModelMappingsToDefaults re-seeds default builtin mappings
func (s *AdminService) ResetModelMappingsToDefaults() error {
	return s.modelMappingRepo.SeedDefaults()
}

// GetAvailableClientTypes returns all available client types for model mapping
func (s *AdminService) GetAvailableClientTypes() []domain.ClientType {
	return []domain.ClientType{
		"",                       // Empty means applies to all
		domain.ClientTypeClaude,
		domain.ClientTypeOpenAI,
		domain.ClientTypeGemini,
	}
}

// ===== Usage Stats API =====

// GetUsageStats queries usage statistics with optional filters
func (s *AdminService) GetUsageStats(filter repository.UsageStatsFilter) ([]*domain.UsageStats, error) {
	return s.usageStatsRepo.Query(filter)
}

// GetDashboardData returns all dashboard data in a single query
func (s *AdminService) GetDashboardData() (*domain.DashboardData, error) {
	return s.usageStatsRepo.QueryDashboardData()
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
	err := s.usageStatsRepo.ClearAndRecalculateWithProgress(progressChan)

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
	Phase       string `json:"phase"`       // "calculating", "updating_attempts", "updating_requests", "aggregating_stats", "completed"
	Current     int    `json:"current"`     // Current item being processed
	Total       int    `json:"total"`       // Total items to process
	Percentage  int    `json:"percentage"`  // 0-100
	Message     string `json:"message"`     // Human-readable message
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

	calculator := pricing.GlobalCalculator()
	processedCount := 0
	const batchSize = 100
	affectedRequestIDs := make(map[uint64]struct{})

	// 2. Stream through attempts, process and update each batch immediately
	err = s.attemptRepo.StreamForCostCalc(batchSize, func(batch []*domain.AttemptCostData) error {
		attemptUpdates := make(map[uint64]uint64, len(batch))

		for _, attempt := range batch {
			// Use responseModel if available, otherwise use mappedModel or requestModel
			model := attempt.ResponseModel
			if model == "" {
				model = attempt.MappedModel
			}
			if model == "" {
				model = attempt.RequestModel
			}

			// Build metrics from attempt data
			metrics := &usage.Metrics{
				InputTokens:          attempt.InputTokenCount,
				OutputTokens:         attempt.OutputTokenCount,
				CacheReadCount:       attempt.CacheReadCount,
				CacheCreationCount:   attempt.CacheWriteCount,
				Cache5mCreationCount: attempt.Cache5mWriteCount,
				Cache1hCreationCount: attempt.Cache1hWriteCount,
			}

			// Calculate new cost
			newCost := calculator.Calculate(model, metrics)

			// Track affected request IDs
			affectedRequestIDs[attempt.ProxyRequestID] = struct{}{}

			// Track if attempt needs update
			if newCost != attempt.Cost {
				attemptUpdates[attempt.ID] = newCost
			}

			processedCount++
		}

		// Batch update attempt costs immediately
		if len(attemptUpdates) > 0 {
			if err := s.attemptRepo.BatchUpdateCosts(attemptUpdates); err != nil {
				log.Printf("[RecalculateCosts] Failed to batch update attempts: %v", err)
			} else {
				result.UpdatedAttempts += len(attemptUpdates)
			}
		}

		// Broadcast progress
		broadcastProgress("calculating", processedCount, int(totalCount),
			fmt.Sprintf("Processed %d/%d attempts", processedCount, totalCount))

		// Small delay to allow UI to update (WebSocket messages need time to be processed)
		time.Sleep(50 * time.Millisecond)

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to stream attempts: %w", err)
	}

	// 3. Recalculate request costs from attempts (with progress via channel)
	progressChan := make(chan domain.Progress, 10)
	go func() {
		for progress := range progressChan {
			broadcastProgress(progress.Phase, progress.Current, progress.Total, progress.Message)
		}
	}()

	updatedRequests, err := s.proxyRequestRepo.RecalculateCostsFromAttemptsWithProgress(progressChan)
	close(progressChan)

	if err != nil {
		log.Printf("[RecalculateCosts] Failed to recalculate request costs: %v", err)
	} else {
		result.UpdatedRequests = int(updatedRequests)
	}

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
func (s *AdminService) RecalculateRequestCost(requestID uint64) (*RecalculateRequestCostResult, error) {
	result := &RecalculateRequestCostResult{RequestID: requestID}

	// 1. Get the request
	request, err := s.proxyRequestRepo.GetByID(requestID)
	if err != nil {
		return nil, fmt.Errorf("failed to get request: %w", err)
	}
	result.OldCost = request.Cost

	// 2. Get all attempts for this request
	attempts, err := s.attemptRepo.ListByProxyRequestID(requestID)
	if err != nil {
		return nil, fmt.Errorf("failed to list attempts: %w", err)
	}

	calculator := pricing.GlobalCalculator()
	var totalCost uint64

	// 3. Recalculate cost for each attempt
	for _, attempt := range attempts {
		// Use responseModel if available, otherwise use mappedModel or requestModel
		model := attempt.ResponseModel
		if model == "" {
			model = attempt.MappedModel
		}
		if model == "" {
			model = attempt.RequestModel
		}

		// Build metrics from attempt data
		metrics := &usage.Metrics{
			InputTokens:          attempt.InputTokenCount,
			OutputTokens:         attempt.OutputTokenCount,
			CacheReadCount:       attempt.CacheReadCount,
			CacheCreationCount:   attempt.CacheWriteCount,
			Cache5mCreationCount: attempt.Cache5mWriteCount,
			Cache1hCreationCount: attempt.Cache1hWriteCount,
		}

		// Calculate new cost
		newCost := calculator.Calculate(model, metrics)
		totalCost += newCost

		// Update attempt cost if changed
		if newCost != attempt.Cost {
			if err := s.attemptRepo.UpdateCost(attempt.ID, newCost); err != nil {
				log.Printf("[RecalculateRequestCost] Failed to update attempt %d cost: %v", attempt.ID, err)
				continue
			}
			result.UpdatedAttempts++
		}
	}

	// 4. Update request cost
	result.NewCost = totalCost
	if err := s.proxyRequestRepo.UpdateCost(requestID, totalCost); err != nil {
		return nil, fmt.Errorf("failed to update request cost: %w", err)
	}

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
