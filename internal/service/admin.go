package service

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider/antigravity"
	"github.com/awsl-project/maxx/internal/adapter/provider/kiro"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
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
	serverAddr          string
	adapterRefresher    ProviderAdapterRefresher
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
	serverAddr string,
	adapterRefresher ProviderAdapterRefresher,
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
		serverAddr:          serverAddr,
		adapterRefresher:    adapterRefresher,
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

func (s *AdminService) GetProxyRequestsCursor(limit int, before, after uint64) (*CursorPaginationResult, error) {
	items, err := s.proxyRequestRepo.ListCursor(limit+1, before, after)
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

func (s *AdminService) GetProxyRequest(id uint64) (*domain.ProxyRequest, error) {
	return s.proxyRequestRepo.GetByID(id)
}

func (s *AdminService) GetProxyUpstreamAttempts(proxyRequestID uint64) ([]*domain.ProxyUpstreamAttempt, error) {
	return s.attemptRepo.ListByProxyRequestID(proxyRequestID)
}

func (s *AdminService) GetProviderStats(clientType string, projectID uint64) (map[uint64]*domain.ProviderStats, error) {
	return s.attemptRepo.GetProviderStats(clientType, projectID)
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
	return s.settingRepo.Set(key, value)
}

func (s *AdminService) DeleteSetting(key string) error {
	return s.settingRepo.Delete(key)
}

// ===== Antigravity Global Settings API =====

// ModelMappingRule represents a single model mapping rule (for API)
type ModelMappingRule struct {
	Pattern string `json:"pattern"` // Source pattern, supports * wildcard
	Target  string `json:"target"`  // Target model name
}

// AntigravityGlobalSettings represents the global Antigravity configuration
type AntigravityGlobalSettings struct {
	ModelMappingRules     []ModelMappingRule `json:"modelMappingRules"`
	AvailableTargetModels []string           `json:"availableTargetModels"`
}

// GetAntigravityGlobalSettings retrieves the global Antigravity settings
// If no custom mapping exists, returns the preset mapping as default
func (s *AdminService) GetAntigravityGlobalSettings() (*AntigravityGlobalSettings, error) {
	settings := &AntigravityGlobalSettings{
		ModelMappingRules:     []ModelMappingRule{},
		AvailableTargetModels: antigravity.GetAvailableTargetModels(),
	}

	// Get model mapping rules from database
	rulesJSON, err := s.settingRepo.Get(domain.SettingKeyAntigravityModelMapping)
	if err == nil && rulesJSON != "" {
		// Use ParseModelMappingRules which handles both new array format and legacy map format
		agRules, parseErr := antigravity.ParseModelMappingRules(rulesJSON)
		if parseErr != nil {
			return nil, parseErr
		}
		// Convert antigravity.ModelMappingRule to service.ModelMappingRule
		settings.ModelMappingRules = make([]ModelMappingRule, len(agRules))
		for i, r := range agRules {
			settings.ModelMappingRules[i] = ModelMappingRule{Pattern: r.Pattern, Target: r.Target}
		}
	}

	// If no rules exist, initialize with preset rules
	if len(settings.ModelMappingRules) == 0 {
		defaultRules := antigravity.GetDefaultModelMappingRules()
		settings.ModelMappingRules = make([]ModelMappingRule, len(defaultRules))
		for i, r := range defaultRules {
			settings.ModelMappingRules[i] = ModelMappingRule{Pattern: r.Pattern, Target: r.Target}
		}
		// Save to database
		if rulesJSON, err := json.Marshal(settings.ModelMappingRules); err == nil {
			s.settingRepo.Set(domain.SettingKeyAntigravityModelMapping, string(rulesJSON))
		}
	}

	return settings, nil
}

// UpdateAntigravityGlobalSettings updates the global Antigravity settings
func (s *AdminService) UpdateAntigravityGlobalSettings(settings *AntigravityGlobalSettings) error {
	// Update model mapping rules
	if settings.ModelMappingRules != nil {
		rulesJSON, err := json.Marshal(settings.ModelMappingRules)
		if err != nil {
			return err
		}
		if err := s.settingRepo.Set(domain.SettingKeyAntigravityModelMapping, string(rulesJSON)); err != nil {
			return err
		}
	} else {
		// Clear rules if nil
		if err := s.settingRepo.Set(domain.SettingKeyAntigravityModelMapping, "[]"); err != nil {
			return err
		}
	}

	return nil
}

// ResetAntigravityGlobalSettings resets the model mapping to preset defaults
func (s *AdminService) ResetAntigravityGlobalSettings() (*AntigravityGlobalSettings, error) {
	defaultRules := antigravity.GetDefaultModelMappingRules()
	rules := make([]ModelMappingRule, len(defaultRules))
	for i, r := range defaultRules {
		rules[i] = ModelMappingRule{Pattern: r.Pattern, Target: r.Target}
	}

	rulesJSON, err := json.Marshal(rules)
	if err != nil {
		return nil, err
	}

	if err := s.settingRepo.Set(domain.SettingKeyAntigravityModelMapping, string(rulesJSON)); err != nil {
		return nil, err
	}

	return &AntigravityGlobalSettings{
		ModelMappingRules:     rules,
		AvailableTargetModels: antigravity.GetAvailableTargetModels(),
	}, nil
}

// ===== Kiro Global Settings API =====

// KiroGlobalSettings contains global Kiro settings
type KiroGlobalSettings struct {
	ModelMappingRules     []ModelMappingRule `json:"modelMappingRules"`
	AvailableTargetModels []string           `json:"availableTargetModels"`
}

// GetKiroGlobalSettings retrieves the global Kiro settings
// If no custom mapping exists, returns the preset mapping as default
func (s *AdminService) GetKiroGlobalSettings() (*KiroGlobalSettings, error) {
	settings := &KiroGlobalSettings{
		ModelMappingRules:     []ModelMappingRule{},
		AvailableTargetModels: kiro.AvailableTargetModels,
	}

	// Get model mapping rules from database
	rulesJSON, err := s.settingRepo.Get(domain.SettingKeyKiroModelMapping)
	if err == nil && rulesJSON != "" {
		// Use ParseModelMappingRules which handles both new array format and legacy map format
		kiroRules, parseErr := kiro.ParseModelMappingRules(rulesJSON)
		if parseErr != nil {
			return nil, parseErr
		}
		// Convert kiro.ModelMappingRule to service.ModelMappingRule
		settings.ModelMappingRules = make([]ModelMappingRule, len(kiroRules))
		for i, r := range kiroRules {
			settings.ModelMappingRules[i] = ModelMappingRule{Pattern: r.Pattern, Target: r.Target}
		}
	}

	// If no rules exist, initialize with preset rules
	if len(settings.ModelMappingRules) == 0 {
		defaultRules := kiro.GetDefaultModelMappingRules()
		settings.ModelMappingRules = make([]ModelMappingRule, len(defaultRules))
		for i, r := range defaultRules {
			settings.ModelMappingRules[i] = ModelMappingRule{Pattern: r.Pattern, Target: r.Target}
		}
		// Save to database
		if rulesJSON, err := json.Marshal(settings.ModelMappingRules); err == nil {
			s.settingRepo.Set(domain.SettingKeyKiroModelMapping, string(rulesJSON))
		}
	}

	return settings, nil
}

// UpdateKiroGlobalSettings updates the global Kiro settings
func (s *AdminService) UpdateKiroGlobalSettings(settings *KiroGlobalSettings) error {
	// Update model mapping rules
	if settings.ModelMappingRules != nil {
		rulesJSON, err := json.Marshal(settings.ModelMappingRules)
		if err != nil {
			return err
		}
		if err := s.settingRepo.Set(domain.SettingKeyKiroModelMapping, string(rulesJSON)); err != nil {
			return err
		}
	} else {
		// Clear rules if nil
		if err := s.settingRepo.Set(domain.SettingKeyKiroModelMapping, "[]"); err != nil {
			return err
		}
	}

	return nil
}

// ResetKiroGlobalSettings resets the model mapping to preset defaults
func (s *AdminService) ResetKiroGlobalSettings() (*KiroGlobalSettings, error) {
	defaultRules := kiro.GetDefaultModelMappingRules()
	rules := make([]ModelMappingRule, len(defaultRules))
	for i, r := range defaultRules {
		rules[i] = ModelMappingRule{Pattern: r.Pattern, Target: r.Target}
	}

	rulesJSON, err := json.Marshal(rules)
	if err != nil {
		return nil, err
	}

	if err := s.settingRepo.Set(domain.SettingKeyKiroModelMapping, string(rulesJSON)); err != nil {
		return nil, err
	}

	return &KiroGlobalSettings{
		ModelMappingRules:     rules,
		AvailableTargetModels: kiro.AvailableTargetModels,
	}, nil
}

// ===== Proxy Status API =====

type ProxyStatus struct {
	Running bool   `json:"running"`
	Address string `json:"address"`
	Port    int    `json:"port"`
}

func (s *AdminService) GetProxyStatus() *ProxyStatus {
	addr := s.serverAddr
	port := 9880 // default
	if idx := strings.LastIndex(addr, ":"); idx >= 0 {
		if p, err := strconv.Atoi(addr[idx+1:]); err == nil {
			port = p
		}
	}

	displayAddr := "localhost"
	if port != 80 {
		displayAddr = "localhost:" + strconv.Itoa(port)
	}

	return &ProxyStatus{
		Running: true,
		Address: displayAddr,
		Port:    port,
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
		// Antigravity natively supports Claude, OpenAI, and Gemini
		provider.SupportedClientTypes = []domain.ClientType{
			domain.ClientTypeClaude,
			domain.ClientTypeOpenAI,
			domain.ClientTypeGemini,
		}
	case "kiro":
		// Kiro natively supports Claude protocol only
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
