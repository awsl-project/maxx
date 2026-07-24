package codex

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	pathpkg "path"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	provideradapter "github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/usage"
	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
)

const (
	codexResponsesWebSocketBetaHeader         = "responses_websockets=2026-02-06"
	codexResponsesWebSocketHandshake          = 30 * time.Second
	codexResponsesWebSocketReadQueueSize      = 256
	codexResponsesWebSocketMaxQueuedBytes     = 8 << 20
	codexResponsesWebSocketMaxFrameBytes      = 32 << 20
	codexResponsesWebSocketWriteTimeout       = 30 * time.Second
	codexResponsesWebSocketFirstEventTimeout  = 20 * time.Second
	codexResponsesWebSocketEventIdleTimeout   = 5 * time.Minute
	codexResponsesWebSocketSessionIdleTimeout = 65 * time.Minute
	codexResponsesWebSocketMaxSessions        = 512
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
	queuedBytes     atomic.Int64
	maxQueuedBytes  int64
	fingerprint     string
	lastUsedAt      atomic.Int64
	activeTurns     atomic.Int32
	releaseProvider func()
}

func newCodexWebSocketSession(
	conn *websocket.Conn,
	handshakeHeader http.Header,
	fingerprint string,
	releaseProvider func(),
) *codexWebSocketSession {
	if releaseProvider == nil {
		releaseProvider = func() {}
	}
	session := &codexWebSocketSession{
		conn:            conn,
		handshakeHeader: handshakeHeader.Clone(),
		reads:           make(chan codexWebSocketRead, codexResponsesWebSocketReadQueueSize),
		done:            make(chan struct{}),
		maxQueuedBytes:  codexResponsesWebSocketMaxQueuedBytes,
		fingerprint:     fingerprint,
		releaseProvider: releaseProvider,
	}
	session.touch()
	conn.SetReadLimit(codexResponsesWebSocketMaxFrameBytes)
	conn.SetPingHandler(func(data string) error {
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
			s.enqueueError(err)
			s.close()
			return
		}
		size := int64(len(payload))
		if s.queuedBytes.Add(size) > s.maxQueuedBytes {
			s.queuedBytes.Add(-size)
			s.enqueueError(errors.New("codex websocket read queue byte limit exceeded"))
			s.close()
			return
		}
		select {
		case s.reads <- codexWebSocketRead{messageType: messageType, payload: payload}:
		case <-s.done:
			s.queuedBytes.Add(-size)
			return
		default:
			s.queuedBytes.Add(-size)
			s.enqueueError(errors.New("codex websocket read queue is full"))
			s.close()
			return
		}
	}
}

func (s *codexWebSocketSession) enqueueError(err error) {
	select {
	case s.reads <- codexWebSocketRead{err: err}:
	default:
	}
}

