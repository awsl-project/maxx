package domain

import "time"

// FailureCount tracks failure counts for a provider+clientType+reason combination
type FailureCount struct {
	ID              uint64    `json:"id"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
	TenantID        uint64    `json:"tenantID"`
	ProviderID      uint64    `json:"providerID"`
	ClientType      string    `json:"clientType"` // Empty for global
	Reason          string    `json:"reason"`     // server_error, network_error, etc.
	Count           int       `json:"count"`      // Number of consecutive failures
	LastFailureAt   time.Time `json:"lastFailureAt"`
}
