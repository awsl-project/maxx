package cooldown

import "time"

// CooldownKey uniquely identifies a cooldown entry
// ClientType is optional - empty string means cooldown applies to all client types
type CooldownKey struct {
	ProviderID uint64
	ClientType string // Empty = all client types
}

// FailureKey tracks failures by provider, client type, and reason
type FailureKey struct {
	ProviderID uint64
	ClientType string
	Reason     CooldownReason
}

// CooldownInfo represents cooldown information for API response
type CooldownInfo struct {
	ProviderID   uint64         `json:"providerID"`
	ProviderName string         `json:"providerName,omitempty"`
	ClientType   string         `json:"clientType,omitempty"` // Empty = all types
	Until        time.Time      `json:"until"`
	Remaining    string         `json:"remaining"` // Human readable remaining time
	Reason       CooldownReason `json:"reason"`    // Cooldown reason
}
