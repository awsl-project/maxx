package domain

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrNotFound             = errors.New("not found")
	ErrAlreadyExists        = errors.New("already exists")
	ErrSlugExists           = errors.New("slug already exists")
	ErrInvalidInput         = errors.New("invalid input")
	ErrInvalidState         = errors.New("invalid state")
	ErrNoRoutes             = errors.New("no routes available")
	ErrNoAvailableProviders = errors.New("no available providers")
	// ErrModelNotSupported means routes/providers existed for the client type but
	// every candidate was rejected because the requested model is not in the
	// provider's SupportModels allowlist — a client-side request error (the model
	// is disallowed), distinct from the transient ErrNoAvailableProviders.
	ErrModelNotSupported                    = errors.New("model not supported by any provider")
	ErrAllRoutesFailed                      = errors.New("all routes failed")
	ErrFirstByteTimeout                     = errors.New("first byte timeout")
	ErrStreamIdleTimeout                    = errors.New("stream idle timeout")
	ErrUpstreamError                        = errors.New("upstream error")
	ErrFormatConversion                     = errors.New("format conversion error")
	ErrUnsupportedFormat                    = errors.New("unsupported format")
	ErrNoResponsesWebSocketProviders        = errors.New("no native responses websocket providers available")
	ErrResponsesWebSocketSessionUnavailable = errors.New("responses websocket session unavailable")
	ErrResponsesWebSocketProtocol           = errors.New("responses websocket protocol error")
	ErrInviteCodeRequired                   = errors.New("invite code required")
	ErrInviteCodeInvalid                    = errors.New("invite code invalid")
	ErrInviteCodeExpired                    = errors.New("invite code expired")
	ErrInviteCodeExhausted                  = errors.New("invite code exhausted")
	ErrInviteCodeDisabled                   = errors.New("invite code disabled")
)

// ErrorScope defines what resource is broken, determining cooldown granularity
type ErrorScope string

const (
	// ScopeRequest: only this request is bad (400, 413, 422, content filter)
	// Action: do NOT cooldown anything, do NOT retry on other providers
	ScopeRequest ErrorScope = "request"

	// ScopeModel: this specific model is unavailable on this provider
	// Action: cooldown (providerID, clientType, model)
	ScopeModel ErrorScope = "model"

	// ScopeKey: the API key has a problem (quota, rate limit, auth failure)
	// Action: cooldown (providerID, clientType, "") — all models
	ScopeKey ErrorScope = "key"

	// ScopeEndpoint: the upstream endpoint for this client type is down
	// Action: cooldown (providerID, clientType, "") — all models for this client type
	ScopeEndpoint ErrorScope = "endpoint"

	// ScopeProvider: entire upstream is unreachable (network, DNS, full outage)
	// Action: cooldown (providerID, "", "") — everything
	ScopeProvider ErrorScope = "provider"
)

// ProxyError represents a structured error during proxy execution
type ProxyError struct {
	Err     error
	Message string
	Code    string

	// Classification
	Scope          ErrorScope     // What resource is broken (determines cooldown granularity)
	Reason         CooldownReason // Why it's broken (maps to cooldown policy)
	HTTPStatusCode int            // Original HTTP status code

	// Cooldown hints
	RetryAfter         time.Duration  // Suggested retry delay
	CooldownUntil      *time.Time     // Absolute cooldown end time
	CooldownUpdateChan chan time.Time // Channel for async cooldown updates (optional)

	// Retry
	Retryable bool

	// Context (for cooldown key construction)
	Model      string // The model that triggered the error (for ScopeModel)
	ClientType string // Affected client type (empty = all)
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

func NewProxyErrorWithMessage(err error, retryable bool, msg string) *ProxyError {
	return &ProxyError{Err: err, Retryable: retryable, Message: msg}
}

func NewScopedProxyError(err error, scope ErrorScope, reason CooldownReason) *ProxyError {
	return &ProxyError{
		Err:       err,
		Scope:     scope,
		Reason:    reason,
		Retryable: scope != ScopeRequest,
	}
}

func NewUpstreamConnectionError(message string) *ProxyError {
	if message == "" {
		message = "failed to connect to upstream"
	}
	proxyErr := NewScopedProxyError(ErrUpstreamError, ScopeProvider, CooldownReasonNetworkError)
	proxyErr.Message = message
	return proxyErr
}
