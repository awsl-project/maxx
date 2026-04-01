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
	Session    string
	ProviderID string
}

// Server is a mock upstream server that supports OpenAI, Claude, Gemini, and Codex protocols.
//
// Provider identification is done via URL path prefix: /p/{providerID}/...
// Each provider's baseURL should be set to "http://mock:port/p/{id}".
// The mock server strips the prefix before protocol detection.
//
// Two modes of operation:
//  1. Control API: POST /__mock/set to pre-register (session, providerID) → directive pairs.
//  2. Legacy: X-Mock-Response header for single-request control.
//  3. No session/header: returns default 200 success for the detected protocol.
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
func (s *Server) Set(session, providerID string, directive MockDirective) string {
	if session == "" {
		session = uuid.New().String()
	}
	s.directives.Store(directiveKey{Session: session, ProviderID: providerID}, directive)
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

// ProviderURL returns the base URL for a provider, e.g. "http://host:port/p/3".
func (s *Server) ProviderURL(providerID string) string {
	return s.URL + ProviderPathPrefix + providerID
}

func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/__mock/set", s.handleSet)
	mux.HandleFunc("/__mock/clear", s.handleClear)
	mux.HandleFunc("/", s.handleProxy)
	return mux
}

// POST /__mock/set
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
	session := s.Set(req.Session, req.ProviderID, req.Directive)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SetResponse{Session: session})
}

// POST /__mock/clear
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

// handleProxy handles actual API requests.
func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	// Extract provider ID from path prefix: /p/{id}/...
	providerID, realPath := extractProviderFromPath(r.URL.Path)
	r.URL.Path = realPath

	body, _ := io.ReadAll(r.Body)
	protocol := DetectProtocol(r)
	model := ExtractModel(protocol, body, r.URL.Path)

	// Resolve directive
	directive := s.resolveDirective(r, providerID)

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

// extractProviderFromPath extracts provider ID from /p/{id}/... prefix.
// Returns ("", originalPath) if no prefix found.
func extractProviderFromPath(path string) (string, string) {
	if !strings.HasPrefix(path, ProviderPathPrefix) {
		return "", path
	}
	rest := path[len(ProviderPathPrefix):]
	slash := strings.Index(rest, "/")
	if slash < 0 {
		return rest, "/"
	}
	return rest[:slash], rest[slash:]
}

// resolveDirective determines the MockDirective for a request.
func (s *Server) resolveDirective(r *http.Request, providerID string) MockDirective {
	// Legacy: X-Mock-Response header takes precedence
	if mockHeader := r.Header.Get(MockHeader); mockHeader != "" {
		var d MockDirective
		if err := json.Unmarshal([]byte(mockHeader), &d); err == nil {
			return d
		}
	}

	session := r.Header.Get(SessionHeader)
	if session == "" {
		return MockDirective{}
	}

	// Exact match by (session, providerID)
	if providerID != "" {
		if d, ok := s.directives.Load(directiveKey{Session: session, ProviderID: providerID}); ok {
			return d.(MockDirective)
		}
	}
	// Wildcard
	if d, ok := s.directives.Load(directiveKey{Session: session, ProviderID: "*"}); ok {
		return d.(MockDirective)
	}

	return MockDirective{}
}
