package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/client"
	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/converter"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/executor"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/repository"
	"github.com/awsl-project/maxx/internal/repository/cached"
	"github.com/awsl-project/maxx/internal/systemsettingcache"
)

const proxyRequestsDisabledMessage = "proxy requests are temporarily disabled by admin"

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
	settingRepo   repository.SystemSettingRepository
	tokenAuth     *TokenAuthMiddleware
	tracker       RequestTracker
	trackerMu     sync.RWMutex
	engine        *flow.Engine
	extra         []flow.HandlerFunc
	uploadLimiter *uploadLimiter
}

func isUserPanelAPIToken(apiToken *domain.APIToken) bool {
	return apiToken != nil && strings.HasPrefix(apiToken.Description, userPanelAPITokenDescriptionPrefix)
}

func canAPITokenUseProjectBinding(apiToken *domain.APIToken) bool {
	return !isUserPanelAPIToken(apiToken)
}

func apiTokenProjectBinding(apiToken *domain.APIToken, currentProjectID uint64) (uint64, bool) {
	if apiToken == nil || currentProjectID != 0 || apiToken.ProjectID == 0 {
		return currentProjectID, false
	}
	if !canAPITokenUseProjectBinding(apiToken) {
		return currentProjectID, false
	}
	return apiToken.ProjectID, true
}

func resolveProxyProjectID(r *http.Request, apiToken *domain.APIToken) (uint64, error) {
	var projectID uint64
	if r != nil {
		if pidStr := r.Header.Get("X-Maxx-Project-ID"); pidStr != "" {
			if isUserPanelAPIToken(apiToken) {
				return 0, errors.New("user panel token cannot select project")
			}
			if pid, err := strconv.ParseUint(pidStr, 10, 64); err == nil {
				projectID = pid
			}
		}
	}
	if tokenProjectID, ok := apiTokenProjectBinding(apiToken, projectID); ok {
		projectID = tokenProjectID
	}
	return projectID, nil
}

