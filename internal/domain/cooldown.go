package domain

import "time"

// CooldownReason represents the reason for cooldown
type CooldownReason string

const (
	CooldownReasonServerError        CooldownReason = "server_error"
	CooldownReasonNetworkError       CooldownReason = "network_error"
	CooldownReasonQuotaExhausted     CooldownReason = "quota_exhausted"
	CooldownReasonRateLimitExceeded  CooldownReason = "rate_limit_exceeded"
	CooldownReasonConcurrentLimit    CooldownReason = "concurrent_limit"
	CooldownReasonAuthFailure        CooldownReason = "auth_failure"
	CooldownReasonModelUnavailable   CooldownReason = "model_unavailable"
	CooldownReasonUnknown            CooldownReason = "unknown"
)

// Cooldown represents a provider cooldown record
type Cooldown struct {
	ID         uint64         `json:"id"`
	CreatedAt  time.Time      `json:"createdAt"`
	UpdatedAt  time.Time      `json:"updatedAt"`
	TenantID   uint64         `json:"tenantID"`
	ProviderID uint64         `json:"providerID"`
	ClientType string         `json:"clientType"` // Empty for global cooldown
	Model      string         `json:"model"`      // Empty for all models
	UntilTime  time.Time      `json:"untilTime"`  // Absolute time when cooldown ends
	Reason     CooldownReason `json:"reason"`     // Reason for cooldown
}
