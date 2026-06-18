package codex

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/usage"
	"github.com/tidwall/gjson"
)

// collectMetricsEvent drains an AdapterEventChan and returns the first
// EventMetrics payload it finds, or nil if none was sent.
func collectMetricsEvent(ch domain.AdapterEventChan) *domain.AdapterMetrics {
	for {
		select {
		case ev := <-ch:
			if ev != nil && ev.Type == domain.EventMetrics {
				return ev.Metrics
			}
		default:
			return nil
		}
	}
}

type scriptedReadCloser struct {
	chunks [][]byte
	err    error
	idx    int
}

func (r *scriptedReadCloser) Read(p []byte) (int, error) {
	if r.idx < len(r.chunks) {
		chunk := r.chunks[r.idx]
		r.idx++
		copy(p, chunk)
		return len(chunk), nil
	}
	if r.err != nil {
		err := r.err
		r.err = nil
		return 0, err
	}
	return 0, io.EOF
}

func (r *scriptedReadCloser) Close() error {
	return nil
}

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

func TestApplyCodexHeadersPreservesProvidedUA(t *testing.T) {
	a := &CodexAdapter{}
	upstreamReq, _ := http.NewRequest("POST", "https://chatgpt.com/backend-api/codex/responses", nil)
	clientReq, _ := http.NewRequest("POST", "http://localhost/responses", nil)
	clientReq.Header.Set("User-Agent", "Mozilla/5.0")
	clientReq.Header.Set("X-Custom", "ok")

	a.applyCodexHeaders(upstreamReq, clientReq, "token-1", "acct-1", true, "")

	if got := upstreamReq.Header.Get("User-Agent"); got != "Mozilla/5.0" {
		t.Fatalf("expected provided User-Agent passthrough, got %q", got)
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

func TestApplyCodexHeadersUsesDefaultUAWhenClientUAIsBlank(t *testing.T) {
	a := &CodexAdapter{}
	upstreamReq, _ := http.NewRequest("POST", "https://chatgpt.com/backend-api/codex/responses", nil)
	clientReq, _ := http.NewRequest("POST", "http://localhost/responses", nil)
	clientReq.Header.Set("User-Agent", "   ")

	a.applyCodexHeaders(upstreamReq, clientReq, "token-1", "acct-1", true, "")

	if got := upstreamReq.Header.Get("User-Agent"); got != CodexUserAgent {
		t.Fatalf("expected default Codex User-Agent when client UA is blank, got %q", got)
	}
}

func TestExtractModelFromSSELine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantSet bool
		wantVal string
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

func TestHandleStreamResponseReturnsProviderErrorWhenStreamEndsBeforeCompleted(t *testing.T) {
	a := &CodexAdapter{}
	req := httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses", nil)
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, req)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n",
		)),
	}

	err := a.handleStreamResponse(ctx, resp)
	if err == nil {
		t.Fatal("expected incomplete stream to return error")
	}
	proxyErr, ok := err.(*domain.ProxyError)
	if !ok {
		t.Fatalf("expected ProxyError, got %T", err)
	}
	if proxyErr.Scope != domain.ScopeProvider {
		t.Fatalf("expected scope %q, got %q", domain.ScopeProvider, proxyErr.Scope)
	}
	if proxyErr.Reason != domain.CooldownReasonNetworkError {
		t.Fatalf("expected reason %q, got %q", domain.CooldownReasonNetworkError, proxyErr.Reason)
	}
	if proxyErr.Message != "stream closed before response.completed" {
		t.Fatalf("expected message %q, got %q", "stream closed before response.completed", proxyErr.Message)
	}
}

func TestHandleStreamResponseReturnsProviderErrorOnReadErrorBeforeCompleted(t *testing.T) {
	a := &CodexAdapter{}
	req := httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses", nil)
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, req)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: &scriptedReadCloser{
			chunks: [][]byte{[]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n")},
			err:    errors.New("boom"),
		},
	}

	err := a.handleStreamResponse(ctx, resp)
	if err == nil {
		t.Fatal("expected read error before completion to return error")
	}
	proxyErr, ok := err.(*domain.ProxyError)
	if !ok {
		t.Fatalf("expected ProxyError, got %T", err)
	}
	if proxyErr.Scope != domain.ScopeProvider {
		t.Fatalf("expected scope %q, got %q", domain.ScopeProvider, proxyErr.Scope)
	}
	if proxyErr.Reason != domain.CooldownReasonNetworkError {
		t.Fatalf("expected reason %q, got %q", domain.CooldownReasonNetworkError, proxyErr.Reason)
	}
	if proxyErr.Message != "stream closed before response.completed" {
		t.Fatalf("expected message %q, got %q", "stream closed before response.completed", proxyErr.Message)
	}
}

