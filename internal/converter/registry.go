package converter

import (
	"encoding/json"
	"fmt"

	"github.com/awsl-project/maxx/internal/domain"
)

// TransformState holds state for streaming response conversion
type TransformState struct {
	MessageID        string
	CurrentIndex     int
	CurrentBlockType string // "text", "thinking", "tool_use"
	ToolCalls        map[int]*ToolCallState
	Buffer           string // SSE line buffer
	Usage            *Usage
	StopReason       string
}

// ToolCallState tracks tool call conversion state
type ToolCallState struct {
	ID           string
	Name         string
	Arguments    string
	ContentIndex int // assigned Claude content block index
}

// Usage tracks token usage during streaming
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	CacheRead    int `json:"cache_read_input_tokens,omitempty"`
	CacheWrite   int `json:"cache_creation_input_tokens,omitempty"`
}

// RequestTransformer transforms request bodies between formats
type RequestTransformer interface {
	Transform(body []byte, model string, stream bool) ([]byte, error)
}

// ResponseTransformer transforms response bodies between formats
type ResponseTransformer interface {
	// Transform converts a non-streaming response
	Transform(body []byte) ([]byte, error)
	// TransformChunk converts a streaming SSE chunk
	TransformChunk(chunk []byte, state *TransformState) ([]byte, error)
}

// Registry holds all format converters
type Registry struct {
	requests  map[domain.ClientType]map[domain.ClientType]RequestTransformer
	responses map[domain.ClientType]map[domain.ClientType]ResponseTransformer
}

// NewRegistry creates a new converter registry with all built-in converters
func NewRegistry() *Registry {
	r := &Registry{
		requests:  make(map[domain.ClientType]map[domain.ClientType]RequestTransformer),
		responses: make(map[domain.ClientType]map[domain.ClientType]ResponseTransformer),
	}
	r.registerBuiltins()
	return r
}

// Register registers a converter pair
func (r *Registry) Register(from, to domain.ClientType, req RequestTransformer, resp ResponseTransformer) {
	if r.requests[from] == nil {
		r.requests[from] = make(map[domain.ClientType]RequestTransformer)
	}
	if r.responses[from] == nil {
		r.responses[from] = make(map[domain.ClientType]ResponseTransformer)
	}
	if req != nil {
		r.requests[from][to] = req
	}
	if resp != nil {
		r.responses[from][to] = resp
	}
}

// NeedConvert checks if conversion is needed
func (r *Registry) NeedConvert(clientType domain.ClientType, supportedTypes []domain.ClientType) bool {
	for _, t := range supportedTypes {
		if t == clientType {
			return false
		}
	}
	return true
}

// GetTargetFormat returns the target format (first supported type)
func (r *Registry) GetTargetFormat(supportedTypes []domain.ClientType) domain.ClientType {
	if len(supportedTypes) > 0 {
		return supportedTypes[0]
	}
	return ""
}

// TransformRequest converts a request body
func (r *Registry) TransformRequest(from, to domain.ClientType, body []byte, model string, stream bool) ([]byte, error) {
	if from == to {
		return body, nil
	}

	fromMap := r.requests[from]
	if fromMap == nil {
		return nil, fmt.Errorf("no request transformer from %s", from)
	}
	transformer := fromMap[to]
	if transformer == nil {
		return nil, fmt.Errorf("no request transformer from %s to %s", from, to)
	}
	return transformer.Transform(body, model, stream)
}

// TransformResponse converts a non-streaming response
func (r *Registry) TransformResponse(from, to domain.ClientType, body []byte) ([]byte, error) {
	if from == to {
		return body, nil
	}

	fromMap := r.responses[from]
	if fromMap == nil {
		return nil, fmt.Errorf("no response transformer from %s", from)
	}
	transformer := fromMap[to]
	if transformer == nil {
		return nil, fmt.Errorf("no response transformer from %s to %s", from, to)
	}
	return transformer.Transform(body)
}

// TransformStreamChunk converts a streaming chunk
func (r *Registry) TransformStreamChunk(from, to domain.ClientType, chunk []byte, state *TransformState) ([]byte, error) {
	if from == to {
		return chunk, nil
	}

	fromMap := r.responses[from]
	if fromMap == nil {
		return nil, fmt.Errorf("no response transformer from %s", from)
	}
	transformer := fromMap[to]
	if transformer == nil {
		return nil, fmt.Errorf("no response transformer from %s to %s", from, to)
	}
	return transformer.TransformChunk(chunk, state)
}

// NewTransformState creates a new transform state
func NewTransformState() *TransformState {
	return &TransformState{
		ToolCalls: make(map[int]*ToolCallState),
		Usage:     &Usage{},
	}
}

// Helper for JSON marshaling
func mustMarshal(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}
