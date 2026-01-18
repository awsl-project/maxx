package executor

import (
	"bytes"
	"net/http"
	"strings"

	"github.com/awsl-project/maxx/internal/converter"
	"github.com/awsl-project/maxx/internal/domain"
)

// URL path mappings for different client types
var clientTypeURLPaths = map[domain.ClientType]string{
	domain.ClientTypeClaude: "/v1/messages",
	domain.ClientTypeOpenAI: "/v1/chat/completions",
	// Gemini uses dynamic paths with model names, handled separately
}

// ConvertRequestURI converts the request URI from one client type to another
func ConvertRequestURI(originalURI string, fromType, toType domain.ClientType) string {
	if fromType == toType {
		return originalURI
	}

	// Get the target path for the destination type
	targetPath, ok := clientTypeURLPaths[toType]
	if !ok {
		// For Gemini or unknown types, return original
		return originalURI
	}

	// Check if the original URI matches a known pattern and replace it
	for _, knownPath := range clientTypeURLPaths {
		if strings.HasPrefix(originalURI, knownPath) {
			// Replace the path prefix, preserving query string if any
			suffix := strings.TrimPrefix(originalURI, knownPath)
			return targetPath + suffix
		}
	}

	// If no known pattern matched, return target path
	// This handles cases where the original path doesn't match expected patterns
	return targetPath
}

// ConvertingResponseWriter wraps http.ResponseWriter to convert response format
// It converts responses from provider's format (targetType) back to client's format (originalType)
type ConvertingResponseWriter struct {
	underlying   http.ResponseWriter
	converter    *converter.Registry
	originalType domain.ClientType // Client's original format
	targetType   domain.ClientType // Provider's format
	isStream     bool
	statusCode   int
	headers      http.Header
	buffer       bytes.Buffer      // Buffer for non-streaming responses
	streamState  *converter.TransformState
	headersSent  bool
}

// NewConvertingResponseWriter creates a new ConvertingResponseWriter
func NewConvertingResponseWriter(
	w http.ResponseWriter,
	conv *converter.Registry,
	originalType, targetType domain.ClientType,
	isStream bool,
) *ConvertingResponseWriter {
	return &ConvertingResponseWriter{
		underlying:   w,
		converter:    conv,
		originalType: originalType,
		targetType:   targetType,
		isStream:     isStream,
		statusCode:   http.StatusOK,
		headers:      make(http.Header),
		streamState:  converter.NewTransformState(),
	}
}

// Header returns the header map
func (c *ConvertingResponseWriter) Header() http.Header {
	return c.underlying.Header()
}

// WriteHeader captures the status code
func (c *ConvertingResponseWriter) WriteHeader(code int) {
	c.statusCode = code
	if c.isStream {
		// For streaming, write headers immediately
		c.underlying.WriteHeader(code)
		c.headersSent = true
	}
	// For non-streaming, defer header writing until we have the converted response
}

// Write handles response body conversion
func (c *ConvertingResponseWriter) Write(b []byte) (int, error) {
	if c.isStream {
		return c.writeStream(b)
	}
	// For non-streaming, buffer the response
	return c.buffer.Write(b)
}

// writeStream handles streaming response conversion
func (c *ConvertingResponseWriter) writeStream(b []byte) (int, error) {
	// Convert the chunk
	converted, err := c.converter.TransformStreamChunk(c.targetType, c.originalType, b, c.streamState)
	if err != nil {
		// On conversion error, pass through original data
		return c.underlying.Write(b)
	}

	if len(converted) > 0 {
		_, writeErr := c.underlying.Write(converted)
		if writeErr != nil {
			return 0, writeErr
		}
	}

	return len(b), nil
}

// Flush implements http.Flusher for streaming support
func (c *ConvertingResponseWriter) Flush() {
	if f, ok := c.underlying.(http.Flusher); ok {
		f.Flush()
	}
}

// Finalize converts and writes buffered non-streaming response
// Must be called after adapter completes for non-streaming responses
func (c *ConvertingResponseWriter) Finalize() error {
	if c.isStream {
		return nil // Streaming responses are already written
	}

	body := c.buffer.Bytes()

	// Convert the response
	converted, err := c.converter.TransformResponse(c.targetType, c.originalType, body)
	if err != nil {
		// On conversion error, use original body
		converted = body
	}

	// Update Content-Type header based on original client type
	c.updateContentType()

	// Write headers and body
	if !c.headersSent {
		c.underlying.WriteHeader(c.statusCode)
		c.headersSent = true
	}
	_, writeErr := c.underlying.Write(converted)
	return writeErr
}

// updateContentType sets the Content-Type header based on client type
func (c *ConvertingResponseWriter) updateContentType() {
	switch c.originalType {
	case domain.ClientTypeClaude:
		c.underlying.Header().Set("Content-Type", "application/json")
	case domain.ClientTypeOpenAI:
		c.underlying.Header().Set("Content-Type", "application/json")
	case domain.ClientTypeGemini:
		c.underlying.Header().Set("Content-Type", "application/json")
	}
}

// StatusCode returns the captured status code
func (c *ConvertingResponseWriter) StatusCode() int {
	return c.statusCode
}

// Body returns the buffered response body (for non-streaming)
func (c *ConvertingResponseWriter) Body() string {
	return c.buffer.String()
}

// NeedsConversion returns true if format conversion is needed
func NeedsConversion(originalType, targetType domain.ClientType) bool {
	return originalType != targetType && originalType != "" && targetType != ""
}

// GetPreferredTargetType returns the best target type for conversion
// Prefers Claude as it has the richest format support
func GetPreferredTargetType(supportedTypes []domain.ClientType, originalType domain.ClientType) domain.ClientType {
	// If original type is supported, no conversion needed
	for _, t := range supportedTypes {
		if t == originalType {
			return originalType
		}
	}

	// Prefer Claude as target (richest format)
	for _, t := range supportedTypes {
		if t == domain.ClientTypeClaude {
			return t
		}
	}

	// Fall back to first supported type
	if len(supportedTypes) > 0 {
		return supportedTypes[0]
	}

	return originalType
}

// IsSSELine checks if a line is an SSE data line
func IsSSELine(line string) bool {
	return strings.HasPrefix(line, "data: ")
}
