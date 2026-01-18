package domain

import "time"

// 各种请求的客户端
type ClientType string

var (
	ClientTypeClaude ClientType = "claude"
	ClientTypeCodex  ClientType = "codex"
	ClientTypeGemini ClientType = "gemini"
	ClientTypeOpenAI ClientType = "openai"
)

type ProviderConfigCustom struct {
	// 中转站的 URL
	BaseURL string `json:"baseURL"`

	// API Key
	APIKey string `json:"apiKey"`

	// 某个 Client 有特殊的 BaseURL
	ClientBaseURL map[ClientType]string `json:"clientBaseURL,omitempty"`

	// Model 映射: RequestModel → MappedModel
	ModelMapping map[string]string `json:"modelMapping,omitempty"`
}

type ProviderConfigAntigravity struct {
	// 邮箱（用于标识帐号）
	Email string `json:"email"`

	// Google OAuth refresh_token
	RefreshToken string `json:"refreshToken"`

	// Google Cloud Project ID
	ProjectID string `json:"projectID"`

	// v1internal 端点
	Endpoint string `json:"endpoint"`

	// Model 映射: RequestModel → MappedModel
	ModelMapping map[string]string `json:"modelMapping,omitempty"`

	// Haiku 模型映射目标 (默认 "gemini-2.5-flash-lite" 省钱，可选 "claude-sonnet-4-5" 更强)
	// 空值使用默认 gemini-2.5-flash-lite
	HaikuTarget string `json:"haikuTarget,omitempty"`
}