func (s *codexWebSocketSession) write(payload []byte) error {
	if s == nil || s.conn == nil || s.isClosed() {
		return errors.New("codex websocket session is closed")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.conn.SetWriteDeadline(time.Now().Add(codexResponsesWebSocketWriteTimeout)); err != nil {
		return err
	}
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
		if s.releaseProvider != nil {
			s.releaseProvider()
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

func (s *codexWebSocketSession) touch() {
	s.lastUsedAt.Store(time.Now().UnixNano())
}

func (s *codexWebSocketSession) lastUsed() time.Time {
	return time.Unix(0, s.lastUsedAt.Load())
}

type codexWebSocketSessionKey struct {
	ConnectionID string
	ProviderID   uint64
}

type codexWebSocketSessionStore struct {
	mu         sync.Mutex
	sessions   map[codexWebSocketSessionKey]*codexWebSocketSession
	maxEntries int
}

var globalCodexWebSocketSessions = &codexWebSocketSessionStore{
	sessions:   make(map[codexWebSocketSessionKey]*codexWebSocketSession),
	maxEntries: codexResponsesWebSocketMaxSessions,
}

func (s *codexWebSocketSessionStore) get(key codexWebSocketSessionKey) *codexWebSocketSession {
	s.mu.Lock()
	session := s.sessions[key]
	if session != nil && (session.isClosed() ||
		(session.activeTurns.Load() == 0 && time.Since(session.lastUsed()) >= codexResponsesWebSocketSessionIdleTimeout)) {
		delete(s.sessions, key)
		s.mu.Unlock()
		session.close()
		return nil
	}
	if session != nil {
		session.activeTurns.Add(1)
		session.touch()
	}
	s.mu.Unlock()
	return session
}

func (s *codexWebSocketSessionStore) put(key codexWebSocketSessionKey, session *codexWebSocketSession) bool {
	var stale []*codexWebSocketSession
	canStore := true
	s.mu.Lock()
	stale = append(stale, s.pruneIdleLocked(time.Now())...)
	if previous := s.sessions[key]; previous != nil && previous != session {
		if previous.activeTurns.Load() > 0 {
			canStore = false
		} else {
			delete(s.sessions, key)
			stale = append(stale, previous)
		}
	}
	if canStore && s.maxEntries > 0 && len(s.sessions) >= s.maxEntries && s.sessions[key] == nil {
		var oldestKey codexWebSocketSessionKey
		var oldest *codexWebSocketSession
		for candidateKey, candidate := range s.sessions {
			if candidate.activeTurns.Load() > 0 {
				continue
			}
			if oldest == nil || candidate.lastUsed().Before(oldest.lastUsed()) {
				oldestKey, oldest = candidateKey, candidate
			}
		}
		if oldest != nil {
			delete(s.sessions, oldestKey)
			stale = append(stale, oldest)
		} else {
			canStore = false
		}
	}
	if canStore {
		s.sessions[key] = session
	}
	s.mu.Unlock()
	for _, candidate := range stale {
		candidate.close()
	}
	return canStore
}

func (s *codexWebSocketSessionStore) remove(key codexWebSocketSessionKey, session *codexWebSocketSession) {
	s.mu.Lock()
	if s.sessions[key] == session {
		delete(s.sessions, key)
	}
	s.mu.Unlock()
	if session != nil {
		session.close()
	}
}

func (s *codexWebSocketSessionStore) closeForConnection(connectionID string) {
	var sessions []*codexWebSocketSession
	s.mu.Lock()
	for key, session := range s.sessions {
		if key.ConnectionID == connectionID {
			delete(s.sessions, key)
			sessions = append(sessions, session)
		}
	}
	s.mu.Unlock()
	for _, session := range sessions {
		session.close()
	}
}

func (s *codexWebSocketSessionStore) pruneIdle(now time.Time) {
	s.mu.Lock()
	stale := s.pruneIdleLocked(now)
	s.mu.Unlock()
	for _, session := range stale {
		session.close()
	}
}

func (s *codexWebSocketSessionStore) pruneIdleLocked(now time.Time) []*codexWebSocketSession {
	var stale []*codexWebSocketSession
	for key, session := range s.sessions {
		if session.isClosed() ||
			(session.activeTurns.Load() == 0 && now.Sub(session.lastUsed()) >= codexResponsesWebSocketSessionIdleTimeout) {
			delete(s.sessions, key)
			stale = append(stale, session)
		}
	}
	return stale
}

func (a *CodexAdapter) CloseResponsesWebSocketConnection(connectionID string) {
	globalCodexWebSocketSessions.closeForConnection(connectionID)
}

func (a *CodexAdapter) ExecuteResponsesWebSocket(
	c *flow.Ctx,
	provider *domain.Provider,
	exchange *domain.ResponsesWebSocketExchange,
) (*domain.ResponsesWebSocketResult, error) {
	if c == nil || provider == nil || c.Request == nil || exchange == nil || exchange.Sink == nil {
		return nil, newCodexWebSocketAttemptError(domain.ErrResponsesWebSocketProtocol, false)
	}
	if err := validateOutboundCodexWebSocketFrame(exchange.Frame); err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(err, false, "invalid Codex websocket request")
		proxyErr.Scope = domain.ScopeRequest
		proxyErr.Code = "websocket_protocol_error"
		proxyErr.HTTPStatusCode = http.StatusBadRequest
		return nil, newCodexWebSocketAttemptError(proxyErr, false)
	}

	ctx := c.Request.Context()
	config := ensureCodexConfig(provider)
	target, err := codexResponsesWebSocketURL(config)
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(err, true, "failed to build Codex websocket URL")
		proxyErr.Scope = domain.ScopeEndpoint
		return nil, newCodexWebSocketAttemptError(proxyErr, false)
	}
	if isCodexWebSocketUnsupported(provider.ID, target) {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrNoResponsesWebSocketProviders, true, "Codex websocket endpoint is temporarily marked unsupported")
		proxyErr.Scope = domain.ScopeEndpoint
		proxyErr.Code = "upstream_websocket_upgrade_rejected"
		attemptErr := newCodexWebSocketAttemptError(proxyErr, true)
		return nil, attemptErr
	}

	accessToken, err := a.getAccessToken(ctx, false, "")
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(err, false, "failed to get access token")
		proxyErr.Scope = domain.ScopeKey
		proxyErr.Reason = domain.CooldownReasonAuthFailure
		return nil, newCodexWebSocketAttemptError(proxyErr, false)
	}
	headers := buildCodexWebSocketHeaders(c, accessToken, config.AccountID, exchange.Frame)
	fingerprint := codexWebSocketFingerprint(provider.ID, target, headers)
	key := codexWebSocketSessionKey{ConnectionID: exchange.ConnectionID, ProviderID: provider.ID}
	session := globalCodexWebSocketSessions.get(key)
	if session != nil && session.fingerprint != fingerprint {
		session.activeTurns.Add(-1)
		globalCodexWebSocketSessions.remove(key, session)
		session = nil
	}
	if session == nil && exchange.PreviousResponseID != "" {
		return nil, newCodexWebSocketAttemptError(domain.ErrResponsesWebSocketSessionUnavailable, false)
	}

	reused := session != nil
	if session == nil {
		releaseProvider, acquired := exchange.AcquireProviderSlot()
		if !acquired {
			proxyErr := domain.NewProxyErrorWithMessage(domain.ErrNoAvailableProviders, true, "provider concurrency limit reached")
			proxyErr.Scope = domain.ScopeProvider
			proxyErr.Code = "websocket_transport_unavailable"
			proxyErr.HTTPStatusCode = http.StatusServiceUnavailable
			return nil, newCodexWebSocketAttemptError(proxyErr, false)
		}
		slotOwnedBySession := false
		defer func() {
			if !slotOwnedBySession {
				releaseProvider()
			}
		}()
		session, err = a.dialResponsesWebSocketSession(ctx, provider, config, target, headers, fingerprint, accessToken, releaseProvider)
		if err != nil {
			return nil, err
		}
		slotOwnedBySession = true
		session.activeTurns.Add(1)
		if !globalCodexWebSocketSessions.put(key, session) {
			session.activeTurns.Add(-1)
			session.close()
			proxyErr := domain.NewProxyErrorWithMessage(errors.New("Codex websocket session capacity reached"), true, "Codex websocket session capacity reached")
			proxyErr.Scope = domain.ScopeRequest
			proxyErr.Code = "upstream_websocket_session_capacity"
			proxyErr.HTTPStatusCode = http.StatusServiceUnavailable
			return nil, newCodexWebSocketAttemptError(proxyErr, false)
		}
	}

	result, execErr := session.executeTurn(ctx, exchange.Frame, exchange.Sink, flow.GetEventChan(c), provider.ID)
	if result != nil {
		result.ProviderID = provider.ID
		result.Reused = reused
	}
	if execErr != nil {
		var wsErr *domain.ResponsesWebSocketAttemptError
		if errors.As(execErr, &wsErr) && (wsErr.RequestFrameMayHaveBeenSent || session.isClosed()) {
			globalCodexWebSocketSessions.remove(key, session)
		}
		return result, execErr
	}
	return result, nil
}

