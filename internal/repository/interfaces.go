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

// ProxyRequestFilter 请求列表过滤条件
type ProxyRequestFilter struct {
	ProviderID *uint64 // Provider ID，nil 表示不过滤
	Status     *string // 状态，nil 表示不过滤
}

type ProxyRequestRepository interface {
	Create(req *domain.ProxyRequest) error
	Update(req *domain.ProxyRequest) error
	GetByID(id uint64) (*domain.ProxyRequest, error)
	List(limit, offset int) ([]*domain.ProxyRequest, error)
	// ListCursor 基于游标的分页查询
	// before: 获取 id < before 的记录 (向后翻页)
	// after: 获取 id > after 的记录 (向前翻页/获取新数据)
	// filter: 可选的过滤条件
	ListCursor(limit int, before, after uint64, filter *ProxyRequestFilter) ([]*domain.ProxyRequest, error)
	// ListActive 获取所有活跃请求 (PENDING 或 IN_PROGRESS 状态)
	ListActive() ([]*domain.ProxyRequest, error)
	Count() (int64, error)
	// CountWithFilter 带过滤条件的计数
	CountWithFilter(filter *ProxyRequestFilter) (int64, error)
	// UpdateProjectIDBySessionID 批量更新指定 sessionID 的所有请求的 projectID
	UpdateProjectIDBySessionID(sessionID string, projectID uint64) (int64, error)
	// MarkStaleAsFailed marks all IN_PROGRESS/PENDING requests from other instances as FAILED
	// Also marks requests that have been IN_PROGRESS for too long (> 30 minutes) as timed out
	MarkStaleAsFailed(currentInstanceID string) (int64, error)
	// FixFailedRequestsWithoutEndTime fixes FAILED requests that have no end_time set
	FixFailedRequestsWithoutEndTime() (int64, error)
	// DeleteOlderThan 删除指定时间之前的请求记录
	DeleteOlderThan(before time.Time) (int64, error)
	// HasRecentRequests 检查指定时间之后是否有请求记录
	HasRecentRequests(since time.Time) (bool, error)
	// UpdateCost updates only the cost field of a request
	UpdateCost(id uint64, cost uint64) error
	// AddCost adds a delta to the cost field of a request (can be negative)
	AddCost(id uint64, delta int64) error
	// BatchUpdateCosts updates costs for multiple requests in a single transaction
	BatchUpdateCosts(updates map[uint64]uint64) error
	// RecalculateCostsFromAttempts recalculates all request costs by summing their attempt costs
	RecalculateCostsFromAttempts() (int64, error)
	// RecalculateCostsFromAttemptsWithProgress recalculates all request costs with progress reporting via channel
	RecalculateCostsFromAttemptsWithProgress(progress chan<- domain.Progress) (int64, error)
	// ClearDetailOlderThan 清理指定时间之前请求的详情字段（request_info 和 response_info）
	ClearDetailOlderThan(before time.Time) (int64, error)
}