// NewProxyHandler creates a new proxy handler
func NewProxyHandler(
	clientAdapter *client.Adapter,
	exec *executor.Executor,
	sessionRepo *cached.SessionRepository,
	settingRepo repository.SystemSettingRepository,
	tokenAuth *TokenAuthMiddleware,
) *ProxyHandler {
	h := &ProxyHandler{
		clientAdapter: clientAdapter,
		executor:      exec,
		sessionRepo:   sessionRepo,
		settingRepo:   settingRepo,
		tokenAuth:     tokenAuth,
		engine:        flow.NewEngine(),
		uploadLimiter: newUploadLimiterFromEnv(),
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
	if isResponsesWebSocketUpgrade(r) {
		tenantID := domain.DefaultTenantID
		var apiToken *domain.APIToken
		if h.tokenAuth != nil {
			var err error
			apiToken, err = h.tokenAuth.ValidateRequest(r, domain.ClientTypeCodex)
			if err != nil {
				writeError(w, http.StatusUnauthorized, err.Error())
				return
			}
			if apiToken != nil {
				if apiToken.TenantID > 0 {
					tenantID = apiToken.TenantID
				}
			}
		}
		// ProjectProxyHandler resolves /project/{slug}/... before this point and
		// passes the project ID in X-Maxx-Project-ID. Session binding is not
		// available until after the WebSocket upgrade, so use the same initial
		// header/token binding rules as HTTP/SSE requests.
		projectID, err := resolveProxyProjectID(r, apiToken)
		if err != nil {
			writeError(w, http.StatusForbidden, err.Error())
			return
		}

		// Codex only immediately falls back to HTTP/SSE when the WebSocket
		// handshake fails with 426 Upgrade Required (not after 101 + JSON error).
		if h.executor == nil || !h.executor.HasResponsesWebSocketProvider(tenantID, projectID) {
			writeResponsesWebSocketUpgradeRequired(w)
			return
		}

		var readLimit int64
		if h.uploadLimiter != nil {
			readLimit = h.uploadLimiter.maxBytes
		}
		h.serveResponsesWebSocket(w, r, readLimit)
		return
	}

	ctx := flow.NewCtx(executor.NewResponseCapture(w), r)
	h.engine.HandleWith(ctx, h.proxyHandlers()...)
}

func (h *ProxyHandler) proxyHandlers() []flow.HandlerFunc {
	handlers := make([]flow.HandlerFunc, len(h.extra)+1)
	copy(handlers, h.extra)
	handlers[len(h.extra)] = h.dispatch
	return handlers
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

	// The proxy surface is POST-only, except the async video-generation poll
	// (GET /v1/video/generations/{task_id}) which the client drives. The bare
	// submit endpoint stays POST-only.
	methodAllowed := r.Method == http.MethodPost ||
		(r.Method == http.MethodGet && client.IsVideoPollPath(r.URL.Path))
	if !methodAllowed {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		c.Abort()
		return
	}

	if h.isProxyRequestsDisabled() {
		log.Printf("[Proxy] Rejecting request because proxy kill switch is enabled: %s %s", r.Method, r.URL.Path)
		writeError(w, http.StatusServiceUnavailable, proxyRequestsDisabledMessage)
		c.Abort()
		return
	}

	// Capture the client's original Responses request URI (path + query) before
	// normalizing /v1 away, so a custom Codex downstream can be forwarded the exact
	// path the client used (passthrough) rather than a hardcoded one.
	if strings.HasPrefix(r.URL.Path, "/responses") || strings.HasPrefix(r.URL.Path, "/v1/responses") {
		c.Set(flow.KeyResponsesClientPath, r.URL.RequestURI())
	}

	if strings.HasPrefix(r.URL.Path, "/v1/responses") {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/v1")
	}

	// 大上传准入控制:在把 body 读进内存之前先门控,避免大量并发大上传同时挤爆堆。
	// 名额持有到本函数返回(c.Next 同步跑完整个请求链路后),覆盖 body 在内存的整个生命周期。
	//
	// 刻意放在 stream 检测/鉴权之前:目的就是在做任何工作、读任何 body 之前廉价地泄洪。
	// 代价是被泄洪的请求即使本是 SSE,拿到的也是 HTTP 层 413/429 而非 SSE 错误事件——
	// 此时 body 还没读、client type 还不知道,无法构造对应协议的错误,可接受。
	if h.uploadLimiter != nil {
		if h.uploadLimiter.tooLarge(r.ContentLength) {
			log.Printf("[Proxy] rejecting over-limit upload: %s %s (len=%d > %d)", r.Method, r.URL.Path, r.ContentLength, h.uploadLimiter.maxBytes)
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			c.Abort()
			return
		}
		release, ok := h.uploadLimiter.acquire(r.Context(), r.ContentLength)
		if !ok {
			log.Printf("[Proxy] large-upload slot unavailable, shedding request: %s %s (len=%d)", r.Method, r.URL.Path, r.ContentLength)
			writeRateLimitError(w, "server busy: too many concurrent large uploads, please retry", 5)
			c.Abort()
			return
		}
		defer release()
	}

	// 硬上限兜底:Content-Length 未知(chunked)时 tooLarge 预判不到,读取时用 LimitReader 封顶。
	// +1 用于区分"恰好等于上限"与"超过上限";maxBytes 接近 MaxInt64 时跳过 +1 防溢出成负数。
	var bodyReader io.Reader = r.Body
	if h.uploadLimiter != nil && h.uploadLimiter.maxBytes > 0 {
		limit := h.uploadLimiter.maxBytes
		if limit < math.MaxInt64 {
			limit++
		}
		bodyReader = io.LimitReader(r.Body, limit)
	}
	body := maxxctx.GetRequestBody(r.Context())
	if body == nil {
		var readErr error
		body, readErr = io.ReadAll(bodyReader)
		if readErr != nil {
			_ = r.Body.Close()
			writeError(w, http.StatusBadRequest, "failed to read request body")
			c.Abort()
			return
		}
	}
	_ = r.Body.Close()
	if h.uploadLimiter != nil && h.uploadLimiter.maxBytes > 0 && int64(len(body)) > h.uploadLimiter.maxBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		c.Abort()
		return
	}

	// Normalize OpenAI Responses payloads sent to chat/completions
	if isOpenAIChatCompletionsPath(r.URL.Path) {
		if normalized, ok := normalizeOpenAIChatCompletionsPayload(body); ok {
			body = normalized
		}
	}

	r.Body = io.NopCloser(bytes.NewReader(body))
	ctx := r.Context()
	stream := h.clientAdapter.IsStreamRequest(r, body)

	clientType := h.clientAdapter.DetectClientType(r, body)
	log.Printf("[Proxy] Detected client type: %s", clientType)
	if clientType == "" {
		writeError(w, http.StatusBadRequest, "unable to detect client type")
		c.Abort()
		return
	}

	var err error
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

	if isClaudeCountTokensRequest(r, clientType) {
		log.Printf("[Proxy] Handling Claude count_tokens locally")
		writeClaudeCountTokensResponse(w, body)
		c.Abort()
		return
	}
	if isClaudeWarmupRequest(r, body, clientType) {
		log.Printf("[Proxy] Handling Claude warmup probe locally")
		writeClaudeWarmupResponse(w, body)
		c.Abort()
		return
	}

	requestModel := h.clientAdapter.ExtractModel(r, body, clientType)
	log.Printf("[Proxy] Extracted model: %s (path: %s)", requestModel, r.URL.Path)
	sessionID := h.clientAdapter.ExtractSessionID(r, body, clientType)
	// originalBody 与 body 内容一致且 body 全程不被就地修改:converter / normalize /
	// InjectCodexUserAgent 都返回新切片,dispatch 里的格式转换也写到局部变量而非
	// state.requestBody。因此别名共享即可,无需再 bytes.Clone 出一整份副本(每个请求
	// 体可达数十 MB,这份拷贝纯属浪费)。真正需要独立副本的下游(converting_writer)
	// 已自行 Clone。
	originalBody := body

	c.Set(flow.KeyClientType, clientType)
	c.Set(flow.KeySessionID, sessionID)
	c.Set(flow.KeyRequestModel, requestModel)
	c.Set(flow.KeyRequestBody, body)
	c.Set(flow.KeyOriginalRequestBody, originalBody)
	c.Set(flow.KeyRequestHeaders, r.Header)
	c.Set(flow.KeyRequestURI, r.URL.RequestURI())
	c.Set(flow.KeyIsStream, stream)
	c.Set(flow.KeyAPITokenID, apiTokenID)

	projectID, err := resolveProxyProjectID(r, apiToken)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		c.Abort()
		return
	}
	if projectID != 0 {
		log.Printf("[Proxy] Using initial project ID: %d", projectID)
	}
	c.Set(flow.KeyProjectID, projectID)

	if apiToken != nil {
		if err := h.tokenAuth.AcquireConcurrency(apiToken); err != nil {
			log.Printf("[Proxy] Token concurrency limit hit: tokenID=%d err=%v", apiToken.ID, err)
			h.executor.RecordRejectedProxyRequest(c, apiToken, http.StatusTooManyRequests, err.Error())
			writeRateLimitError(w, err.Error(), 1)
			c.Abort()
			return
		}
		defer h.tokenAuth.ReleaseConcurrency(apiToken)
	}

	// Determine tenantID from API token or use default
	var tenantID uint64
	if apiToken != nil && apiToken.TenantID > 0 {
		tenantID = apiToken.TenantID
	} else {
		tenantID = domain.DefaultTenantID
	}
	ctx = maxxctx.WithTenantID(ctx, tenantID)

	now := time.Now()

	session, sessionErr := h.sessionRepo.GetBySessionID(tenantID, sessionID)
	if sessionErr != nil {
		log.Printf("[Proxy] Failed to load session %s: %v", sessionID, sessionErr)
	}
	if session != nil {
		if !isUserPanelAPIToken(apiToken) && session.ProjectID > 0 {
			projectID = session.ProjectID
			log.Printf("[Proxy] Using project ID from session binding: %d", projectID)
		} else if tokenProjectID, ok := apiTokenProjectBinding(apiToken, projectID); ok {
			projectID = tokenProjectID
			log.Printf("[Proxy] Using project ID from token: %d", projectID)
		}
		if touchErr := h.sessionRepo.Touch(tenantID, sessionID, now); touchErr != nil {
			log.Printf("[Proxy] Failed to touch session %s: %v", sessionID, touchErr)
		}
	} else {
		if tokenProjectID, ok := apiTokenProjectBinding(apiToken, projectID); ok {
			projectID = tokenProjectID
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
	if flow.GetResponsesWebSocketExchange(c) != nil {
		c.Err = err
		c.Abort()
		return
	}
	proxyErr, ok := asHandlerProxyError(err)
	if ok {
		if stream {
			writeProxyStreamError(c.Writer, proxyErr)
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

func (h *ProxyHandler) isProxyRequestsDisabled() bool {
	return systemsettingcache.GetBoolean(h.settingRepo, domain.SettingKeyProxyRequestsDisabled)
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

// writeResponsesWebSocketUpgradeRequired rejects the WebSocket upgrade with
// HTTP 426 so official Codex clients immediately switch to HTTP/SSE.
// See openai/codex codex-rs/core/src/client.rs (UPGRADE_REQUIRED → FallbackToHttp)
// and core/tests/suite/websocket_fallback.rs.
func writeResponsesWebSocketUpgradeRequired(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Connection", "close")
	w.WriteHeader(http.StatusUpgradeRequired)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message":   "no provider supports Codex Responses WebSocket; use HTTP/SSE",
			"type":      "proxy_error",
			"code":      "websocket_not_supported",
			"fallback":  "http_sse",
			"retryable": true,
		},
	})
}

func writeRateLimitError(w http.ResponseWriter, message string, retryAfterSeconds int64) {
	w.Header().Set("Content-Type", "application/json")
	if retryAfterSeconds <= 0 {
		retryAfterSeconds = 1
	}
	w.Header().Set("Retry-After", strconv.FormatInt(retryAfterSeconds, 10))
	w.WriteHeader(http.StatusTooManyRequests)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"type":    "rate_limit_error",
		},
	})
}

