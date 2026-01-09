package repository

import "github.com/Bowl42/maxx-next/internal/domain"

type ProviderRepository interface {
	Create(provider *domain.Provider) error
	Update(provider *domain.Provider) error
	Delete(id uint64) error
	GetByID(id uint64) (*domain.Provider, error)
	List() ([]*domain.Provider, error)
}

type RouteRepository interface {
	Create(route *domain.Route) error
	Update(route *domain.Route) error
	Delete(id uint64) error
	GetByID(id uint64) (*domain.Route, error)
	List() ([]*domain.Route, error)
}

type RoutingStrategyRepository interface {
	Create(strategy *domain.RoutingStrategy) error
	Update(strategy *domain.RoutingStrategy) error
	Delete(id uint64) error
	GetByProjectID(projectID uint64) (*domain.RoutingStrategy, error)
	List() ([]*domain.RoutingStrategy, error)
}

type RetryConfigRepository interface {
	Create(config *domain.RetryConfig) error
	Update(config *domain.RetryConfig) error
	Delete(id uint64) error
	GetByID(id uint64) (*domain.RetryConfig, error)
	GetDefault() (*domain.RetryConfig, error)
	List() ([]*domain.RetryConfig, error)
}

type ProjectRepository interface {
	Create(project *domain.Project) error
	Update(project *domain.Project) error
	Delete(id uint64) error
	GetByID(id uint64) (*domain.Project, error)
	List() ([]*domain.Project, error)
}

type SessionRepository interface {
	Create(session *domain.Session) error
	Update(session *domain.Session) error
	GetBySessionID(sessionID string) (*domain.Session, error)
	List() ([]*domain.Session, error)
}

type ProxyRequestRepository interface {
	Create(req *domain.ProxyRequest) error
	Update(req *domain.ProxyRequest) error
	GetByID(id uint64) (*domain.ProxyRequest, error)
	List(limit, offset int) ([]*domain.ProxyRequest, error)
}

type ProxyUpstreamAttemptRepository interface {
	Create(attempt *domain.ProxyUpstreamAttempt) error
	Update(attempt *domain.ProxyUpstreamAttempt) error
	ListByProxyRequestID(proxyRequestID uint64) ([]*domain.ProxyUpstreamAttempt, error)
	GetProviderStats(clientType string) (map[uint64]*domain.ProviderStats, error)
}

type SystemSettingRepository interface {
	Get(key string) (string, error)
	Set(key, value string) error
	GetAll() ([]*domain.SystemSetting, error)
	Delete(key string) error
}
