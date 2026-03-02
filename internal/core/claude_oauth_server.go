package core

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider/claude"
	"github.com/awsl-project/maxx/internal/handler"
)

// ClaudeOAuthServer handles OAuth callbacks on localhost:1456
// This is required because Anthropic uses a fixed redirect URI
type ClaudeOAuthServer struct {
	claudeHandler *handler.ClaudeHandler
	httpServer    *http.Server
	mu            sync.Mutex
	isRunning     bool
}

// NewClaudeOAuthServer creates a new OAuth callback server
func NewClaudeOAuthServer(claudeHandler *handler.ClaudeHandler) *ClaudeOAuthServer {
	return &ClaudeOAuthServer{
		claudeHandler: claudeHandler,
	}
}

// Start starts the OAuth callback server on port 1456
func (s *ClaudeOAuthServer) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning {
		log.Printf("[ClaudeOAuth] Server already running")
		return nil
	}

	mux := http.NewServeMux()

	// Handle OAuth callback at /auth/callback (matches OAuthRedirectURI)
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[ClaudeOAuth] Received callback: %s", r.URL.Path)
		newURL := *r.URL
		newURL.Path = "/claude/oauth/callback"
		newReq := r.Clone(r.Context())
		newReq.URL = &newURL
		s.claudeHandler.ServeHTTP(w, newReq)
	})

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","service":"claude-oauth"}`))
	})

	addr := fmt.Sprintf("localhost:%d", claude.OAuthCallbackPort)

	// Pre-listen to verify the port is available before marking as running
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	s.httpServer = &http.Server{
		Handler: mux,
	}

	go func() {
		log.Printf("[ClaudeOAuth] Starting OAuth callback server on %s", addr)
		if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("[ClaudeOAuth] Server error: %v", err)
		}
	}()

	s.isRunning = true
	log.Printf("[ClaudeOAuth] OAuth callback server started on port %d", claude.OAuthCallbackPort)
	return nil
}

// Stop stops the OAuth callback server
func (s *ClaudeOAuthServer) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning {
		return nil
	}

	log.Printf("[ClaudeOAuth] Stopping OAuth callback server")

	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("[ClaudeOAuth] Graceful shutdown failed: %v", err)
		s.httpServer.Close()
	}

	s.isRunning = false
	log.Printf("[ClaudeOAuth] OAuth callback server stopped")
	return nil
}

// IsRunning checks if the server is running
func (s *ClaudeOAuthServer) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isRunning
}