func writeProxyError(w http.ResponseWriter, err *domain.ProxyError) {
	w.Header().Set("Content-Type", "application/json")
	retryAfter := err.RetryAfter
	if retryAfter <= 0 && err.CooldownUntil != nil {
		retryAfter = time.Until(*err.CooldownUntil)
	}
	if retryAfter > 0 {
		sec := int64(retryAfter.Seconds())
		if sec <= 0 {
			sec = 1
		}
		w.Header().Set("Retry-After", strconv.FormatInt(sec, 10))
	}
	statusCode := http.StatusBadGateway
	if err.HTTPStatusCode >= 400 && err.HTTPStatusCode < 600 {
		statusCode = err.HTTPStatusCode
	}
	w.WriteHeader(statusCode)
	payload := map[string]interface{}{
		"message":   err.Error(),
		"type":      "upstream_error",
		"retryable": err.Retryable,
	}
	if err.Code != "" {
		payload["code"] = err.Code
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": payload,
	})
}

func writeStreamRateLimitError(w http.ResponseWriter, message string, retryAfterSeconds int64) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	if retryAfterSeconds <= 0 {
		retryAfterSeconds = 1
	}
	w.Header().Set("Retry-After", strconv.FormatInt(retryAfterSeconds, 10))
	w.WriteHeader(http.StatusTooManyRequests)

	errorEvent := map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"message": message,
			"type":    "rate_limit_error",
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

