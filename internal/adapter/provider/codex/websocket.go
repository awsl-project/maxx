package codex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/codexutil"
	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/payloadoverride"
	"github.com/awsl-project/maxx/internal/usage"
	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	codexResponsesWebSocketBetaHeader = "responses_websockets=2026-02-06"
	codexResponsesWebSocketHandshake  = 30 * time.Second
)

type codexWebSocketRead struct {
	messageType int
	payload     []byte
	err         error
}

type codexWebSocketSession struct {
	requestMu sync.Mutex
	writeMu   sync.Mutex
	closeOnce sync.Once

	conn            *websocket.Conn
	handshakeHeader http.Header
	reads           chan codexWebSocketRead
	done            chan struct{}
}

func newCodexWebSocketSession(conn *websocket.Conn, handshakeHeader http.Header) *codexWebSocketSession {
	session := &codexWebSocketSession{
		conn:            conn,
		handshakeHeader: handshakeHeader.Clone(),
		reads:           make(chan codexWebSocketRead, 4096),
		done:            make(chan struct{}),
	}
	conn.SetPingHandler(func(data string) error {
		session.writeMu.Lock()
		defer session.writeMu.Unlock()
		return conn.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(10*time.Second))
	})
	go session.readLoop()
	return session
}

func (s *codexWebSocketSession) readLoop() {
	defer close(s.reads)
	for {
		messageType, payload, err := s.conn.ReadMessage()
		if err != nil {
			select {
			case s.reads <- codexWebSocketRead{err: err}:
			case <-s.done:
			}
			s.close()
			return
		}
		select {
		case s.reads <- codexWebSocketRead{messageType: messageType, payload: payload}:
		case <-s.done:
			return
		}
	}
}

func (s *codexWebSocketSession) write(payload []byte) error {
	if s == nil || s.conn == nil {
		return errors.New("codex websocket session is not connected")
	}
	select {
	case <-s.done:
		return errors.New("codex websocket session is closed")
	default:
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.WriteMessage(websocket.TextMessage, payload)
}

func (s *codexWebSocketSession) close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		close(s.done)
		if s.conn != nil {
			_ = s.conn.Close()
		}
	})
}

func (s *codexWebSocketSession) isClosed() bool {
	if s == nil {
		return true
	}
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

type codexWebSocketSessionStore struct {
	mu       sync.Mutex
	sessions map[string]*codexWebSocketSession
}

var globalCodexWebSocketSessions = &codexWebSocketSessionStore{
	sessions: make(map[string]*codexWebSocketSession),
}

func (s *codexWebSocketSessionStore) get(key string) *codexWebSocketSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.sessions[key]
	if session != nil && session.isClosed() {
		delete(s.sessions, key)
		return nil
	}
	return session
}

func (s *codexWebSocketSessionStore) put(key string, session *codexWebSocketSession) {
	s.mu.Lock()
	previous := s.sessions[key]
	s.sessions[key] = session
	s.mu.Unlock()
	if previous != nil && previous != session {
		previous.close()
	}
}

func (s *codexWebSocketSessionStore) remove(key string, session *codexWebSocketSession) {
	s.mu.Lock()
	if s.sessions[key] == session {
		delete(s.sessions, key)
	}
	s.mu.Unlock()
	if session != nil {
		session.close()
	}
}

