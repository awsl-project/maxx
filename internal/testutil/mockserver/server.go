package mockserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// directiveKey is the lookup key for stored directives.
type directiveKey struct {
	Session string
	APIKey  string
}

// Server is a mock upstream server that supports OpenAI, Claude, Gemini, and Codex protocols.
//
// Two modes of operation:
//  1. Control API: POST /__mock/set to pre-register (session, apiKey) → directive pairs.
//     Proxy requests include X-Mock-Session header; the server matches by (session, apiKey).
//  2. No session: returns default 200 success for the detected protocol.
type Server struct {
	*httptest.Server
	directives sync.Map // map[directiveKey]MockDirective
}

// Handler returns the HTTP handler for standalone use.
func Handler() http.Handler {
	s := &Server{}
	return s.handler()
}

// New creates and starts a new mock server.
func New() *Server {
	s := &Server{}
	s.Server = httptest.NewServer(s.handler())
	return s
}

// Set programmatically registers a directive (for use in Go tests).
// Returns the session ID.
func (s *Server) Set(session, apiKey string, directive MockDirective) string {
	if session == "" {
		session = uuid.New().String()
	}
	s.directives.Store(directiveKey{Session: session, APIKey: apiKey}, directive)
	return session
}

// Clear removes all directives for a session.
func (s *Server) Clear(session string) {
	s.directives.Range(func(key, _ any) bool {
		if k, ok := key.(directiveKey); ok && k.Session == session {
			s.directives.Delete(key)
		}
		return true
	})
}

func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/__mock/set", s.handleSet)
	mux.HandleFunc("/__mock/clear", s.handleClear)
	mux.HandleFunc("/", s.handleProxy)
	return mux
}

// POST /__mock/set — register a directive
func (s *Server) handleSet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req SetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	session := s.Set(req.Session, req.APIKey, req.Directive)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SetResponse{Session: session})
}

// POST /__mock/clear — clear all directives for a session
func (s *Server) handleClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Session string `json:"session"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Session != "" {
		s.Clear(body.Session)
	}
	w.WriteHeader(http.StatusOK)
}

// handleProxy handles actual API requests (OpenAI, Claude, Gemini, Codex).
func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	protocol := DetectProtocol(r)
	model := ExtractModel(protocol, body, r.URL.Path)

	// Resolve directive: X-Mock-Response header (legacy) > session-based lookup > empty (200 OK)
	directive := s.resolveDirective(r)

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

// resolveDirective determines the MockDirective for a request.
// Priority: X-Mock-Response header (legacy) → exact (session, apiKey) → wildcard (session, "*") → empty (200 OK).
func (s *Server) resolveDirective(r *http.Request) MockDirective {
	// Legacy: X-Mock-Response header takes precedence
	if mockHeader := r.Header.Get(MockHeader); mockHeader != "" {
		var d MockDirective
		if err := json.Unmarshal([]byte(mockHeader), &d); err == nil {
			return d
		}
	}
	return s.lookupDirective(r)
}

// lookupDirective finds the matching session-based directive for a request.
func (s *Server) lookupDirective(r *http.Request) MockDirective {
	session := r.Header.Get(SessionHeader)
	if session == "" {
		return MockDirective{}
	}

	apiKey := extractAPIKey(r)

	// Exact match
	if d, ok := s.directives.Load(directiveKey{Session: session, APIKey: apiKey}); ok {
		return d.(MockDirective)
	}
	// Wildcard match
	if d, ok := s.directives.Load(directiveKey{Session: session, APIKey: "*"}); ok {
		return d.(MockDirective)
	}

	return MockDirective{}
}

// extractAPIKey extracts the API key from request headers across all protocols.
func extractAPIKey(r *http.Request) string {
	// Claude: x-api-key
	if key := r.Header.Get("x-api-key"); key != "" {
		return key
	}
	// Gemini: x-goog-api-key
	if key := r.Header.Get("x-goog-api-key"); key != "" {
		return key
	}
	// OpenAI/Codex: Authorization: Bearer xxx
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}