func writeStreamError(w http.ResponseWriter, err *domain.ProxyError) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	retryAfter := err.RetryAfter
	if retryAfter <= 0 && err.CooldownUntil != nil {
		retryAfter = time.Until(*err.CooldownUntil)
	}
	if retryAfter > 0 {
		sec := int64(retryAfter.Seconds())
		if sec <= 0 {
			sec = 1
		}
		w.Header().Set("Retry-After", strconv.FormatInt(sec, 10))
	}
	statusCode := http.StatusOK
	if err.HTTPStatusCode >= 400 && err.HTTPStatusCode < 600 {
		statusCode = err.HTTPStatusCode
	}
	w.WriteHeader(statusCode)

	payload := map[string]interface{}{
		"message":   err.Error(),
		"type":      "upstream_error",
		"retryable": err.Retryable,
	}
	if err.Code != "" {
		payload["code"] = err.Code
	}
	errorEvent := map[string]interface{}{
		"type":  "error",
		"error": payload,
	}
	data, _ := json.Marshal(errorEvent)
	w.Write([]byte("data: "))
	w.Write(data)
	w.Write([]byte("\n\n"))

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func writeProxyStreamError(w http.ResponseWriter, err *domain.ProxyError) {
	if responseStarted(w) {
		writeStreamErrorEvent(w, err)
		return
	}
	writeProxyError(w, err)
}

