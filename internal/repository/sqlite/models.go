package sqlite

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// ==================== GORM Models ====================
// These models map directly to the database schema.
// Domain models are converted to/from these in repository methods.

// BaseModel contains common fields for all entities
type BaseModel struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	CreatedAt int64  `gorm:"not null"`
	UpdatedAt int64  `gorm:"not null"`
}

// SoftDeleteModel adds soft delete support
type SoftDeleteModel struct {
	BaseModel
	DeletedAt int64 `gorm:"default:0;index"`
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

// ==================== JSON Types ====================

// JSONMap is a map that serializes to JSON in the database
type JSONMap map[string]any

func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return "", nil
	}
	b, err := json.Marshal(j)
	return string(b), err
}

func (j *JSONMap) Scan(value any) error {
	if value == nil {
		*j = nil
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case string:
		bytes = []byte(v)
	case []byte:
		bytes = v
	default:
		return nil
	}
	if len(bytes) == 0 {
		*j = nil
		return nil
	}
	return json.Unmarshal(bytes, j)
}

// JSONSlice is a slice that serializes to JSON in the database
type JSONSlice[T any] []T

func (j JSONSlice[T]) Value() (driver.Value, error) {
	if j == nil {
		return "[]", nil
	}
	b, err := json.Marshal(j)
	return string(b), err
}

func (j *JSONSlice[T]) Scan(value any) error {
	if value == nil {
		*j = nil
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case string:
		bytes = []byte(v)
	case []byte:
		bytes = v
	default:
		return nil
	}
	if len(bytes) == 0 {
		*j = nil
		return nil
	}
	return json.Unmarshal(bytes, j)
}

// ==================== Entity Models ====================

// Provider model
type Provider struct {
	SoftDeleteModel
	Type                 string `gorm:"not null"`
	Name                 string `gorm:"not null"`
	Config               string `gorm:"type:text"`
	SupportedClientTypes string `gorm:"type:text"`
}

func (Provider) TableName() string { return "providers" }

// Project model
type Project struct {
	SoftDeleteModel
	Name                string `gorm:"not null"`
	Slug                string `gorm:"not null;default:''"`
	EnabledCustomRoutes string `gorm:"type:text;default:'[]'"`
}

func (Project) TableName() string { return "projects" }

// Session model
type Session struct {
	SoftDeleteModel
	SessionID  string `gorm:"not null;uniqueIndex"`
	ClientType string `gorm:"not null"`
	ProjectID  uint64 `gorm:"default:0"`
	RejectedAt int64  `gorm:"default:0"`
}

func (Session) TableName() string { return "sessions" }

// Route model
type Route struct {
	SoftDeleteModel
	IsEnabled     int    `gorm:"default:1"`
	IsNative      int    `gorm:"default:1"`
	ProjectID     uint64 `gorm:"default:0"`
	ClientType    string `gorm:"not null"`
	ProviderID    uint64 `gorm:"not null"`
	Position      int    `gorm:"default:0"`
	RetryConfigID uint64 `gorm:"default:0"`
}

func (Route) TableName() string { return "routes" }

// RetryConfig model
type RetryConfig struct {
	SoftDeleteModel
	Name              string  `gorm:"not null"`
	IsDefault         int     `gorm:"default:0"`
	MaxRetries        int     `gorm:"default:3"`
	InitialIntervalMs int     `gorm:"default:1000"`
	BackoffRate       float64 `gorm:"default:2.0"`
	MaxIntervalMs     int     `gorm:"default:30000"`
}

func (RetryConfig) TableName() string { return "retry_configs" }

// RoutingStrategy model
type RoutingStrategy struct {
	SoftDeleteModel
	ProjectID uint64 `gorm:"default:0"`
	Type      string `gorm:"not null"`
	Config    string `gorm:"type:text"`
}

func (RoutingStrategy) TableName() string { return "routing_strategies" }

// APIToken model
type APIToken struct {
	SoftDeleteModel
	Token       string `gorm:"not null;uniqueIndex"`
	TokenPrefix string `gorm:"not null"`
	Name        string `gorm:"not null"`
	Description string `gorm:"default:''"`
	ProjectID   uint64 `gorm:"default:0"`
	IsEnabled   int    `gorm:"default:1"`
	ExpiresAt   int64  `gorm:"default:0"`
	LastUsedAt  int64  `gorm:"default:0"`
	UseCount    uint64 `gorm:"default:0"`
}

