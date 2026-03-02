package core

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider/claude"
	"github.com/awsl-project/maxx/internal/handler"
)

// ClaudeOAuthServer handles OAuth callbacks on localhost:1456
// This is required because Anthropic uses a fixed redirect URI
type ClaudeOAuthServer struct {
	claudeHandler *handler.ClaudeHandler
	httpServer    *http.Server
	isRunning     bool
}

// NewClaudeOAuthServer creates a new OAuth callback server
func NewClaudeOAuthServer(claudeHandler *handler.ClaudeHandler) *ClaudeOAuthServer {
	return &ClaudeOAuthServer{
		claudeHandler: claudeHandler,
		isRunning:     false,
	}
}

// Start starts the OAuth callback server on port 1456
func (s *ClaudeOAuthServer) Start(ctx context.Context) error {
	if s.isRunning {
		log.Printf("[ClaudeOAuth] Server already running")
		return nil
	}

	mux := http.NewServeMux()

	// Handle OAuth callback at /auth/callback (matches OAuthRedirectURI)
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[ClaudeOAuth] Received callback: %s", r.URL.String())
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

	addr := fmt.Sprintf(":%d", claude.OAuthCallbackPort)
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		log.Printf("[ClaudeOAuth] Starting OAuth callback server on %s", addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[ClaudeOAuth] Server error: %v", err)
		}
	}()

	s.isRunning = true
	log.Printf("[ClaudeOAuth] OAuth callback server started on port %d", claude.OAuthCallbackPort)
	return nil
}

// Stop stops the OAuth callback server
func (s *ClaudeOAuthServer) Stop(ctx context.Context) error {
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
	return s.isRunning
}