type ProxyUpstreamAttemptRepository interface {
	Create(attempt *domain.ProxyUpstreamAttempt) error
	Update(attempt *domain.ProxyUpstreamAttempt) error
	ListByProxyRequestID(proxyRequestID uint64) ([]*domain.ProxyUpstreamAttempt, error)
	// ListAll returns all attempts (for cost recalculation)
	ListAll() ([]*domain.ProxyUpstreamAttempt, error)
	// CountAll returns total count of attempts
	CountAll() (int64, error)
	// StreamForCostCalc iterates through all attempts for cost calculation
	// Calls the callback with batches of minimal data, returns early if callback returns error
	StreamForCostCalc(batchSize int, callback func(batch []*domain.AttemptCostData) error) error
	// UpdateCost updates only the cost field of an attempt
	UpdateCost(id uint64, cost uint64) error
	// BatchUpdateCosts updates costs for multiple attempts in a single transaction
	BatchUpdateCosts(updates map[uint64]uint64) error
	// MarkStaleAttemptsFailed marks stale attempts as failed with proper end_time and duration
	MarkStaleAttemptsFailed() (int64, error)
	// FixFailedAttemptsWithoutEndTime fixes FAILED attempts that have no end_time set
	FixFailedAttemptsWithoutEndTime() (int64, error)
	// ClearDetailOlderThan 清理指定时间之前 attempt 的详情字段（request_info 和 response_info）
	ClearDetailOlderThan(before time.Time) (int64, error)
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

type CodexQuotaRepository interface {
	// Upsert 更新或插入配额（基于邮箱）
	Upsert(quota *domain.CodexQuota) error
	// GetByEmail 根据邮箱获取配额
	GetByEmail(email string) (*domain.CodexQuota, error)
	// List 获取所有配额
	List() ([]*domain.CodexQuota, error)
	// Delete 删除配额
	Delete(email string) error
}

type UsageStatsRepository interface {
	// Upsert 更新或插入统计记录
	Upsert(stats *domain.UsageStats) error
	// BatchUpsert 批量更新或插入统计记录
	BatchUpsert(stats []*domain.UsageStats) error
	// Query 查询统计数据（包含当前时间桶的实时数据补全）
	Query(filter UsageStatsFilter) ([]*domain.UsageStats, error)
	// QueryDashboardData 查询 Dashboard 所需的所有数据（单次请求，并发执行）
	QueryDashboardData() (*domain.DashboardData, error)
	// GetSummary 获取汇总统计数据（总计）
	GetSummary(filter UsageStatsFilter) (*domain.UsageStatsSummary, error)
	// GetSummaryByProvider 按 Provider 维度获取汇总统计
	GetSummaryByProvider(filter UsageStatsFilter) (map[uint64]*domain.UsageStatsSummary, error)
	// GetSummaryByRoute 按 Route 维度获取汇总统计
	GetSummaryByRoute(filter UsageStatsFilter) (map[uint64]*domain.UsageStatsSummary, error)
	// GetSummaryByProject 按 Project 维度获取汇总统计
	GetSummaryByProject(filter UsageStatsFilter) (map[uint64]*domain.UsageStatsSummary, error)
	// GetSummaryByAPIToken 按 APIToken 维度获取汇总统计
	GetSummaryByAPIToken(filter UsageStatsFilter) (map[uint64]*domain.UsageStatsSummary, error)
	// GetSummaryByClientType 按 ClientType 维度获取汇总统计
	GetSummaryByClientType(filter UsageStatsFilter) (map[string]*domain.UsageStatsSummary, error)
	// DeleteOlderThan 删除指定粒度下指定时间之前的统计记录
	DeleteOlderThan(granularity domain.Granularity, before time.Time) (int64, error)
	// GetLatestTimeBucket 获取指定粒度的最新时间桶
	GetLatestTimeBucket(granularity domain.Granularity) (*time.Time, error)
	// GetProviderStats 获取 Provider 统计数据
	GetProviderStats(clientType string, projectID uint64) (map[uint64]*domain.ProviderStats, error)
	// AggregateAndRollUp 聚合原始数据到分钟级别，并自动 rollup 到各个粗粒度
	// 返回一个 channel，发送每个阶段的进度事件，channel 会在完成后关闭
	// 调用者可以 range 遍历 channel 获取进度，或直接忽略（异步执行）
	AggregateAndRollUp() <-chan domain.AggregateEvent
	// ClearAndRecalculate 清空统计数据并重新从原始数据计算
	ClearAndRecalculate() error
	// ClearAndRecalculateWithProgress 清空统计数据并重新计算，通过 channel 报告进度
	ClearAndRecalculateWithProgress(progress chan<- domain.Progress) error
}

// UsageStatsFilter 统计查询过滤条件
type UsageStatsFilter struct {
	Granularity domain.Granularity // 时间粒度（必填）
	StartTime   *time.Time         // 开始时间
	EndTime     *time.Time         // 结束时间
	RouteID     *uint64            // 路由 ID
	ProviderID  *uint64            // Provider ID
	ProjectID   *uint64            // 项目 ID
	APITokenID  *uint64            // API Token ID
	ClientType  *string            // 客户端类型
	Model       *string            // 模型名称
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
	ClearAll() error     // Delete all mappings
	SeedDefaults() error // Re-seed default mappings
}

type ResponseModelRepository interface {
	// Upsert 更新或插入 response model（基于 name）
	Upsert(name string) error
	// BatchUpsert 批量更新或插入 response models
	BatchUpsert(names []string) error
	// List 获取所有 response models
	List() ([]*domain.ResponseModel, error)
	// ListNames 获取所有 response model 名称
	ListNames() ([]string, error)
}

type ModelPriceRepository interface {
	// Create 创建新的价格记录（用于价格变更）
	Create(price *domain.ModelPrice) error
	// BatchCreate 批量创建价格记录
	BatchCreate(prices []*domain.ModelPrice) error
	// GetByID 获取指定ID的价格记录
	GetByID(id uint64) (*domain.ModelPrice, error)
	// GetCurrentByModelID 获取模型的当前价格（最新记录），支持前缀匹配
	GetCurrentByModelID(modelID string) (*domain.ModelPrice, error)
	// ListCurrentPrices 获取所有模型的当前价格（用于初始化 Calculator）
	ListCurrentPrices() ([]*domain.ModelPrice, error)
	// ListByModelID 获取模型的价格历史
	ListByModelID(modelID string) ([]*domain.ModelPrice, error)
	// Count 获取价格记录总数
	Count() (int64, error)
	// Delete 删除价格记录（软删除）
	Delete(id uint64) error
	// Update 更新价格记录
	Update(price *domain.ModelPrice) error
	// SoftDeleteAll 软删除所有价格记录
	SoftDeleteAll() error
	// ResetToDefaults 重置为默认价格（软删除现有记录，插入默认价格）
	ResetToDefaults() ([]*domain.ModelPrice, error)
}