func (a *CodexAdapter) executeResponsesWebSocket(c *flow.Ctx, provider *domain.Provider) (bool, error) {
	if c == nil || provider == nil || c.Request == nil ||
		flow.GetClientType(c) != domain.ClientTypeCodex ||
		flow.GetOriginalClientType(c) != domain.ClientTypeCodex ||
		flow.IsProtocolConversion(c) {
		return false, nil
	}

	metadata, ok := maxxctx.GetResponsesWebSocketRequest(c.Request.Context())
	if !ok {
		return false, nil
	}

	rawRequest, err := prepareCodexWebSocketRequest(metadata.Payload, c, provider)
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(err, false, "invalid Codex websocket request")
		proxyErr.Scope = domain.ScopeRequest
		proxyErr.HTTPStatusCode = http.StatusBadRequest
		return true, proxyErr
	}

	key := strconv.FormatUint(provider.ID, 10) + ":" + metadata.SessionID
	session := globalCodexWebSocketSessions.get(key)
	previousResponseID := strings.TrimSpace(gjson.GetBytes(rawRequest, "previous_response_id").String())
	if session == nil && previousResponseID != "" {
		// A response ID is tied to the provider/session that produced it. If route
		// selection moved elsewhere, use the handler's full-transcript HTTP fallback.
		log.Printf("[Codex] Responses WebSocket session unavailable for provider=%d; falling back to HTTP/SSE", provider.ID)
		return false, nil
	}

	ctx := c.Request.Context()
	config := ensureCodexConfig(provider)
	accessToken, err := a.getAccessToken(ctx, false, "")
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(err, false, "failed to get access token")
		proxyErr.Scope = domain.ScopeKey
		proxyErr.Reason = domain.CooldownReasonAuthFailure
		return true, proxyErr
	}

	webSocketURL, err := codexResponsesWebSocketURL(c, config)
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(err, false, "failed to build Codex websocket URL")
		proxyErr.Scope = domain.ScopeEndpoint
		return true, proxyErr
	}
	headers := buildCodexWebSocketHeaders(c, accessToken, config.AccountID, rawRequest)

	if session == nil {
		var response *http.Response
		session, response, err = dialCodexWebSocket(ctx, webSocketURL, headers)
		if err != nil && response != nil && response.StatusCode == http.StatusUnauthorized {
			body := readCodexWebSocketHandshakeBody(response)
			accessToken, err = a.getAccessToken(ctx, true, accessToken)
			if err != nil {
				return true, classifyCodexHTTPError(http.StatusUnauthorized, body, response.Header, flow.GetMappedModel(c))
			}
			headers.Set("Authorization", "Bearer "+accessToken)
			session, response, err = dialCodexWebSocket(ctx, webSocketURL, headers)
		}
		if err != nil {
			if response != nil {
				body := readCodexWebSocketHandshakeBody(response)
				if response.StatusCode == http.StatusUpgradeRequired {
					log.Printf("[Codex] Responses WebSocket upgrade rejected with status=%d url=%s; falling back to HTTP/SSE", response.StatusCode, webSocketURL)
					return false, nil
				}
				return true, classifyCodexHTTPError(response.StatusCode, body, response.Header, flow.GetMappedModel(c))
			}
			// A transport-level upgrade failure is safe to retry through the existing
			// HTTP/SSE path because no upstream WebSocket event reached the client.
			log.Printf("[Codex] Responses WebSocket upgrade failed url=%s error=%v; falling back to HTTP/SSE", webSocketURL, err)
			return false, nil
		}
		globalCodexWebSocketSessions.put(key, session)
		if done := ctx.Done(); done != nil {
			go func() {
				<-done
				globalCodexWebSocketSessions.remove(key, session)
			}()
		}
	}

	session.requestMu.Lock()
	defer session.requestMu.Unlock()

	eventChan := flow.GetEventChan(c)
	if eventChan != nil {
		eventChan.SendRequestInfo(&domain.RequestInfo{
			Method:  "WEBSOCKET",
			URL:     webSocketURL,
			Headers: flattenHeaders(headers),
			Body:    string(rawRequest),
		})
	}

	if err = session.write(rawRequest); err != nil {
		globalCodexWebSocketSessions.remove(key, session)
		log.Printf("[Codex] Responses WebSocket write failed provider=%d error=%v; falling back to HTTP/SSE", provider.ID, err)
		return false, nil
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		globalCodexWebSocketSessions.remove(key, session)
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrUpstreamError, false, "streaming not supported")
		proxyErr.Scope = domain.ScopeRequest
		return true, proxyErr
	}

	var collector usage.StreamCollector
	responseModel := ""
	firstChunkSent := false
	for {
		select {
		case <-ctx.Done():
			globalCodexWebSocketSessions.remove(key, session)
			proxyErr := domain.NewProxyErrorWithMessage(ctx.Err(), false, "client disconnected")
			proxyErr.Scope = domain.ScopeRequest
			return true, proxyErr
		case read, open := <-session.reads:
			if !open || read.err != nil {
				globalCodexWebSocketSessions.remove(key, session)
				readErr := read.err
				if readErr == nil {
					readErr = io.ErrUnexpectedEOF
				}
				// The request frame was already written successfully, so retrying it
				// through HTTP or another provider could duplicate side effects.
				proxyErr := domain.NewProxyErrorWithMessage(readErr, false, "Codex websocket closed before response.completed")
				proxyErr.Scope = domain.ScopeRequest
				proxyErr.HTTPStatusCode = http.StatusBadGateway
				return true, proxyErr
			}
			if read.messageType != websocket.TextMessage {
				continue
			}
			payload := bytes.TrimSpace(read.payload)
			if len(payload) == 0 {
				continue
			}

			eventType := gjson.GetBytes(payload, "type").String()
			if eventType == "error" || eventType == "response.failed" {
				globalCodexWebSocketSessions.remove(key, session)
				return true, classifyCodexWebSocketEvent(payload, flow.GetMappedModel(c))
			}

			collector.ProcessSSEPayload(payload)
			if model := strings.TrimSpace(gjson.GetBytes(payload, "response.model").String()); model != "" {
				responseModel = model
			} else if model := strings.TrimSpace(gjson.GetBytes(payload, "model").String()); model != "" {
				responseModel = model
			}

			if _, err = c.Writer.Write([]byte("data: ")); err == nil {
				_, err = c.Writer.Write(payload)
			}
			if err == nil {
				_, err = c.Writer.Write([]byte("\n\n"))
			}
			if err != nil {
				globalCodexWebSocketSessions.remove(key, session)
				proxyErr := domain.NewProxyErrorWithMessage(err, false, "client disconnected")
				proxyErr.Scope = domain.ScopeRequest
				return true, proxyErr
			}
			flusher.Flush()
			if !firstChunkSent {
				firstChunkSent = true
				if eventChan != nil {
					eventChan.SendFirstToken(time.Now().UnixMilli())
				}
			}

			if eventType == "response.completed" || eventType == "response.done" {
				sendCodexWebSocketFinalEvents(eventChan, session.handshakeHeader, &collector, responseModel)
				return true, nil
			}
		}
	}
}

