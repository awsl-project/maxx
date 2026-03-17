package handler

import (
	"bufio"
	"net"
	"net/http"
)

type responseStateWriter struct {
	http.ResponseWriter
	started bool
}

func newResponseStateWriter(w http.ResponseWriter) http.ResponseWriter {
	if _, ok := w.(*responseStateWriter); ok {
		return w
	}
	return &responseStateWriter{ResponseWriter: w}
}

func (w *responseStateWriter) WriteHeader(statusCode int) {
	w.started = true
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseStateWriter) Write(b []byte) (int, error) {
	w.started = true
	return w.ResponseWriter.Write(b)
}

func (w *responseStateWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		w.started = true
		flusher.Flush()
	}
}

func (w *responseStateWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	w.started = true
	return hijacker.Hijack()
}

func (w *responseStateWriter) Push(target string, opts *http.PushOptions) error {
	pusher, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, opts)
}

func (w *responseStateWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func responseHasStarted(w http.ResponseWriter) bool {
	state, ok := w.(*responseStateWriter)
	return ok && state.started
}
