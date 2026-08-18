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
