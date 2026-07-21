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

type ResponsesWebSocketAttemptError struct {
	Err error

	SafeToTryNextProvider       bool
	RequestFrameMayHaveBeenSent bool
	FirstEventReceived          bool
	ClientEventSent             bool
	TerminalErrorEventSent      bool
	CapabilityFailure           bool
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

func (e *ResponsesWebSocketAttemptError) CanTryNextProvider() bool {
	return e != nil && e.SafeToTryNextProvider &&
		!e.RequestFrameMayHaveBeenSent && !e.FirstEventReceived && !e.ClientEventSent
}
