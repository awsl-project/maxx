package sqlite

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// ==================== GORM Models ====================
// These models map directly to the database schema.
// Domain models are converted to/from these in repository methods.

// ==================== Custom Types ====================

// LongText is a string type that maps to LONGTEXT in MySQL and TEXT in SQLite/PostgreSQL
type LongText string

// GormDBDataType returns the database-specific data type
func (LongText) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	switch db.Name() {
	case "mysql":
		return "longtext"
	default:
		return "text"
	}
}

// BaseModel contains common fields for all entities
type BaseModel struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	CreatedAt int64
	UpdatedAt int64
}

// SoftDeleteModel adds soft delete support
type SoftDeleteModel struct {
	BaseModel
	DeletedAt int64 `gorm:"index"`
}

// BeforeCreate sets timestamps before creating
func (m *BaseModel) BeforeCreate(tx *gorm.DB) error {
	now := time.Now().UnixMilli()
	if m.CreatedAt == 0 {
		m.CreatedAt = now
	}
	if m.UpdatedAt == 0 {
		m.UpdatedAt = now
	}
	return nil
}

// BeforeUpdate sets updated_at before updating
func (m *BaseModel) BeforeUpdate(tx *gorm.DB) error {
	m.UpdatedAt = time.Now().UnixMilli()
	return nil
}

// ==================== Entity Models ====================

// Provider model
type Provider struct {
	SoftDeleteModel
	Type                 string `gorm:"size:64"`
	Name                 string `gorm:"size:255"`
	Config               LongText
	SupportedClientTypes LongText
	SupportModels        LongText
}

func (Provider) TableName() string { return "providers" }

// Project model
type Project struct {
	SoftDeleteModel
	Name                string `gorm:"size:255"`
	Slug                string `gorm:"size:128"`
	EnabledCustomRoutes LongText
}

func (Project) TableName() string { return "projects" }

// Session model
type Session struct {
	SoftDeleteModel
	SessionID  string `gorm:"size:255;uniqueIndex"`
	ClientType string `gorm:"size:64"`
	ProjectID  uint64
	RejectedAt int64
}

func (Session) TableName() string { return "sessions" }

// Route model
type Route struct {
	SoftDeleteModel
	IsEnabled     int `gorm:"default:1"`
	IsNative      int `gorm:"default:1"`
	ProjectID     uint64
	ClientType    string `gorm:"size:64"`
	ProviderID    uint64
	Position      int
	RetryConfigID uint64
}

func (Route) TableName() string { return "routes" }

// RetryConfig model
type RetryConfig struct {
	SoftDeleteModel
	Name              string `gorm:"size:255"`
	IsDefault         int
	MaxRetries        int     `gorm:"default:3"`
	InitialIntervalMs int     `gorm:"default:1000"`
	BackoffRate       float64 `gorm:"default:2.0"`
	MaxIntervalMs     int     `gorm:"default:30000"`
}

func (RetryConfig) TableName() string { return "retry_configs" }

// RoutingStrategy model
type RoutingStrategy struct {
	SoftDeleteModel
	ProjectID uint64
	Type      string `gorm:"size:64"`
	Config    LongText
}

func (RoutingStrategy) TableName() string { return "routing_strategies" }

// APIToken model
type APIToken struct {
	SoftDeleteModel
	Token       string `gorm:"size:255;uniqueIndex"`
	TokenPrefix string `gorm:"size:32"`
	Name        string `gorm:"size:255"`
	Description LongText
	ProjectID   uint64
	IsEnabled   int `gorm:"default:1"`
	ExpiresAt   int64
	LastUsedAt  int64
	UseCount    uint64
}

func (APIToken) TableName() string { return "api_tokens" }

// ModelMapping model
type ModelMapping struct {
	SoftDeleteModel
	Scope        string `gorm:"size:64;default:'global'"`
	ClientType   string `gorm:"size:64"`
	ProviderType string `gorm:"size:64"`
	ProviderID   uint64
	ProjectID    uint64
	RouteID      uint64
	APITokenID   uint64
	Pattern      string `gorm:"size:255"`
	Target       string `gorm:"size:255"`
	Priority     int
}

