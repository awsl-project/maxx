package service

import (
	"strconv"
	"strings"

	"github.com/Bowl42/maxx-next/internal/domain"
	"github.com/Bowl42/maxx-next/internal/repository"
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
	if err := s.providerRepo.Create(provider); err != nil {
		return err
	}
	// Refresh adapter cache for the new provider
	if s.adapterRefresher != nil {
		s.adapterRefresher.RefreshAdapter(provider)
	}
	// Auto-create routes for each supported client type
	s.syncProviderRoutes(provider, nil)
	return nil
}

func (s *AdminService) UpdateProvider(provider *domain.Provider) error {
	// Get old provider to compare supportedClientTypes
	var oldSupportedClientTypes []domain.ClientType
	if oldProvider, err := s.providerRepo.GetByID(provider.ID); err == nil && oldProvider != nil {
		oldSupportedClientTypes = make([]domain.ClientType, len(oldProvider.SupportedClientTypes))
		copy(oldSupportedClientTypes, oldProvider.SupportedClientTypes)
	}

	if err := s.providerRepo.Update(provider); err != nil {
		return err
	}
	// Refresh adapter cache for the updated provider
	if s.adapterRefresher != nil {
		s.adapterRefresher.RefreshAdapter(provider)
	}
	// Sync routes based on supportedClientTypes changes
	s.syncProviderRoutesWithOldTypes(provider, oldSupportedClientTypes)
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

func (s *AdminService) GetProxyRequest(id uint64) (*domain.ProxyRequest, error) {
	return s.proxyRequestRepo.GetByID(id)
}

func (s *AdminService) GetProxyUpstreamAttempts(proxyRequestID uint64) ([]*domain.ProxyUpstreamAttempt, error) {
	return s.attemptRepo.ListByProxyRequestID(proxyRequestID)
}

func (s *AdminService) GetProviderStats(clientType string) (map[uint64]*domain.ProviderStats, error) {
	return s.attemptRepo.GetProviderStats(clientType)
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

func (s *AdminService) syncProviderRoutes(provider *domain.Provider, oldProvider *domain.Provider) {
	var oldClientTypes []domain.ClientType
	if oldProvider != nil {
		oldClientTypes = oldProvider.SupportedClientTypes
	}
	s.syncProviderRoutesWithOldTypes(provider, oldClientTypes)
}

func (s *AdminService) syncProviderRoutesWithOldTypes(provider *domain.Provider, oldClientTypes []domain.ClientType) {
	allRoutes, _ := s.routeRepo.List()

	oldClientTypesSet := make(map[domain.ClientType]bool)
	for _, ct := range oldClientTypes {
		oldClientTypesSet[ct] = true
	}

	newClientTypes := make(map[domain.ClientType]bool)
	for _, ct := range provider.SupportedClientTypes {
		newClientTypes[ct] = true
	}

	// Delete native routes for removed client types
	// Use FindByKey to correctly locate the route by (projectID=0, providerID, clientType)
	for ct := range oldClientTypesSet {
		if !newClientTypes[ct] {
			// Find route by key instead of iterating all routes
			if route, err := s.routeRepo.FindByKey(0, provider.ID, ct); err == nil && route != nil && route.IsNative {
				s.routeRepo.Delete(route.ID)
			}
		}
	}

	// Create or update native routes for added client types
	for ct := range newClientTypes {
		if !oldClientTypesSet[ct] {
			// Check if route already exists for this (projectID=0, providerID, clientType)
			existingRoute, err := s.routeRepo.FindByKey(0, provider.ID, ct)
			if err == nil && existingRoute != nil {
				// Route exists, just update isNative if needed
				if !existingRoute.IsNative {
					existingRoute.IsNative = true
					s.routeRepo.Update(existingRoute)
				}
				continue
			}

			// Create new native route
			maxPosition := 0
			for _, route := range allRoutes {
				if route.ClientType == ct && route.Position > maxPosition {
					maxPosition = route.Position
				}
			}

			newRoute := &domain.Route{
				IsEnabled:     true,
				IsNative:      true,
				ProjectID:     0,
				ClientType:    ct,
				ProviderID:    provider.ID,
				Position:      maxPosition + 1,
				RetryConfigID: 0,
			}
			s.routeRepo.Create(newRoute)
		}
	}
}
