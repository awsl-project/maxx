package mockserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"time"
)

// Server is a mock upstream server that supports OpenAI, Claude, Gemini, and Codex protocols.
// Behavior is controlled via the X-Mock-Response request header.
type Server struct {
	*httptest.Server
}

// Handler returns the mock server HTTP handler (for use in standalone servers).
func Handler() http.Handler {
	return http.HandlerFunc(handle)
}

// New creates and starts a new mock server using httptest.
func New() *Server {
	return &Server{httptest.NewServer(Handler())}
}

func handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	protocol := DetectProtocol(r)
	model := ExtractModel(protocol, body, r.URL.Path)

	// Parse mock directive from header
	var directive MockDirective
	if mockHeader := r.Header.Get(MockHeader); mockHeader != "" {
		if err := json.Unmarshal([]byte(mockHeader), &directive); err != nil {
			http.Error(w, "invalid "+MockHeader+": "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	// Apply delay
	if directive.Delay != "" {
		if d, err := time.ParseDuration(directive.Delay); err == nil {
			time.Sleep(d)
		}
	}

	// Apply custom response headers
	for k, v := range directive.Headers {
		w.Header().Set(k, v)
	}

	// Streaming response
	if directive.Stream != nil {
		WriteSSEStream(w, protocol, model, directive.Stream)
		return
	}

	// Determine status code
	status := directive.Status
	if status == 0 {
		status = http.StatusOK
	}

	// Write response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if directive.Body != nil {
		w.Write(directive.Body)
	} else if status >= 400 {
		w.Write(DefaultErrorResponse(protocol, status, http.StatusText(status)))
	} else {
		w.Write(DefaultSuccessResponse(protocol, model))
	}
}