func prepareCodexWebSocketRequest(raw []byte, c *flow.Ctx, provider *domain.Provider) ([]byte, error) {
	if !gjson.ValidBytes(raw) || !gjson.ParseBytes(raw).IsObject() {
		return nil, errors.New("expected a JSON object")
	}
	body := bytes.Clone(raw)
	body, _ = sjson.SetBytes(body, "type", "response.create")
	if model := strings.TrimSpace(flow.GetMappedModel(c)); model != "" {
		body, _ = sjson.SetBytes(body, "model", model)
	}
	body, _ = sjson.DeleteBytes(body, "stream")
	body, _ = sjson.DeleteBytes(body, "background")
	body, _ = sjson.DeleteBytes(body, "prompt_cache_retention")
	body, _ = sjson.DeleteBytes(body, "safety_identifier")
	if maxOutput := gjson.GetBytes(body, "max_output_tokens"); maxOutput.Exists() {
		if !gjson.GetBytes(body, "max_tokens").Exists() {
			body, _ = sjson.SetBytes(body, "max_tokens", maxOutput.Value())
		}
		body, _ = sjson.DeleteBytes(body, "max_output_tokens")
	}
	if !gjson.GetBytes(body, "previous_response_id").Exists() && !gjson.GetBytes(body, "instructions").Exists() {
		body, _ = sjson.SetBytes(body, "instructions", "")
	}
	body = codexutil.NormalizeCodexInput(body)

	config := ensureCodexConfig(provider)
	if config.Reasoning != "" {
		body, _ = sjson.SetBytes(body, "reasoning.effort", config.Reasoning)
	}
	if config.ServiceTier != "" {
		body, _ = sjson.SetBytes(body, "service_tier", config.ServiceTier)
	}
	return payloadoverride.ApplyGlobal(body, "codex", flow.GetMappedModel(c)), nil
}

func codexResponsesWebSocketURL(c *flow.Ctx, config *domain.ProviderConfigCodex) (string, error) {
	baseURL := CodexBaseURL
	custom := config != nil && strings.TrimSpace(config.BaseURL) != ""
	if custom {
		baseURL = strings.TrimRight(config.BaseURL, "/")
	}

	path := "/responses"
	if custom && domain.ResponsesPassthroughEnabled(config.ResponsesPassthrough) {
		path = flow.GetResponsesClientPath(c)
		if path == "" {
			path = "/v1/responses"
		}
	}
	parsed, err := url.Parse(baseURL + path)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http":
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	default:
		return "", fmt.Errorf("unsupported URL scheme %q", parsed.Scheme)
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", errors.New("websocket URL host is empty")
	}
	return parsed.String(), nil
}

func buildCodexWebSocketHeaders(c *flow.Ctx, accessToken, accountID string, body []byte) http.Header {
	headers := make(http.Header)
	if c != nil && c.Request != nil {
		for key, values := range c.Request.Header {
			lowerKey := strings.ToLower(key)
			if codexFilteredHeaders[lowerKey] || lowerKey == "authorization" || strings.HasPrefix(lowerKey, "sec-websocket-") {
				continue
			}
			for _, value := range values {
				headers.Add(key, value)
			}
		}
	}
	if strings.TrimSpace(accessToken) != "" {
		headers.Set("Authorization", "Bearer "+accessToken)
	}
	betaHeader := strings.TrimSpace(headers.Get("OpenAI-Beta"))
	if !strings.Contains(betaHeader, "responses_websockets=") {
		headers.Set("OpenAI-Beta", codexResponsesWebSocketBetaHeader)
	}
	if c != nil {
		headers.Set("User-Agent", flow.ResolveUpstreamUserAgent(c, CodexUserAgent))
	}
	if sessionID := strings.TrimSpace(headers.Get("Session_id")); sessionID == "" {
		if promptCacheKey := strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String()); promptCacheKey != "" {
			headers.Set("Session_id", promptCacheKey)
			headers.Set("Conversation_id", promptCacheKey)
		}
	}
	if strings.TrimSpace(accessToken) != "" {
		if headers.Get("Originator") == "" {
			headers.Set("Originator", CodexOriginator)
		}
		if accountID != "" {
			headers.Set("Chatgpt-Account-Id", accountID)
		}
	}
	return headers
}

