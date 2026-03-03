package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/awsl-project/maxx/internal/adapter/client"
	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/converter"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/executor"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/repository/cached"
)

// RequestTracker interface for tracking active requests
type RequestTracker interface {
	Add() bool
	Done()
	IsShuttingDown() bool
}

// ProxyHandler handles AI API proxy requests
type ProxyHandler struct {
	clientAdapter *client.Adapter
	executor      *executor.Executor
	sessionRepo   *cached.SessionRepository
	tokenAuth     *TokenAuthMiddleware
	tracker       RequestTracker
	trackerMu     sync.RWMutex
	engine        *flow.Engine
	extra         []flow.HandlerFunc
}

// NewProxyHandler creates a new proxy handler
func NewProxyHandler(
	clientAdapter *client.Adapter,
	exec *executor.Executor,
	sessionRepo *cached.SessionRepository,
	tokenAuth *TokenAuthMiddleware,
) *ProxyHandler {
	h := &ProxyHandler{
		clientAdapter: clientAdapter,
		executor:      exec,
		sessionRepo:   sessionRepo,
		tokenAuth:     tokenAuth,
		engine:        flow.NewEngine(),
	}
	h.engine.Use(h.ingress)
	return h
}

func (h *ProxyHandler) Use(handlers ...flow.HandlerFunc) {
	h.extra = append(h.extra, handlers...)
}

// SetRequestTracker sets the request tracker for graceful shutdown
func (h *ProxyHandler) SetRequestTracker(tracker RequestTracker) {
	h.trackerMu.Lock()
	defer h.trackerMu.Unlock()
	h.tracker = tracker
}

// ServeHTTP handles proxy requests
func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := flow.NewCtx(w, r)
	handlers := make([]flow.HandlerFunc, len(h.extra)+1)
	copy(handlers, h.extra)
	handlers[len(h.extra)] = h.dispatch
	h.engine.HandleWith(ctx, handlers...)
}

