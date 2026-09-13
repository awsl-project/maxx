package custom

import (
	"errors"
	"testing"

	"github.com/awsl-project/maxx/internal/converter"
	"github.com/awsl-project/maxx/internal/domain"
)

func TestResponseConversionUnexpectedEOFClassifiesAsNetworkRetryable(t *testing.T) {
	proxyErr := newResponseConversionProxyError(converter.NewResponseConversionError(errors.New("unexpected end of JSON input")))

	if !proxyErr.Retryable {
		t.Fatal("unexpected EOF conversion error should be retryable")
	}
	if proxyErr.Scope != domain.ScopeProvider {
		t.Fatalf("scope = %s, want provider", proxyErr.Scope)
	}
	if proxyErr.Reason != domain.CooldownReasonNetworkError {
		t.Fatalf("reason = %s, want network_error", proxyErr.Reason)
	}
}

func TestResponseConversionSchemaErrorStaysServerError(t *testing.T) {
	proxyErr := newResponseConversionProxyError(converter.NewResponseConversionError(errors.New("missing required field choices")))

	if !proxyErr.Retryable {
		t.Fatal("provider conversion errors remain retryable before commit")
	}
	if proxyErr.Scope != domain.ScopeProvider {
		t.Fatalf("scope = %s, want provider", proxyErr.Scope)
	}
	if proxyErr.Reason != domain.CooldownReasonServerError {
		t.Fatalf("reason = %s, want server_error", proxyErr.Reason)
	}
}
