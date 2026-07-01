package antigravity

import (
	"net/http"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/tidwall/gjson"
)

func TestApplyAntigravityRequestTuning(t *testing.T) {
	input := `{
  "request": {
    "systemInstruction": {
      "parts": [{"text":"original"}]
    },
    "tools": [{
      "functionDeclarations": [{
        "name": "t1",
        "parametersJsonSchema": {"type":"object","properties":{"x":{"type":"string"}}}
      }]
    }]
  },
  "model": "claude-sonnet-4-5"
}`
	out := applyAntigravityRequestTuning([]byte(input), "claude-sonnet-4-5")

	if !gjson.GetBytes(out, "request.systemInstruction.role").Exists() {
		t.Fatalf("expected systemInstruction.role to be set")
	}
	if gjson.GetBytes(out, "request.systemInstruction.parts.0.text").String() == "" {
		t.Fatalf("expected systemInstruction parts[0].text to be injected")
	}
	if gjson.GetBytes(out, "request.systemInstruction.parts.1.text").String() == "" {
		t.Fatalf("expected systemInstruction parts[1].text to be injected")
	}
	if gjson.GetBytes(out, "request.toolConfig.functionCallingConfig.mode").String() != "VALIDATED" {
		t.Fatalf("expected toolConfig.functionCallingConfig.mode=VALIDATED")
	}
	if gjson.GetBytes(out, "request.tools.0.functionDeclarations.0.parametersJsonSchema").Exists() {
		t.Fatalf("expected parametersJsonSchema to be renamed")
	}
	if !gjson.GetBytes(out, "request.tools.0.functionDeclarations.0.parameters").Exists() {
		t.Fatalf("expected parameters to exist after rename")
	}
}

func TestClassifyAntigravityHTTPError402InsufficientBalanceIsKeyQuota(t *testing.T) {
	proxyErr := domain.NewProxyErrorWithMessage(
		domain.ErrUpstreamError,
		true,
		"upstream returned status 402",
	)
	proxyErr.HTTPStatusCode = http.StatusPaymentRequired

	classifyAntigravityHTTPError(proxyErr, http.StatusPaymentRequired)

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