func dialCodexWebSocket(ctx context.Context, target string, headers http.Header) (*codexWebSocketSession, *http.Response, error) {
	dialer := &websocket.Dialer{
		Proxy:             http.ProxyFromEnvironment,
		HandshakeTimeout:  codexResponsesWebSocketHandshake,
		EnableCompression: true,
		NetDialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}
	conn, response, err := dialer.DialContext(ctx, target, headers)
	if err != nil {
		return nil, response, err
	}
	responseHeaders := make(http.Header)
	if response != nil {
		responseHeaders = response.Header.Clone()
		if response.Body != nil {
			_ = response.Body.Close()
		}
	}
	return newCodexWebSocketSession(conn, responseHeaders), response, nil
}

func readCodexWebSocketHandshakeBody(response *http.Response) []byte {
	if response == nil || response.Body == nil {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	_ = response.Body.Close()
	return body
}

func classifyCodexWebSocketEvent(payload []byte, model string) *domain.ProxyError {
	message := strings.TrimSpace(gjson.GetBytes(payload, "error.message").String())
	if message == "" {
		message = strings.TrimSpace(gjson.GetBytes(payload, "response.error.message").String())
	}
	if message == "" {
		message = "upstream websocket returned an error"
	}
	code := strings.TrimSpace(gjson.GetBytes(payload, "error.code").String())
	if code == "" {
		code = strings.TrimSpace(gjson.GetBytes(payload, "response.error.code").String())
	}
	errorType := strings.ToLower(gjson.GetBytes(payload, "error.type").String() + " " + code)
	proxyErr := domain.NewProxyErrorWithMessage(errors.New(string(payload)), true, message)
	proxyErr.Code = code
	proxyErr.ClientType = string(domain.ClientTypeCodex)
	switch {
	case strings.Contains(errorType, "rate_limit") || strings.Contains(errorType, "quota"):
		proxyErr.Scope = domain.ScopeKey
		proxyErr.Reason = domain.CooldownReasonRateLimitExceeded
		proxyErr.HTTPStatusCode = http.StatusTooManyRequests
	case strings.Contains(errorType, "auth") || strings.Contains(errorType, "unauthorized"):
		proxyErr.Scope = domain.ScopeKey
		proxyErr.Reason = domain.CooldownReasonAuthFailure
		proxyErr.HTTPStatusCode = http.StatusUnauthorized
		proxyErr.Retryable = false
	case strings.Contains(errorType, "model"):
		proxyErr.Scope = domain.ScopeModel
		proxyErr.Reason = domain.CooldownReasonModelUnavailable
		proxyErr.Model = model
		proxyErr.HTTPStatusCode = http.StatusNotFound
		proxyErr.Retryable = false
	case strings.Contains(errorType, "invalid"):
		proxyErr.Scope = domain.ScopeRequest
		proxyErr.HTTPStatusCode = http.StatusBadRequest
		proxyErr.Retryable = false
	default:
		proxyErr.Scope = domain.ScopeProvider
		proxyErr.Reason = domain.CooldownReasonServerError
		proxyErr.HTTPStatusCode = http.StatusBadGateway
	}
	return proxyErr
}

func sendCodexWebSocketFinalEvents(eventChan domain.AdapterEventChan, headers http.Header, collector *usage.StreamCollector, model string) {
	if eventChan == nil {
		return
	}
	eventChan.SendResponseInfo(&domain.ResponseInfo{
		Status:  http.StatusSwitchingProtocols,
		Headers: flattenHeaders(headers),
		Body:    "[websocket streaming]",
	})
	if collector != nil && collector.Metrics != nil && !collector.Metrics.IsEmpty() {
		metrics := usage.AdjustForClientType(collector.Metrics, domain.ClientTypeCodex)
		eventChan.SendMetrics(&domain.AdapterMetrics{
			InputTokens:    metrics.InputTokens,
			OutputTokens:   metrics.OutputTokens,
			CacheReadCount: metrics.CacheReadCount,
		})
	}
	eventChan.SendResponseModel(model)
}
