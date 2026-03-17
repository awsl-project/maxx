package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
)

func TestWriteDispatchErrorSkipsWhenResponseAlreadyStarted(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := newResponseStateWriter(recorder)
	ctx := flow.NewCtx(writer, httptest.NewRequest("POST", "/v1/messages", nil))

	if _, err := ctx.Writer.Write([]byte("chunk-1")); err != nil {
		t.Fatalf("initial write failed: %v", err)
	}

	handler := &ProxyHandler{}
	proxyErr := domain.NewProxyErrorWithMessage(domain.ErrStreamIdleTimeout, false, "stream stalled")
	handler.writeDispatchError(ctx, proxyErr, true)

	if body := recorder.Body.String(); body != "chunk-1" {
		t.Fatalf("response body = %q, want original partial response only", body)
	}
}

func TestWriteProxyErrorUsesProxyHTTPStatusCode(t *testing.T) {
	recorder := httptest.NewRecorder()
	proxyErr := domain.NewProxyErrorWithMessage(domain.ErrFirstByteTimeout, true, "first token timeout")
	proxyErr.HTTPStatusCode = http.StatusGatewayTimeout

	writeProxyError(recorder, proxyErr)

	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusGatewayTimeout)
	}
}

func TestWriteStreamErrorUsesProxyHTTPStatusCodeBeforeStreamStarts(t *testing.T) {
	recorder := httptest.NewRecorder()
	proxyErr := domain.NewProxyErrorWithMessage(domain.ErrFirstByteTimeout, true, "first token timeout")
	proxyErr.HTTPStatusCode = http.StatusGatewayTimeout

	writeStreamError(recorder, proxyErr)

	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusGatewayTimeout)
	}
}
