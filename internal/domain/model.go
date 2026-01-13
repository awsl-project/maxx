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

type ProviderConfig struct {
	Custom      *ProviderConfigCustom      `json:"custom,omitempty"`
	Antigravity *ProviderConfigAntigravity `json:"antigravity,omitempty"`
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

	// 配置
	Config *ProviderConfig `json:"config"`

	// 支持的 Client
	SupportedClientTypes []ClientType `json:"supportedClientTypes"`
}

type Project struct {
	ID        uint64    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	Name string `json:"name"`
	Slug string `json:"slug"`

	// 启用自定义路由的 ClientType 列表，空数组表示所有 ClientType 都使用全局路由
	EnabledCustomRoutes []ClientType `json:"enabledCustomRoutes"`
}

type Session struct {
	ID        uint64    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	SessionID  string     `json:"sessionID"`
	ClientType ClientType `json:"clientType"`

	// 0 表示没有项目
	ProjectID uint64 `json:"projectID"`
}

// 路由
type Route struct {
	ID        uint64    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

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

	// Model 映射: RequestModel → MappedModel，优先级高于 Provider
	ModelMapping map[string]string `json:"modelMapping,omitempty"`
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

	// PENDING, IN_PROGRESS, COMPLETED, FAILED
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
	SettingKeyProxyPort = "proxy_port" // 代理服务器端口，默认 9880
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

	// 邮箱作为唯一标识
	Email string `json:"email"`

	// 用户名
	Name string `json:"name"`

	// 头像 URL
	Picture string `json:"picture"`

	// Google Cloud Project ID
	ProjectID string `json:"projectID"`

	// 订阅等级：FREE, PRO, ULTRA
	SubscriptionTier string `json:"subscriptionTier"`

	// 是否被禁止访问 (403)
	IsForbidden bool `json:"isForbidden"`

	// 各模型配额
	Models []AntigravityModelQuota `json:"models"`

	// 上次更新时间（Unix timestamp）
	LastUpdated int64 `json:"lastUpdated"`
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
