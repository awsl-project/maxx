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

// Tenant model
type Tenant struct {
	SoftDeleteModel
	Name      string `gorm:"size:255"`
	Slug      string `gorm:"size:128;uniqueIndex"`
	IsDefault int
}

func (Tenant) TableName() string { return "tenants" }

// User model
type User struct {
	SoftDeleteModel
	TenantID           uint64 `gorm:"index"`
	Username           string `gorm:"size:255;uniqueIndex"`
	PasswordHash       string `gorm:"size:255"`
	PasskeyCredentials LongText
	InviteCodeID       uint64 `gorm:"index"`
	Role               string `gorm:"size:64;default:'admin'"`
	Status             string `gorm:"size:64;default:'pending'"`
	IsDefault          int
	LastLoginAt        int64
}

func (User) TableName() string { return "users" }

// Provider model
type Provider struct {
	SoftDeleteModel
	TenantID             uint64 `gorm:"index"`
	Type                 string `gorm:"size:64"`
	Name                 string `gorm:"size:255"`
	Logo                 LongText
	Config               LongText
	SupportedClientTypes LongText
	SupportModels        LongText
	ExcludeFromExport    int `gorm:"default:0"`
}

func (Provider) TableName() string { return "providers" }

// Project model
type Project struct {
	SoftDeleteModel
	TenantID            uint64 `gorm:"index"`
	Name                string `gorm:"size:255"`
	Slug                string `gorm:"size:128"`
	EnabledCustomRoutes LongText
}

func (Project) TableName() string { return "projects" }

// Session model
type Session struct {
	SoftDeleteModel
	TenantID   uint64 `gorm:"index"`
	SessionID  string `gorm:"size:255;uniqueIndex"`
	ClientType string `gorm:"size:64"`
	ProjectID  uint64
	RejectedAt int64
}

func (Session) TableName() string { return "sessions" }

// Route model
type Route struct {
	SoftDeleteModel
	TenantID      uint64 `gorm:"index"`
	IsEnabled     int    `gorm:"default:1"`
	IsNative      int    `gorm:"default:1"`
	ProjectID     uint64
	ClientType    string `gorm:"size:64"`
	ProviderID    uint64
	Position      int
	Weight        int `gorm:"default:1"`
	RetryConfigID uint64
}

func (Route) TableName() string { return "routes" }

// RetryConfig model
type RetryConfig struct {
	SoftDeleteModel
	TenantID          uint64 `gorm:"index"`
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
	TenantID  uint64 `gorm:"index"`
	ProjectID uint64
	Type      string `gorm:"size:64"`
	Config    LongText
}

func (RoutingStrategy) TableName() string { return "routing_strategies" }

// APIToken model
type APIToken struct {
	SoftDeleteModel
	TenantID    uint64 `gorm:"index"`
	Token       string `gorm:"size:255;uniqueIndex"`
	TokenPrefix string `gorm:"size:32"`
	Name        string `gorm:"size:255"`
	Description LongText
	ProjectID   uint64
	IsEnabled   int `gorm:"default:1"`
	DevMode     int `gorm:"default:0"`
	ExpiresAt   int64
	LastUsedAt  int64
	LastIP      string `gorm:"size:64"`
	LastIPAt    int64
	UseCount    uint64
}

func (APIToken) TableName() string { return "api_tokens" }

// InviteCode model
type InviteCode struct {
	SoftDeleteModel
	TenantID        uint64 `gorm:"index;uniqueIndex:idx_invite_codes_tenant_hash"`
	CodeHash        string `gorm:"size:128;uniqueIndex:idx_invite_codes_tenant_hash"`
	CodePrefix      string `gorm:"size:32"`
	Status          string `gorm:"size:32;default:'active'"`
	MaxUses         uint64
	UsedCount       uint64
	ExpiresAt       int64
	CreatedByUserID uint64 `gorm:"index"`
	Note            LongText
}

func (InviteCode) TableName() string { return "invite_codes" }

// InviteCodeUsage model
type InviteCodeUsage struct {
	BaseModel
	TenantID     uint64 `gorm:"index"`
	InviteCodeID uint64 `gorm:"index"`
	UserID       uint64 `gorm:"index"`
	Username     string `gorm:"size:255"`
	UsedAt       int64  `gorm:"index"`
	IP           string `gorm:"size:64"`
	UserAgent    string `gorm:"size:512"`
	Result       string `gorm:"size:32"`
	Reason       string `gorm:"size:255"`
	RolledBack   int    `gorm:"default:0"`
}

func (InviteCodeUsage) TableName() string { return "invite_code_usages" }

