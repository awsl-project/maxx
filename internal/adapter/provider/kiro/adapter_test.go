package kiro

import (
	"net/http"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestClassifyKiroHTTPError402InsufficientBalanceIsKeyQuota(t *testing.T) {
	proxyErr := classifyKiroHTTPError(
		http.StatusPaymentRequired,
		[]byte(`{"error":{"message":"Insufficient balance","type":"bad_response_status_code"}}`),
		make(http.Header),
		"kiro-model",
	)

	if proxyErr.Scope != domain.ScopeKey {
		t.Fatalf("Scope = %v, want ScopeKey", proxyErr.Scope)
	}
	if proxyErr.Reason != domain.CooldownReasonQuotaExhausted {
		t.Fatalf("Reason = %v, want CooldownReasonQuotaExhausted", proxyErr.Reason)
	}
	if proxyErr.Retryable {
		t.Fatal("402 insufficient balance should not retry the same provider")
	}
}
