package custom

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
)

type terminalUnexpectedEOFReadCloser struct {
	data []byte
	done bool
}

func (r *terminalUnexpectedEOFReadCloser) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	return copy(p, r.data), io.ErrUnexpectedEOF
}

func (r *terminalUnexpectedEOFReadCloser) Close() error { return nil }

type oneChunkThenBlockReadCloser struct {
	data []byte
	done bool
	stop chan struct{}
}

func (r *oneChunkThenBlockReadCloser) Read(p []byte) (int, error) {
	if !r.done {
		r.done = true
		return copy(p, r.data), nil
	}
	<-r.stop
	return 0, io.EOF
}

func (r *oneChunkThenBlockReadCloser) Close() error {
	select {
	case <-r.stop:
	default:
		close(r.stop)
	}
	return nil
}

func TestCustomAdapterStreamTimesOutIdleAfterFirstEvent(t *testing.T) {
	adapter := newTestCustomAdapter()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, req)
	ctx.Set(flow.KeyClientType, domain.ClientTypeOpenAI)
	ctx.Set("stream_idle_timeout", 10*time.Millisecond)
	ctx.Set("stream_first_event_timeout", time.Second)

	body := &oneChunkThenBlockReadCloser{
		data: []byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"),
		stop: make(chan struct{}),
	}
	defer body.Close()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       body,
	}

	err := adapter.handleStreamResponse(ctx, resp, domain.ClientTypeOpenAI, false)
	proxyErr, ok := err.(*domain.ProxyError)
	if !ok {
		t.Fatalf("handleStreamResponse error = %T %v, want *domain.ProxyError", err, err)
	}
	if !errors.Is(proxyErr.Err, domain.ErrStreamIdleTimeout) {
		t.Fatalf("proxyErr.Err = %v, want ErrStreamIdleTimeout", proxyErr.Err)
	}
	if proxyErr.Retryable {
		t.Fatal("idle timeout after first event must not retry a committed stream")
	}
	if proxyErr.HTTPStatusCode != http.StatusGatewayTimeout {
		t.Fatalf("HTTPStatusCode = %d, want 504", proxyErr.HTTPStatusCode)
	}
	if body := rec.Body.String(); !strings.Contains(body, "content\":\"ok") {
		t.Fatalf("completed line was not forwarded before timeout: %q", body)
	}
}

func TestCustomAdapterStreamDoesNotFailDoneWithoutTrailingNewline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]")
	}))
	defer server.Close()

	adapter, err := NewAdapter(&domain.Provider{
		Name:                 "test-custom",
		Type:                 "custom",
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeOpenAI},
		Config: &domain.ProviderConfig{
			Custom: &domain.ProviderConfigCustom{
				BaseURL: server.URL,
				APIKey:  "sk-test",
			},
		},
	})
	if err != nil {
		t.Fatalf("NewAdapter error: %v", err)
	}

	body := []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"hello"}],"stream":true}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, req)
	ctx.Set(flow.KeyClientType, domain.ClientTypeOpenAI)
	ctx.Set(flow.KeyOriginalClientType, domain.ClientTypeOpenAI)
	ctx.Set(flow.KeyRequestHeaders, req.Header.Clone())
	ctx.Set(flow.KeyRequestURI, "/v1/chat/completions")
	ctx.Set(flow.KeyRequestBody, body)

	if err := adapter.Execute(ctx, &domain.Provider{}); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if body := rec.Body.String(); !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("stream body missing DONE event: %q", body)
	}
}

func TestCustomAdapterStreamDoesNotFailUnexpectedEOFAfterDone(t *testing.T) {
	adapter := newTestCustomAdapter()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, req)
	ctx.Set(flow.KeyClientType, domain.ClientTypeOpenAI)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: &terminalUnexpectedEOFReadCloser{data: []byte(
			"data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]",
		)},
	}

	if err := adapter.handleStreamResponse(ctx, resp, domain.ClientTypeOpenAI, false); err != nil {
		t.Fatalf("handleStreamResponse error: %v", err)
	}
	if body := rec.Body.String(); !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("stream body missing DONE event: %q", body)
	}
}

func TestCustomAdapterStreamDoesNotFlushPartialLineOnUnexpectedEOF(t *testing.T) {
	adapter := newTestCustomAdapter()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, req)
	ctx.Set(flow.KeyClientType, domain.ClientTypeOpenAI)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: &terminalUnexpectedEOFReadCloser{data: []byte(
			"data: {\"choices\":[{\"delta\":{\"content\":\"partial",
		)},
	}

	if err := adapter.handleStreamResponse(ctx, resp, domain.ClientTypeOpenAI, false); err == nil {
		t.Fatal("handleStreamResponse error = nil, want unexpected EOF error")
	}
	if body := rec.Body.String(); body != "" {
		t.Fatalf("partial line was forwarded: %q", body)
	}
}

func TestCustomAdapterStreamClassifiesUnexpectedEOFAfterResponseStarted(t *testing.T) {
	adapter := newTestCustomAdapter()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, req)
	ctx.Set(flow.KeyClientType, domain.ClientTypeOpenAI)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: &terminalUnexpectedEOFReadCloser{data: []byte(
			"data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"partial",
		)},
	}

	err := adapter.handleStreamResponse(ctx, resp, domain.ClientTypeOpenAI, false)
	proxyErr, ok := err.(*domain.ProxyError)
	if !ok {
		t.Fatalf("handleStreamResponse error = %T %v, want *domain.ProxyError", err, err)
	}
	if proxyErr.Scope != domain.ScopeProvider || proxyErr.Reason != domain.CooldownReasonNetworkError {
		t.Fatalf("proxyErr scope/reason = %s/%s", proxyErr.Scope, proxyErr.Reason)
	}
	if proxyErr.Message != "upstream stream read error after response started" {
		t.Fatalf("proxyErr message = %q", proxyErr.Message)
	}
	if body := rec.Body.String(); !strings.Contains(body, "content\":\"ok") {
		t.Fatalf("completed line was not forwarded before EOF: %q", body)
	}
	if strings.Contains(rec.Body.String(), "partial") {
		t.Fatalf("partial line was forwarded: %q", rec.Body.String())
	}
}

func newTestCustomAdapter() *CustomAdapter {
	return &CustomAdapter{provider: &domain.Provider{
		Type: "custom",
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL: "http://upstream.test",
		}},
	}}
}