type ProviderConfigKiro struct {
	// 认证方式: "social" 或 "idc"
	AuthMethod string `json:"authMethod"`

	// 通用字段
	RefreshToken string `json:"refreshToken"`
	Region       string `json:"region,omitempty"` // 默认 us-east-1

	// IdC 认证特有字段
	ClientID     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`

	// 可选: 用于标识账号
	Email string `json:"email,omitempty"`

	// Model 映射: RequestModel → MappedModel
	ModelMapping map[string]string `json:"modelMapping,omitempty"`
}

type ProviderConfig struct {
	Custom      *ProviderConfigCustom      `json:"custom,omitempty"`
	Antigravity *ProviderConfigAntigravity `json:"antigravity,omitempty"`
	Kiro        *ProviderConfigKiro        `json:"kiro,omitempty"`
}

// Provider 供应商
type Provider struct {
	ID        uint64    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// 软删除时间，nil 表示未删除
	DeletedAt *time.Time `json:"deletedAt,omitempty"`

	// 1. Custom ，主要用来各种中转站
	// 2. Antigravity
	Type string `json:"type"`

	// 展示的名称
	Name string `json:"name"`

	// Logo URL 或 data URI
	Logo string `json:"logo,omitempty"`

	// 配置
	Config *ProviderConfig `json:"config"`

	// 支持的 Client
	SupportedClientTypes []ClientType `json:"supportedClientTypes"`

	// 支持的模型列表（通配符模式）
	// 如果配置了，在 Route 匹配时会检查前置映射后的模型是否在支持列表中
	// 空数组表示支持所有模型
	SupportModels []string `json:"supportModels,omitempty"`
}

type Project struct {
	ID        uint64    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// 软删除时间
	DeletedAt *time.Time `json:"deletedAt,omitempty"`

	Name string `json:"name"`
	Slug string `json:"slug"`

	// 启用自定义路由的 ClientType 列表，空数组表示所有 ClientType 都使用全局路由
	EnabledCustomRoutes []ClientType `json:"enabledCustomRoutes"`
}

type Session struct {
	ID        uint64    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// 软删除时间
	DeletedAt *time.Time `json:"deletedAt,omitempty"`

	SessionID  string     `json:"sessionID"`
	ClientType ClientType `json:"clientType"`

	// 0 表示没有项目
	ProjectID uint64 `json:"projectID"`

	// RejectedAt 记录会话被拒绝的时间，nil 表示未被拒绝
	RejectedAt *time.Time `json:"rejectedAt,omitempty"`
}

// 路由
type Route struct {
	ID        uint64    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// 软删除时间
	DeletedAt *time.Time `json:"deletedAt,omitempty"`

	IsEnabled bool `json:"isEnabled"`

	// 是否为原生支持的路由（自动创建，跟随 Provider 设置）
	// false 表示通过 API 转换支持（手动创建，独立管理）
	IsNative bool `json:"isNative"`

	// 0 表示没有项目即全局
	ProjectID  uint64     `json:"projectID"`
	ClientType ClientType `json:"clientType"`
	ProviderID uint64     `json:"providerID"`

	// 位置，数字越小越优先
	Position int `json:"position"`

	// 重试配置，0 表示使用系统默认
	RetryConfigID uint64 `json:"retryConfigID"`
}

// RoutePositionUpdate represents a route position update
type RoutePositionUpdate struct {
	ID       uint64 `json:"id"`
	Position int    `json:"position"`
}

type RequestInfo struct {
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	URL     string            `json:"url"`
	Body    string            `json:"body"`
}
type ResponseInfo struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

// 追踪
type ProxyRequest struct {
	ID        uint64    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// 服务实例 ID，用于识别请求属于哪个实例
	InstanceID string `json:"instanceID"`

	RequestID  string     `json:"requestID"`
	SessionID  string     `json:"sessionID"`
	ClientType ClientType `json:"clientType"`

	RequestModel  string `json:"requestModel"`
	ResponseModel string `json:"responseModel"`

	StartTime time.Time     `json:"startTime"`
	EndTime   time.Time     `json:"endTime"`
	Duration  time.Duration `json:"duration"`

	// 是否为 SSE 流式请求
	IsStream bool `json:"isStream"`

	// PENDING, IN_PROGRESS, COMPLETED, FAILED, REJECTED
	// REJECTED: 请求被拒绝（如：强制项目绑定超时）
	Status string `json:"status"`

	// HTTP 状态码（冗余存储，用于列表查询性能优化）
	StatusCode int `json:"statusCode"`

	// 原始请求的信息
	RequestInfo  *RequestInfo  `json:"requestInfo"`
	ResponseInfo *ResponseInfo `json:"responseInfo"`

	// 错误信息
	Error                       string `json:"error"`
	ProxyUpstreamAttemptCount   uint64 `json:"proxyUpstreamAttemptCount"`
	FinalProxyUpstreamAttemptID uint64 `json:"finalProxyUpstreamAttemptID"`

	// 当前使用的 Route 和 Provider (用于实时追踪)
	RouteID    uint64 `json:"routeID"`
	ProviderID uint64 `json:"providerID"`
	ProjectID  uint64 `json:"projectID"`

	// Token 使用情况
	InputTokenCount  uint64 `json:"inputTokenCount"`
	OutputTokenCount uint64 `json:"outputTokenCount"`

	// 缓存使用情况
	// - CacheReadCount: 缓存命中读取的 tokens (价格: input × 0.1)
	// - CacheWriteCount: 缓存创建的总 tokens (兼容字段，= Cache5mWriteCount + Cache1hWriteCount)
	// - Cache5mWriteCount: 5分钟 TTL 缓存创建 tokens (价格: input × 1.25)
	// - Cache1hWriteCount: 1小时 TTL 缓存创建 tokens (价格: input × 2.0)
	CacheReadCount    uint64 `json:"cacheReadCount"`
	CacheWriteCount   uint64 `json:"cacheWriteCount"`
	Cache5mWriteCount uint64 `json:"cache5mWriteCount"`
	Cache1hWriteCount uint64 `json:"cache1hWriteCount"`

	// 成本 (微美元，1 USD = 1,000,000)
	Cost uint64 `json:"cost"`

	// 使用的 API Token ID，0 表示未使用 Token
	APITokenID uint64 `json:"apiTokenID"`
}

type ProxyUpstreamAttempt struct {
	ID        uint64    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// 实际开始和结束时间
	StartTime time.Time     `json:"startTime"`
	EndTime   time.Time     `json:"endTime"`
	Duration  time.Duration `json:"duration"`

	// PENDING, IN_PROGRESS, COMPLETED, FAILED
	Status string `json:"status"`

	ProxyRequestID uint64 `json:"proxyRequestID"`

	// 是否为 SSE 流式请求
	IsStream bool `json:"isStream"`

	// 模型信息
	// RequestModel: 客户端请求的原始模型
	// MappedModel: 经过映射后实际发送给上游的模型
	// ResponseModel: 上游响应中返回的模型名称
	RequestModel  string `json:"requestModel"`
	MappedModel   string `json:"mappedModel"`
	ResponseModel string `json:"responseModel"`

	RequestInfo  *RequestInfo  `json:"requestInfo"`
	ResponseInfo *ResponseInfo `json:"responseInfo"`

	RouteID    uint64 `json:"routeID"`
	ProviderID uint64 `json:"providerID"`

	// Token 使用情况
	InputTokenCount  uint64 `json:"inputTokenCount"`
	OutputTokenCount uint64 `json:"outputTokenCount"`

	// 缓存使用情况
	// - CacheReadCount: 缓存命中读取的 tokens
	// - CacheWriteCount: 缓存创建的总 tokens (兼容字段，= Cache5mWriteCount + Cache1hWriteCount)
	// - Cache5mWriteCount: 5分钟 TTL 缓存创建 tokens
	// - Cache1hWriteCount: 1小时 TTL 缓存创建 tokens
	CacheReadCount    uint64 `json:"cacheReadCount"`
	CacheWriteCount   uint64 `json:"cacheWriteCount"`
	Cache5mWriteCount uint64 `json:"cache5mWriteCount"`
	Cache1hWriteCount uint64 `json:"cache1hWriteCount"`

	Cost uint64 `json:"cost"`
}

// 重试配置
type RetryConfig struct {
	ID        uint64    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// 软删除时间
	DeletedAt *time.Time `json:"deletedAt,omitempty"`

	// 配置名称，便于复用
	Name string `json:"name"`

	// 是否为系统默认配置
	IsDefault bool `json:"isDefault"`

	// 最大重试次数
	MaxRetries int `json:"maxRetries"`

	// 初始重试间隔
	InitialInterval time.Duration `json:"initialInterval"`

	// 退避倍率，1.0 表示固定间隔
	BackoffRate float64 `json:"backoffRate"`

	// 最大间隔上限
	MaxInterval time.Duration `json:"maxInterval"`
}

// 路由策略类型
type RoutingStrategyType string

var (
	// 按 Position 优先级排序
	RoutingStrategyPriority RoutingStrategyType = "priority"
	// 加权随机
	RoutingStrategyWeightedRandom RoutingStrategyType = "weighted_random"
)

// 路由策略配置（策略特定参数）
type RoutingStrategyConfig struct {
	// 加权随机策略的权重配置等
	// 根据具体策略扩展
}

// 路由策略
type RoutingStrategy struct {
	ID        uint64    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// 软删除时间
	DeletedAt *time.Time `json:"deletedAt,omitempty"`

	// 0 表示全局策略
	ProjectID uint64 `json:"projectID"`

	// 策略类型
	Type RoutingStrategyType `json:"type"`

	// 策略特定配置
	Config *RoutingStrategyConfig `json:"config"`
}

// 系统设置（键值对字典表）
type SystemSetting struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// 系统设置 Key 常量
const (
	SettingKeyProxyPort             = "proxy_port"              // 代理服务器端口，默认 9880
	SettingKeyRequestRetentionHours = "request_retention_hours" // 请求记录保留小时数，默认 168 小时（7天），0 表示不清理
	SettingKeyTimezone              = "timezone"                // 时区设置，默认 Asia/Shanghai
)

// Antigravity 模型配额
type AntigravityModelQuota struct {
	Name       string `json:"name"`       // 模型名称
	Percentage int    `json:"percentage"` // 剩余配额百分比 0-100
	ResetTime  string `json:"resetTime"`  // 重置时间 ISO8601
}

// Antigravity 账户配额（基于邮箱存储）
type AntigravityQuota struct {
	ID        uint64    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// 软删除时间
	DeletedAt *time.Time `json:"deletedAt,omitempty"`

	// 邮箱作为唯一标识
	Email string `json:"email"`

	// 用户名
	Name string `json:"name"`

	// 头像 URL
	Picture string `json:"picture"`

	// Google Cloud Project ID
	GCPProjectID string `json:"gcpProjectID"`

	// 订阅等级：FREE, PRO, ULTRA
	SubscriptionTier string `json:"subscriptionTier"`

	// 是否被禁止访问 (403)
	IsForbidden bool `json:"isForbidden"`

	// 各模型配额
	Models []AntigravityModelQuota `json:"models"`
}

// Provider 统计信息
type ProviderStats struct {
	ProviderID uint64 `json:"providerID"`

	// 请求统计
	TotalRequests     uint64  `json:"totalRequests"`
	SuccessfulRequests uint64  `json:"successfulRequests"`
	FailedRequests    uint64  `json:"failedRequests"`
	SuccessRate       float64 `json:"successRate"` // 0-100

	// 活动请求（正在处理中）
	ActiveRequests uint64 `json:"activeRequests"`

	// Token 统计
	TotalInputTokens  uint64 `json:"totalInputTokens"`
	TotalOutputTokens uint64 `json:"totalOutputTokens"`
	TotalCacheRead    uint64 `json:"totalCacheRead"`
	TotalCacheWrite   uint64 `json:"totalCacheWrite"`

	// 成本 (微美元)
	TotalCost uint64 `json:"totalCost"`
}

// Granularity 统计数据的时间粒度
type Granularity string

const (
	GranularityMinute Granularity = "minute"
	GranularityHour   Granularity = "hour"
	GranularityDay    Granularity = "day"
	GranularityWeek   Granularity = "week"
	GranularityMonth  Granularity = "month"
)

// UsageStats 使用统计汇总（多层级时间聚合）
type UsageStats struct {
	ID        uint64    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`

	// 时间维度
	TimeBucket  time.Time   `json:"timeBucket"`  // 时间桶（根据粒度截断）
	Granularity Granularity `json:"granularity"` // 时间粒度

	// 聚合维度
	RouteID    uint64 `json:"routeId"`    // 路由 ID，0 表示未知
	ProviderID uint64 `json:"providerId"` // Provider ID
	ProjectID  uint64 `json:"projectId"`  // 项目 ID，0 表示未知
	APITokenID uint64 `json:"apiTokenId"` // API Token ID，0 表示未知
	ClientType string `json:"clientType"` // 客户端类型
	Model      string `json:"model"`      // 请求的模型名称

	// 请求统计
	TotalRequests      uint64 `json:"totalRequests"`
	SuccessfulRequests uint64 `json:"successfulRequests"`
	FailedRequests     uint64 `json:"failedRequests"`
	TotalDurationMs    uint64 `json:"totalDurationMs"` // 累计请求耗时（毫秒）

	// Token 统计
	InputTokens  uint64 `json:"inputTokens"`
	OutputTokens uint64 `json:"outputTokens"`
	CacheRead    uint64 `json:"cacheRead"`
	CacheWrite   uint64 `json:"cacheWrite"`

	// 成本 (微美元)
	Cost uint64 `json:"cost"`
}

