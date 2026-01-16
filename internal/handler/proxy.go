package handler

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"

	"github.com/awsl-project/maxx/internal/adapter/client"
	ctxutil "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/executor"
	"github.com/awsl-project/maxx/internal/repository/cached"
)

// ProxyHandler handles AI API proxy requests
type ProxyHandler struct {
	clientAdapter *client.Adapter
	executor      *executor.Executor
	sessionRepo   *cached.SessionRepository
	tokenAuth     *TokenAuthMiddleware
}

// NewProxyHandler creates a new proxy handler
func NewProxyHandler(
	clientAdapter *client.Adapter,
	exec *executor.Executor,
	sessionRepo *cached.SessionRepository,
	tokenAuth *TokenAuthMiddleware,
) *ProxyHandler {
	return &ProxyHandler{
		clientAdapter: clientAdapter,
		executor:      exec,
		sessionRepo:   sessionRepo,
		tokenAuth:     tokenAuth,
	}
}

// ServeHTTP handles proxy requests
func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Proxy] Received request: %s %s", r.Method, r.URL.Path)

	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Claude Desktop / Anthropic compatibility: count_tokens placeholder
	if r.URL.Path == "/v1/messages/count_tokens" {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"input_tokens":  0,
			"output_tokens": 0,
		})
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

	// Token authentication (uses clientType for primary header, with fallback)
	var apiToken *domain.APIToken
	var apiTokenID uint64
	if h.tokenAuth != nil {
		apiToken, err = h.tokenAuth.ValidateRequest(r, clientType)
		if err != nil {
			log.Printf("[Proxy] Token auth failed: %v", err)
			writeError(w, http.StatusUnauthorized, err.Error())
			return
		}
		if apiToken != nil {
			apiTokenID = apiToken.ID
			log.Printf("[Proxy] Token authenticated: id=%d, name=%s, projectID=%d", apiToken.ID, apiToken.Name, apiToken.ProjectID)
		}
	}

	requestModel := h.clientAdapter.ExtractModel(r, body, clientType)
	log.Printf("[Proxy] Extracted model: %s (path: %s)", requestModel, r.URL.Path)
	sessionID := h.clientAdapter.ExtractSessionID(r, body, clientType)
	stream := h.clientAdapter.IsStreamRequest(r, body)

	// Build context
	ctx := r.Context()
	ctx = ctxutil.WithClientType(ctx, clientType)
	ctx = ctxutil.WithSessionID(ctx, sessionID)
	ctx = ctxutil.WithRequestModel(ctx, requestModel)
	ctx = ctxutil.WithRequestBody(ctx, body)
	ctx = ctxutil.WithRequestHeaders(ctx, r.Header)
	ctx = ctxutil.WithRequestURI(ctx, r.URL.RequestURI())
	ctx = ctxutil.WithIsStream(ctx, stream)
	ctx = ctxutil.WithAPITokenID(ctx, apiTokenID)

	// Check for project ID from header (set by ProjectProxyHandler)
	var projectID uint64
	if pidStr := r.Header.Get("X-Maxx-Project-ID"); pidStr != "" {
		if pid, err := strconv.ParseUint(pidStr, 10, 64); err == nil {
			projectID = pid
			log.Printf("[Proxy] Using project ID from header: %d", projectID)
		}
	}

	// Get or create session to get project ID
	session, _ := h.sessionRepo.GetBySessionID(sessionID)
	if session != nil {
		// Priority: Session binding (Admin configured) > Token association > Header > 0
		if session.ProjectID > 0 {
			projectID = session.ProjectID
			log.Printf("[Proxy] Using project ID from session binding: %d", projectID)
		} else if projectID == 0 && apiToken != nil && apiToken.ProjectID > 0 {
			projectID = apiToken.ProjectID
			log.Printf("[Proxy] Using project ID from token: %d", projectID)
		}
	} else {
		// Create new session
		// If no project from header, use token's project
		if projectID == 0 && apiToken != nil && apiToken.ProjectID > 0 {
			projectID = apiToken.ProjectID
			log.Printf("[Proxy] Using project ID from token for new session: %d", projectID)
		}
		session = &domain.Session{
			SessionID:  sessionID,
			ClientType: clientType,
			ProjectID:  projectID,
		}
		_ = h.sessionRepo.Create(session)
	}

	ctx = ctxutil.WithProjectID(ctx, projectID)

	// Execute request (executor handles request recording, project binding, routing, etc.)
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
	if err.RetryAfter > 0 {
		sec := int64(err.RetryAfter.Seconds())
		if sec <= 0 {
			sec = 1
		}
		w.Header().Set("Retry-After", strconv.FormatInt(sec, 10))
	}
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
	if err.RetryAfter > 0 {
		sec := int64(err.RetryAfter.Seconds())
		if sec <= 0 {
			sec = 1
		}
		w.Header().Set("Retry-After", strconv.FormatInt(sec, 10))
	}
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

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
