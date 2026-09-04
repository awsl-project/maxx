package executor

import (
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

// TestGetRetryConfigFallbackKeepsBackoff covers the state proxy_request 75010
// ran into: the tenant had no default retry config yet, so the fallback's zero
// intervals turned a disableErrorCooldown provider's retries into a busy loop
// firing ~90 upstream calls per second.
func TestGetRetryConfigFallbackKeepsBackoff(t *testing.T) {
	e := &Executor{retryConfigRepo: &staticRetryConfigRepo{}}

	config := e.getRetryConfig(domain.DefaultTenantID, nil)

	if config.MaxRetries != 0 {
		t.Errorf("MaxRetries = %d, want 0 (no default config still means no configured retry)", config.MaxRetries)
	}
	if config.InitialInterval <= 0 {
		t.Errorf("InitialInterval = %v, want a non-zero fallback", config.InitialInterval)
	}
	if config.MaxInterval <= 0 {
		t.Errorf("MaxInterval = %v, want a non-zero fallback", config.MaxInterval)
	}
	if got := e.calculateBackoff(config, 0); got <= 0 {
		t.Errorf("first backoff = %v, want > 0", got)
	}
}

func TestGetRetryConfigPrefersRouteConfig(t *testing.T) {
	e := &Executor{retryConfigRepo: &staticRetryConfigRepo{
		defaultConfig: &domain.RetryConfig{MaxRetries: 9},
	}}
	routeConfig := &domain.RetryConfig{MaxRetries: 2}

	if got := e.getRetryConfig(domain.DefaultTenantID, routeConfig); got != routeConfig {
		t.Errorf("getRetryConfig = %+v, want the route's own config", got)
	}
	if got := e.getRetryConfig(domain.DefaultTenantID, nil); got.MaxRetries != 9 {
		t.Errorf("MaxRetries = %d, want the tenant default (9)", got.MaxRetries)
	}
}

func TestCalculateBackoff(t *testing.T) {
	e := &Executor{}
	tests := []struct {
		name    string
		config  *domain.RetryConfig
		attempt int
		want    time.Duration
	}{
		{
			name:    "grows exponentially",
			config:  &domain.RetryConfig{InitialInterval: time.Second, BackoffRate: 2, MaxInterval: 30 * time.Second},
			attempt: 3,
			want:    8 * time.Second,
		},
		{
			name:    "caps at MaxInterval",
			config:  &domain.RetryConfig{InitialInterval: time.Second, BackoffRate: 2, MaxInterval: 30 * time.Second},
			attempt: 10,
			want:    30 * time.Second,
		},
		{
			// MaxInterval 0 used to mean "clamp everything to zero", silently
			// disabling backoff for any config that left the ceiling unset.
			name:    "unset MaxInterval means no ceiling",
			config:  &domain.RetryConfig{InitialInterval: time.Second, BackoffRate: 2, MaxInterval: 0},
			attempt: 2,
			want:    4 * time.Second,
		},
		{
			// A rate below 1 would shrink the wait on every retry.
			name:    "shrinking rate is treated as flat",
			config:  &domain.RetryConfig{InitialInterval: time.Second, BackoffRate: 0, MaxInterval: 30 * time.Second},
			attempt: 4,
			want:    time.Second,
		},
		{
			name:    "explicit zero interval stays zero",
			config:  &domain.RetryConfig{InitialInterval: 0, BackoffRate: 2, MaxInterval: 30 * time.Second},
			attempt: 3,
			want:    0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := e.calculateBackoff(tt.config, tt.attempt); got != tt.want {
				t.Errorf("calculateBackoff(attempt=%d) = %v, want %v", tt.attempt, got, tt.want)
			}
		})
	}
}