// UsageStatsSummary 统计数据汇总（用于仪表盘）
type UsageStatsSummary struct {
	TotalRequests      uint64  `json:"totalRequests"`
	SuccessfulRequests uint64  `json:"successfulRequests"`
	FailedRequests     uint64  `json:"failedRequests"`
	SuccessRate        float64 `json:"successRate"`
	TotalInputTokens   uint64  `json:"totalInputTokens"`
	TotalOutputTokens  uint64  `json:"totalOutputTokens"`
	TotalCacheRead     uint64  `json:"totalCacheRead"`
	TotalCacheWrite    uint64  `json:"totalCacheWrite"`
	TotalCost          uint64  `json:"totalCost"`
}

// APIToken API 访问令牌
type APIToken struct {
	ID        uint64    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// Token 明文（直接存储）
	Token string `json:"token"`

	// Token 前缀（用于显示，如 "maxx_abc1..."）
	TokenPrefix string `json:"tokenPrefix"`

	// 名称和描述
	Name        string `json:"name"`
	Description string `json:"description"`

	// 关联的项目 ID，0 表示使用全局路由
	ProjectID uint64 `json:"projectID"`

	// 是否启用
	IsEnabled bool `json:"isEnabled"`

	// 过期时间，nil 表示永不过期
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`

	// 最后使用时间
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`

	// 使用次数
	UseCount uint64 `json:"useCount"`

	// 软删除时间
	DeletedAt *time.Time `json:"deletedAt,omitempty"`
}