func (ModelMapping) TableName() string { return "model_mappings" }

// AntigravityQuota model
type AntigravityQuota struct {
	SoftDeleteModel
	Email            string `gorm:"size:255;uniqueIndex"`
	SubscriptionTier string `gorm:"size:64;default:'FREE'"`
	IsForbidden      int
	Models           LongText
	Name             string `gorm:"size:255"`
	Picture          LongText
	GCPProjectID     string `gorm:"size:128;column:gcp_project_id"`
}

func (AntigravityQuota) TableName() string { return "antigravity_quotas" }

// ==================== Log/Status/Stats Models (no soft delete) ====================

// ProxyRequest model
type ProxyRequest struct {
	BaseModel
	InstanceID                  string `gorm:"size:64"`
	RequestID                   string `gorm:"size:64"`
	SessionID                   string `gorm:"size:255;index"`
	ClientType                  string `gorm:"size:64"`
	RequestModel                string `gorm:"size:128"`
	ResponseModel               string `gorm:"size:128"`
	StartTime                   int64
	EndTime                     int64 `gorm:"index"`
	DurationMs                  int64
	TTFTMs                      int64
	Status                      string `gorm:"size:64"`
	RequestInfo                 LongText
	ResponseInfo                LongText
	Error                       LongText
	ProxyUpstreamAttemptCount   uint64
	FinalProxyUpstreamAttemptID uint64
	InputTokenCount             uint64
	OutputTokenCount            uint64
	CacheReadCount              uint64
	CacheWriteCount             uint64
	Cache5mWriteCount           uint64 `gorm:"column:cache_5m_write_count"`
	Cache1hWriteCount           uint64 `gorm:"column:cache_1h_write_count"`
	ModelPriceID                uint64 // 使用的模型价格记录ID
	Multiplier                  uint64 // 倍率（10000=1倍）
	Cost                        uint64
	RouteID                     uint64
	ProviderID                  uint64
	IsStream                    int
	StatusCode                  int
	ProjectID                   uint64
	APITokenID                  uint64
}

func (ProxyRequest) TableName() string { return "proxy_requests" }

// ProxyUpstreamAttempt model
type ProxyUpstreamAttempt struct {
	BaseModel
	Status            string `gorm:"size:64"`
	ProxyRequestID    uint64 `gorm:"index"`
	RequestInfo       LongText
	ResponseInfo      LongText
	RouteID           uint64
	ProviderID        uint64
	InputTokenCount   uint64
	OutputTokenCount  uint64
	CacheReadCount    uint64
	CacheWriteCount   uint64
	Cache5mWriteCount uint64 `gorm:"column:cache_5m_write_count"`
	Cache1hWriteCount uint64 `gorm:"column:cache_1h_write_count"`
	ModelPriceID      uint64 // 使用的模型价格记录ID
	Multiplier        uint64 // 倍率（10000=1倍）
	Cost              uint64
	IsStream          int
	StartTime         int64
	EndTime           int64
	DurationMs        int64
	TTFTMs            int64
	RequestModel      string `gorm:"size:128"`
	MappedModel       string `gorm:"size:128"`
	ResponseModel     string `gorm:"size:128"`
}

func (ProxyUpstreamAttempt) TableName() string { return "proxy_upstream_attempts" }

// SystemSetting model
type SystemSetting struct {
	Key       string `gorm:"column:setting_key;size:255;primaryKey"`
	Value     LongText
	CreatedAt int64
	UpdatedAt int64
}

func (SystemSetting) TableName() string { return "system_settings" }

// Cooldown model
type Cooldown struct {
	BaseModel
	ProviderID uint64 `gorm:"uniqueIndex:idx_cooldowns_provider_client"`
	ClientType string `gorm:"size:255;uniqueIndex:idx_cooldowns_provider_client"`
	UntilTime  int64  `gorm:"index"`
	Reason     string `gorm:"size:64;default:'unknown'"`
}

func (Cooldown) TableName() string { return "cooldowns" }

