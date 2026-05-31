package repository

import (
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

type TenantRepository interface {
	Create(tenant *domain.Tenant) error
	Update(tenant *domain.Tenant) error
	Delete(id uint64) error
	GetByID(id uint64) (*domain.Tenant, error)
	GetBySlug(slug string) (*domain.Tenant, error)
	GetDefault() (*domain.Tenant, error)
	List() ([]*domain.Tenant, error)
}

type UserRepository interface {
	Create(user *domain.User) error
	Update(user *domain.User) error
	Delete(tenantID uint64, id uint64) error
	GetByID(tenantID uint64, id uint64) (*domain.User, error)
	GetByUsername(username string) (*domain.User, error)
	GetDefault() (*domain.User, error)
	List() ([]*domain.User, error)
	ListByTenant(tenantID uint64) ([]*domain.User, error)
	ListByTenantAndStatus(tenantID uint64, status domain.UserStatus) ([]*domain.User, error)
	CountActive() (int64, error)
}

type InviteCodeRepository interface {
	Create(code *domain.InviteCode) error
	Update(tenantID uint64, code *domain.InviteCode) error
	Delete(tenantID uint64, id uint64) error
	GetByID(tenantID uint64, id uint64) (*domain.InviteCode, error)
	GetByCodeHash(tenantID uint64, codeHash string) (*domain.InviteCode, error)
	List(tenantID uint64) ([]*domain.InviteCode, error)
	Consume(tenantID uint64, codeHash string, now time.Time) (*domain.InviteCode, error)
	RollbackConsume(tenantID uint64, usageID uint64) error
	GetByCodeHashAny(codeHash string) (*domain.InviteCode, error)
}

type InviteCodeUsageRepository interface {
	Create(usage *domain.InviteCodeUsage) error
	ListByCodeID(tenantID uint64, codeID uint64) ([]*domain.InviteCodeUsage, error)
	ListByUserID(tenantID uint64, userID uint64) ([]*domain.InviteCodeUsage, error)
}

type ProviderRepository interface {
	Create(provider *domain.Provider) error
	Update(provider *domain.Provider) error
	Delete(tenantID uint64, id uint64) error
	GetByID(tenantID uint64, id uint64) (*domain.Provider, error)
	List(tenantID uint64) ([]*domain.Provider, error)
}

type RouteRepository interface {
	Create(route *domain.Route) error
	Update(route *domain.Route) error
	Delete(tenantID uint64, id uint64) error
	GetByID(tenantID uint64, id uint64) (*domain.Route, error)
	// FindByKey finds a route by the unique key (projectID, providerID, clientType)
	FindByKey(tenantID uint64, projectID, providerID uint64, clientType domain.ClientType) (*domain.Route, error)
	List(tenantID uint64) ([]*domain.Route, error)
	// BatchUpdatePositions updates positions for multiple routes in a transaction
	BatchUpdatePositions(tenantID uint64, updates []domain.RoutePositionUpdate) error
}

type RoutingStrategyRepository interface {
	Create(strategy *domain.RoutingStrategy) error
	Update(strategy *domain.RoutingStrategy) error
	Delete(tenantID uint64, id uint64) error
	GetByID(tenantID uint64, id uint64) (*domain.RoutingStrategy, error)
	GetByProjectID(tenantID uint64, projectID uint64) (*domain.RoutingStrategy, error)
	List(tenantID uint64) ([]*domain.RoutingStrategy, error)
}

type RetryConfigRepository interface {
	Create(config *domain.RetryConfig) error
	Update(config *domain.RetryConfig) error
	Delete(tenantID uint64, id uint64) error
	GetByID(tenantID uint64, id uint64) (*domain.RetryConfig, error)
	GetDefault(tenantID uint64) (*domain.RetryConfig, error)
	List(tenantID uint64) ([]*domain.RetryConfig, error)
}

type ProjectRepository interface {
	Create(project *domain.Project) error
	Update(project *domain.Project) error
	Delete(tenantID uint64, id uint64) error
	GetByID(tenantID uint64, id uint64) (*domain.Project, error)
	GetBySlug(tenantID uint64, slug string) (*domain.Project, error)
	List(tenantID uint64) ([]*domain.Project, error)
}

type SessionRepository interface {
	Create(session *domain.Session) error
	Update(session *domain.Session) error
	Touch(tenantID uint64, sessionID string, touchedAt time.Time) error
	GetBySessionID(tenantID uint64, sessionID string) (*domain.Session, error)
	List(tenantID uint64) ([]*domain.Session, error)
	// ListExpiredKeys 返回 updated_at 早于 before 的 session 标识。
	// 用于跨实例 KV 同步:DeleteOlderThan 之前先取要删的 keys,
	// DB 删除完成后用这些 keys 同步删除 coordinator KV 上的副本,
	// 避免其他实例仍从 KV 读到已被 hard-delete 的 session(stale read)。
	ListExpiredKeys(before time.Time) ([]SessionKey, error)
	DeleteOlderThan(before time.Time) (int64, error)
}

// SessionKey 是 session 的最小标识,用于 ListExpiredKeys 等只需 (tenant, session_id)
// 的批量操作,避免拉整行 domain.Session 浪费内存/带宽。
type SessionKey struct {
	TenantID  uint64
	SessionID string
}

// ProxyRequestFilter 请求列表过滤条件
type ProxyRequestFilter struct {
	TenantID   *uint64 // Tenant ID，nil 表示不过滤
	ProviderID *uint64 // Provider ID，nil 表示不过滤
	Status     *string // 状态，nil 表示不过滤
	APITokenID *uint64 // API Token ID，nil 表示不过滤
	ProjectID  *uint64 // Project ID，nil 表示不过滤
}

type ProxyRequestRepository interface {
	Create(req *domain.ProxyRequest) error
	Update(req *domain.ProxyRequest) error
	GetByID(tenantID uint64, id uint64) (*domain.ProxyRequest, error)
	List(tenantID uint64, limit, offset int) ([]*domain.ProxyRequest, error)
	// ListCursor 基于游标的分页查询
	// before: 获取 id < before 的记录 (向后翻页)
	// after: 获取 id > after 的记录 (向前翻页/获取新数据)
	// filter: 可选的过滤条件
	ListCursor(tenantID uint64, limit int, before, after uint64, filter *ProxyRequestFilter) ([]*domain.ProxyRequest, error)
	// ListActive 获取所有活跃请求 (PENDING 或 IN_PROGRESS 状态)
	ListActive(tenantID uint64) ([]*domain.ProxyRequest, error)
	Count(tenantID uint64) (int64, error)
	// CountWithFilter 带过滤条件的计数
	CountWithFilter(tenantID uint64, filter *ProxyRequestFilter) (int64, error)
	// UpdateProjectIDBySessionID 批量更新指定 sessionID 的所有请求的 projectID
	UpdateProjectIDBySessionID(tenantID uint64, sessionID string, projectID uint64) (int64, error)
	// MarkStaleAsFailed marks IN_PROGRESS/PENDING requests as FAILED when their
	// owning instance is no longer alive, or when start_time is older than 30 minutes.
	//
	// aliveInstanceIDs 必须由 coordinator.ListAliveInstances 提供。一个安全门:
	// 当 aliveInstanceIDs 为空(说明 coordinator 异常或刚启动)时,实现应跳过
	// 清理,绝不基于"没有活实例"的假设把所有 in-progress 请求都标记 FAILED。
	// 这样多实例环境下后启动的实例不会误杀先启动实例的在飞请求。
	MarkStaleAsFailed(aliveInstanceIDs []string) (int64, error)
	// FixFailedRequestsWithoutEndTime fixes FAILED requests that have no end_time set
	FixFailedRequestsWithoutEndTime() (int64, error)
	// DeleteOlderThan 删除指定时间之前的请求记录
	DeleteOlderThan(before time.Time) (int64, error)
	// HasRecentRequests 检查指定时间之后是否有请求记录
	HasRecentRequests(since time.Time) (bool, error)
	// UpdateCost updates only the cost field of a request
	UpdateCost(id uint64, cost uint64) error
	// UpdateCostAtomically updates the request cost AND a batch of attempt cost updates
	// in a single transaction. Used by RecalculateRequestCost to keep
	// proxy_requests.cost == SUM(proxy_upstream_attempts.cost) atomic.
	UpdateCostAtomically(requestID, requestCost uint64, attemptUpdates map[uint64]domain.AttemptCostUpdate) error
	// RecalculateCostsFromAttempts recalculates all request costs by summing their attempt costs
	RecalculateCostsFromAttempts() (int64, error)
	// RecalculateCostsFromAttemptsWithProgress recalculates all request costs with progress reporting via channel
	RecalculateCostsFromAttemptsWithProgress(progress chan<- domain.Progress) (int64, error)
	// ClearDetailOlderThan 清理指定时间之前请求的详情字段（request_info 和 response_info）
	// statuses 为空表示不按状态过滤（统一清理）；非空时仅清理 status IN (statuses) 的记录
	ClearDetailOlderThan(before time.Time, statuses []string) (int64, error)
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
	// BatchUpdateCosts 批量更新 attempt 的 cost 和 model_price_id。
	// model_price_id 跟随 cost 一起更新到当前匹配的价格记录,保证审计字段与金额一致。
	BatchUpdateCosts(updates map[uint64]domain.AttemptCostUpdate) error
	// MarkStaleAttemptsFailed marks stale attempts as failed with proper end_time and duration
	MarkStaleAttemptsFailed() (int64, error)
	// FixFailedAttemptsWithoutEndTime fixes FAILED attempts that have no end_time set
	FixFailedAttemptsWithoutEndTime() (int64, error)
	// ClearDetailOlderThan 清理指定时间之前 attempt 的详情字段（request_info 和 response_info）
	// statuses 为空表示不按状态过滤；非空时仅清理所属 ProxyRequest.status IN (statuses) 的 attempt
	ClearDetailOlderThan(before time.Time, statuses []string) (int64, error)
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
	GetByEmail(tenantID uint64, email string) (*domain.AntigravityQuota, error)
	// List 获取所有配额
	List(tenantID uint64) ([]*domain.AntigravityQuota, error)
	// Delete 删除配额
	Delete(tenantID uint64, email string) error
}

type CodexQuotaRepository interface {
	// Upsert 更新或插入配额（优先基于 identityKey，其次回退邮箱）
	Upsert(quota *domain.CodexQuota) error
	// GetByIdentityKey 根据身份键获取配额
	GetByIdentityKey(tenantID uint64, identityKey string) (*domain.CodexQuota, error)
	// GetByEmail 根据邮箱获取配额
	GetByEmail(tenantID uint64, email string) (*domain.CodexQuota, error)
	// List 获取所有配额
	List(tenantID uint64) ([]*domain.CodexQuota, error)
	// Delete 删除配额
	Delete(tenantID uint64, email string) error
}

type UsageStatsRepository interface {
	// Upsert 更新或插入统计记录
	Upsert(stats *domain.UsageStats) error
	// BatchUpsert 批量更新或插入统计记录
	BatchUpsert(stats []*domain.UsageStats) error
	// Query 查询统计数据（包含当前时间桶的实时数据补全）
	Query(tenantID uint64, filter UsageStatsFilter) ([]*domain.UsageStats, error)
	// QueryDashboardData 查询 Dashboard 所需的所有数据（单次请求，并发执行）
	QueryDashboardData(tenantID uint64) (*domain.DashboardData, error)
	// GetSummary 获取汇总统计数据（总计）
	GetSummary(tenantID uint64, filter UsageStatsFilter) (*domain.UsageStatsSummary, error)
	// DeleteOlderThan 删除指定粒度下指定时间之前的统计记录
	DeleteOlderThan(granularity domain.Granularity, before time.Time) (int64, error)
	// GetLatestTimeBucket 获取指定粒度的最新时间桶
	GetLatestTimeBucket(tenantID uint64, granularity domain.Granularity) (*time.Time, error)
	// GetProviderStats 获取 Provider 统计数据
	GetProviderStats(tenantID uint64, clientType string, projectID uint64) (map[uint64]*domain.ProviderStats, error)
	// AggregateAndRollUp 聚合原始数据到分钟级别，并自动 rollup 到各个粗粒度
	// 返回一个 channel，发送每个阶段的进度事件，channel 会在完成后关闭
	// 调用者可以 range 遍历 channel 获取进度，或直接忽略（异步执行）
	AggregateAndRollUp(tenantID uint64) <-chan domain.AggregateEvent
	// ClearAndRecalculateWithProgress 清空统计数据并重新计算，通过 channel 报告进度
	ClearAndRecalculateWithProgress(tenantID uint64, progress chan<- domain.Progress) error
}

// UsageStatsFilter 统计查询过滤条件
type UsageStatsFilter struct {
	TenantID    *uint64            // Tenant ID
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
	Delete(tenantID uint64, id uint64) error
	GetByID(tenantID uint64, id uint64) (*domain.APIToken, error)
	GetByToken(tenantID uint64, token string) (*domain.APIToken, error)
	List(tenantID uint64) ([]*domain.APIToken, error)
	UpdateLastSeen(tenantID uint64, id uint64, lastIP string, lastSeenAt time.Time) error
}

type ModelMappingRepository interface {
	Create(mapping *domain.ModelMapping) error
	Update(mapping *domain.ModelMapping) error
	Delete(tenantID uint64, id uint64) error
	GetByID(tenantID uint64, id uint64) (*domain.ModelMapping, error)
	List(tenantID uint64) ([]*domain.ModelMapping, error)
	ListEnabled(tenantID uint64) ([]*domain.ModelMapping, error)
	ListByClientType(tenantID uint64, clientType domain.ClientType) ([]*domain.ModelMapping, error)
	ListByQuery(tenantID uint64, query *domain.ModelMappingQuery) ([]*domain.ModelMapping, error)
	Count(tenantID uint64) (int, error)
	DeleteAll(tenantID uint64) error
	ClearAll(tenantID uint64) error     // Delete all mappings
	SeedDefaults(tenantID uint64) error // Re-seed default mappings
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
	// GetByID 获取指定ID的价格记录（仅未软删，用于 admin 读路径）
	GetByID(id uint64) (*domain.ModelPrice, error)
	// GetByIDIncludingDeleted 按 ID 取价格记录，包括已软删的历史版本（用于 attempt 历史快照反查）
	GetByIDIncludingDeleted(id uint64) (*domain.ModelPrice, error)
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

// CooldownRepository 接口
type CooldownRepository interface {
	// GetAll returns all active cooldowns
	GetAll() ([]*domain.Cooldown, error)

	// GetByProvider returns cooldowns for a specific provider
	GetByProvider(providerID uint64) ([]*domain.Cooldown, error)

	// Upsert creates or updates a cooldown
	Upsert(cooldown *domain.Cooldown) error

	// Delete removes a cooldown
	Delete(providerID uint64, clientType string, model string) error

	// DeleteAll removes all cooldowns for a provider
	DeleteAll(providerID uint64) error

	// DeleteExpired removes all expired cooldowns
	DeleteExpired() error

	// Get retrieves a specific cooldown
	Get(providerID uint64, clientType string, model string) (*domain.Cooldown, error)
}

// FailureCountRepository manages failure count persistence
type FailureCountRepository interface {
	// Get retrieves a failure count by tenant, provider, client type, model, and reason
	Get(tenantID uint64, providerID uint64, clientType string, model string, reason string) (*domain.FailureCount, error)

	// GetAll retrieves all failure counts for a tenant (use TenantIDAll for all)
	GetAll(tenantID uint64) ([]*domain.FailureCount, error)

	// Upsert inserts or updates a failure count
	Upsert(fc *domain.FailureCount) error

	// Delete deletes a failure count
	Delete(tenantID uint64, providerID uint64, clientType string, model string, reason string) error

	// DeleteAll deletes all failure counts for a provider+clientType+model
	// When model is empty, delete all for that provider+clientType (existing behavior)
	DeleteAll(tenantID uint64, providerID uint64, clientType string, model string) error

	// DeleteExpired deletes failure counts where last failure was too long ago
	DeleteExpired(olderThan int64) error
}

// BedrockDiscoveryRepository persists per-provider Bedrock discovery
// catalogs across process restarts so the profileDiscoverer can warm its
// in-memory cache at startup and skip the ~1-5s
// ListInferenceProfiles + ListFoundationModels round-trip on the first
// request. Scoped per provider: different Bedrock providers may hold
// different IAM permissions and therefore see different catalogs.
//
// Rows are fingerprinted with (region, accessKeyID) so a config edit
// that retargets the provider at a new region or IAM principal
// invalidates the cache — the adapter would otherwise happily serve
// profile IDs from the old region until the next TTL refresh.
type BedrockDiscoveryRepository interface {
	// Load returns cached entries for a provider that match the current
	// (region, accessKeyID) pair, plus the timestamp of the most recent
	// matching fetch. Rows that don't match are silently ignored (they
	// will be overwritten on the next Replace). Zero entries + zero time
	// means no usable cache; nil error.
	Load(providerID uint64, region, accessKeyID string) ([]*domain.BedrockDiscoveryEntry, time.Time, error)

	// Replace atomically swaps the stored catalog for a provider and
	// stamps every row with the supplied (region, accessKeyID).
	// Clears any pre-existing rows for this provider regardless of
	// their stored region/accessKeyID, so a config edit doesn't leave
	// orphan rows behind.
	Replace(providerID uint64, region, accessKeyID string, entries []*domain.BedrockDiscoveryEntry, fetchedAt time.Time) error
}
