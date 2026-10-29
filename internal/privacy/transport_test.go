package privacy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/privacy/redact"
)

func maskedExchange(t *testing.T, text string) *Exchange {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Words = []string{text}
	svc := configuredService(t, cfg)
	body := TextRequest(text)
	e, rejection := svc.Prepare(context.Background(), 1, body, body, "application/json")
	if rejection != nil {
		t.Fatal(rejection)
	}
	return e
}

func TestUnusedResponseRestorerDoesNotCommitAttempt(t *testing.T) {
	e := maskedExchange(t, "Nightjar")
	target := httptest.NewRecorder()
	writer := e.WrapResponse(target)
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	if target.Flushed || target.Body.Len() != 0 {
		t.Fatal("unused restorer committed a response before retry/error handling")
	}
	target.WriteHeader(http.StatusBadGateway)
	if target.Code != http.StatusBadGateway {
		t.Fatalf("error status was replaced: %d", target.Code)
	}
}
func TestResponseRestorationEverySplitAndRequestIsolation(t *testing.T) {
	e := maskedExchange(t, "Nightjar")
	placeholder := e.Hits[0].Placeholder
	for at := 1; at < len(placeholder); at++ {
		t.Run(fmt.Sprint(at), func(t *testing.T) {
			target := httptest.NewRecorder()
			target.Header().Set("Content-Type", "text/event-stream")
			writer := e.WrapResponse(target)
			fmt.Fprintf(writer, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":%q}}]}\n\n", placeholder[:at])
			writer.Flush()
			fmt.Fprintf(writer, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":%q}}]}\n\ndata: [DONE]\n\n", placeholder[at:])
			if err := writer.Finish(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(target.Body.String(), "Nightjar") || strings.Contains(target.Body.String(), placeholder) || !strings.Contains(target.Body.String(), "[DONE]") {
				t.Fatal(target.Body.String())
			}
		})
	}
	other := e.scope.state.Mask
	old, _ := other("other-private-value-123", redact.Options{Secrets: true, Words: []string{"other-private-value-123"}})
	if restored := e.RestoreState().Restore(old, false); restored != old {
		t.Fatal("response could access another request's mapping")
	}
	target := httptest.NewRecorder()
	target.Header().Set("Content-Type", "application/json")
	target.Header().Set("Content-Length", "1")
	writer := e.WrapResponse(target)
	writer.WriteHeader(200)
	fmt.Fprintf(writer, `{"content":%q}`, placeholder)
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	if target.Header().Get("Content-Length") != "" || !strings.Contains(target.Body.String(), "Nightjar") {
		t.Fatal("whole-body response was not restored")
	}
}

type failedPrivacyWriter struct{ header http.Header }

func (w *failedPrivacyWriter) Header() http.Header     { return w.header }
func (*failedPrivacyWriter) WriteHeader(int)           {}
func (*failedPrivacyWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestRestorationPreservesWriteFailuresAndBinary(t *testing.T) {
	e := maskedExchange(t, "Nightjar")
	w := e.WrapResponse(&failedPrivacyWriter{header: http.Header{"Content-Type": []string{"application/json"}}})
	w.Write([]byte(`{"content":"` + e.Hits[0].Placeholder + `"}`))
	err := w.Finish()
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	var proxy *domain.ProxyError
	if !errors.As(ClientWriteError(err), &proxy) || proxy.Scope != domain.ScopeRequest || proxy.Retryable {
		t.Fatal("client failure was provider-scoped")
	}
	for _, kind := range []string{"image/png", "audio/mpeg", "application/octet-stream"} {
		target := httptest.NewRecorder()
		target.Header().Set("Content-Type", kind)
		w := e.WrapResponse(target)
		payload := []byte("binary\x00" + e.Hits[0].Placeholder)
		w.Write(payload)
		if !bytes.Equal(target.Body.Bytes(), payload) {
			t.Fatal("binary response was buffered or restored")
		}
		w.Finish()
	}
}

type privacyFrames struct {
	frames [][]byte
	fail   bool
}

func (f *privacyFrames) WriteTextFrame(p []byte) error {
	if f.fail {
		return io.ErrClosedPipe
	}
	f.frames = append(f.frames, bytes.Clone(p))
	return nil
}
func TestWebSocketRestorationSplitFrames(t *testing.T) {
	e := maskedExchange(t, "Nightjar")
	p := e.Hits[0].Placeholder
	for at := 1; at < len(p); at++ {
		sink := &privacyFrames{}
		w := e.WrapFrames(sink)
		for _, delta := range []string{p[:at], p[at:]} {
			if err := w.WriteTextFrame([]byte(fmt.Sprintf(`{"type":"response.output_text.delta","item_id":"item-1","output_index":0,"content_index":0,"delta":%q}`, delta))); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.WriteTextFrame([]byte(`{"type":"response.completed","response":{"status":"completed"}}`)); err != nil {
			t.Fatal(err)
		}
		if err := w.Finish(); err != nil {
			t.Fatal(err)
		}
		joined := string(bytes.Join(sink.frames, nil))
		if !strings.Contains(joined, "Nightjar") || strings.Contains(joined, p) || !strings.Contains(joined, "response.completed") {
			t.Fatal(joined)
		}
	}
	sink := &privacyFrames{fail: true}
	w := e.WrapFrames(sink)
	w.WriteTextFrame([]byte(`{"type":"response.completed"}`))
	if !errors.Is(w.Finish(), io.ErrClosedPipe) {
		t.Fatal("sink failure lost")
	}
}
func TestRecordsAreScopedMetadataOnlyAndPreviewIsEphemeral(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Personal = true
	cfg.Words = []string{"Nightjar"}
	svc := configuredService(t, cfg)
	directory := t.TempDir()
	svc.SetDirectory(directory)
	body := TextRequest("Nightjar email person@corp-real.com")
	e, rejection := svc.Prepare(context.Background(), 1, body, body, "application/json")
	if rejection != nil {
		t.Fatal(rejection)
	}
	req := &domain.ProxyRequest{ID: 7, RequestID: "record-7", Status: "COMPLETED", ProviderID: 9}
	e.BeginRecord(req)
	e.addRestored(map[string]int{e.Hits[0].Placeholder: 1})
	e.FinishRecord(req)
	records, err := svc.Records(1, RecordsQuery{})
	if err != nil || records.Total != 1 || records.Items[0].ProviderID != 9 {
		t.Fatalf("%+v %v", records, err)
	}
	value, err := svc.Original(1, req.RequestID, e.Hits[0].Placeholder)
	if err != nil || value == "" {
		t.Fatal(err)
	}
	if _, err := svc.Original(2, req.RequestID, e.Hits[0].Placeholder); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("cross-tenant original lookup")
	}
	if _, err := svc.Sent(1, req.RequestID); err != nil {
		t.Fatal(err)
	}
	filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			data, _ := os.ReadFile(path)
			if strings.Contains(string(data), "Nightjar") || strings.Contains(string(data), "person@corp-real.com") {
				t.Errorf("original on disk: %s", path)
			}
		}
		return err
	})
	restarted := NewService(svc.settings)
	restarted.SetDirectory(directory)
	page, err := restarted.Records(1, RecordsQuery{Kind: "TERM"})
	if err != nil || page.Total != 1 {
		t.Fatalf("persisted metadata missing: %+v %v", page, err)
	}
	if _, err := restarted.Original(1, req.RequestID, e.Hits[0].Placeholder); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("original persisted across restart")
	}
	before := len(svc.runtime.scopes)
	preview, rejection := svc.Preview(context.Background(), cfg, TextRequest("Nightjar"))
	if rejection != nil || len(preview.Hits) == 0 || len(svc.runtime.scopes) != before {
		t.Fatal("preview failed or retained a new scope")
	}
	if page, _ := svc.Records(1, RecordsQuery{}); page.Total != 1 {
		t.Fatal("preview recorded history")
	}
	if err := svc.ClearRecords(1); err != nil {
		t.Fatal(err)
	}
	if page, _ := svc.Records(1, RecordsQuery{}); page.Total != 0 {
		t.Fatal("clear left records")
	}
	if _, err := svc.Original(1, req.RequestID, e.Hits[0].Placeholder); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("clear retained originals")
	}
}
func TestAdvancedArchiveRemovesWarnedModelWordsAndSplitPII(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Personal = true
	cfg.Mode = "warn"
	cfg.Words = []string{"Nightjar"}
	cfg.Model.Enabled = true
	svc := configuredService(t, cfg)
	calls := 0
	svc.runtime.find = func(_ context.Context, _ string, texts []string) ([][]redact.Found, error) {
		calls++
		out := make([][]redact.Found, len(texts))
		for i, text := range texts {
			at := strings.Index(text, "Lina Kowalski")
			if at >= 0 {
				out[i] = []redact.Found{{Start: at, End: at + len("Lina Kowalski"), Label: "private_person", Score: 0.99}}
			}
		}
		return out, nil
	}
	body := TextRequest("Nightjar Lina Kowalski person@corp-real.com")
	e, rejection := svc.Prepare(context.Background(), 1, body, body, "application/json")
	if rejection != nil {
		t.Fatal(rejection)
	}
	snapshot := calls
	for _, value := range []string{string(body), "https://corp-real.com/Nightjar", `{"text":"Lina Kowalski"}`, "data: {\"delta\":\"Night\"}\n\ndata: {\"delta\":\"jar\"}\n\n"} {
		filtered := e.Archive(value)
		if strings.Contains(filtered, "Nightjar") || strings.Contains(filtered, "Lina Kowalski") || strings.Contains(filtered, "person@corp-real.com") {
			t.Fatal(filtered)
		}
	}
	if calls != snapshot {
		t.Fatal("archive write ran model inference")
	}
	if !bytes.Equal(e.Body, body) {
		t.Fatal("warn mode modified original request")
	}
}

