package redact

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The writer counts each place it put a value back once: a placeholder a
// stream splits across events is one place, and a whole answer's places
// are each counted.
func TestWriterPutCounts(t *testing.T) {
	pw, _ := Mask(openaiKey, Options{Secrets: true})
	event := func(s string) string {
		b, _ := json.Marshal(map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": s}})
		return "event: content_block_delta\ndata: " + string(b) + "\n\n"
	}
	all := event("use "+pw[:5]) + event(pw[5:12]) + event(pw[12:]+" and ") + event(pw+".") + "data: [DONE]\n\n"
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "text/event-stream")
	w := NewWriter(rec)
	w.WriteHeader(200)
	for i := 0; i < len(all); i += 7 {
		w.Write([]byte(all[i:min(i+7, len(all))]))
	}
	w.Finish()
	if !strings.Contains(rec.Body.String(), openaiKey) {
		t.Fatalf("not put back: %s", rec.Body.String())
	}
	if got := w.Put(); len(got) != 1 || got[pw] != 2 {
		t.Fatalf("stream put %v, want %s twice", got, pw)
	}

	b, _ := json.Marshal(map[string]any{"content": []any{map[string]any{"type": "text", "text": pw + " " + pw + " " + pw}}})
	rec = httptest.NewRecorder()
	rec.Header().Set("Content-Type", "application/json")
	w = NewWriter(rec)
	w.WriteHeader(http.StatusOK)
	w.Write(b)
	w.Finish()
	if got := w.Put(); got[pw] != 3 {
		t.Fatalf("whole put %v, want 3", got)
	}

	// nothing to put back: nothing counted
	rec = httptest.NewRecorder()
	rec.Header().Set("Content-Type", "application/json")
	w = NewWriter(rec)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"content":"plain"}`))
	w.Finish()
	if got := w.Put(); len(got) != 0 {
		t.Fatalf("plain put %v", got)
	}
}
