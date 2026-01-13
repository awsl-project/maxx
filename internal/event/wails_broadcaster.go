package event

import (
	"context"
	"sync"

	"github.com/Bowl42/maxx/internal/domain"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// WailsBroadcaster wraps an existing Broadcaster and adds Wails event emission
// This allows events to be broadcast both via WebSocket (for HTTP clients)
// and via Wails Events (for desktop clients)
type WailsBroadcaster struct {
	inner Broadcaster
	ctx   context.Context
	mu    sync.RWMutex
}

// NewWailsBroadcaster creates a new WailsBroadcaster wrapping the given broadcaster
func NewWailsBroadcaster(inner Broadcaster) *WailsBroadcaster {
	return &WailsBroadcaster{
		inner: inner,
	}
}

// SetContext sets the Wails context for event emission
// This should be called from DesktopApp.Startup() once the context is available
func (w *WailsBroadcaster) SetContext(ctx context.Context) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ctx = ctx
}

// emitWailsEvent emits an event via Wails runtime if context is available
func (w *WailsBroadcaster) emitWailsEvent(eventType string, data interface{}) {
	w.mu.RLock()
	ctx := w.ctx
	w.mu.RUnlock()

	if ctx != nil {
		runtime.EventsEmit(ctx, eventType, data)
	}
}

// BroadcastProxyRequest broadcasts a proxy request update
func (w *WailsBroadcaster) BroadcastProxyRequest(req *domain.ProxyRequest) {
	// Broadcast via inner broadcaster (WebSocket)
	if w.inner != nil {
		w.inner.BroadcastProxyRequest(req)
	}
	// Also emit via Wails Events
	w.emitWailsEvent("proxy_request_update", req)
}

// BroadcastProxyUpstreamAttempt broadcasts a proxy upstream attempt update
func (w *WailsBroadcaster) BroadcastProxyUpstreamAttempt(attempt *domain.ProxyUpstreamAttempt) {
	if w.inner != nil {
		w.inner.BroadcastProxyUpstreamAttempt(attempt)
	}
	w.emitWailsEvent("proxy_upstream_attempt_update", attempt)
}

// BroadcastLog broadcasts a log message
func (w *WailsBroadcaster) BroadcastLog(message string) {
	if w.inner != nil {
		w.inner.BroadcastLog(message)
	}
	w.emitWailsEvent("log_message", message)
}

// BroadcastStats broadcasts stats update
func (w *WailsBroadcaster) BroadcastStats(stats interface{}) {
	if w.inner != nil {
		w.inner.BroadcastStats(stats)
	}
	w.emitWailsEvent("stats_update", stats)
}

// BroadcastMessage broadcasts a custom message
func (w *WailsBroadcaster) BroadcastMessage(messageType string, data interface{}) {
	if w.inner != nil {
		w.inner.BroadcastMessage(messageType, data)
	}
	w.emitWailsEvent(messageType, data)
}