func (APIToken) TableName() string { return "api_tokens" }

// ModelMapping model
type ModelMapping struct {
	SoftDeleteModel
	Scope        string `gorm:"default:'global'"`
	ClientType   string `gorm:"default:''"`
	ProviderType string `gorm:"default:''"`
	ProviderID   uint64 `gorm:"default:0"`
	ProjectID    uint64 `gorm:"default:0"`
	RouteID      uint64 `gorm:"default:0"`
	APITokenID   uint64 `gorm:"default:0"`
	Pattern      string `gorm:"not null"`
	Target       string `gorm:"not null"`
	Priority     int    `gorm:"default:0"`
}

func (ModelMapping) TableName() string { return "model_mappings" }

// AntigravityQuota model
type AntigravityQuota struct {
	SoftDeleteModel
	Email            string `gorm:"not null;uniqueIndex"`
	SubscriptionTier string `gorm:"default:'FREE'"`
	IsForbidden      int    `gorm:"default:0"`
	Models           string `gorm:"type:text"`
	Name             string `gorm:"default:''"`
	Picture          string `gorm:"default:''"`
	GCPProjectID     string `gorm:"column:gcp_project_id;default:''"`
}

func (AntigravityQuota) TableName() string { return "antigravity_quotas" }

// ==================== Log/Status/Stats Models (no soft delete) ====================

// ProxyRequest model
type ProxyRequest struct {
	BaseModel
	InstanceID                  string `gorm:"type:text"`
	RequestID                   string `gorm:"type:text"`
	SessionID                   string `gorm:"type:text;index"`
	ClientType                  string `gorm:"type:text"`
	RequestModel                string `gorm:"type:text"`
	ResponseModel               string `gorm:"type:text"`
	StartTime                   int64  `gorm:"default:0"`
	EndTime                     int64  `gorm:"default:0"`
	DurationMs                  int64  `gorm:"default:0"`
	Status                      string `gorm:"type:text"`
	RequestInfo                 string `gorm:"type:text"`
	ResponseInfo                string `gorm:"type:text"`
	Error                       string `gorm:"type:text"`
	ProxyUpstreamAttemptCount   uint64 `gorm:"default:0"`
	FinalProxyUpstreamAttemptID uint64 `gorm:"default:0"`
	InputTokenCount             uint64 `gorm:"default:0"`
	OutputTokenCount            uint64 `gorm:"default:0"`
	CacheReadCount              uint64 `gorm:"default:0"`
	CacheWriteCount             uint64 `gorm:"default:0"`
	Cache5mWriteCount           uint64 `gorm:"column:cache_5m_write_count;default:0"`
	Cache1hWriteCount           uint64 `gorm:"column:cache_1h_write_count;default:0"`
	Cost                        uint64 `gorm:"default:0"`
	RouteID                     uint64 `gorm:"default:0"`
	ProviderID                  uint64 `gorm:"default:0"`
	IsStream                    int    `gorm:"default:0"`
	StatusCode                  int    `gorm:"default:0"`
	ProjectID                   uint64 `gorm:"default:0"`
	APITokenID                  uint64 `gorm:"default:0"`
}

func (ProxyRequest) TableName() string { return "proxy_requests" }

// ProxyUpstreamAttempt model
type ProxyUpstreamAttempt struct {
	BaseModel
	Status            string `gorm:"type:text"`
	ProxyRequestID    uint64 `gorm:"index"`
	RequestInfo       string `gorm:"type:text"`
	ResponseInfo      string `gorm:"type:text"`
	RouteID           uint64
	ProviderID        uint64
	InputTokenCount   uint64 `gorm:"default:0"`
	OutputTokenCount  uint64 `gorm:"default:0"`
	CacheReadCount    uint64 `gorm:"default:0"`
	CacheWriteCount   uint64 `gorm:"default:0"`
	Cache5mWriteCount uint64 `gorm:"column:cache_5m_write_count;default:0"`
	Cache1hWriteCount uint64 `gorm:"column:cache_1h_write_count;default:0"`
	Cost              uint64 `gorm:"default:0"`
	IsStream          int    `gorm:"default:0"`
	StartTime         int64  `gorm:"default:0"`
	EndTime           int64  `gorm:"default:0"`
	DurationMs        int64  `gorm:"default:0"`
	RequestModel      string `gorm:"default:''"`
	MappedModel       string `gorm:"default:''"`
	ResponseModel     string `gorm:"default:''"`
}

