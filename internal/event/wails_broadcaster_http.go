//go:build !desktop

package event

import (
	"context"
	"sync"

	"github.com/Bowl42/maxx/internal/domain"
)

// WailsBroadcaster wraps an existing Broadcaster
// In HTTP mode, this simply delegates to the inner broadcaster without Wails event emission
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

// SetContext is a no-op in HTTP mode (kept for API compatibility)
func (w *WailsBroadcaster) SetContext(ctx context.Context) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.ctx = ctx
}

// BroadcastProxyRequest broadcasts a proxy request update
func (w *WailsBroadcaster) BroadcastProxyRequest(req *domain.ProxyRequest) {
	if w.inner != nil {
		w.inner.BroadcastProxyRequest(req)
	}
}

// BroadcastProxyUpstreamAttempt broadcasts a proxy upstream attempt update
func (w *WailsBroadcaster) BroadcastProxyUpstreamAttempt(attempt *domain.ProxyUpstreamAttempt) {
	if w.inner != nil {
		w.inner.BroadcastProxyUpstreamAttempt(attempt)
	}
}

// BroadcastLog broadcasts a log message
func (w *WailsBroadcaster) BroadcastLog(message string) {
	if w.inner != nil {
		w.inner.BroadcastLog(message)
	}
}

// BroadcastStats broadcasts stats update
func (w *WailsBroadcaster) BroadcastStats(stats interface{}) {
	if w.inner != nil {
		w.inner.BroadcastStats(stats)
	}
}

// BroadcastMessage broadcasts a custom message
func (w *WailsBroadcaster) BroadcastMessage(messageType string, data interface{}) {
	if w.inner != nil {
		w.inner.BroadcastMessage(messageType, data)
	}
}