// TestHandleStreamResponseScopesModelOnInStreamModelError exercises the path
// where the upstream emits a structured "model not supported" event and then
// closes the stream without response.completed. Without scope refinement the
// adapter would freeze the entire provider; with it, only the failing model
// should be cooled down.
func TestHandleStreamResponseScopesModelOnInStreamModelError(t *testing.T) {
	a := &CodexAdapter{}
	req := httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses", nil)
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, req)

	// Real-world shape observed for ChatGPT-account Codex on unsupported models:
	// upstream returns 200 OK, sends an error event, then EOFs.
	stream := strings.Join([]string{
		`data: {"type":"response.created","response":{"model":"gpt-5.5-codex"}}`,
		`data: {"type":"response.failed","response":{"model":"gpt-5.5-codex","error":{"code":"model_not_supported","message":"The 'gpt-5.5-codex' model is not supported when using Codex with a ChatGPT account."}}}`,
		``,
	}, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(stream)),
	}

	err := a.handleStreamResponse(ctx, resp)
	if err == nil {
		t.Fatal("expected stream close without response.completed to return error")
	}
	proxyErr, ok := err.(*domain.ProxyError)
	if !ok {
		t.Fatalf("expected ProxyError, got %T", err)
	}
	if proxyErr.Scope != domain.ScopeModel {
		t.Errorf("Scope = %q, want %q (in-stream model error should narrow scope)", proxyErr.Scope, domain.ScopeModel)
	}
	if proxyErr.Reason != domain.CooldownReasonModelUnavailable {
		t.Errorf("Reason = %q, want %q", proxyErr.Reason, domain.CooldownReasonModelUnavailable)
	}
	if proxyErr.Model != "gpt-5.5-codex" {
		t.Errorf("Model = %q, want %q", proxyErr.Model, "gpt-5.5-codex")
	}
}

// TestHandleStreamResponseScopesModelOnFastAPIDetail covers the observed
// ChatGPT-account Codex shape: a single SSE event with no `type` field and a
// `detail` message naming the unsupported model. The error event itself
// carries no model field, so the adapter must attribute the model from the
// flow context's mapped model.
func TestHandleStreamResponseScopesModelOnFastAPIDetail(t *testing.T) {
	a := &CodexAdapter{}
	req := httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses", nil)
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, req)
	ctx.Set(flow.KeyMappedModel, "gpt-5.5-codex")

	stream := `data: {"detail":"The 'gpt-5.5-codex' model is not supported when using Codex with a ChatGPT account."}` + "\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(stream)),
	}

	err := a.handleStreamResponse(ctx, resp)
	proxyErr, ok := err.(*domain.ProxyError)
	if !ok {
		t.Fatalf("expected ProxyError, got %T", err)
	}
	if proxyErr.Scope != domain.ScopeModel {
		t.Errorf("Scope = %q, want %q", proxyErr.Scope, domain.ScopeModel)
	}
	if proxyErr.Model != "gpt-5.5-codex" {
		t.Errorf("Model = %q, want %q (must attribute from flow context when error event lacks model)", proxyErr.Model, "gpt-5.5-codex")
	}
}

// TestHandleStreamResponseDowngradesToProviderWhenModelUnattributable guards
// the safety net: if classifyCodexStreamError returns ScopeModel but no model
// can be attributed from anywhere, ScopeModel with empty Model would collapse
// to a (provider, "", "") cooldown key — i.e. provider-wide — defeating the
// scope refinement. The adapter must fall back to the explicit ScopeProvider.
func TestHandleStreamResponseDowngradesToProviderWhenModelUnattributable(t *testing.T) {
	a := &CodexAdapter{}
	req := httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses", nil)
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, req)
	// Deliberately do not set KeyMappedModel.

	stream := `data: {"detail":"unknown model"}` + "\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(stream)),
	}

	err := a.handleStreamResponse(ctx, resp)
	proxyErr, ok := err.(*domain.ProxyError)
	if !ok {
		t.Fatalf("expected ProxyError, got %T", err)
	}
	if proxyErr.Scope != domain.ScopeProvider {
		t.Errorf("Scope = %q, want %q (unattributable model should not emit ScopeModel)", proxyErr.Scope, domain.ScopeProvider)
	}
	if proxyErr.Reason != domain.CooldownReasonNetworkError {
		t.Errorf("Reason = %q, want %q", proxyErr.Reason, domain.CooldownReasonNetworkError)
	}
}

