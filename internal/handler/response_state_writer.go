package handler

import "net/http"

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
		flusher.Flush()
	}
}

func responseHasStarted(w http.ResponseWriter) bool {
	state, ok := w.(*responseStateWriter)
	return ok && state.started
}
