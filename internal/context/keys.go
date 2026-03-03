package context

import "context"

type contextKey string

const (
	CtxKeyClientType         contextKey = "client_type"
	CtxKeyOriginalClientType contextKey = "original_client_type"
	CtxKeySessionID          contextKey = "session_id"
	CtxKeyProjectID          contextKey = "project_id"
	CtxKeyRequestModel       contextKey = "request_model"
	CtxKeyMappedModel        contextKey = "mapped_model"
	CtxKeyResponseModel      contextKey = "response_model"
	CtxKeyProxyRequest       contextKey = "proxy_request"
	CtxKeyRequestBody        contextKey = "request_body"
	CtxKeyUpstreamAttempt    contextKey = "upstream_attempt"
	CtxKeyRequestHeaders     contextKey = "request_headers"
	CtxKeyRequestURI         contextKey = "request_uri"
	CtxKeyBroadcaster        contextKey = "broadcaster"
	CtxKeyIsStream           contextKey = "is_stream"
	CtxKeyAPITokenID         contextKey = "api_token_id"
	CtxKeyEventChan          contextKey = "event_chan"
	CtxKeyTenantID           contextKey = "tenant_id"
	CtxKeyUserID             contextKey = "user_id"
	CtxKeyUserRole           contextKey = "user_role"
)

// WithTenantID sets tenant ID in context
func WithTenantID(ctx context.Context, tenantID uint64) context.Context {
	return context.WithValue(ctx, CtxKeyTenantID, tenantID)
}

// GetTenantID gets tenant ID from context, returns 0 if not set
func GetTenantID(ctx context.Context) uint64 {
	if v, ok := ctx.Value(CtxKeyTenantID).(uint64); ok {
		return v
	}
	return 0
}

// WithUserID sets user ID in context
func WithUserID(ctx context.Context, userID uint64) context.Context {
	return context.WithValue(ctx, CtxKeyUserID, userID)
}

// GetUserID gets user ID from context, returns 0 if not set
func GetUserID(ctx context.Context) uint64 {
	if v, ok := ctx.Value(CtxKeyUserID).(uint64); ok {
		return v
	}
	return 0
}

// WithUserRole sets user role in context
func WithUserRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, CtxKeyUserRole, role)
}

// GetUserRole gets user role from context, returns empty string if not set
func GetUserRole(ctx context.Context) string {
	if v, ok := ctx.Value(CtxKeyUserRole).(string); ok {
		return v
	}
	return ""
}
