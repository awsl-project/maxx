package privacy

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/privacy/redact"
)

// ResponseWriter restores only values sent in this exchange. The archive/capture
// stays upstream-visible; this writer belongs at the client-facing boundary.
type ResponseWriter struct {
	http.ResponseWriter
	writer   *redact.Writer
	target   *restoreTarget
	exchange *Exchange
	finished bool
}

type restoreTarget struct {
	http.ResponseWriter
	err error
}

func (t *restoreTarget) Write(p []byte) (int, error) {
	if t.err != nil {
		return 0, t.err
	}
	n, err := t.ResponseWriter.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err != nil {
		t.err = err
	}
	return n, err
}
func (t *restoreTarget) Flush() {
	if f, ok := t.ResponseWriter.(http.Flusher); ok && t.err == nil {
		f.Flush()
	}
}
func (t *restoreTarget) Unwrap() http.ResponseWriter { return t.ResponseWriter }

func (e *Exchange) WrapResponse(w http.ResponseWriter) *ResponseWriter {
	if !e.HasMappings() {
		return nil
	}
	target := &restoreTarget{ResponseWriter: w}
	return &ResponseWriter{ResponseWriter: w, writer: e.RestoreState().NewWriter(target), target: target, exchange: e}
}
func (w *ResponseWriter) Header() http.Header         { return w.writer.Header() }
func (w *ResponseWriter) WriteHeader(status int)      { w.writer.WriteHeader(status) }
func (w *ResponseWriter) Write(p []byte) (int, error) { return w.writer.Write(p) }
func (w *ResponseWriter) Flush()                      { w.writer.Flush() }
func (w *ResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *ResponseWriter) Finish() error {
	if w == nil {
		return nil
	}
	if w.finished {
		return w.target.err
	}
	w.finished = true
	w.writer.Finish()
	w.exchange.addRestored(w.writer.Put())
	return w.target.err
}

// The same SSE restorer handles JSON WebSocket deltas, including placeholders
// split across frames. Protocol IDs and signed/opaque fields retain upstream's
// exclusions. A turn never shares its restore map with another request.
type FrameRestorer struct {
	finished bool
	writer   *redact.Writer
	target   *frameTarget
	exchange *Exchange
}
type frameTarget struct {
	sink   domain.ResponsesWebSocketFrameSink
	header http.Header
	buf    bytes.Buffer
	err    error
}

func (t *frameTarget) Header() http.Header { return t.header }
func (t *frameTarget) WriteHeader(int)     {}
func (t *frameTarget) Write(p []byte) (int, error) {
	if t.err != nil {
		return 0, t.err
	}
	t.buf.Write(p)
	for {
		b := t.buf.Bytes()
		at := bytes.Index(b, []byte("\n\n"))
		if at < 0 {
			break
		}
		event := append([]byte(nil), b[:at]...)
		t.buf.Next(at + 2)
		for _, line := range bytes.Split(event, []byte("\n")) {
			if !bytes.HasPrefix(line, []byte("data:")) {
				continue
			}
			payload := bytes.TrimSpace(line[5:])
			if len(payload) == 0 {
				continue
			}
			if err := t.sink.WriteTextFrame(payload); err != nil {
				t.err = err
				return len(p), err
			}
		}
	}
	return len(p), nil
}
func (e *Exchange) WrapFrames(sink domain.ResponsesWebSocketFrameSink) *FrameRestorer {
	if !e.HasMappings() || sink == nil {
		return nil
	}
	t := &frameTarget{sink: sink, header: http.Header{"Content-Type": []string{"text/event-stream"}}}
	return &FrameRestorer{writer: e.RestoreState().NewWriter(t), target: t, exchange: e}
}
func (f *FrameRestorer) WriteTextFrame(payload []byte) error {
	var compact bytes.Buffer
	if err := compactJSON(&compact, payload); err != nil {
		return err
	}
	_, err := f.writer.Write(append(append([]byte("data: "), compact.Bytes()...), []byte("\n\n")...))
	return err
}
func (f *FrameRestorer) Finish() error {
	if f == nil {
		return nil
	}
	if f.finished {
		return f.target.err
	}
	f.finished = true
	f.writer.Finish()
	f.exchange.addRestored(f.writer.Put())
	return f.target.err
}

// ClientWriteError keeps downstream transport failures request-scoped. They must
// not cause provider cooldown or replay an already committed response.
func ClientWriteError(err error) error {
	if err == nil {
		return nil
	}
	proxyErr := domain.NewProxyErrorWithMessage(err, false, "client response write failed")
	proxyErr.Scope = domain.ScopeRequest
	proxyErr.HTTPStatusCode = 499
	proxyErr.Code = "client_disconnected"
	return proxyErr
}

func compactJSON(dst *bytes.Buffer, payload []byte) error {
	if !strings.HasPrefix(strings.TrimSpace(string(payload)), "{") {
		return errors.New("invalid websocket response frame")
	}
	return json.Compact(dst, payload)
}