func (ProxyUpstreamAttempt) TableName() string { return "proxy_upstream_attempts" }

// SystemSetting model
type SystemSetting struct {
	Key       string `gorm:"primaryKey"`
	Value     string `gorm:"not null"`
	CreatedAt int64  `gorm:"not null"`
	UpdatedAt int64  `gorm:"not null"`
}

func (SystemSetting) TableName() string { return "system_settings" }

// Cooldown model
type Cooldown struct {
	BaseModel
	ProviderID uint64 `gorm:"not null;uniqueIndex:idx_cooldowns_provider_client"`
	ClientType string `gorm:"not null;default:'';uniqueIndex:idx_cooldowns_provider_client"`
	UntilTime  int64  `gorm:"not null;index"`
	Reason     string `gorm:"not null;default:'unknown'"`
}

func (Cooldown) TableName() string { return "cooldowns" }

// FailureCount model
type FailureCount struct {
	BaseModel
	ProviderID    uint64 `gorm:"not null;uniqueIndex:idx_failure_counts_provider_client_reason"`
	ClientType    string `gorm:"not null;default:'';uniqueIndex:idx_failure_counts_provider_client_reason"`
	Reason        string `gorm:"not null;uniqueIndex:idx_failure_counts_provider_client_reason"`
	Count         int    `gorm:"default:0"`
	LastFailureAt int64  `gorm:"not null;index"`
}

func (FailureCount) TableName() string { return "failure_counts" }

// UsageStats model
type UsageStats struct {
	ID                 uint64 `gorm:"primaryKey;autoIncrement"`
	CreatedAt          int64  `gorm:"not null"`
	TimeBucket         int64  `gorm:"not null;uniqueIndex:idx_usage_stats_unique"`
	Granularity        string `gorm:"not null;uniqueIndex:idx_usage_stats_unique;index:idx_usage_stats_granularity_time"`
	RouteID            uint64 `gorm:"default:0;uniqueIndex:idx_usage_stats_unique;index:idx_usage_stats_route_id"`
	ProviderID         uint64 `gorm:"default:0;uniqueIndex:idx_usage_stats_unique;index:idx_usage_stats_provider_id"`
	ProjectID          uint64 `gorm:"default:0;uniqueIndex:idx_usage_stats_unique;index:idx_usage_stats_project_id"`
	APITokenID         uint64 `gorm:"default:0;uniqueIndex:idx_usage_stats_unique;index:idx_usage_stats_api_token_id"`
	ClientType         string `gorm:"default:'';uniqueIndex:idx_usage_stats_unique"`
	Model              string `gorm:"default:'';uniqueIndex:idx_usage_stats_unique;index:idx_usage_stats_model"`
	TotalRequests      uint64 `gorm:"default:0"`
	SuccessfulRequests uint64 `gorm:"default:0"`
	FailedRequests     uint64 `gorm:"default:0"`
	TotalDurationMs    uint64 `gorm:"default:0"`
	InputTokens        uint64 `gorm:"default:0"`
	OutputTokens       uint64 `gorm:"default:0"`
	CacheRead          uint64 `gorm:"default:0"`
	CacheWrite         uint64 `gorm:"default:0"`
	Cost               uint64 `gorm:"default:0"`
}

func (UsageStats) TableName() string { return "usage_stats" }

// ResponseModel tracks all response models seen
type ResponseModel struct {
	ID         uint64 `gorm:"primaryKey;autoIncrement"`
	CreatedAt  int64  `gorm:"not null"`
	Name       string `gorm:"not null;uniqueIndex"`
	LastSeenAt int64  `gorm:"not null"`
	UseCount   uint64 `gorm:"default:0"`
}

func (ResponseModel) TableName() string { return "response_models" }

// SchemaMigration tracks applied migrations
type SchemaMigration struct {
	Version     int    `gorm:"primaryKey"`
	Description string `gorm:"not null"`
	AppliedAt   int64  `gorm:"not null"`
}

func (SchemaMigration) TableName() string { return "schema_migrations" }

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
		&SchemaMigration{},
	}
}
