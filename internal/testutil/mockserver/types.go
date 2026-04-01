package mockserver

import "encoding/json"

// SessionHeader is sent in proxy requests so the mock server
// can look up per-(session, providerID) directives.
const SessionHeader = "X-Mock-Session"

// MockHeader is the legacy header for single-request directive control.
// If set, overrides any session-based directive lookup.
const MockHeader = "X-Mock-Response"

// ProviderPathPrefix is the URL path prefix used to identify providers.
// Provider baseURL should be set to "http://mock:port/p/{id}" so that
// requests arrive as /p/{id}/v1/chat/completions etc.
const ProviderPathPrefix = "/p/"

// MockDirective controls how the mock server responds to a request.
type MockDirective struct {
	Status  int               `json:"status,omitempty"`
	Delay   string            `json:"delay,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    json.RawMessage   `json:"body,omitempty"`
	Stream  *MockStreamDirective `json:"stream,omitempty"`
}

// MockStreamDirective controls SSE streaming responses.
type MockStreamDirective struct {
	Chunks []MockStreamChunk `json:"chunks"`
}

// MockStreamChunk is a single chunk in a streaming response.
type MockStreamChunk struct {
	Data  json.RawMessage  `json:"data,omitempty"`
	Delay string           `json:"delay,omitempty"`
	Error *MockStreamError `json:"error,omitempty"`
}

// MockStreamError terminates a stream mid-way with an error.
type MockStreamError struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body,omitempty"`
}

// SetRequest is the body for POST /__mock/set.
type SetRequest struct {
	Session    string        `json:"session,omitempty"`    // empty = auto-generate
	ProviderID string        `json:"providerID"`           // which provider; "*" = wildcard
	Directive  MockDirective `json:"directive"`
}

// SetResponse is returned from POST /__mock/set.
type SetResponse struct {
	Session string `json:"session"`
}

// Protocol represents the detected API protocol.
type Protocol string

const (
	ProtocolClaude Protocol = "claude"
	ProtocolOpenAI Protocol = "openai"
	ProtocolGemini Protocol = "gemini"
	ProtocolCodex  Protocol = "codex"
)
