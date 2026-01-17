package context

import (
	"context"
	"net/http"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/event"
)

type contextKey string

const (
	CtxKeyClientType      contextKey = "client_type"
	CtxKeySessionID       contextKey = "session_id"
	CtxKeyProjectID       contextKey = "project_id"
	CtxKeyRequestModel    contextKey = "request_model"
	CtxKeyMappedModel     contextKey = "mapped_model"
	CtxKeyResponseModel   contextKey = "response_model"
	CtxKeyProxyRequest    contextKey = "proxy_request"
	CtxKeyRequestBody     contextKey = "request_body"
	CtxKeyUpstreamAttempt contextKey = "upstream_attempt"
	CtxKeyRequestHeaders  contextKey = "request_headers"
	CtxKeyRequestURI      contextKey = "request_uri"
	CtxKeyBroadcaster     contextKey = "broadcaster"
	CtxKeyIsStream        contextKey = "is_stream"
	CtxKeyAPITokenID      contextKey = "api_token_id"
	CtxKeyEventChan       contextKey = "event_chan"
)

// Setters
func WithClientType(ctx context.Context, ct domain.ClientType) context.Context {
	return context.WithValue(ctx, CtxKeyClientType, ct)
}

func WithSessionID(ctx context.Context, sid string) context.Context {
	return context.WithValue(ctx, CtxKeySessionID, sid)
}

func WithProjectID(ctx context.Context, pid uint64) context.Context {
	return context.WithValue(ctx, CtxKeyProjectID, pid)
}

func WithRequestModel(ctx context.Context, model string) context.Context {
	return context.WithValue(ctx, CtxKeyRequestModel, model)
}

func WithMappedModel(ctx context.Context, model string) context.Context {
	return context.WithValue(ctx, CtxKeyMappedModel, model)
}

func WithResponseModel(ctx context.Context, model string) context.Context {
	return context.WithValue(ctx, CtxKeyResponseModel, model)
}

func WithProxyRequest(ctx context.Context, pr *domain.ProxyRequest) context.Context {
	return context.WithValue(ctx, CtxKeyProxyRequest, pr)
}

func WithRequestBody(ctx context.Context, body []byte) context.Context {
	return context.WithValue(ctx, CtxKeyRequestBody, body)
}

func WithUpstreamAttempt(ctx context.Context, attempt *domain.ProxyUpstreamAttempt) context.Context {
	return context.WithValue(ctx, CtxKeyUpstreamAttempt, attempt)
}

func WithRequestHeaders(ctx context.Context, headers http.Header) context.Context {
	return context.WithValue(ctx, CtxKeyRequestHeaders, headers)
}

func WithRequestURI(ctx context.Context, uri string) context.Context {
	return context.WithValue(ctx, CtxKeyRequestURI, uri)
}

// Getters
func GetClientType(ctx context.Context) domain.ClientType {
	if v, ok := ctx.Value(CtxKeyClientType).(domain.ClientType); ok {
		return v
	}
	return ""
}

func GetSessionID(ctx context.Context) string {
	if v, ok := ctx.Value(CtxKeySessionID).(string); ok {
		return v
	}
	return ""
}

func GetProjectID(ctx context.Context) uint64 {
	if v, ok := ctx.Value(CtxKeyProjectID).(uint64); ok {
		return v
	}
	return 0
}

func GetRequestModel(ctx context.Context) string {
	if v, ok := ctx.Value(CtxKeyRequestModel).(string); ok {
		return v
	}
	return ""
}

func GetMappedModel(ctx context.Context) string {
	if v, ok := ctx.Value(CtxKeyMappedModel).(string); ok {
		return v
	}
	return ""
}

func GetResponseModel(ctx context.Context) string {
	if v, ok := ctx.Value(CtxKeyResponseModel).(string); ok {
		return v
	}
	return ""
}

func GetProxyRequest(ctx context.Context) *domain.ProxyRequest {
	if v, ok := ctx.Value(CtxKeyProxyRequest).(*domain.ProxyRequest); ok {
		return v
	}
	return nil
}

func GetRequestBody(ctx context.Context) []byte {
	if v, ok := ctx.Value(CtxKeyRequestBody).([]byte); ok {
		return v
	}
	return nil
}

func GetUpstreamAttempt(ctx context.Context) *domain.ProxyUpstreamAttempt {
	if v, ok := ctx.Value(CtxKeyUpstreamAttempt).(*domain.ProxyUpstreamAttempt); ok {
		return v
	}
	return nil
}

func GetRequestHeaders(ctx context.Context) http.Header {
	if v, ok := ctx.Value(CtxKeyRequestHeaders).(http.Header); ok {
		return v
	}
	return nil
}

func GetRequestURI(ctx context.Context) string {
	if v, ok := ctx.Value(CtxKeyRequestURI).(string); ok {
		return v
	}
	return ""
}

func WithBroadcaster(ctx context.Context, bc event.Broadcaster) context.Context {
	return context.WithValue(ctx, CtxKeyBroadcaster, bc)
}

func GetBroadcaster(ctx context.Context) event.Broadcaster {
	if v, ok := ctx.Value(CtxKeyBroadcaster).(event.Broadcaster); ok {
		return v
	}
	return nil
}

func WithIsStream(ctx context.Context, isStream bool) context.Context {
	return context.WithValue(ctx, CtxKeyIsStream, isStream)
}

func GetIsStream(ctx context.Context) bool {
	if v, ok := ctx.Value(CtxKeyIsStream).(bool); ok {
		return v
	}
	return false
}

func WithAPITokenID(ctx context.Context, id uint64) context.Context {
	return context.WithValue(ctx, CtxKeyAPITokenID, id)
}

func GetAPITokenID(ctx context.Context) uint64 {
	if v, ok := ctx.Value(CtxKeyAPITokenID).(uint64); ok {
		return v
	}
	return 0
}

func WithEventChan(ctx context.Context, ch domain.AdapterEventChan) context.Context {
	return context.WithValue(ctx, CtxKeyEventChan, ch)
}

func GetEventChan(ctx context.Context) domain.AdapterEventChan {
	if v, ok := ctx.Value(CtxKeyEventChan).(domain.AdapterEventChan); ok {
		return v
	}
	return nil
}