// FailureCount model
type FailureCount struct {
	BaseModel
	ProviderID    uint64 `gorm:"uniqueIndex:idx_failure_counts_provider_client_reason"`
	ClientType    string `gorm:"size:255;uniqueIndex:idx_failure_counts_provider_client_reason"`
	Reason        string `gorm:"size:255;uniqueIndex:idx_failure_counts_provider_client_reason"`
	Count         int
	LastFailureAt int64 `gorm:"index"`
}

func (FailureCount) TableName() string { return "failure_counts" }

// UsageStats model
type UsageStats struct {
	ID                 uint64 `gorm:"primaryKey;autoIncrement"`
	CreatedAt          int64
	TimeBucket         int64  `gorm:"uniqueIndex:idx_usage_stats_unique"`
	Granularity        string `gorm:"size:32;uniqueIndex:idx_usage_stats_unique;index:idx_usage_stats_granularity_time"`
	RouteID            uint64 `gorm:"uniqueIndex:idx_usage_stats_unique;index:idx_usage_stats_route_id"`
	ProviderID         uint64 `gorm:"uniqueIndex:idx_usage_stats_unique;index:idx_usage_stats_provider_id"`
	ProjectID          uint64 `gorm:"uniqueIndex:idx_usage_stats_unique;index:idx_usage_stats_project_id"`
	APITokenID         uint64 `gorm:"uniqueIndex:idx_usage_stats_unique;index:idx_usage_stats_api_token_id"`
	ClientType         string `gorm:"size:64;uniqueIndex:idx_usage_stats_unique"`
	Model              string `gorm:"size:128;uniqueIndex:idx_usage_stats_unique;index:idx_usage_stats_model"`
	TotalRequests      uint64
	SuccessfulRequests uint64
	FailedRequests     uint64
	TotalDurationMs    uint64
	TotalTTFTMs        uint64
	InputTokens        uint64
	OutputTokens       uint64
	CacheRead          uint64
	CacheWrite         uint64
	Cost               uint64
}

func (UsageStats) TableName() string { return "usage_stats" }

// ResponseModel tracks all response models seen
type ResponseModel struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement"`
	CreatedAt  int64
	Name       string `gorm:"size:255;uniqueIndex"`
	LastSeenAt int64
	UseCount   uint64
}

func (ResponseModel) TableName() string { return "response_models" }

// SchemaMigration tracks applied migrations
type SchemaMigration struct {
	Version     int    `gorm:"primaryKey"`
	Description string `gorm:"size:255"`
	AppliedAt   int64
}

func (SchemaMigration) TableName() string { return "schema_migrations" }

// ModelPrice model - 模型价格（每个模型可有多条记录，每条代表一个版本）
type ModelPrice struct {
	ID                     uint64 `gorm:"primaryKey;autoIncrement"`
	CreatedAt              int64
	DeletedAt              int64  `gorm:"index"` // 软删除时间
	ModelID                string `gorm:"size:128;index"`
	InputPriceMicro        uint64
	OutputPriceMicro       uint64
	CacheReadPriceMicro    uint64
	Cache5mWritePriceMicro uint64 `gorm:"column:cache_5m_write_price_micro"`
	Cache1hWritePriceMicro uint64 `gorm:"column:cache_1h_write_price_micro"`
	Has1MContext           int
	Context1MThreshold     uint64 `gorm:"column:context_1m_threshold"`
	InputPremiumNum        uint64
	InputPremiumDenom      uint64
	OutputPremiumNum       uint64
	OutputPremiumDenom     uint64
}

func (ModelPrice) TableName() string { return "model_prices" }

// ==================== All Models for AutoMigrate ====================

// AllModels returns all GORM models for auto-migration
func AllModels() []any {
	return []any{
		&Provider{},
		&Project{},
		&Session{},
		&Route{},
		&RetryConfig{},
		&RoutingStrategy{},
		&APIToken{},
		&ModelMapping{},
		&AntigravityQuota{},
		&ProxyRequest{},
		&ProxyUpstreamAttempt{},
		&SystemSetting{},
		&Cooldown{},
		&FailureCount{},
		&UsageStats{},
		&ResponseModel{},
		&ModelPrice{},
		&SchemaMigration{},
	}
}