// APITokenCreateResult 创建 Token 的返回结果（包含明文 Token，仅返回一次）
type APITokenCreateResult struct {
	Token    string    `json:"token"`    // 明文 Token（仅创建时返回）
	APIToken *APIToken `json:"apiToken"` // Token 元数据
}

// ModelMappingScope 模型映射作用域
type ModelMappingScope string

const (
	// ModelMappingScopeGlobal 全局作用域，优先级最低
	ModelMappingScopeGlobal ModelMappingScope = "global"
	// ModelMappingScopeProvider 供应商作用域
	ModelMappingScopeProvider ModelMappingScope = "provider"
	// ModelMappingScopeRoute 路由作用域，优先级最高
	ModelMappingScopeRoute ModelMappingScope = "route"
)

// ModelMapping 模型映射规则
// 支持多种条件筛选，类似 Route 的配置方式
type ModelMapping struct {
	ID        uint64    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// 软删除时间
	DeletedAt *time.Time `json:"deletedAt,omitempty"`

	// 作用域类型
	Scope ModelMappingScope `json:"scope"` // global, provider, route

	// 作用域条件（全部为空表示全局规则）
	ClientType   ClientType `json:"clientType,omitempty"`   // 客户端类型，空表示所有
	ProviderType string     `json:"providerType,omitempty"` // 供应商类型（如 antigravity, kiro, custom），空表示所有
	ProviderID   uint64     `json:"providerID,omitempty"`   // 供应商 ID，0 表示所有
	ProjectID    uint64     `json:"projectID,omitempty"`    // 项目 ID，0 表示所有
	RouteID      uint64     `json:"routeID,omitempty"`      // 路由 ID，0 表示所有
	APITokenID   uint64     `json:"apiTokenID,omitempty"`   // Token ID，0 表示所有

	// 映射规则
	Pattern string `json:"pattern"` // 源模式，支持通配符 *
	Target  string `json:"target"`  // 目标模型

	// 优先级，数字越小优先级越高
	Priority int `json:"priority"`
}

