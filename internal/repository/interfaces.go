package repository

import (
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

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
	// FindByKey finds a route by the unique key (projectID, providerID, clientType)
	FindByKey(projectID, providerID uint64, clientType domain.ClientType) (*domain.Route, error)
	List() ([]*domain.Route, error)
	// BatchUpdatePositions updates positions for multiple routes in a transaction
	BatchUpdatePositions(updates []domain.RoutePositionUpdate) error
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
	GetBySlug(slug string) (*domain.Project, error)
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
	// ListCursor 基于游标的分页查询
	// before: 获取 id < before 的记录 (向后翻页)
	// after: 获取 id > after 的记录 (向前翻页/获取新数据)
	ListCursor(limit int, before, after uint64) ([]*domain.ProxyRequest, error)
	Count() (int64, error)
	// UpdateProjectIDBySessionID 批量更新指定 sessionID 的所有请求的 projectID
	UpdateProjectIDBySessionID(sessionID string, projectID uint64) (int64, error)
	// MarkStaleAsFailed marks all IN_PROGRESS/PENDING requests from other instances as FAILED
	// Also marks requests that have been IN_PROGRESS for too long (> 30 minutes) as timed out
	MarkStaleAsFailed(currentInstanceID string) (int64, error)
	// DeleteOlderThan 删除指定时间之前的请求记录
	DeleteOlderThan(before time.Time) (int64, error)
}

type ProxyUpstreamAttemptRepository interface {
	Create(attempt *domain.ProxyUpstreamAttempt) error
	Update(attempt *domain.ProxyUpstreamAttempt) error
	ListByProxyRequestID(proxyRequestID uint64) ([]*domain.ProxyUpstreamAttempt, error)
}

type SystemSettingRepository interface {
	Get(key string) (string, error)
	Set(key, value string) error
	GetAll() ([]*domain.SystemSetting, error)
	Delete(key string) error
}

type AntigravityQuotaRepository interface {
	// Upsert 更新或插入配额（基于邮箱）
	Upsert(quota *domain.AntigravityQuota) error
	// GetByEmail 根据邮箱获取配额
	GetByEmail(email string) (*domain.AntigravityQuota, error)
	// List 获取所有配额
	List() ([]*domain.AntigravityQuota, error)
	// Delete 删除配额
	Delete(email string) error
}

type UsageStatsRepository interface {
	// Upsert 更新或插入统计记录（基于 hour + route_id + provider_id + project_id + client_type）
	Upsert(stats *domain.UsageStats) error
	// Query 查询统计数据，支持按时间范围、路由、Provider、项目过滤
	Query(filter UsageStatsFilter) ([]*domain.UsageStats, error)
	// DeleteOlderThan 删除指定时间之前的统计记录
	DeleteOlderThan(before time.Time) (int64, error)
	// GetLatestHour 获取最新的聚合小时，如果没有记录返回 nil
	GetLatestHour() (*time.Time, error)
	// GetProviderStats 获取 Provider 统计数据（合并历史聚合数据和当前小时实时数据）
	GetProviderStats(clientType string, projectID uint64) (map[uint64]*domain.ProviderStats, error)
	// Aggregate 聚合统计数据（从 proxy_upstream_attempts 聚合到 usage_stats）
	Aggregate()
}

// UsageStatsFilter 统计查询过滤条件
type UsageStatsFilter struct {
	StartTime  *time.Time // 开始时间
	EndTime    *time.Time // 结束时间
	RouteID    *uint64    // 路由 ID
	ProviderID *uint64    // Provider ID
	ProjectID  *uint64    // 项目 ID
	APITokenID *uint64    // API Token ID
	ClientType *string    // 客户端类型
}

type APITokenRepository interface {
	Create(token *domain.APIToken) error
	Update(token *domain.APIToken) error
	Delete(id uint64) error
	GetByID(id uint64) (*domain.APIToken, error)
	GetByToken(token string) (*domain.APIToken, error)
	List() ([]*domain.APIToken, error)
	IncrementUseCount(id uint64) error
}

type ModelMappingRepository interface {
	Create(mapping *domain.ModelMapping) error
	Update(mapping *domain.ModelMapping) error
	Delete(id uint64) error
	GetByID(id uint64) (*domain.ModelMapping, error)
	List() ([]*domain.ModelMapping, error)
	ListEnabled() ([]*domain.ModelMapping, error)
	ListByClientType(clientType domain.ClientType) ([]*domain.ModelMapping, error)
	ListByQuery(query *domain.ModelMappingQuery) ([]*domain.ModelMapping, error)
	Count() (int, error)
	DeleteAll() error
	DeleteBuiltin() error
}
