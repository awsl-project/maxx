package mockserver

import "encoding/json"

// MockHeader is the HTTP header name used to control mock server behavior.
const MockHeader = "X-Mock-Response"

// MockDirective controls how the mock server responds to a request.
// Sent as JSON in the X-Mock-Response request header.
type MockDirective struct {
	// Status is the HTTP status code to return. Default: 200.
	Status int `json:"status,omitempty"`

	// Delay delays the response by this duration (e.g. "2s", "500ms").
	Delay string `json:"delay,omitempty"`

	// Headers are additional response headers to set (e.g. {"Retry-After": "5"}).
	Headers map[string]string `json:"headers,omitempty"`

	// Body overrides the entire response body. If set, returned as-is.
	// If nil, the mock server generates a protocol-appropriate response.
	Body json.RawMessage `json:"body,omitempty"`

	// Stream controls SSE streaming behavior. If non-nil, response is streamed.
	Stream *MockStreamDirective `json:"stream,omitempty"`
}

// MockStreamDirective controls SSE streaming responses.
type MockStreamDirective struct {
	// Chunks is a list of SSE chunks to send sequentially.
	Chunks []MockStreamChunk `json:"chunks"`
}

// MockStreamChunk is a single chunk in a streaming response.
type MockStreamChunk struct {
	// Data is the JSON object to send as an SSE data event.
	Data json.RawMessage `json:"data,omitempty"`

	// Delay pauses before sending this chunk (e.g. "200ms").
	Delay string `json:"delay,omitempty"`

	// Error terminates the stream with an error at this point.
	Error *MockStreamError `json:"error,omitempty"`
}

// MockStreamError terminates a stream mid-way with an error.
type MockStreamError struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body,omitempty"`
}

// Protocol represents the detected API protocol.
type Protocol string

const (
	ProtocolClaude Protocol = "claude"
	ProtocolOpenAI Protocol = "openai"
	ProtocolGemini Protocol = "gemini"
	ProtocolCodex  Protocol = "codex"
)
