package handler

import (
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
