package executor

import (
	"context"
	"errors"
	"strings"

	"github.com/awsl-project/maxx/internal/domain"
)

func requestFailureStatusAndError(ctx context.Context, err error) (string, string) {
	if ctx == nil || ctx.Err() == nil {
		if err != nil {
			return "FAILED", err.Error()
		}
		return "FAILED", "request did not complete"
	}

	if errors.Is(ctx.Err(), context.Canceled) && isClientDisconnectError(err) {
		return "CANCELLED", "client disconnected"
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "FAILED", "request timeout"
	}

	if err != nil {
		return "FAILED", err.Error()
	}
	return "FAILED", ctx.Err().Error()
}

func attemptFailureStatus(ctx context.Context, err error) string {
	status, _ := requestFailureStatusAndError(ctx, err)
	return status
}

func isClientDisconnectError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	if proxyErr, ok := err.(*domain.ProxyError); ok {
		if errors.Is(proxyErr.Err, context.Canceled) || errors.Is(proxyErr.Err, context.DeadlineExceeded) {
			return false
		}
		if isClientDisconnectText(proxyErr.Message) || isClientDisconnectText(proxyErr.Code) || isClientDisconnectText(proxyErr.Err) {
			return true
		}
	}

	return isClientDisconnectText(err)
}

func isClientDisconnectText(v interface{}) bool {
	var text string
	switch x := v.(type) {
	case nil:
		return false
	case string:
		text = x
	case error:
		text = x.Error()
	default:
		return false
	}
	text = strings.ToLower(text)
	return strings.Contains(text, "client disconnected") ||
		strings.Contains(text, "client cancelled") ||
		strings.Contains(text, "client canceled") ||
		strings.Contains(text, "broken pipe") ||
		strings.Contains(text, "connection reset by peer")
}