func (h *ProxyHandler) ingress(c *flow.Ctx) {
	r := c.Request
	w := c.Writer
	log.Printf("[Proxy] Received request: %s %s", r.Method, r.URL.Path)

	// Track request for graceful shutdown
	h.trackerMu.RLock()
	tracker := h.tracker
	h.trackerMu.RUnlock()

	if tracker != nil {
		if !tracker.Add() {
			log.Printf("[Proxy] Rejecting request during shutdown: %s %s", r.Method, r.URL.Path)
			writeError(w, http.StatusServiceUnavailable, "server is shutting down")
			c.Abort()
			return
		}
		defer tracker.Done()
	}

	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		c.Abort()
		return
	}

	if strings.HasPrefix(r.URL.Path, "/v1/responses") {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/v1")
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body")
		c.Abort()
		return
	}
	_ = r.Body.Close()

	// Normalize OpenAI Responses payloads sent to chat/completions
	if strings.HasPrefix(r.URL.Path, "/v1/chat/completions") {
		if normalized, ok := normalizeOpenAIChatCompletionsPayload(body); ok {
			body = normalized
		}
	}

	r.Body = io.NopCloser(bytes.NewReader(body))
	ctx := r.Context()

	clientType := h.clientAdapter.DetectClientType(r, body)
	log.Printf("[Proxy] Detected client type: %s", clientType)
	if clientType == "" {
		writeError(w, http.StatusBadRequest, "unable to detect client type")
		c.Abort()
		return
	}

	var apiToken *domain.APIToken
	var apiTokenID uint64
	if h.tokenAuth != nil {
		apiToken, err = h.tokenAuth.ValidateRequest(r, clientType)
		if err != nil {
			log.Printf("[Proxy] Token auth failed: %v", err)
			writeError(w, http.StatusUnauthorized, err.Error())
			c.Abort()
			return
		}
		if apiToken != nil {
			apiTokenID = apiToken.ID
			log.Printf("[Proxy] Token authenticated: id=%d, name=%s, projectID=%d", apiToken.ID, apiToken.Name, apiToken.ProjectID)
			c.Set(flow.KeyAPITokenDevMode, apiToken.DevMode)
		}
	}

	// Determine tenantID from API token or use default
	var tenantID uint64
	if apiToken != nil && apiToken.TenantID > 0 {
		tenantID = apiToken.TenantID
	} else {
		tenantID = domain.DefaultTenantID
	}
	ctx = maxxctx.WithTenantID(ctx, tenantID)

	requestModel := h.clientAdapter.ExtractModel(r, body, clientType)
	log.Printf("[Proxy] Extracted model: %s (path: %s)", requestModel, r.URL.Path)
	sessionID := h.clientAdapter.ExtractSessionID(r, body, clientType)
	stream := h.clientAdapter.IsStreamRequest(r, body)

	c.Set(flow.KeyClientType, clientType)
	c.Set(flow.KeySessionID, sessionID)
	c.Set(flow.KeyRequestModel, requestModel)
	originalBody := bytes.Clone(body)
	c.Set(flow.KeyRequestBody, body)
	c.Set(flow.KeyOriginalRequestBody, originalBody)
	c.Set(flow.KeyRequestHeaders, r.Header)
	c.Set(flow.KeyRequestURI, r.URL.RequestURI())
	c.Set(flow.KeyIsStream, stream)
	c.Set(flow.KeyAPITokenID, apiTokenID)

	var projectID uint64
	if pidStr := r.Header.Get("X-Maxx-Project-ID"); pidStr != "" {
		if pid, err := strconv.ParseUint(pidStr, 10, 64); err == nil {
			projectID = pid
			log.Printf("[Proxy] Using project ID from header: %d", projectID)
		}
	}

	session, sessionErr := h.sessionRepo.GetBySessionID(tenantID, sessionID)
	if sessionErr != nil {
		log.Printf("[Proxy] Failed to load session %s: %v", sessionID, sessionErr)
	}
	if session != nil {
		if session.ProjectID > 0 {
			projectID = session.ProjectID
			log.Printf("[Proxy] Using project ID from session binding: %d", projectID)
		} else if projectID == 0 && apiToken != nil && apiToken.ProjectID > 0 {
			projectID = apiToken.ProjectID
			log.Printf("[Proxy] Using project ID from token: %d", projectID)
		}
	} else {
		if projectID == 0 && apiToken != nil && apiToken.ProjectID > 0 {
			projectID = apiToken.ProjectID
			log.Printf("[Proxy] Using project ID from token for new session: %d", projectID)
		}
		session = &domain.Session{
			TenantID:   tenantID,
			SessionID:  sessionID,
			ClientType: clientType,
			ProjectID:  projectID,
		}
		_ = h.sessionRepo.Create(session)
	}

	c.Set(flow.KeyProjectID, projectID)

	r = r.WithContext(ctx)
	c.Request = r
	c.InboundBody = body
	c.IsStream = stream
	c.Set(flow.KeyProxyContext, ctx)
	c.Set(flow.KeyProxyStream, stream)
	c.Set(flow.KeyProxyRequestModel, requestModel)

	c.Next()
}

func (h *ProxyHandler) dispatch(c *flow.Ctx) {
	stream := c.IsStream
	if v, ok := c.Get(flow.KeyProxyStream); ok {
		if s, ok := v.(bool); ok {
			stream = s
		}
	}

	err := h.executor.ExecuteWith(c)
	if err == nil {
		return
	}
	proxyErr, ok := err.(*domain.ProxyError)
	if ok {
		if stream {
			writeStreamError(c.Writer, proxyErr)
		} else {
			writeProxyError(c.Writer, proxyErr)
		}
		c.Err = err
		c.Abort()
		return
	}
	writeError(c.Writer, http.StatusInternalServerError, err.Error())
	c.Err = err
	c.Abort()
}

func normalizeOpenAIChatCompletionsPayload(body []byte) ([]byte, bool) {
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, false
	}
	if _, hasMessages := data["messages"]; hasMessages {
		return nil, false
	}
	if _, hasInput := data["input"]; !hasInput {
		if _, hasInstructions := data["instructions"]; !hasInstructions {
			return nil, false
		}
	}

	model, _ := data["model"].(string)
	stream, _ := data["stream"].(bool)
	converted, err := converter.GetGlobalRegistry().TransformRequest(
		domain.ClientTypeCodex,
		domain.ClientTypeOpenAI,
		body,
		model,
		stream,
	)
	if err != nil {
		return nil, false
	}
	return converted, true
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
