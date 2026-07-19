package domain

import "testing"

func TestNewUpstreamConnectionErrorIsRetryableProviderNetworkError(t *testing.T) {
	proxyErr := NewUpstreamConnectionError("failed to connect to upstream")

	if proxyErr.Err != ErrUpstreamError {
		t.Fatalf("err = %v, want ErrUpstreamError", proxyErr.Err)
	}
	if proxyErr.Message != "failed to connect to upstream" {
		t.Fatalf("message = %q", proxyErr.Message)
	}
	if proxyErr.Scope != ScopeProvider {
		t.Fatalf("scope = %q, want provider", proxyErr.Scope)
	}
	if proxyErr.Reason != CooldownReasonNetworkError {
		t.Fatalf("reason = %q, want network_error", proxyErr.Reason)
	}
	if !proxyErr.Retryable {
		t.Fatal("upstream connection errors must be retryable")
	}
}
