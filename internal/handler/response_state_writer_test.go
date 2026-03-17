package handler

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResponseStateWriterFlushMarksStarted(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := newResponseStateWriter(recorder)

	flusher, ok := writer.(http.Flusher)
	if !ok {
		t.Fatalf("writer type = %T, want http.Flusher", writer)
	}

	flusher.Flush()

	if !responseHasStarted(writer) {
		t.Fatal("responseHasStarted = false, want true after Flush")
	}
}

func TestResponseStateWriterFlushWithoutUnderlyingFlusherDoesNotMarkStarted(t *testing.T) {
	base := &basicResponseWriter{header: make(http.Header)}
	writer := newResponseStateWriter(base)

	flusher, ok := writer.(http.Flusher)
	if !ok {
		t.Fatalf("writer type = %T, want http.Flusher wrapper", writer)
	}

	flusher.Flush()

	if responseHasStarted(writer) {
		t.Fatal("responseHasStarted = true, want false when underlying writer does not support Flush")
	}
}

func TestResponseStateWriterPreservesHijacker(t *testing.T) {
	base := &hijackableResponseWriter{ResponseRecorder: httptest.NewRecorder()}
	writer := newResponseStateWriter(base)

	hijacker, ok := writer.(http.Hijacker)
	if !ok {
		t.Fatalf("writer type = %T, want http.Hijacker", writer)
	}

	conn, rw, err := hijacker.Hijack()
	if err != nil {
		t.Fatalf("Hijack() error = %v, want nil", err)
	}
	if !base.hijacked {
		t.Fatal("base writer was not hijacked")
	}
	if conn == nil || rw == nil {
		t.Fatal("Hijack() returned nil values")
	}
	_ = conn.Close()
}

func TestResponseStateWriterPreservesPusher(t *testing.T) {
	base := &pushableResponseWriter{ResponseRecorder: httptest.NewRecorder()}
	writer := newResponseStateWriter(base)

	pusher, ok := writer.(http.Pusher)
	if !ok {
		t.Fatalf("writer type = %T, want http.Pusher", writer)
	}

	if err := pusher.Push("/assets/app.js", nil); err != nil {
		t.Fatalf("Push() error = %v, want nil", err)
	}
	if base.pushedTarget != "/assets/app.js" {
		t.Fatalf("pushed target = %q, want /assets/app.js", base.pushedTarget)
	}
}

func TestResponseStateWriterUnwrapsUnderlyingWriter(t *testing.T) {
	base := httptest.NewRecorder()
	writer := newResponseStateWriter(base)

	type unwrapper interface {
		Unwrap() http.ResponseWriter
	}
	u, ok := writer.(unwrapper)
	if !ok {
		t.Fatalf("writer type = %T, want Unwrap support", writer)
	}
	if u.Unwrap() != base {
		t.Fatalf("Unwrap() = %T, want original writer %T", u.Unwrap(), base)
	}
}

func TestResponseStateWriterHijackReturnsNotSupportedWhenUnavailable(t *testing.T) {
	writer := newResponseStateWriter(httptest.NewRecorder())

	hijacker, ok := writer.(http.Hijacker)
	if !ok {
		t.Fatalf("writer type = %T, want http.Hijacker wrapper", writer)
	}

	_, _, err := hijacker.Hijack()
	if !errors.Is(err, http.ErrNotSupported) {
		t.Fatalf("Hijack() error = %v, want http.ErrNotSupported", err)
	}
}

func TestResponseStateWriterPushReturnsNotSupportedWhenUnavailable(t *testing.T) {
	writer := newResponseStateWriter(httptest.NewRecorder())

	pusher, ok := writer.(http.Pusher)
	if !ok {
		t.Fatalf("writer type = %T, want http.Pusher wrapper", writer)
	}

	err := pusher.Push("/assets/app.js", nil)
	if !errors.Is(err, http.ErrNotSupported) {
		t.Fatalf("Push() error = %v, want http.ErrNotSupported", err)
	}
}

type hijackableResponseWriter struct {
	*httptest.ResponseRecorder
	hijacked bool
}

func (w *hijackableResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	server, client := net.Pipe()
	w.hijacked = true
	reader := bufio.NewReader(server)
	writer := bufio.NewWriter(server)
	_ = client.Close()
	return server, bufio.NewReadWriter(reader, writer), nil
}

type pushableResponseWriter struct {
	*httptest.ResponseRecorder
	pushedTarget string
}

func (w *pushableResponseWriter) Push(target string, _ *http.PushOptions) error {
	w.pushedTarget = target
	return nil
}

type basicResponseWriter struct {
	header http.Header
	status int
	body   []byte
}

func (w *basicResponseWriter) Header() http.Header {
	return w.header
}

func (w *basicResponseWriter) WriteHeader(statusCode int) {
	w.status = statusCode
}

func (w *basicResponseWriter) Write(b []byte) (int, error) {
	w.body = append(w.body, b...)
	return len(b), nil
}
