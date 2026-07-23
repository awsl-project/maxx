package domain

// ResponsesWebSocketFrameSink writes upstream application frames to the
// downstream WebSocket. Implementations must serialize writes.
type ResponsesWebSocketFrameSink interface {
	WriteTextFrame(payload []byte) error
}

// ResponsesWebSocketExchange describes one response.create turn on a
// downstream WebSocket connection.
type ResponsesWebSocketExchange struct {
	ConnectionID       string
	Frame              []byte
	PreviousResponseID string
	PinnedProviderID   uint64
	Sink               ResponsesWebSocketFrameSink
	// TryAcquireProviderSlot is called only when an adapter creates a new
	// persistent upstream session. The returned release function belongs to
	// that session and must run exactly once when the session closes.
	TryAcquireProviderSlot func() (release func(), acquired bool)
}

// AcquireProviderSlot reserves a slot for a new persistent upstream session.
// A nil callback means the caller does not use provider concurrency limiting.
func (e *ResponsesWebSocketExchange) AcquireProviderSlot() (func(), bool) {
	if e == nil || e.TryAcquireProviderSlot == nil {
		return func() {}, true
	}
	return e.TryAcquireProviderSlot()
}

type ResponsesWebSocketResult struct {
	ProviderID uint64
	Reused     bool

	RequestFrameMayHaveBeenSent bool
	FirstEventReceived          bool
	ClientEventSent             bool
	TerminalErrorEventSent      bool

	TerminalEvent string
	ResponseModel string
}

// ResponsesWebSocketAttemptError describes a failed WS turn.
// CapabilityFailure distinguishes unsupported-WS endpoints from ordinary
// network/model errors for cooldown/unsupported-cache only — it must never
// switch providers.
type ResponsesWebSocketAttemptError struct {
	Err error

	RequestFrameMayHaveBeenSent bool
	FirstEventReceived          bool
	ClientEventSent             bool
	TerminalErrorEventSent      bool

	// CapabilityFailure is used only for cooldown/unsupported cache.
	// It must never trigger cross-provider fallback.
	CapabilityFailure bool
}

func (e *ResponsesWebSocketAttemptError) Error() string {
	if e == nil || e.Err == nil {
		return "responses websocket attempt failed"
	}
	return e.Err.Error()
}

func (e *ResponsesWebSocketAttemptError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
