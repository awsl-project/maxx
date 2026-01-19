package cooldown

import (
	"time"
)

// CooldownPolicy defines the interface for cooldown calculation strategies
type CooldownPolicy interface {
	// CalculateCooldown calculates cooldown duration based on failure count
	CalculateCooldown(failureCount int) time.Duration
}

// FixedDurationPolicy returns a fixed cooldown duration regardless of failure count
type FixedDurationPolicy struct {
	Duration time.Duration
}

func (p *FixedDurationPolicy) CalculateCooldown(failureCount int) time.Duration {
	return p.Duration
}

// LinearIncrementalPolicy increases cooldown linearly with each failure
// Formula: baseSeconds * failureCount
type LinearIncrementalPolicy struct {
	BaseSeconds int
	MaxSeconds  int // Optional cap, 0 means no limit
}

func (p *LinearIncrementalPolicy) CalculateCooldown(failureCount int) time.Duration {
	seconds := p.BaseSeconds * failureCount
	if p.MaxSeconds > 0 && seconds > p.MaxSeconds {
		seconds = p.MaxSeconds
	}
	return time.Duration(seconds) * time.Second
}

// ExponentialBackoffPolicy increases cooldown exponentially with each failure
// Formula: baseSeconds * (2 ^ (failureCount - 1))
type ExponentialBackoffPolicy struct {
	BaseSeconds int
	MaxSeconds  int // Optional cap, 0 means no limit
}

func (p *ExponentialBackoffPolicy) CalculateCooldown(failureCount int) time.Duration {
	if failureCount == 0 {
		return 0
	}

	seconds := p.BaseSeconds
	for i := 1; i < failureCount; i++ {
		seconds *= 2
		if p.MaxSeconds > 0 && seconds > p.MaxSeconds {
			seconds = p.MaxSeconds
			break
		}
	}

	return time.Duration(seconds) * time.Second
}

// CooldownReason represents the reason for cooldown
type CooldownReason string

const (
	ReasonServerError     CooldownReason = "server_error"          // 5xx errors
	ReasonNetworkError    CooldownReason = "network_error"         // Connection timeout, DNS failure, etc.
	ReasonQuotaExhausted  CooldownReason = "quota_exhausted"       // API quota exhausted (fallback when no explicit time)
	ReasonRateLimit       CooldownReason = "rate_limit_exceeded"   // Rate limit (fallback when no explicit time)
	ReasonConcurrentLimit CooldownReason = "concurrent_limit"      // Concurrent request limit (fallback when no explicit time)
	ReasonUnknown         CooldownReason = "unknown"               // Unknown error
)

// DefaultPolicies returns the default policy configuration
// Note: For quota/rate limit errors with explicit reset times from API,
// those times will be used directly instead of these policies
func DefaultPolicies() map[CooldownReason]CooldownPolicy {
	return map[CooldownReason]CooldownPolicy{
		// Server errors (5xx): linear increment (5s, 10s, 15s, ... max 10min)
		ReasonServerError: &LinearIncrementalPolicy{
			BaseSeconds: 5,
			MaxSeconds:  600, // 10 minutes
		},
		// Network errors: exponential backoff (5s, 10s, 20s, 40s, ... max 30min)
		ReasonNetworkError: &ExponentialBackoffPolicy{
			BaseSeconds: 5,
			MaxSeconds:  1800, // 30 minutes
		},
		// Quota exhausted: fixed 1 hour (only used as fallback when API doesn't return reset time)
		ReasonQuotaExhausted: &FixedDurationPolicy{
			Duration: 1 * time.Hour,
		},
		// Rate limit: fixed 5 seconds (only used as fallback when API doesn't return Retry-After)
		ReasonRateLimit: &FixedDurationPolicy{
			Duration: 5 * time.Second,
		},
		// Concurrent limit: fixed 5 seconds (only used as fallback)
		ReasonConcurrentLimit: &FixedDurationPolicy{
			Duration: 5 * time.Second,
		},
		// Unknown error: linear increment (5s, 10s, 15s, ... max 5min)
		ReasonUnknown: &LinearIncrementalPolicy{
			BaseSeconds: 5,
			MaxSeconds:  300, // 5 minutes
		},
	}
}
