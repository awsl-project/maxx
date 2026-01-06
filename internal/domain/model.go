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
	BaseURL string

	// API Key
	APIKey string

	// 某个 Client 有特殊的 BaseURL
	ClientBaseURL map[ClientType]string
}

type ProviderConfigAntigravity struct {
	// 邮箱
	Email string
	// Refresh Token
	RefreshToken string
}

type ProviderConfig struct {
	Custom      *ProviderConfigCustom
	Antigravity *ProviderConfigAntigravity
}

// Provider 供应商
type Provider struct {
	ID        uint64
	CreatedAt time.Time
	UpdatedAt time.Time

	// 1. Custom ，主要用来各种中转站
	// 2. Antigravity
	Type string

	// 展示的名称
	Name string

	// 配置
	Config *ProviderConfig

	// 支持的 Client
	SupportedClientTypes []ClientType
}

type Project struct {
	ID        uint64
	CreatedAt time.Time
	UpdatedAt time.Time

	Name string
}

type Session struct {
	ID        uint64
	CreatedAt time.Time
	UpdatedAt time.Time

	SessionID  string
	ClientType ClientType

	// 0 表示没有项目
	ProjectID uint64
}

// 路由
type Route struct {
	ID        uint64
	CreatedAt time.Time
	UpdatedAt time.Time

	IsEnabled bool

	// 0 表示没有项目即全局
	ProjectID  uint64
	ClientType ClientType
	ProviderID uint64

	// 数字越小越优先
	Priority int
}

type RequestInfo struct {
	Method  string
	Headers map[string]string
	URL     string
	Body    string
}
type ResponseInfo struct {
	Status  int
	Headers map[string]string
	Body    string
}

// 追踪
type ProxyRequest struct {
	ID        uint64
	CreatedAt time.Time
	UpdatedAt time.Time

	RequestID  string
	SessionID  string
	ClientType ClientType

	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration

	// PENDING, IN_PROGRESS, COMPLETED, FAILED
	Status string

	// 原始请求的信息
	RequestInfo  *RequestInfo
	ResponseInfo *ResponseInfo

	// 错误信息
	Error                       string
	ProxyUpstreamAttemptCount   uint64
	FinalProxyUpstreamAttemptID uint64

	// Token 使用情况
	InputTokenCount  uint64
	OutputTokenCount uint64
	CacheReadCount   uint64
	CacheWriteCount  uint64
	Cost             uint64
}

type ProxyUpstreamAttempt struct {
	ID        uint64
	CreatedAt time.Time
	UpdatedAt time.Time

	// PENDING, IN_PROGRESS, COMPLETED, FAILED
	Status string

	ProxyRequestID uint64

	RequestInfo  *RequestInfo
	ResponseInfo *ResponseInfo

	RouteID    uint64
	ProviderID uint64

	// Token 使用情况
	InputTokenCount  uint64
	OutputTokenCount uint64
	CacheReadCount   uint64
	CacheWriteCount  uint64
	Cost             uint64
}