type responseStartTracker interface {
	WroteToClient() bool
}

func responseStarted(w http.ResponseWriter) bool {
	tracker, ok := w.(responseStartTracker)
	return ok && tracker.WroteToClient()
}

func writeStreamErrorEvent(w http.ResponseWriter, err *domain.ProxyError) {
	payload := map[string]interface{}{
		"message":   err.Error(),
		"type":      "upstream_error",
		"retryable": err.Retryable,
	}
	if err.Code != "" {
		payload["code"] = err.Code
	}
	errorEvent := map[string]interface{}{
		"type":  "error",
		"error": payload,
	}
	data, _ := json.Marshal(errorEvent)
	w.Write([]byte("data: "))
	w.Write(data)
	w.Write([]byte("\n\n"))

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func isClaudeCountTokensRequest(req *http.Request, clientType domain.ClientType) bool {
	return clientType == domain.ClientTypeClaude && req != nil && req.URL.Path == "/v1/messages/count_tokens"
}

func writeClaudeCountTokensResponse(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]int{
		"input_tokens": estimateClaudeInputTokens(body),
	})
}

func isClaudeWarmupRequest(req *http.Request, body []byte, clientType domain.ClientType) bool {
	if clientType != domain.ClientTypeClaude || req == nil || req.URL.Path != "/v1/messages" {
		return false
	}
	if req.Header.Get("anthropic-beta") == "" {
		return false
	}

	var claudeReq converter.ClaudeRequest
	if err := json.Unmarshal(body, &claudeReq); err != nil {
		return false
	}
	if len(claudeReq.Tools) > 0 {
		return false
	}
	return !isClaudeCompactRequest(claudeReq) && isClaudeWarmupProbePayload(claudeReq)
}

func isClaudeWarmupProbePayload(req converter.ClaudeRequest) bool {
	if !req.Stream || req.MaxTokens != 1 || req.System != nil || len(req.Messages) != 1 {
		return false
	}
	msg := req.Messages[0]
	if msg.Role != "user" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(extractClaudeText(msg.Content))) {
	case "hi", "hello", "ok", "say hi", "say hello":
		return true
	default:
		return false
	}
}

func isClaudeCompactRequest(req converter.ClaudeRequest) bool {
	if strings.HasPrefix(extractClaudeText(req.System), "You are a helpful AI assistant tasked with summarizing conversations") {
		return true
	}
	if len(req.Messages) == 0 {
		return false
	}
	last := req.Messages[len(req.Messages)-1]
	if last.Role != "user" {
		return false
	}
	text := extractClaudeText(last.Content)
	return strings.Contains(text, "CRITICAL: Respond with TEXT ONLY. Do NOT call any tools.") ||
		(strings.Contains(text, "Pending Tasks:") && strings.Contains(text, "Current Work:"))
}

