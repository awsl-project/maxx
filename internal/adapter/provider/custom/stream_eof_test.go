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

func newTestCustomAdapter() *CustomAdapter {
	return &CustomAdapter{provider: &domain.Provider{
		Type: "custom",
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL: "http://upstream.test",
		}},
	}}
}
