package responsemodifier

import (
	"bytes"
	"net/http"
	"strconv"

	"github.com/awsl-project/maxx/internal/domain"
)

type responseModifier interface {
	modifyBody(body []byte) []byte
	modifyStreamEvent(event []byte) []byte
}

// ResponseModifierWriter buffers a response and applies provider-specific response modifications before sending it.
type ResponseModifierWriter struct {
	underlying  http.ResponseWriter
	modifier    responseModifier
	isStream    bool
	statusCode  int
	buffer      bytes.Buffer
	headersSent bool
}

func NewResponseModifierWriter(w http.ResponseWriter, provider *domain.Provider, clientType domain.ClientType, isStream bool) *ResponseModifierWriter {
	modifier := newResponseModifier(provider, clientType)
	if modifier == nil {
		return nil
	}
	return &ResponseModifierWriter{underlying: w, modifier: modifier, isStream: isStream, statusCode: http.StatusOK}
}

func newResponseModifier(provider *domain.Provider, clientType domain.ClientType) responseModifier {
	if provider == nil {
		return nil
	}
	modifier := newClaudeResponseModifier(provider, clientType)
	if modifier == nil {
		return nil
	}
	return modifier
}

func (w *ResponseModifierWriter) Header() http.Header {
	return w.underlying.Header()
}

func (w *ResponseModifierWriter) WriteHeader(code int) {
	w.statusCode = code
	if w.isStream {
		w.writeHeaderIfNeeded()
	}
}

func (w *ResponseModifierWriter) Write(b []byte) (int, error) {
	if !w.isStream {
		_, err := w.buffer.Write(b)
		return len(b), err
	}
	if _, err := w.buffer.Write(b); err != nil {
		return 0, err
	}
	if err := w.flushCompleteStreamEvents(false); err != nil {
		return 0, err
	}
	return len(b), nil
}

func (w *ResponseModifierWriter) Flush() {
	if w.isStream {
		_ = w.flushCompleteStreamEvents(false)
	}
	if f, ok := w.underlying.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *ResponseModifierWriter) Finalize() error {
	if w.isStream {
		return w.flushCompleteStreamEvents(true)
	}
	body := w.modifier.modifyBody(w.buffer.Bytes())
	if w.underlying.Header().Get("Content-Length") != "" {
		w.underlying.Header().Set("Content-Length", strconv.Itoa(len(body)))
	}
	w.writeHeaderIfNeeded()
	_, err := w.underlying.Write(body)
	return err
}

func (w *ResponseModifierWriter) flushCompleteStreamEvents(final bool) error {
	for {
		eventLen := completeSSEEventLen(w.buffer.Bytes())
		if eventLen == 0 {
			break
		}
		if err := w.writeStreamEvent(w.buffer.Next(eventLen)); err != nil {
			return err
		}
	}
	if final && w.buffer.Len() > 0 {
		if err := w.writeStreamEvent(w.buffer.Next(w.buffer.Len())); err != nil {
			return err
		}
	}
	if final {
		w.writeHeaderIfNeeded()
	}
	return nil
}

func (w *ResponseModifierWriter) writeStreamEvent(event []byte) error {
	body := w.modifier.modifyStreamEvent(event)
	w.writeHeaderIfNeeded()
	_, err := w.underlying.Write(body)
	if err != nil {
		return err
	}
	if f, ok := w.underlying.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

func (w *ResponseModifierWriter) writeHeaderIfNeeded() {
	if w.headersSent {
		return
	}
	w.underlying.WriteHeader(w.statusCode)
	w.headersSent = true
}

func completeSSEEventLen(data []byte) int {
	lf := bytes.Index(data, []byte("\n\n"))
	crlf := bytes.Index(data, []byte("\r\n\r\n"))
	if lf == -1 && crlf == -1 {
		return 0
	}
	if lf == -1 || (crlf != -1 && crlf < lf) {
		return crlf + len("\r\n\r\n")
	}
	return lf + len("\n\n")
}
