package handler

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/Bowl42/maxx-next/internal/adapter/client"
	ctxutil "github.com/Bowl42/maxx-next/internal/context"
	"github.com/Bowl42/maxx-next/internal/domain"
	"github.com/Bowl42/maxx-next/internal/executor"
	"github.com/Bowl42/maxx-next/internal/repository"
)

// ProxyHandler handles AI API proxy requests
type ProxyHandler struct {
	clientAdapter *client.Adapter
	executor      *executor.Executor
	sessionRepo   repository.SessionRepository
}

// NewProxyHandler creates a new proxy handler
func NewProxyHandler(
	clientAdapter *client.Adapter,
	exec *executor.Executor,
	sessionRepo repository.SessionRepository,
) *ProxyHandler {
	return &ProxyHandler{
		clientAdapter: clientAdapter,
		executor:      exec,
		sessionRepo:   sessionRepo,
	}
}

// ServeHTTP handles proxy requests
func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Proxy] Received request: %s %s", r.Method, r.URL.Path)

	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer r.Body.Close()

	// Detect client type and extract info
	clientType := h.clientAdapter.DetectClientType(r, body)
	log.Printf("[Proxy] Detected client type: %s", clientType)
	if clientType == "" {
		writeError(w, http.StatusBadRequest, "unable to detect client type")
		return
	}

	requestModel := h.clientAdapter.ExtractModel(r, body, clientType)
	log.Printf("[Proxy] Extracted model: %s (path: %s)", requestModel, r.URL.Path)
	sessionID := h.clientAdapter.ExtractSessionID(r, body, clientType)
	stream := h.clientAdapter.IsStreamRequest(body)

	// Build context
	ctx := r.Context()
	ctx = ctxutil.WithClientType(ctx, clientType)
	ctx = ctxutil.WithSessionID(ctx, sessionID)
	ctx = ctxutil.WithRequestModel(ctx, requestModel)
	ctx = ctxutil.WithRequestBody(ctx, body)
	ctx = ctxutil.WithRequestHeaders(ctx, r.Header)
	ctx = ctxutil.WithRequestPath(ctx, r.URL.Path)

	// Get or create session to get project ID
	session, _ := h.sessionRepo.GetBySessionID(sessionID)
	if session != nil {
		ctx = ctxutil.WithProjectID(ctx, session.ProjectID)
	} else {
		// Create new session
		newSession := &domain.Session{
			SessionID:  sessionID,
			ClientType: clientType,
			ProjectID:  0, // Global
		}
		_ = h.sessionRepo.Create(newSession)
	}

	// Execute request
	err = h.executor.Execute(ctx, w, r)
	if err != nil {
		proxyErr, ok := err.(*domain.ProxyError)
		if ok {
			if stream {
				writeStreamError(w, proxyErr)
			} else {
				writeProxyError(w, proxyErr)
			}
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
	}
}

// Helper functions

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"type":    "proxy_error",
		},
	})
}

func writeProxyError(w http.ResponseWriter, err *domain.ProxyError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadGateway)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message":   err.Error(),
			"type":      "upstream_error",
			"retryable": err.Retryable,
		},
	})
}

func writeStreamError(w http.ResponseWriter, err *domain.ProxyError) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	errorEvent := map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"message":   err.Error(),
			"type":      "upstream_error",
			"retryable": err.Retryable,
		},
	}
	data, _ := json.Marshal(errorEvent)
	w.Write([]byte("data: "))
	w.Write(data)
	w.Write([]byte("\n\n"))
	w.Write([]byte("data: [DONE]\n\n"))

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
