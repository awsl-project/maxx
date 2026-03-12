package codex

import (
	"net/http"
	"strings"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/tidwall/gjson"
)

func TestApplyCodexRequestTuning(t *testing.T) {
	c := flow.NewCtx(nil, nil)
	c.Set(flow.KeyOriginalClientType, domain.ClientTypeClaude)
	c.Set(flow.KeyOriginalRequestBody, []byte(`{"metadata":{"user_id":"user-123"}}`))

	body := []byte(`{"model":"gpt-5","stream":false,"instructions":"x","previous_response_id":"r1","prompt_cache_retention":123,"safety_identifier":"s1","max_output_tokens":77,"input":[{"type":"message","role":"user","content":"hi"},{"type":"function_call","role":"assistant","name":"t","arguments":"{}","id":"toolu_01"},{"type":"function_call","name":"t2","arguments":"{}"},{"type":"function_call_output","call_id":"c1"},{"role":"tool","call_id":"c1","output":"ok"}]}`)
	cacheID, tuned := applyCodexRequestTuning(c, body)

	if cacheID == "" {
		t.Fatalf("expected cacheID to be set")
	}
	if gjson.GetBytes(tuned, "prompt_cache_key").String() == "" {
		t.Fatalf("expected prompt_cache_key to be set")
	}
	if !gjson.GetBytes(tuned, "stream").Bool() {
		t.Fatalf("expected stream=true")
	}
	if gjson.GetBytes(tuned, "previous_response_id").Exists() {
		t.Fatalf("expected previous_response_id to be removed")
	}
	if gjson.GetBytes(tuned, "prompt_cache_retention").Exists() {
		t.Fatalf("expected prompt_cache_retention to be removed")
	}
	if gjson.GetBytes(tuned, "safety_identifier").Exists() {
		t.Fatalf("expected safety_identifier to be removed")
	}
	if gjson.GetBytes(tuned, "max_output_tokens").Exists() {
		t.Fatalf("expected max_output_tokens to be removed")
	}
	if gjson.GetBytes(tuned, "max_tokens").Int() != 77 {
		t.Fatalf("expected max_tokens to be set from max_output_tokens")
	}
	if gjson.GetBytes(tuned, "input.0.role").String() != "user" {
		t.Fatalf("expected role to be preserved for message input")
	}
	if gjson.GetBytes(tuned, "input.1.role").Exists() || gjson.GetBytes(tuned, "input.2.role").Exists() || gjson.GetBytes(tuned, "input.3.role").Exists() || gjson.GetBytes(tuned, "input.4.role").Exists() {
		t.Fatalf("expected role to be removed for non-message inputs")
	}
	if gjson.GetBytes(tuned, "input.1.id").String() != "fc_toolu_01" {
		t.Fatalf("expected function_call id to be prefixed with fc_")
	}
	missingID := gjson.GetBytes(tuned, "input.2.id").String()
	if !strings.HasPrefix(missingID, "fc_") || missingID == "fc_" {
		t.Fatalf("expected generated function_call id to be set and prefixed with fc_")
	}
	if gjson.GetBytes(tuned, "input.3.output").String() != "" {
		t.Fatalf("expected missing function_call_output output to default to empty string")
	}
}

func TestApplyCodexHeadersFiltersSensitiveAndPreservesUA(t *testing.T) {
	a := &CodexAdapter{}
	upstreamReq, _ := http.NewRequest("POST", "https://chatgpt.com/backend-api/codex/responses", nil)
	clientReq, _ := http.NewRequest("POST", "http://localhost/responses", nil)
	clientReq.Header.Set("User-Agent", "codex-cli/1.2.3")
	clientReq.Header.Set("X-Forwarded-For", "1.2.3.4")
	clientReq.Header.Set("Traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00")
	clientReq.Header.Set("X-Request-Id", "rid-1")
	clientReq.Header.Set("X-Custom", "ok")

	a.applyCodexHeaders(upstreamReq, clientReq, "token-1", "acct-1", true, "")

	if got := upstreamReq.Header.Get("X-Forwarded-For"); got != "" {
		t.Fatalf("expected X-Forwarded-For filtered, got %q", got)
	}
	if got := upstreamReq.Header.Get("Traceparent"); got != "" {
		t.Fatalf("expected Traceparent filtered, got %q", got)
	}
	if got := upstreamReq.Header.Get("X-Request-Id"); got != "" {
		t.Fatalf("expected X-Request-Id filtered, got %q", got)
	}
	if got := upstreamReq.Header.Get("User-Agent"); got != "codex-cli/1.2.3" {
		t.Fatalf("expected User-Agent passthrough, got %q", got)
	}
	if got := upstreamReq.Header.Get("X-Custom"); got != "ok" {
		t.Fatalf("expected X-Custom passthrough, got %q", got)
	}
}