// TestHandleStreamResponseKeepsProviderScopeWithoutErrorEvent guards the
// fallback: a stream that closes without ANY structured error signal should
// still cool down the whole provider so genuine outages keep tripping the
// wider cooldown.
func TestHandleStreamResponseKeepsProviderScopeWithoutErrorEvent(t *testing.T) {
	a := &CodexAdapter{}
	req := httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses", nil)
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, req)

	stream := `data: {"type":"response.output_text.delta","delta":"hello"}` + "\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(stream)),
	}

	err := a.handleStreamResponse(ctx, resp)
	if err == nil {
		t.Fatal("expected error for incomplete stream")
	}
	proxyErr, ok := err.(*domain.ProxyError)
	if !ok {
		t.Fatalf("expected ProxyError, got %T", err)
	}
	if proxyErr.Scope != domain.ScopeProvider {
		t.Errorf("Scope = %q, want %q (no error event → provider-wide fallback)", proxyErr.Scope, domain.ScopeProvider)
	}
	if proxyErr.Reason != domain.CooldownReasonNetworkError {
		t.Errorf("Reason = %q, want %q", proxyErr.Reason, domain.CooldownReasonNetworkError)
	}
}

func TestHandleStreamResponseAllowsCompletedStreamWithoutTrailingNewline(t *testing.T) {
	a := &CodexAdapter{}
	req := httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses", nil)
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, req)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.completed\",\"response\":{}}",
		)),
	}

	if err := a.handleStreamResponse(ctx, resp); err != nil {
		t.Fatalf("expected completed stream without trailing newline to succeed, got %v", err)
	}
}

func TestPersistRefreshedTokenFailsOnUpdateError(t *testing.T) {
	orig := &domain.Provider{
		ID:   42,
		Name: "codex-account",
		Config: &domain.ProviderConfig{Codex: &domain.ProviderConfigCodex{
			AccessToken:  "old-at",
			RefreshToken: "old-rt",
			ExpiresAt:    "old-exp",
		}},
	}
	updateErr := errors.New("db down")
	a := &CodexAdapter{
		providerUpdate: func(p *domain.Provider) error {
			if p == orig {
				t.Fatal("provider update must receive a clone, not the shared provider pointer")
			}
			if got := p.Config.Codex.AccessToken; got != "new-at" {
				t.Fatalf("persisted access token = %q, want new-at", got)
			}
			if got := p.Config.Codex.RefreshToken; got != "new-rt" {
				t.Fatalf("persisted refresh token = %q, want new-rt", got)
			}
			return updateErr
		},
	}

	err := a.persistRefreshedToken(orig, &TokenResponse{AccessToken: "new-at", RefreshToken: "new-rt", ExpiresIn: 3600}, time.Unix(123, 0).UTC())
	if err == nil || !strings.Contains(err.Error(), "failed to persist refreshed token") || !errors.Is(err, updateErr) {
		t.Fatalf("expected wrapped persistence error, got %v", err)
	}
	if got := orig.Config.Codex.AccessToken; got != "old-at" {
		t.Fatalf("original provider was mutated: access token = %q", got)
	}
	if got := orig.Config.Codex.RefreshToken; got != "old-rt" {
		t.Fatalf("original provider was mutated: refresh token = %q", got)
	}
}

func TestPersistRefreshedTokenRequiresUpdaterForRotatedRefreshToken(t *testing.T) {
	a := &CodexAdapter{}
	orig := &domain.Provider{Config: &domain.ProviderConfig{Codex: &domain.ProviderConfigCodex{RefreshToken: "old-rt"}}}

	err := a.persistRefreshedToken(orig, &TokenResponse{AccessToken: "new-at", RefreshToken: "new-rt", ExpiresIn: 3600}, time.Unix(123, 0).UTC())
	if err == nil || !strings.Contains(err.Error(), "provider update callback not configured") {
		t.Fatalf("expected missing updater error for rotated refresh token, got %v", err)
	}
}