func (a *CodexAdapter) dialResponsesWebSocketSession(
	ctx context.Context,
	provider *domain.Provider,
	config *domain.ProviderConfigCodex,
	target string,
	headers http.Header,
	fingerprint string,
	accessToken string,
	releaseProvider func(),
) (*codexWebSocketSession, error) {
	session, response, err := dialCodexWebSocket(ctx, target, headers, fingerprint, releaseProvider)
	if err != nil && response != nil && response.StatusCode == http.StatusUnauthorized && canRefreshCodexAccessToken(config) {
		body := readCodexWebSocketHandshakeBody(response)
		_ = body
		refreshed, refreshErr := a.getAccessToken(ctx, true, accessToken)
		if refreshErr != nil {
			proxyErr := classifyCodexHTTPError(http.StatusUnauthorized, body, response.Header, "")
			return nil, newCodexWebSocketAttemptError(proxyErr, false)
		}
		headers.Set("Authorization", "Bearer "+refreshed)
		fingerprint = codexWebSocketFingerprint(provider.ID, target, headers)
		session, response, err = dialCodexWebSocket(ctx, target, headers, fingerprint, releaseProvider)
	}
	if err == nil {
		return session, nil
	}

	if response != nil {
		status := response.StatusCode
		body := readCodexWebSocketHandshakeBody(response)
		proxyErr := classifyCodexHTTPError(status, body, response.Header, "")
		proxyErr.Code = "upstream_websocket_upgrade_rejected"
		capabilityFailure := isCodexWebSocketCapabilityStatus(status)
		if capabilityFailure {
			markCodexWebSocketUnsupported(provider.ID, target, http.StatusText(status))
		}
		return nil, newCodexWebSocketAttemptError(proxyErr, capabilityFailure)
	}

	proxyErr := domain.NewProxyErrorWithMessage(err, true, "failed to upgrade Codex websocket connection")
	proxyErr.Scope = domain.ScopeProvider
	proxyErr.Reason = domain.CooldownReasonNetworkError
	proxyErr.Code = "upstream_websocket_upgrade_rejected"
	proxyErr.HTTPStatusCode = http.StatusBadGateway
	return nil, newCodexWebSocketAttemptError(proxyErr, false)
}