func TestIsCodexResponseCompletedLine(t *testing.T) {
	if !isCodexResponseCompletedLine("data: {\"type\":\"response.completed\",\"response\":{}}\n") {
		t.Fatal("expected response.completed line to be detected")
	}
	if isCodexResponseCompletedLine("data: {\"type\":\"response.delta\"}\n") {
		t.Fatal("expected non-completed line to be false")
	}
	if isCodexResponseCompletedLine("data: not-json\n") {
		t.Fatal("expected invalid json line to be false")
	}
}

func TestApplyCodexHeadersUsesDefaultUAForNonCLI(t *testing.T) {
	a := &CodexAdapter{}
	upstreamReq, _ := http.NewRequest("POST", "https://chatgpt.com/backend-api/codex/responses", nil)
	clientReq, _ := http.NewRequest("POST", "http://localhost/responses", nil)
	clientReq.Header.Set("User-Agent", "Mozilla/5.0")
	clientReq.Header.Set("X-Custom", "ok")

	a.applyCodexHeaders(upstreamReq, clientReq, "token-1", "acct-1", true, "")

	if got := upstreamReq.Header.Get("User-Agent"); got != CodexUserAgent {
		t.Fatalf("expected default Codex User-Agent for non-CLI client, got %q", got)
	}
	if got := upstreamReq.Header.Get("X-Custom"); got != "ok" {
		t.Fatalf("expected X-Custom passthrough, got %q", got)
	}
}

func TestApplyCodexHeadersUsesDefaultUAWhenClientReqNil(t *testing.T) {
	a := &CodexAdapter{}
	upstreamReq, _ := http.NewRequest("POST", "https://chatgpt.com/backend-api/codex/responses", nil)

	a.applyCodexHeaders(upstreamReq, nil, "token-1", "acct-1", true, "")

	if got := upstreamReq.Header.Get("User-Agent"); got != CodexUserAgent {
		t.Fatalf("expected default Codex User-Agent when client request is nil, got %q", got)
	}
}

func TestExtractModelFromSSELine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantSet  bool
		wantVal  string
	}{
		{"extracts model", `data: {"model":"gpt-4.1","type":"response.created"}`, true, "gpt-4.1"},
		{"ignores non-data", `event: response.created`, false, ""},
		{"ignores DONE", `data: [DONE]`, false, ""},
		{"ignores invalid json", `data: not-json`, false, ""},
		{"ignores empty model", `data: {"model":"","type":"response.created"}`, false, ""},
		{"handles trailing newline", "data: {\"model\":\"o3-pro\"}\n", true, "o3-pro"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var model string
			extractModelFromSSELine(tc.line, &model)
			if tc.wantSet && model != tc.wantVal {
				t.Fatalf("expected model=%q, got %q", tc.wantVal, model)
			}
			if !tc.wantSet && model != "" {
				t.Fatalf("expected model to remain empty, got %q", model)
			}
		})
	}
}

func TestExtractModelFromSSELine_KeepsLast(t *testing.T) {
	lines := []string{
		`data: {"model":"gpt-4.1","type":"response.created"}`,
		`data: {"type":"response.output_text.delta","delta":"hello"}`,
		`data: {"model":"gpt-4.1-2025-04-14","type":"response.completed","response":{}}`,
	}
	var model string
	for _, line := range lines {
		extractModelFromSSELine(line, &model)
	}
	if model != "gpt-4.1-2025-04-14" {
		t.Fatalf("expected last model, got %q", model)
	}
}
