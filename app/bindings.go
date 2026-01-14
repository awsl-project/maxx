package app

import (
	"context"
	"fmt"

	"github.com/awsl-project/maxx/internal/cooldown"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/service"
)

// Provider methods

//wails:bind
func (a *App) GetProviders() ([]*domain.Provider, error) {
	return a.adminService.GetProviders()
}

//wails:bind
func (a *App) GetProvider(id uint64) (*domain.Provider, error) {
	return a.adminService.GetProvider(id)
}

//wails:bind
func (a *App) CreateProvider(provider *domain.Provider) error {
	return a.adminService.CreateProvider(provider)
}

//wails:bind
func (a *App) UpdateProvider(provider *domain.Provider) error {
	return a.adminService.UpdateProvider(provider)
}

//wails:bind
func (a *App) DeleteProvider(id uint64) error {
	return a.adminService.DeleteProvider(id)
}

//wails:bind
func (a *App) ExportProviders() ([]*domain.Provider, error) {
	return a.adminService.ExportProviders()
}

//wails:bind
func (a *App) ImportProviders(providers []*domain.Provider) (*service.ImportResult, error) {
	return a.adminService.ImportProviders(providers)
}

// Project methods

//wails:bind
func (a *App) GetProjects() ([]*domain.Project, error) {
	return a.adminService.GetProjects()
}

//wails:bind
func (a *App) GetProject(id uint64) (*domain.Project, error) {
	return a.adminService.GetProject(id)
}

//wails:bind
func (a *App) CreateProject(project *domain.Project) error {
	return a.adminService.CreateProject(project)
}

//wails:bind
func (a *App) UpdateProject(project *domain.Project) error {
	return a.adminService.UpdateProject(project)
}

//wails:bind
func (a *App) DeleteProject(id uint64) error {
	return a.adminService.DeleteProject(id)
}

//wails:bind
func (a *App) GetProjectBySlug(slug string) (*domain.Project, error) {
	return a.adminService.GetProjectBySlug(slug)
}

// Route methods

//wails:bind
func (a *App) GetRoutes() ([]*domain.Route, error) {
	return a.adminService.GetRoutes()
}

//wails:bind
func (a *App) GetRoute(id uint64) (*domain.Route, error) {
	return a.adminService.GetRoute(id)
}

//wails:bind
func (a *App) CreateRoute(route *domain.Route) error {
	return a.adminService.CreateRoute(route)
}

//wails:bind
func (a *App) UpdateRoute(route *domain.Route) error {
	return a.adminService.UpdateRoute(route)
}

//wails:bind
func (a *App) DeleteRoute(id uint64) error {
	return a.adminService.DeleteRoute(id)
}

// Session methods

//wails:bind
func (a *App) GetSessions() ([]*domain.Session, error) {
	return a.adminService.GetSessions()
}

//wails:bind
func (a *App) UpdateSessionProject(sessionID string, projectID uint64) (*service.UpdateSessionProjectResult, error) {
	return a.adminService.UpdateSessionProject(sessionID, projectID)
}

// RetryConfig methods

//wails:bind
func (a *App) GetRetryConfigs() ([]*domain.RetryConfig, error) {
	return a.adminService.GetRetryConfigs()
}

//wails:bind
func (a *App) GetRetryConfig(id uint64) (*domain.RetryConfig, error) {
	return a.adminService.GetRetryConfig(id)
}

//wails:bind
func (a *App) CreateRetryConfig(config *domain.RetryConfig) error {
	return a.adminService.CreateRetryConfig(config)
}

//wails:bind
func (a *App) UpdateRetryConfig(config *domain.RetryConfig) error {
	return a.adminService.UpdateRetryConfig(config)
}

//wails:bind
func (a *App) DeleteRetryConfig(id uint64) error {
	return a.adminService.DeleteRetryConfig(id)
}

// RoutingStrategy methods

//wails:bind
func (a *App) GetRoutingStrategies() ([]*domain.RoutingStrategy, error) {
	return a.adminService.GetRoutingStrategies()
}

//wails:bind
func (a *App) GetRoutingStrategy(id uint64) (*domain.RoutingStrategy, error) {
	return a.adminService.GetRoutingStrategy(id)
}

