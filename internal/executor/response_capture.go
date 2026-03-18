package executor

import (
	"bytes"
	"net/http"
)

// ResponseCapture wraps http.ResponseWriter to capture the response
// This allows us to record the actual response sent to the client
type ResponseCapture struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
	headers    http.Header
}

// NewResponseCapture creates a new ResponseCapture wrapper
func NewResponseCapture(w http.ResponseWriter) *ResponseCapture {
	return &ResponseCapture{
		ResponseWriter: w,
		statusCode:     http.StatusOK, // Default status
		headers:        make(http.Header),
	}
}

// WriteHeader captures the status code and forwards to underlying writer
func (rc *ResponseCapture) WriteHeader(code int) {
	rc.statusCode = code
	rc.ResponseWriter.WriteHeader(code)
}

// Write captures the body and forwards to underlying writer
func (rc *ResponseCapture) Write(b []byte) (int, error) {
	rc.body.Write(b)
	return rc.ResponseWriter.Write(b)
}

// Header returns the header map (for setting headers)
func (rc *ResponseCapture) Header() http.Header {
	return rc.ResponseWriter.Header()
}

// Flush implements http.Flusher for streaming support
func (rc *ResponseCapture) Flush() {
	if f, ok := rc.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// StatusCode returns the captured status code
func (rc *ResponseCapture) StatusCode() int {
	return rc.statusCode
}

// Body returns the captured response body
func (rc *ResponseCapture) Body() string {
	return rc.body.String()
}

// CapturedHeaders returns the headers that were set
func (rc *ResponseCapture) CapturedHeaders() map[string]string {
	result := make(map[string]string)
	for key, values := range rc.ResponseWriter.Header() {
		if len(values) > 0 {
			result[key] = values[0]
		}
	}
	return result
}
