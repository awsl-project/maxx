package domain

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrNotFound            = errors.New("not found")
	ErrAlreadyExists       = errors.New("already exists")
	ErrSlugExists          = errors.New("slug already exists")
	ErrInvalidInput        = errors.New("invalid input")
	ErrInvalidState        = errors.New("invalid state")
	ErrNoRoutes            = errors.New("no routes available")
	ErrAllRoutesFailed     = errors.New("all routes failed")
	ErrFirstByteTimeout    = errors.New("first byte timeout")
	ErrStreamIdleTimeout   = errors.New("stream idle timeout")
	ErrUpstreamError       = errors.New("upstream error")
	ErrFormatConversion    = errors.New("format conversion error")
	ErrUnsupportedFormat   = errors.New("unsupported format")
	ErrInviteCodeRequired  = errors.New("invite code required")
	ErrInviteCodeInvalid   = errors.New("invite code invalid")
	ErrInviteCodeExpired   = errors.New("invite code expired")
	ErrInviteCodeExhausted = errors.New("invite code exhausted")
	ErrInviteCodeDisabled  = errors.New("invite code disabled")
)

// ProxyError represents an error during proxy execution
type ProxyError struct {
	Err                error
	Retryable          bool
	Message            string
	RetryAfter         time.Duration  // Suggested retry delay (from 429 responses)
	CooldownUntil      *time.Time     // Absolute cooldown end time
	CooldownClientType string         // ClientType for cooldown (empty = all client types)
	CooldownUpdateChan chan time.Time // Channel for async cooldown updates (optional)
	RateLimitInfo      *RateLimitInfo // Additional rate limit information
	IsServerError      bool           // True for 5xx errors (triggers incremental cooldown)
	IsNetworkError     bool           // True for network errors (connection timeout, DNS failure, etc.)
	HTTPStatusCode     int            // HTTP status code (for logging and error handling)
}

// RateLimitInfo contains detailed rate limit information from providers
type RateLimitInfo struct {
	Type             string    // Type of rate limit: "quota_exhausted", "rate_limit_exceeded", "concurrent", etc.
	QuotaResetTime   time.Time // When quota resets (for quota exhaustion)
	RetryHintMessage string    // Original error message with retry hints
	ClientType       string    // Affected client type (empty = all)
}

func (e *ProxyError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Err.Error()
}

func (e *ProxyError) Unwrap() error {
	return e.Err
}

func NewProxyError(err error, retryable bool) *ProxyError {
	return &ProxyError{Err: err, Retryable: retryable}
}

func NewProxyErrorWithMessage(err error, retryable bool, msg string) *ProxyError {
	return &ProxyError{Err: err, Retryable: retryable, Message: msg}
}
