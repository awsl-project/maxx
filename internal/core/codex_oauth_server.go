package core

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider/codex"
	"github.com/awsl-project/maxx/internal/handler"
)

// CodexOAuthServer handles OAuth callbacks on localhost:1455
// This is required because OpenAI uses a fixed redirect URI
type CodexOAuthServer struct {
	codexHandler *handler.CodexHandler
	httpServer   *http.Server
	isRunning    bool
}

// NewCodexOAuthServer creates a new OAuth callback server
func NewCodexOAuthServer(codexHandler *handler.CodexHandler) *CodexOAuthServer {
	return &CodexOAuthServer{
		codexHandler: codexHandler,
		isRunning:    false,
	}
}

// Start starts the OAuth callback server on port 1455
func (s *CodexOAuthServer) Start(ctx context.Context) error {
	if s.isRunning {
		log.Printf("[CodexOAuth] Server already running")
		return nil
	}

	mux := http.NewServeMux()

	// Handle OAuth callback at /auth/callback (matches OAuthRedirectURI)
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[CodexOAuth] Received callback: %s", r.URL.String())
		// Create a new request with rewritten path to match handler expectations
		newURL := *r.URL
		newURL.Path = "/codex/oauth/callback"
		newReq := r.Clone(r.Context())
		newReq.URL = &newURL
		s.codexHandler.ServeHTTP(w, newReq)
	})

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","service":"codex-oauth"}`))
	})

	addr := fmt.Sprintf(":%d", codex.OAuthCallbackPort)
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		log.Printf("[CodexOAuth] Starting OAuth callback server on %s", addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[CodexOAuth] Server error: %v", err)
		}
	}()

	s.isRunning = true
	log.Printf("[CodexOAuth] OAuth callback server started on port %d", codex.OAuthCallbackPort)
	return nil
}

// Stop stops the OAuth callback server
func (s *CodexOAuthServer) Stop(ctx context.Context) error {
	if !s.isRunning {
		return nil
	}

	log.Printf("[CodexOAuth] Stopping OAuth callback server")

	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("[CodexOAuth] Graceful shutdown failed: %v", err)
		s.httpServer.Close()
	}

	s.isRunning = false
	log.Printf("[CodexOAuth] OAuth callback server stopped")
	return nil
}

// IsRunning checks if the server is running
func (s *CodexOAuthServer) IsRunning() bool {
	return s.isRunning
}
