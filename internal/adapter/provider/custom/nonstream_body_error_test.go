package custom

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
)

// mkNonStreamResp builds a minimal 200 JSON response for handleNonStreamResponse.
func mkNonStreamResp(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// drainEvents closes an event channel and collects every event that was sent.
func drainEvents(ch domain.AdapterEventChan) []*domain.AdapterEvent {
	ch.Close()
	var events []*domain.AdapterEvent
	for ev := range ch {
		events = append(events, ev)
	}
	return events
}

func TestNonStreamDetectsClaudeErrorBodyOn200(t *testing.T) {
	adapter := newTestCustomAdapter()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, req)
	ctx.Set(flow.KeyClientType, domain.ClientTypeClaude)
	eventChan := domain.NewAdapterEventChan()
	ctx.Set(flow.KeyEventChan, eventChan)

	resp := mkNonStreamResp(`{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`)

	err := adapter.handleNonStreamResponse(ctx, resp, domain.ClientTypeClaude, false)
	proxyErr, ok := err.(*domain.ProxyError)
	if !ok {
		t.Fatalf("error = %T %v, want *domain.ProxyError", err, err)
	}
	if proxyErr.Scope != domain.ScopeProvider || proxyErr.Reason != domain.CooldownReasonServerError {
		t.Fatalf("scope/reason = %s/%s, want provider/server_error", proxyErr.Scope, proxyErr.Reason)
	}
	if proxyErr.Message != "Overloaded" {
		t.Fatalf("message = %q, want %q", proxyErr.Message, "Overloaded")
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("error body was forwarded to client: %q", rec.Body.String())
	}
	// A 200-body error must NOT emit a success-shaped ResponseInfo event, which
	// would record the upstream error as a successful 200 response.
	for _, ev := range drainEvents(eventChan) {
		if ev.Type == domain.EventResponseInfo {
			t.Fatalf("error envelope emitted a ResponseInfo event: %+v", ev.ResponseInfo)
		}
	}
}

func TestNonStreamDetectsOpenAIErrorBodyOn200(t *testing.T) {
	adapter := newTestCustomAdapter()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, req)
	ctx.Set(flow.KeyClientType, domain.ClientTypeOpenAI)

	resp := mkNonStreamResp(`{"error":{"message":"insufficient upstream capacity","type":"server_error","code":500}}`)

	err := adapter.handleNonStreamResponse(ctx, resp, domain.ClientTypeOpenAI, false)
	proxyErr, ok := err.(*domain.ProxyError)
	if !ok {
		t.Fatalf("error = %T %v, want *domain.ProxyError", err, err)
	}
	if proxyErr.Message != "insufficient upstream capacity" {
		t.Fatalf("message = %q", proxyErr.Message)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("error body was forwarded to client: %q", rec.Body.String())
	}
}

func TestNonStreamForwardsNormalBody(t *testing.T) {
	adapter := newTestCustomAdapter()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, req)
	ctx.Set(flow.KeyClientType, domain.ClientTypeOpenAI)
	eventChan := domain.NewAdapterEventChan()
	ctx.Set(flow.KeyEventChan, eventChan)

	// A normal success body that carries a null "error" field must NOT be
	// treated as an error.
	success := `{"id":"cmpl-1","object":"chat.completion","error":null,"choices":[{"message":{"role":"assistant","content":"hi"}}]}`
	resp := mkNonStreamResp(success)

	if err := adapter.handleNonStreamResponse(ctx, resp, domain.ClientTypeOpenAI, false); err != nil {
		t.Fatalf("handleNonStreamResponse error = %v, want nil", err)
	}
	if !strings.Contains(rec.Body.String(), `"content":"hi"`) {
		t.Fatalf("normal body was not forwarded: %q", rec.Body.String())
	}
	// The success path must still emit a ResponseInfo event (the reorder only
	// suppresses it for error envelopes).
	sawResponseInfo := false
	for _, ev := range drainEvents(eventChan) {
		if ev.Type == domain.EventResponseInfo {
			sawResponseInfo = true
		}
	}
	if !sawResponseInfo {
		t.Fatal("normal body did not emit a ResponseInfo event")
	}
}

// mkStreamResp builds a 200 response whose body is not necessarily SSE.
func mkStreamResp(contentType, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// A stream request can get HTTP 200 with a plain JSON error body (no SSE
// `data:` framing). That must be detected and fail before anything is written
// to the client, so retry/cooldown can engage.
func TestStreamDetectsPlainJSONErrorBodyOn200(t *testing.T) {
	adapter := newTestCustomAdapter()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, req)
	ctx.Set(flow.KeyClientType, domain.ClientTypeOpenAI)
	eventChan := domain.NewAdapterEventChan()
	ctx.Set(flow.KeyEventChan, eventChan)

	resp := mkStreamResp("application/json", `{"error":{"message":"upstream boom","type":"server_error","code":500}}`)

	err := adapter.handleStreamResponse(ctx, resp, domain.ClientTypeOpenAI, false)
	proxyErr, ok := err.(*domain.ProxyError)
	if !ok {
		t.Fatalf("error = %T %v, want *domain.ProxyError", err, err)
	}
	if proxyErr.Message != "upstream boom" {
		t.Fatalf("message = %q, want %q", proxyErr.Message, "upstream boom")
	}
	if proxyErr.Scope != domain.ScopeProvider {
		t.Fatalf("scope = %s, want provider", proxyErr.Scope)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("error body was forwarded to client: %q", rec.Body.String())
	}
	drainEvents(eventChan)
}

// A non-error plain JSON body on a stream request is forwarded through
// unchanged (no false positive from the sniff).
func TestStreamForwardsPlainJSONNonErrorBodyOn200(t *testing.T) {
	adapter := newTestCustomAdapter()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, req)
	ctx.Set(flow.KeyClientType, domain.ClientTypeOpenAI)
	eventChan := domain.NewAdapterEventChan()
	ctx.Set(flow.KeyEventChan, eventChan)

	body := `{"id":"cmpl-1","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"hi"}}]}`
	resp := mkStreamResp("application/json", body)

	if err := adapter.handleStreamResponse(ctx, resp, domain.ClientTypeOpenAI, false); err != nil {
		t.Fatalf("handleStreamResponse error = %v, want nil", err)
	}
	if !strings.Contains(rec.Body.String(), `"content":"hi"`) {
		t.Fatalf("non-error JSON body was not forwarded: %q", rec.Body.String())
	}
	drainEvents(eventChan)
}

// A genuine SSE stream (frames begin with `data:`) is untouched by the sniff.
func TestStreamSSEUnaffectedBySniff(t *testing.T) {
	adapter := newTestCustomAdapter()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, req)
	ctx.Set(flow.KeyClientType, domain.ClientTypeOpenAI)
	eventChan := domain.NewAdapterEventChan()
	ctx.Set(flow.KeyEventChan, eventChan)

	resp := mkStreamResp("text/event-stream", "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n")

	if err := adapter.handleStreamResponse(ctx, resp, domain.ClientTypeOpenAI, false); err != nil {
		t.Fatalf("handleStreamResponse error = %v, want nil", err)
	}
	if !strings.Contains(rec.Body.String(), `content":"ok`) || !strings.Contains(rec.Body.String(), "[DONE]") {
		t.Fatalf("SSE stream not forwarded intact: %q", rec.Body.String())
	}
	drainEvents(eventChan)
}