// ModelMappingRule 简化的映射规则（用于 API 和内部逻辑）
type ModelMappingRule struct {
	Pattern string `json:"pattern"` // 源模式，支持通配符 *
	Target  string `json:"target"`  // 目标模型
}

// ModelMappingQuery 查询条件
type ModelMappingQuery struct {
	ClientType   ClientType
	ProviderType string // 供应商类型（如 antigravity, kiro, custom）
	ProviderID   uint64
	ProjectID    uint64
	RouteID      uint64
	APITokenID   uint64
}

// ResponseModel 记录所有出现过的 response model
// 用于快速查询可选的模型列表，避免每次 DISTINCT 查询
type ResponseModel struct {
	ID        uint64    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`

	// 模型名称
	Name string `json:"name"`

	// 最后一次使用时间
	LastSeenAt time.Time `json:"lastSeenAt"`

	// 使用次数
	UseCount uint64 `json:"useCount"`
}

// MatchWildcard 检查输入是否匹配通配符模式
func MatchWildcard(pattern, input string) bool {
	// 简单情况
	if pattern == "*" {
		return true
	}
	if !containsWildcard(pattern) {
		return pattern == input
	}

	parts := splitByWildcard(pattern)

	// 处理 prefix* 模式
	if len(parts) == 2 && parts[1] == "" {
		return hasPrefix(input, parts[0])
	}

	// 处理 *suffix 模式
	if len(parts) == 2 && parts[0] == "" {
		return hasSuffix(input, parts[1])
	}

	// 处理多通配符模式
	pos := 0
	for i, part := range parts {
		if part == "" {
			continue
		}

		idx := indexOf(input[pos:], part)
		if idx < 0 {
			return false
		}

		// 第一部分必须在开头（如果模式不以 * 开头）
		if i == 0 && idx != 0 {
			return false
		}

		pos += idx + len(part)
	}

	// 最后一部分必须在结尾（如果模式不以 * 结尾）
	if parts[len(parts)-1] != "" && !hasSuffix(input, parts[len(parts)-1]) {
		return false
	}

	return true
}

// 辅助函数
func containsWildcard(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '*' {
			return true
		}
	}
	return false
}

func splitByWildcard(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '*' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// ===== Dashboard API Types =====

// DashboardDaySummary 日统计摘要
type DashboardDaySummary struct {
	Requests    uint64  `json:"requests"`
	Tokens      uint64  `json:"tokens"`
	Cost        uint64  `json:"cost"`
	SuccessRate float64 `json:"successRate,omitempty"`
	RPM         float64 `json:"rpm,omitempty"` // Requests Per Minute (今日平均)
	TPM         float64 `json:"tpm,omitempty"` // Tokens Per Minute (今日平均)
}

// DashboardAllTimeSummary 全量统计摘要
type DashboardAllTimeSummary struct {
	Requests          uint64     `json:"requests"`
	Tokens            uint64     `json:"tokens"`
	Cost              uint64     `json:"cost"`
	FirstUseDate      *time.Time `json:"firstUseDate,omitempty"`
	DaysSinceFirstUse int        `json:"daysSinceFirstUse"`
}

// DashboardHeatmapPoint 热力图数据点
type DashboardHeatmapPoint struct {
	Date  string `json:"date"`
	Count uint64 `json:"count"`
}

// DashboardModelStats 模型统计
type DashboardModelStats struct {
	Model    string `json:"model"`
	Requests uint64 `json:"requests"`
	Tokens   uint64 `json:"tokens"`
}

// DashboardTrendPoint 趋势数据点
type DashboardTrendPoint struct {
	Hour     string `json:"hour"`
	Requests uint64 `json:"requests"`
}

// DashboardProviderStats Provider 统计
type DashboardProviderStats struct {
	Requests    uint64  `json:"requests"`
	SuccessRate float64 `json:"successRate"`
	RPM         float64 `json:"rpm,omitempty"` // Requests Per Minute (今日平均)
	TPM         float64 `json:"tpm,omitempty"` // Tokens Per Minute (今日平均)
}

// DashboardData Dashboard 聚合数据
type DashboardData struct {
	Today         DashboardDaySummary               `json:"today"`
	Yesterday     DashboardDaySummary               `json:"yesterday"`
	AllTime       DashboardAllTimeSummary           `json:"allTime"`
	Heatmap       []DashboardHeatmapPoint           `json:"heatmap"`
	TopModels     []DashboardModelStats             `json:"topModels"`
	Trend24h      []DashboardTrendPoint             `json:"trend24h"`
	ProviderStats map[uint64]DashboardProviderStats `json:"providerStats"`
	Timezone      string                            `json:"timezone"` // 配置的时区，如 "Asia/Shanghai"
}
