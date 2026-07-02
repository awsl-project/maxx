package executor

import (
	"bytes"
	"net/http"
	"net/url"
	"strings"

	"github.com/awsl-project/maxx/internal/converter"
	"github.com/awsl-project/maxx/internal/domain"
)

// URL path mappings for different client types
var clientTypeURLPaths = map[domain.ClientType]string{
	domain.ClientTypeClaude: "/v1/messages",
	domain.ClientTypeOpenAI: "/v1/chat/completions",
	domain.ClientTypeCodex:  "/responses",
	// Gemini uses dynamic paths with model names, handled separately
}

// ConvertRequestURI converts the request URI from one client type to another
func ConvertRequestURI(originalURI string, fromType, toType domain.ClientType, mappedModel string, isStream bool) string {
	if fromType == toType {
		return originalURI
	}

	path, rawQuery := splitURI(originalURI)

	if toType == domain.ClientTypeGemini {
		newPath := buildGeminiRequestPath(path, mappedModel, isStream)
		return withQuery(newPath, rawQuery)
	}

	// Get the target path for the destination type
	targetPath, ok := clientTypeURLPaths[toType]
	if !ok {
		// For unknown types, return original
		return originalURI
	}

	// Check if the original URI matches a known pattern and replace it
	suffix := ""
	for _, knownPath := range clientTypeURLPaths {
		if strings.HasPrefix(path, knownPath) {
			suffix = strings.TrimPrefix(path, knownPath)
			break
		}
	}

	if isClaudeCountTokensPath(path) && toType != domain.ClientTypeClaude {
		suffix = ""
	}

	return withQuery(targetPath+suffix, rawQuery)
}

func splitURI(originalURI string) (string, string) {
	parsed, err := url.ParseRequestURI(originalURI)
	if err == nil {
		return parsed.Path, parsed.RawQuery
	}
	if strings.Contains(originalURI, "?") {
		parts := strings.SplitN(originalURI, "?", 2)
		return parts[0], parts[1]
	}
	return originalURI, ""
}

func withQuery(path, rawQuery string) string {
	if rawQuery == "" {
		return path
	}
	return path + "?" + rawQuery
}

const geminiDefaultVersion = "v1beta"

func buildGeminiRequestPath(originalPath, mappedModel string, isStream bool) string {
	version, pathModel, action, ok := parseGeminiPath(originalPath)
	if !ok {
		version = geminiDefaultVersion
	}

	model := mappedModel
	if model == "" {
		model = pathModel
	}
	if model == "" {
		return originalPath
	}

	if action == "" {
		if isClaudeCountTokensPath(originalPath) {
			action = "countTokens"
		} else if isStream {
			action = "streamGenerateContent"
		} else {
			action = "generateContent"
		}
	}

	return "/" + version + "/models/" + model + ":" + action
}

func parseGeminiPath(path string) (string, string, string, bool) {
	if strings.HasPrefix(path, "/v1beta/models/") {
		return parseGeminiPathWithVersion(path, "v1beta", "/v1beta/models/")
	}
	if strings.HasPrefix(path, "/v1internal/models/") {
		return parseGeminiPathWithVersion(path, "v1internal", "/v1internal/models/")
	}
	return "", "", "", false
}

func parseGeminiPathWithVersion(path, version, prefix string) (string, string, string, bool) {
	rest := strings.TrimPrefix(path, prefix)
	if rest == "" {
		return version, "", "", true
	}
	model := rest
	action := ""
	if strings.Contains(rest, ":") {
		parts := strings.SplitN(rest, ":", 2)
		model = parts[0]
		action = parts[1]
	}
	return version, model, action, true
}

func isClaudeCountTokensPath(path string) bool {
	return strings.HasPrefix(path, "/v1/messages/count_tokens")
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
	buffer       bytes.Buffer // Buffer for non-streaming responses
	streamState  *converter.TransformState
	headersSent  bool
}

// NewConvertingResponseWriter creates a new ConvertingResponseWriter
func NewConvertingResponseWriter(
	w http.ResponseWriter,
	conv *converter.Registry,
	originalType, targetType domain.ClientType,
	isStream bool,
	originalRequestBody []byte,
) *ConvertingResponseWriter {
	state := converter.NewTransformState()
	if len(originalRequestBody) > 0 {
		state.OriginalRequestBody = bytes.Clone(originalRequestBody)
	}
	return &ConvertingResponseWriter{
		underlying:   w,
		converter:    conv,
		originalType: originalType,
		targetType:   targetType,
		isStream:     isStream,
		statusCode:   http.StatusOK,
		headers:      make(http.Header),
		streamState:  state,
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
		return 0, converter.NewResponseConversionError(err)
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
	converted, err := c.converter.TransformResponseWithState(c.targetType, c.originalType, body, c.streamState)
	if err != nil {
		return converter.NewResponseConversionError(err)
	}
	if converted == nil {
		return converter.NewResponseConversionError(converter.ErrNilConvertedResponse)
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

// GetPreferredTargetType returns the best target type for conversion.
// Prefers Codex only for codex providers, otherwise Gemini then Claude.
func GetPreferredTargetType(supportedTypes []domain.ClientType, originalType domain.ClientType, providerType string) domain.ClientType {
	// If original type is supported, no conversion needed
	for _, t := range supportedTypes {
		if t == originalType {
			return originalType
		}
	}

	if providerType == "codex" {
		// Prefer Codex when available (best fit for Codex provider)
		for _, t := range supportedTypes {
			if t == domain.ClientTypeCodex {
				return t
			}
		}
	}

	// Prefer Gemini as target (best fit for Antigravity)
	for _, t := range supportedTypes {
		if t == domain.ClientTypeGemini {
			return t
		}
	}

	// Prefer Claude as target (fallback)
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