func TestHandleNonStreamResponseForwardsCacheReadCount(t *testing.T) {
	a := &CodexAdapter{}
	req := httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses", nil)
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, req)
	eventChan := domain.NewAdapterEventChan()
	ctx.Set(flow.KeyEventChan, eventChan)

	body := `{"id":"resp_1","object":"response","model":"gpt-5","usage":{"input_tokens":120,"output_tokens":40,"input_tokens_details":{"cached_tokens":80}}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	if err := a.handleNonStreamResponse(ctx, resp); err != nil {
		t.Fatalf("handleNonStreamResponse: %v", err)
	}

	metrics := collectMetricsEvent(eventChan)
	if metrics == nil {
		t.Fatal("expected EventMetrics to be emitted")
	}
	// Codex usage.input_tokens includes cached_tokens; AdjustForClientType
	// subtracts so pricing does not bill the cached portion at the input rate
	// on top of the cache-read rate. Expect input_tokens (120) - cached (80) = 40.
	if metrics.InputTokens != 40 || metrics.OutputTokens != 40 {
		t.Fatalf("input/output mismatch: got input=%d output=%d, want input=40 output=40", metrics.InputTokens, metrics.OutputTokens)
	}
	if metrics.CacheReadCount != 80 {
		t.Fatalf("expected CacheReadCount=80, got %d", metrics.CacheReadCount)
	}
}

func TestHandleStreamResponseForwardsCacheReadCount(t *testing.T) {
	a := &CodexAdapter{}
	req := httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses", nil)
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, req)
	eventChan := domain.NewAdapterEventChan()
	ctx.Set(flow.KeyEventChan, eventChan)

	// Real Codex Responses SSE: usage is only carried on the terminating
	// response.completed event.
	stream := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"hi"}`,
		`data: {"type":"response.completed","response":{"model":"gpt-5","usage":{"input_tokens":200,"output_tokens":50,"input_tokens_details":{"cached_tokens":150}}}}`,
		``,
	}, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(stream)),
	}

	if err := a.handleStreamResponse(ctx, resp); err != nil {
		t.Fatalf("handleStreamResponse: %v", err)
	}

	metrics := collectMetricsEvent(eventChan)
	if metrics == nil {
		t.Fatal("expected EventMetrics to be emitted from response.completed")
	}
	// After AdjustForClientType: input_tokens (200) - cached (150) = 50.
	if metrics.InputTokens != 50 || metrics.OutputTokens != 50 {
		t.Fatalf("input/output mismatch: got input=%d output=%d, want input=50 output=50", metrics.InputTokens, metrics.OutputTokens)
	}
	if metrics.CacheReadCount != 150 {
		t.Fatalf("expected CacheReadCount=150, got %d", metrics.CacheReadCount)
	}
}

// A cache-only metric (no fresh input/output tokens, just a prompt-cache hit)
// must still flow through sendFinalStreamEvents. Metrics.IsEmpty is the gate;
// regressing it to drop cache-only metrics would silently zero out cache stats
// for some streams.
func TestSendFinalStreamEventsEmitsCacheOnlyMetrics(t *testing.T) {
	a := &CodexAdapter{}
	req := httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses", nil)
	rec := httptest.NewRecorder()
	_ = flow.NewCtx(rec, req)
	eventChan := domain.NewAdapterEventChan()

	collector := &usage.StreamCollector{Metrics: &usage.Metrics{CacheReadCount: 42}}
	model := ""
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}

	a.sendFinalStreamEvents(eventChan, collector, &model, resp)

	metrics := collectMetricsEvent(eventChan)
	if metrics == nil {
		t.Fatal("expected EventMetrics for cache-only metrics")
	}
	if metrics.CacheReadCount != 42 {
		t.Fatalf("expected CacheReadCount=42, got %d", metrics.CacheReadCount)
	}
	if metrics.InputTokens != 0 || metrics.OutputTokens != 0 {
		t.Fatalf("expected zero input/output tokens, got input=%d output=%d", metrics.InputTokens, metrics.OutputTokens)

	}
}