// ModelMapping model
type ModelMapping struct {
	SoftDeleteModel
	TenantID     uint64 `gorm:"index"`
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
	TenantID         uint64 `gorm:"uniqueIndex:idx_antigravity_quotas_tenant_email"`
	Email            string `gorm:"size:255;uniqueIndex:idx_antigravity_quotas_tenant_email"`
	SubscriptionTier string `gorm:"size:64;default:'FREE'"`
	IsForbidden      int
	Models           LongText
	Name             string `gorm:"size:255"`
	Picture          LongText
	GCPProjectID     string `gorm:"size:128;column:gcp_project_id"`
}

func (AntigravityQuota) TableName() string { return "antigravity_quotas" }

// CodexQuota model
// NOTE: identity/email indexes are intentionally managed by explicit migrations,
// not by GORM AutoMigrate. Creating the new unique identity index during
// AutoMigrate can fail on historical data before the dedupe migration runs.
type CodexQuota struct {
	SoftDeleteModel
	TenantID         uint64
	IdentityKey      string `gorm:"size:255;column:identity_key"`
	Email            string `gorm:"size:255"`
	AccountID        string `gorm:"size:128;column:account_id"`
	PlanType         string `gorm:"size:64"`
	IsForbidden      int
	PrimaryWindow    LongText `gorm:"column:primary_window"`     // JSON
	SecondaryWindow  LongText `gorm:"column:secondary_window"`   // JSON
	CodeReviewWindow LongText `gorm:"column:code_review_window"` // JSON
}

func (CodexQuota) TableName() string { return "codex_quotas" }

// ==================== Log/Status/Stats Models (no soft delete) ====================

// ProxyRequest model
type ProxyRequest struct {
	BaseModel
	TenantID                    uint64 `gorm:"index"`
	InstanceID                  string `gorm:"size:64"`
	RequestID                   string `gorm:"size:64"`
	SessionID                   string `gorm:"size:255;index"`
	ClientType                  string `gorm:"size:64"`
	RequestModel                string `gorm:"size:128"`
	ResponseModel               string `gorm:"size:128"`
	StartTime                   int64
	EndTime                     int64 `gorm:"index;index:idx_requests_status_endtime"`
	DurationMs                  int64
	TTFTMs                      int64
	Status                      string `gorm:"size:64;index;index:idx_requests_status_endtime"`
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
	ProviderID                  uint64 `gorm:"index"`
	IsStream                    int
	StatusCode                  int
	ProjectID                   uint64 `gorm:"index"`
	APITokenID                  uint64
	DevMode                     int `gorm:"default:0"`
}

func (ProxyRequest) TableName() string { return "proxy_requests" }

// ProxyUpstreamAttempt model
type ProxyUpstreamAttempt struct {
	BaseModel
	TenantID          uint64 `gorm:"index"`
	Status            string `gorm:"size:64;index:idx_attempts_status_endtime;index"`
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
	EndTime           int64 `gorm:"index:idx_attempts_status_endtime"`
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
	TenantID   uint64 `gorm:"index"`
	ProviderID uint64 `gorm:"uniqueIndex:idx_cooldowns_provider_client"`
	ClientType string `gorm:"size:255;uniqueIndex:idx_cooldowns_provider_client"`
	UntilTime  int64  `gorm:"index"`
	Reason     string `gorm:"size:64;default:'unknown'"`
}

func (Cooldown) TableName() string { return "cooldowns" }

// FailureCount model
type FailureCount struct {
	BaseModel
	TenantID      uint64 `gorm:"uniqueIndex:idx_failure_counts_tenant_provider_client_reason"`
	ProviderID    uint64 `gorm:"uniqueIndex:idx_failure_counts_tenant_provider_client_reason"`
	ClientType    string `gorm:"size:255;uniqueIndex:idx_failure_counts_tenant_provider_client_reason"`
	Reason        string `gorm:"size:255;uniqueIndex:idx_failure_counts_tenant_provider_client_reason"`
	Count         int
	LastFailureAt int64 `gorm:"index"`
}

func (FailureCount) TableName() string { return "failure_counts" }

// UsageStats model
type UsageStats struct {
	ID                 uint64 `gorm:"primaryKey;autoIncrement"`
	CreatedAt          int64
	TenantID           uint64 `gorm:"index;uniqueIndex:idx_usage_stats_unique"`
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
		&Tenant{},
		&User{},
		&Provider{},
		&Project{},
		&Session{},
		&Route{},
		&RetryConfig{},
		&RoutingStrategy{},
		&APIToken{},
		&InviteCode{},
		&InviteCodeUsage{},
		&ModelMapping{},
		&AntigravityQuota{},
		&CodexQuota{},
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