func TestResponseFinishIsIdempotent(t *testing.T) {
	e := maskedExchange(t, "Nightjar")
	target := httptest.NewRecorder()
	target.Header().Set("Content-Type", "application/json")
	writer := e.WrapResponse(target)
	fmt.Fprintf(writer, `{"text":%q}`, e.Hits[0].Placeholder)
	writer.Finish()
	writer.Finish()
	if e.restored[e.Hits[0].Placeholder] != 1 {
		t.Fatal("Finish counted a restoration twice")
	}
}

func TestClearRecordsFencesInFlightAfterScopeEviction(t *testing.T) {
	e := maskedExchange(t, "Nightjar")
	req := &domain.ProxyRequest{ID: 1, RequestID: "inflight", Status: "COMPLETED"}
	e.BeginRecord(req)
	// Other tenant scopes evict the original learned-value cache, while active
	// exchanges retain their pointer until the administrator clears records.
	svc := &Service{settings: &memoryPrivacySettings{}, runtime: e.runtime}
	for i := uint64(2); i < 20; i++ {
		e.runtime.scope(scopeKey{i})
	}
	if err := svc.ClearRecords(1); err != nil {
		t.Fatal(err)
	}
	e.FinishRecord(req)
	page, err := svc.Records(1, RecordsQuery{})
	if err != nil || page.Total != 0 {
		t.Fatal("evicted in-flight record reappeared after clearing")
	}
}