func newCodexWebSocketAttemptError(err error, capabilityFailure bool) *domain.ResponsesWebSocketAttemptError {
	return &domain.ResponsesWebSocketAttemptError{
		Err:               err,
		CapabilityFailure: capabilityFailure,
	}
}

func canRefreshCodexAccessToken(config *domain.ProviderConfigCodex) bool {
	return config != nil && strings.TrimSpace(config.RefreshToken) != ""
}

func isCodexWebSocketCapabilityStatus(status int) bool {
	switch status {
	case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusUpgradeRequired, http.StatusNotImplemented:
		return true
	default:
		return false
	}
}

func validateOutboundCodexWebSocketFrame(frame []byte) error {
	if !gjson.ValidBytes(frame) || !gjson.ParseBytes(frame).IsObject() {
		return domain.ErrResponsesWebSocketProtocol
	}
	if gjson.GetBytes(frame, "type").String() != "response.create" {
		return domain.ErrResponsesWebSocketProtocol
	}
	if strings.TrimSpace(gjson.GetBytes(frame, "model").String()) == "" {
		return domain.ErrResponsesWebSocketProtocol
	}
	return nil
}

func (s *codexWebSocketSession) executeTurn(
	ctx context.Context,
	frame []byte,
	sink domain.ResponsesWebSocketFrameSink,
	eventChan domain.AdapterEventChan,
	providerID uint64,
) (*domain.ResponsesWebSocketResult, error) {
	defer func() { s.activeTurns.Add(-1) }()
	s.requestMu.Lock()
	defer s.requestMu.Unlock()
	s.touch()

	result := &domain.ResponsesWebSocketResult{ProviderID: providerID}
	if s.isClosed() {
		return result, newCodexWebSocketAttemptError(errors.New("codex websocket session is closed"), false)
	}
	if eventChan != nil {
		eventChan.SendRequestInfo(&domain.RequestInfo{
			Method:  "WEBSOCKET",
			Headers: flattenHeaders(s.handshakeHeader),
			Body:    string(frame),
		})
	}

	result.RequestFrameMayHaveBeenSent = true
	if err := s.write(frame); err != nil {
		return result, &domain.ResponsesWebSocketAttemptError{
			Err:                         err,
			RequestFrameMayHaveBeenSent: true,
		}
	}

	var collector usage.StreamCollector
	firstEvent := true
	timer := time.NewTimer(codexResponsesWebSocketFirstEventTimeout)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return result, &domain.ResponsesWebSocketAttemptError{
				Err:                         ctx.Err(),
				RequestFrameMayHaveBeenSent: true,
				FirstEventReceived:          result.FirstEventReceived,
				ClientEventSent:             result.ClientEventSent,
			}
		case <-timer.C:
			code := "upstream_websocket_first_event_timeout"
			message := "Codex websocket first event timeout"
			if result.FirstEventReceived {
				code = "upstream_websocket_closed_before_terminal"
				message = "Codex websocket event idle timeout"
			}
			proxyErr := domain.NewProxyErrorWithMessage(context.DeadlineExceeded, false, message)
			proxyErr.Scope = domain.ScopeProvider
			proxyErr.Reason = domain.CooldownReasonNetworkError
			proxyErr.Code = code
			proxyErr.HTTPStatusCode = http.StatusGatewayTimeout
			return result, &domain.ResponsesWebSocketAttemptError{
				Err:                         proxyErr,
				RequestFrameMayHaveBeenSent: true,
				FirstEventReceived:          result.FirstEventReceived,
				ClientEventSent:             result.ClientEventSent,
			}
		case read, open := <-s.reads:
			if len(read.payload) > 0 {
				s.queuedBytes.Add(-int64(len(read.payload)))
			}
			if !open || read.err != nil {
				readErr := read.err
				if readErr == nil {
					readErr = io.ErrUnexpectedEOF
				}
				message := "Codex websocket closed before terminal event"
				closeCode, closeReason, hasClose := provideradapter.ClassifyUpstreamResponsesWebSocketClose(readErr, providerID)
				if hasClose && closeReason != "" {
					message = "Codex websocket closed: " + closeReason
				}
				proxyErr := domain.NewProxyErrorWithMessage(readErr, false, message)
				proxyErr.Scope = domain.ScopeProvider
				proxyErr.Reason = domain.CooldownReasonNetworkError
				proxyErr.Code = "upstream_websocket_closed_before_terminal"
				proxyErr.HTTPStatusCode = http.StatusBadGateway
				attemptErr := &domain.ResponsesWebSocketAttemptError{
					Err:                         proxyErr,
					RequestFrameMayHaveBeenSent: true,
					FirstEventReceived:          result.FirstEventReceived,
					ClientEventSent:             result.ClientEventSent,
				}
				if hasClose {
					attemptErr.UpstreamCloseCode = closeCode
					attemptErr.UpstreamCloseReason = closeReason
				}
				return result, attemptErr
			}
			if read.messageType != websocket.TextMessage || !gjson.ValidBytes(read.payload) || !gjson.ParseBytes(read.payload).IsObject() {
				proxyErr := domain.NewProxyErrorWithMessage(domain.ErrResponsesWebSocketProtocol, false, "invalid Codex websocket application frame")
				proxyErr.Scope = domain.ScopeProvider
				proxyErr.Code = "websocket_protocol_error"
				proxyErr.HTTPStatusCode = http.StatusBadGateway
				return result, &domain.ResponsesWebSocketAttemptError{
					Err:                         proxyErr,
					RequestFrameMayHaveBeenSent: true,
					FirstEventReceived:          result.FirstEventReceived,
					ClientEventSent:             result.ClientEventSent,
				}
			}

			result.FirstEventReceived = true
			if firstEvent {
				firstEvent = false
				if eventChan != nil {
					eventChan.SendFirstToken(time.Now().UnixMilli())
				}
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(codexResponsesWebSocketEventIdleTimeout)

			if err := sink.WriteTextFrame(read.payload); err != nil {
				return result, &domain.ResponsesWebSocketAttemptError{
					Err:                         err,
					RequestFrameMayHaveBeenSent: true,
					FirstEventReceived:          true,
					ClientEventSent:             result.ClientEventSent,
				}
			}
			result.ClientEventSent = true
			collector.ProcessSSEPayload(read.payload)
			if model := strings.TrimSpace(gjson.GetBytes(read.payload, "response.model").String()); model != "" {
				result.ResponseModel = model
			} else if model := strings.TrimSpace(gjson.GetBytes(read.payload, "model").String()); model != "" {
				result.ResponseModel = model
			}

			eventType := gjson.GetBytes(read.payload, "type").String()
			if !isCodexWebSocketTerminalEvent(eventType) {
				continue
			}
			result.TerminalEvent = eventType
			sendCodexWebSocketFinalEvents(eventChan, s.handshakeHeader, &collector, result.ResponseModel)
			if eventType == "response.completed" || eventType == "response.incomplete" {
				return result, nil
			}
			proxyErr := classifyCodexWebSocketEvent(read.payload, result.ResponseModel)
			result.TerminalErrorEventSent = true
			return result, &domain.ResponsesWebSocketAttemptError{
				Err:                         proxyErr,
				RequestFrameMayHaveBeenSent: true,
				FirstEventReceived:          true,
				ClientEventSent:             true,
				TerminalErrorEventSent:      true,
			}
		}
	}
}