//wails:bind
func (a *App) CreateRoutingStrategy(strategy *domain.RoutingStrategy) error {
	return a.adminService.CreateRoutingStrategy(strategy)
}

//wails:bind
func (a *App) UpdateRoutingStrategy(strategy *domain.RoutingStrategy) error {
	return a.adminService.UpdateRoutingStrategy(strategy)
}

//wails:bind
func (a *App) DeleteRoutingStrategy(id uint64) error {
	return a.adminService.DeleteRoutingStrategy(id)
}

// ProxyRequest methods

//wails:bind
func (a *App) GetProxyRequests(limit int, before uint64, after uint64) (*service.CursorPaginationResult, error) {
	return a.adminService.GetProxyRequestsCursor(limit, before, after)
}

//wails:bind
func (a *App) GetProxyRequestsCount() (int64, error) {
	return a.adminService.GetProxyRequestsCount()
}

//wails:bind
func (a *App) GetProxyRequest(id uint64) (*domain.ProxyRequest, error) {
	return a.adminService.GetProxyRequest(id)
}

//wails:bind
func (a *App) GetProxyUpstreamAttempts(proxyRequestID uint64) ([]*domain.ProxyUpstreamAttempt, error) {
	return a.adminService.GetProxyUpstreamAttempts(proxyRequestID)
}

// ProxyStatus methods

//wails:bind
func (a *App) GetProxyStatus() *service.ProxyStatus {
	return a.adminService.GetProxyStatus()
}

// ProviderStats methods

//wails:bind
func (a *App) GetProviderStats(clientType string, projectID uint64) (map[uint64]*domain.ProviderStats, error) {
	return a.adminService.GetProviderStats(clientType, projectID)
}

// Settings methods

//wails:bind
func (a *App) GetSettings() (map[string]string, error) {
	return a.adminService.GetSettings()
}

//wails:bind
func (a *App) GetSetting(key string) (string, error) {
	return a.adminService.GetSetting(key)
}

//wails:bind
func (a *App) UpdateSetting(key, value string) error {
	return a.adminService.UpdateSetting(key, value)
}

//wails:bind
func (a *App) DeleteSetting(key string) error {
	return a.adminService.DeleteSetting(key)
}

// Logs methods

//wails:bind
func (a *App) GetLogs(limit int) (*service.LogsResult, error) {
	return a.adminService.GetLogs(limit)
}

// Cooldown methods

//wails:bind
func (a *App) GetCooldowns() map[string]domain.Cooldown {
	cdMap := cooldown.Default().GetAllCooldowns()
	result := make(map[string]domain.Cooldown)
	for key, untilTime := range cdMap {
		keyStr := fmt.Sprintf("%d:%s", key.ProviderID, key.ClientType)
		result[keyStr] = domain.Cooldown{
			ProviderID: key.ProviderID,
			ClientType: key.ClientType,
			UntilTime:  untilTime,
		}
	}
	return result
}

//wails:bind
func (a *App) ClearCooldown(providerID uint64, clientType string) {
	cooldown.Default().ClearCooldown(providerID, clientType)
}

// Antigravity methods

//wails:bind
func (a *App) ValidateAntigravityToken(ctx context.Context, refreshToken string) (*TokenValidationResult, error) {
	return a.antigravitySvc.ValidateToken(ctx, refreshToken)
}

//wails:bind
func (a *App) ValidateAntigravityTokens(ctx context.Context, tokens []string) []*TokenValidationResult {
	return a.antigravitySvc.ValidateTokens(ctx, tokens)
}

//wails:bind
func (a *App) ValidateAntigravityTokenText(ctx context.Context, tokenText string) []*TokenValidationResult {
	return a.antigravitySvc.ValidateTokenText(ctx, tokenText)
}

//wails:bind
func (a *App) GetAntigravityProviderQuota(ctx context.Context, providerID uint64, forceRefresh bool) (*QuotaData, error) {
	return a.antigravitySvc.GetProviderQuota(ctx, providerID, forceRefresh)
}

//wails:bind
func (a *App) StartAntigravityOAuth(ctx context.Context) (map[string]interface{}, error) {
	redirectURI := "http://localhost:19380/antigravity/oauth/callback"
	return a.antigravitySvc.StartOAuth(ctx, redirectURI)
}