func writeClaudeWarmupResponse(w http.ResponseWriter, body []byte) {
	var req converter.ClaudeRequest
	_ = json.Unmarshal(body, &req)
	model := req.Model
	if model == "" {
		model = "claude"
	}
	if req.Stream {
		writeClaudeWarmupStreamResponse(w, model)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(converter.ClaudeResponse{
		ID:    "msg_maxx_warmup",
		Type:  "message",
		Role:  "assistant",
		Model: model,
		Content: []converter.ClaudeContentBlock{{
			Type: "text",
			Text: "ok",
		}},
		StopReason: "end_turn",
		Usage: converter.ClaudeUsage{
			InputTokens:  estimateClaudeInputTokens(body),
			OutputTokens: 1,
		},
	})
}

func writeClaudeWarmupStreamResponse(w http.ResponseWriter, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	writeClaudeSSE(w, "message_start", map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":            "msg_maxx_warmup",
			"type":          "message",
			"role":          "assistant",
			"model":         model,
			"content":       []interface{}{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]int{
				"input_tokens":  0,
				"output_tokens": 0,
			},
		},
	})
	writeClaudeSSE(w, "content_block_start", map[string]interface{}{
		"type":  "content_block_start",
		"index": 0,
		"content_block": map[string]string{
			"type": "text",
			"text": "",
		},
	})
	writeClaudeSSE(w, "content_block_delta", map[string]interface{}{
		"type":  "content_block_delta",
		"index": 0,
		"delta": map[string]string{
			"type": "text_delta",
			"text": "ok",
		},
	})
	writeClaudeSSE(w, "content_block_stop", map[string]interface{}{
		"type":  "content_block_stop",
		"index": 0,
	})
	writeClaudeSSE(w, "message_delta", map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]interface{}{
			"stop_reason":   "end_turn",
			"stop_sequence": nil,
		},
		"usage": map[string]int{
			"output_tokens": 1,
		},
	})
	writeClaudeSSE(w, "message_stop", map[string]string{
		"type": "message_stop",
	})
}

func writeClaudeSSE(w http.ResponseWriter, event string, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = w.Write([]byte("event: " + event + "\n"))
	_, _ = w.Write([]byte("data: "))
	_, _ = w.Write(data)
	_, _ = w.Write([]byte("\n\n"))
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func extractClaudeText(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []interface{}:
		var b strings.Builder
		for _, item := range x {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(extractClaudeText(item))
		}
		return b.String()
	case map[string]interface{}:
		if text, ok := x["text"].(string); ok {
			return text
		}
		if content, ok := x["content"]; ok {
			return extractClaudeText(content)
		}
		return ""
	default:
		return ""
	}
}

func estimateClaudeInputTokens(body []byte) int {
	var req converter.ClaudeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return estimateTextTokens(string(body))
	}

	total := 4
	total += estimateJSONTokens(req.System)
	for _, msg := range req.Messages {
		total += 3
		total += estimateJSONTokens(msg.Content)
	}
	for _, tool := range req.Tools {
		total += 16
		total += estimateTextTokens(tool.Name)
		total += estimateTextTokens(tool.Description)
		total += estimateJSONTokens(tool.InputSchema)
	}
	if total < 1 {
		return 1
	}
	return total
}

func estimateJSONTokens(v interface{}) int {
	switch x := v.(type) {
	case nil:
		return 0
	case string:
		return estimateTextTokens(x)
	default:
		data, err := json.Marshal(x)
		if err != nil {
			return 0
		}
		return estimateTextTokens(string(data))
	}
}

func estimateTextTokens(text string) int {
	runes := len([]rune(text))
	if runes == 0 {
		return 0
	}
	tokens := (runes + 3) / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}

func isOpenAIChatCompletionsPath(path string) bool {
	return strings.HasPrefix(path, "/v1/chat/completions") || strings.HasPrefix(path, "/chat/completions")
}

func asHandlerProxyError(err error) (*domain.ProxyError, bool) {
	if err == nil {
		return nil, false
	}
	var proxyErr *domain.ProxyError
	if errors.As(err, &proxyErr) {
		return proxyErr, true
	}
	return nil, false
}