func isCodexWebSocketTerminalEvent(eventType string) bool {
	switch eventType {
	case "response.completed", "response.failed", "response.incomplete", "error":
		return true
	default:
		return false
	}
}

func codexResponsesWebSocketURL(config *domain.ProviderConfigCodex) (string, error) {
	base := CodexBaseURL
	if config != nil && strings.TrimSpace(config.BaseURL) != "" {
		base = strings.TrimSpace(config.BaseURL)
	}
	return joinResponsesWebSocketURL(base)
}

func joinResponsesWebSocketURL(base string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", errors.New("websocket URL host is empty")
	}
	if pathpkg.Base(strings.TrimRight(parsed.Path, "/")) != "responses" {
		parsed.Path = pathpkg.Join(parsed.Path, "responses")
		if !strings.HasPrefix(parsed.Path, "/") {
			parsed.Path = "/" + parsed.Path
		}
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http":
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	case "ws", "wss":
	default:
		return "", fmt.Errorf("unsupported URL scheme %q", parsed.Scheme)
	}
	return parsed.String(), nil
}

func buildCodexWebSocketHeaders(c *flow.Ctx, accessToken, accountID string, frame []byte) http.Header {
	headers := make(http.Header)
	if c != nil && c.Request != nil {
		for key, values := range c.Request.Header {
			if isForbiddenCodexWebSocketHeader(key) {
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
	headers.Set("OpenAI-Beta", mergeCommaSeparatedHeader(headers.Get("OpenAI-Beta"), codexResponsesWebSocketBetaHeader))
	if c != nil {
		headers.Set("User-Agent", flow.ResolveUpstreamUserAgent(c, CodexUserAgent))
	}
	if promptCacheKey := strings.TrimSpace(gjson.GetBytes(frame, "prompt_cache_key").String()); promptCacheKey != "" {
		if headers.Get("Session_id") == "" {
			headers.Set("Session_id", promptCacheKey)
		}
		if headers.Get("Conversation_id") == "" {
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

func mergeCommaSeparatedHeader(existing, required string) string {
	parts := make([]string, 0, 4)
	seen := make(map[string]struct{})
	for _, value := range []string{existing, required} {
		for _, part := range strings.Split(value, ",") {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				continue
			}
			key := strings.ToLower(trimmed)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, ", ")
}

func isForbiddenCodexWebSocketHeader(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "authorization" || lower == "host" || lower == "accept" || lower == "content-type" ||
		lower == "content-length" || lower == "transfer-encoding" || lower == "connection" || lower == "upgrade" ||
		lower == "cookie" || lower == "origin" || lower == "x-api-key" || lower == "x-goog-api-key" || lower == "api-key" ||
		strings.HasPrefix(lower, "x-maxx-") ||
		strings.HasPrefix(lower, "sec-websocket-") {
		return true
	}
	return lower != "user-agent" && codexFilteredHeaders[lower]
}

func codexWebSocketFingerprint(providerID uint64, target string, headers http.Header) string {
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, strings.ToLower(key))
	}
	sort.Strings(keys)
	var builder strings.Builder
	fmt.Fprintf(&builder, "%d\n%s\n", providerID, target)
	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteByte(':')
		builder.WriteString(strings.Join(headers.Values(key), "\x00"))
		builder.WriteByte('\n')
	}
	sum := sha256.Sum256([]byte(builder.String()))
	return fmt.Sprintf("%x", sum[:])
}

func dialCodexWebSocket(
	ctx context.Context,
	target string,
	headers http.Header,
	fingerprint string,
	releaseProvider func(),
) (*codexWebSocketSession, *http.Response, error) {
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
	return newCodexWebSocketSession(conn, responseHeaders, fingerprint, releaseProvider), response, nil
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
		message = strings.TrimSpace(gjson.GetBytes(payload, "message").String())
	}
	if message == "" {
		message = "upstream websocket returned an error"
	}
	code := strings.TrimSpace(gjson.GetBytes(payload, "error.code").String())
	if code == "" {
		code = strings.TrimSpace(gjson.GetBytes(payload, "response.error.code").String())
	}
	if code == "" {
		code = strings.TrimSpace(gjson.GetBytes(payload, "code").String())
	}
	errorType := strings.ToLower(strings.Join([]string{
		gjson.GetBytes(payload, "error.type").String(),
		gjson.GetBytes(payload, "response.error.type").String(),
		gjson.GetBytes(payload, "type").String(),
		code,
	}, " "))
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
