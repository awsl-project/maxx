package custom

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	pathpkg "path"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/usage"
	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
)

// Custom providers that natively declare Codex support (SupportedClientTypes)
// must be able to dial the upstream Responses WebSocket. Production configs
// often use type=custom with clientBaseURL.codex pointing at Codex-compatible
// gateways; without this path Maxx correctly rejects WS with
// websocket_transport_unavailable even though HTTP /v1/responses works.

const (
	customResponsesWebSocketBetaHeader         = "responses_websockets=2026-02-06"
	customResponsesWebSocketHandshake          = 30 * time.Second
	customResponsesWebSocketReadQueueSize      = 256
	customResponsesWebSocketMaxQueuedBytes     = 8 << 20
	customResponsesWebSocketMaxFrameBytes      = 32 << 20
	customResponsesWebSocketWriteTimeout       = 30 * time.Second
	customResponsesWebSocketFirstEventTimeout  = 20 * time.Second
	customResponsesWebSocketEventIdleTimeout   = 5 * time.Minute
	customResponsesWebSocketSessionIdleTimeout = 65 * time.Minute
	customResponsesWebSocketMaxSessions        = 512
	customCodexUserAgent                       = "codex_cli_rs/0.0.0"
)

type customWebSocketRead struct {
	messageType int
	payload     []byte
	err         error
}

type customWebSocketSession struct {
	requestMu sync.Mutex
	writeMu   sync.Mutex
	closeOnce sync.Once

	conn            *websocket.Conn
	handshakeHeader http.Header
	reads           chan customWebSocketRead
	done            chan struct{}
	queuedBytes     atomic.Int64
	maxQueuedBytes  int64
	fingerprint     string
	lastUsedAt      atomic.Int64
	activeTurns     atomic.Int32
}

type customWebSocketSessionKey struct {
	ConnectionID string
	ProviderID   uint64
}

type customWebSocketSessionStore struct {
	mu         sync.Mutex
	sessions   map[customWebSocketSessionKey]*customWebSocketSession
	maxEntries int
}

var globalCustomWebSocketSessions = &customWebSocketSessionStore{
	sessions:   make(map[customWebSocketSessionKey]*customWebSocketSession),
	maxEntries: customResponsesWebSocketMaxSessions,
}

func newCustomWebSocketSession(conn *websocket.Conn, handshakeHeader http.Header, fingerprint string) *customWebSocketSession {
	session := &customWebSocketSession{
		conn:            conn,
		handshakeHeader: handshakeHeader.Clone(),
		reads:           make(chan customWebSocketRead, customResponsesWebSocketReadQueueSize),
		done:            make(chan struct{}),
		maxQueuedBytes:  customResponsesWebSocketMaxQueuedBytes,
		fingerprint:     fingerprint,
	}
	session.touch()
	conn.SetReadLimit(customResponsesWebSocketMaxFrameBytes)
	conn.SetPingHandler(func(data string) error {
		return conn.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(10*time.Second))
	})
	go session.readLoop()
	return session
}

func (s *customWebSocketSession) touch() {
	s.lastUsedAt.Store(time.Now().UnixNano())
}

func (s *customWebSocketSession) lastUsed() time.Time {
	return time.Unix(0, s.lastUsedAt.Load())
}

func (s *customWebSocketSession) isClosed() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

func (s *customWebSocketSession) close() {
	s.closeOnce.Do(func() {
		close(s.done)
		_ = s.conn.Close()
	})
}

func (s *customWebSocketSession) enqueueError(err error) {
	select {
	case s.reads <- customWebSocketRead{err: err}:
	case <-s.done:
	default:
	}
}

func (s *customWebSocketSession) readLoop() {
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
			s.enqueueError(errors.New("custom websocket read queue byte limit exceeded"))
			s.close()
			return
		}
		select {
		case s.reads <- customWebSocketRead{messageType: messageType, payload: payload}:
		case <-s.done:
			s.queuedBytes.Add(-size)
			return
		default:
			s.queuedBytes.Add(-size)
			s.enqueueError(errors.New("custom websocket read queue is full"))
			s.close()
			return
		}
	}
}

func (s *customWebSocketSession) write(frame []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.conn.SetWriteDeadline(time.Now().Add(customResponsesWebSocketWriteTimeout)); err != nil {
		return err
	}
	return s.conn.WriteMessage(websocket.TextMessage, frame)
}

func (st *customWebSocketSessionStore) get(key customWebSocketSessionKey) *customWebSocketSession {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.evictLocked(time.Now())
	session := st.sessions[key]
	if session == nil || session.isClosed() {
		if session != nil {
			delete(st.sessions, key)
		}
		return nil
	}
	session.activeTurns.Add(1)
	return session
}

func (st *customWebSocketSessionStore) put(key customWebSocketSessionKey, session *customWebSocketSession) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.evictLocked(time.Now())
	if len(st.sessions) >= st.maxEntries {
		return false
	}
	if existing := st.sessions[key]; existing != nil && existing != session {
		existing.close()
	}
	st.sessions[key] = session
	return true
}

func (st *customWebSocketSessionStore) remove(key customWebSocketSessionKey, session *customWebSocketSession) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if current := st.sessions[key]; current == session {
		delete(st.sessions, key)
	}
	session.close()
}

func (st *customWebSocketSessionStore) closeForConnection(connectionID string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for key, session := range st.sessions {
		if key.ConnectionID == connectionID {
			delete(st.sessions, key)
			session.close()
		}
	}
}

func (st *customWebSocketSessionStore) evictLocked(now time.Time) {
	for key, session := range st.sessions {
		if session.isClosed() ||
			(session.activeTurns.Load() == 0 && now.Sub(session.lastUsed()) >= customResponsesWebSocketSessionIdleTimeout) {
			delete(st.sessions, key)
			session.close()
		}
	}
}

// CloseResponsesWebSocketConnection implements provider.ResponsesWebSocketSessionCleaner.
func (a *CustomAdapter) CloseResponsesWebSocketConnection(connectionID string) {
	globalCustomWebSocketSessions.closeForConnection(connectionID)
}

// ExecuteResponsesWebSocket implements provider.ResponsesWebSocketAdapter for
// custom providers that natively support the Codex client type.
func (a *CustomAdapter) ExecuteResponsesWebSocket(
	c *flow.Ctx,
	provider *domain.Provider,
	exchange *domain.ResponsesWebSocketExchange,
) (*domain.ResponsesWebSocketResult, error) {
	if c == nil || provider == nil || c.Request == nil || exchange == nil || exchange.Sink == nil {
		return nil, newCustomWebSocketAttemptError(domain.ErrResponsesWebSocketProtocol, false)
	}
	if !customSupportsCodex(provider) || !domain.ProviderResponsesWebSocketEnabled(provider) {
		proxyErr := domain.NewProxyErrorWithMessage(
			domain.ErrNoResponsesWebSocketProviders,
			true,
			"custom provider does not enable Codex Responses WebSocket",
		)
		proxyErr.Scope = domain.ScopeEndpoint
		proxyErr.Code = "websocket_not_supported"
		proxyErr.HTTPStatusCode = http.StatusServiceUnavailable
		return nil, newCustomWebSocketAttemptError(proxyErr, true)
	}
	if err := validateOutboundCustomCodexWebSocketFrame(exchange.Frame); err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(err, false, "invalid Codex websocket request")
		proxyErr.Scope = domain.ScopeRequest
		proxyErr.Code = "websocket_protocol_error"
		proxyErr.HTTPStatusCode = http.StatusBadRequest
		return nil, newCustomWebSocketAttemptError(proxyErr, false)
	}

	cfg := a.provider.Config
	if provider.Config != nil && provider.Config.Custom != nil {
		cfg = provider.Config
	}
	if cfg == nil || cfg.Custom == nil {
		proxyErr := domain.NewProxyErrorWithMessage(domain.ErrInvalidInput, false, "custom provider config missing")
		proxyErr.Scope = domain.ScopeEndpoint
		return nil, newCustomWebSocketAttemptError(proxyErr, true)
	}

	baseURL := customCodexBaseURL(cfg.Custom)
	target, err := joinCustomResponsesWebSocketURL(baseURL)
	if err != nil {
		proxyErr := domain.NewProxyErrorWithMessage(err, true, "failed to build custom Codex websocket URL")
		proxyErr.Scope = domain.ScopeEndpoint
		return nil, newCustomWebSocketAttemptError(proxyErr, false)
	}
	apiKey := strings.TrimSpace(cfg.Custom.APIKey)
	if apiKey == "" {
		proxyErr := domain.NewProxyErrorWithMessage(errors.New("missing custom API key"), false, "custom provider API key is required for websocket")
		proxyErr.Scope = domain.ScopeKey
		proxyErr.Reason = domain.CooldownReasonAuthFailure
		return nil, newCustomWebSocketAttemptError(proxyErr, false)
	}

	ctx := c.Request.Context()
	headers := buildCustomCodexWebSocketHeaders(c, apiKey, exchange.Frame)
	fingerprint := customWebSocketFingerprint(provider.ID, target, headers)
	key := customWebSocketSessionKey{ConnectionID: exchange.ConnectionID, ProviderID: provider.ID}
	session := globalCustomWebSocketSessions.get(key)
	if session != nil && session.fingerprint != fingerprint {
		session.activeTurns.Add(-1)
		globalCustomWebSocketSessions.remove(key, session)
		session = nil
	}
	if session == nil && exchange.PreviousResponseID != "" {
		return nil, newCustomWebSocketAttemptError(domain.ErrResponsesWebSocketSessionUnavailable, false)
	}

	reused := session != nil
	if session == nil {
		session, err = dialCustomResponsesWebSocket(ctx, target, headers, fingerprint)
		if err != nil {
			proxyErr := domain.NewProxyErrorWithMessage(err, true, "failed to upgrade custom Codex websocket connection")
			proxyErr.Scope = domain.ScopeProvider
			proxyErr.Reason = domain.CooldownReasonNetworkError
			proxyErr.Code = "upstream_websocket_upgrade_rejected"
			proxyErr.HTTPStatusCode = http.StatusBadGateway
			log.Printf("[CustomWS] provider=%d dial %s failed: %v", provider.ID, target, err)
			return nil, newCustomWebSocketAttemptError(proxyErr, false)
		}
		session.activeTurns.Add(1)
		if !globalCustomWebSocketSessions.put(key, session) {
			session.activeTurns.Add(-1)
			session.close()
			proxyErr := domain.NewProxyErrorWithMessage(errors.New("custom websocket session capacity reached"), true, "custom websocket session capacity reached")
			proxyErr.Scope = domain.ScopeRequest
			proxyErr.Code = "upstream_websocket_session_capacity"
			proxyErr.HTTPStatusCode = http.StatusServiceUnavailable
			return nil, newCustomWebSocketAttemptError(proxyErr, false)
		}
		log.Printf("[CustomWS] provider=%d dialed %s", provider.ID, target)
	}

	result, execErr := session.executeTurn(ctx, exchange.Frame, exchange.Sink, flow.GetEventChan(c), provider.ID)
	if result != nil {
		result.ProviderID = provider.ID
		result.Reused = reused
	}
	if execErr != nil {
		var wsErr *domain.ResponsesWebSocketAttemptError
		if errors.As(execErr, &wsErr) && (wsErr.RequestFrameMayHaveBeenSent || session.isClosed()) {
			globalCustomWebSocketSessions.remove(key, session)
		}
		return result, execErr
	}
	return result, nil
}

func (s *customWebSocketSession) executeTurn(
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
		return result, newCustomWebSocketAttemptError(errors.New("custom websocket session is closed"), false)
	}
	if eventChan != nil {
		eventChan.SendRequestInfo(&domain.RequestInfo{
			Method:  "WEBSOCKET",
			Headers: flattenCustomHeaders(s.handshakeHeader),
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
	timer := time.NewTimer(customResponsesWebSocketFirstEventTimeout)
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
			message := "custom Codex websocket first event timeout"
			if result.FirstEventReceived {
				code = "upstream_websocket_closed_before_terminal"
				message = "custom Codex websocket event idle timeout"
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
				message := "custom Codex websocket closed before terminal event"
				var closeErr *websocket.CloseError
				if errors.As(readErr, &closeErr) && strings.TrimSpace(closeErr.Text) != "" {
					message = "custom Codex websocket closed: " + strings.TrimSpace(closeErr.Text)
				}
				proxyErr := domain.NewProxyErrorWithMessage(readErr, false, message)
				proxyErr.Scope = domain.ScopeProvider
				proxyErr.Reason = domain.CooldownReasonNetworkError
				proxyErr.Code = "upstream_websocket_closed_before_terminal"
				proxyErr.HTTPStatusCode = http.StatusBadGateway
				return result, &domain.ResponsesWebSocketAttemptError{
					Err:                         proxyErr,
					RequestFrameMayHaveBeenSent: true,
					FirstEventReceived:          result.FirstEventReceived,
					ClientEventSent:             result.ClientEventSent,
				}
			}
			if read.messageType != websocket.TextMessage || !gjson.ValidBytes(read.payload) || !gjson.ParseBytes(read.payload).IsObject() {
				proxyErr := domain.NewProxyErrorWithMessage(domain.ErrResponsesWebSocketProtocol, false, "invalid custom Codex websocket application frame")
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
			timer.Reset(customResponsesWebSocketEventIdleTimeout)

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
			if !isCustomCodexWebSocketTerminalEvent(eventType) {
				continue
			}
			result.TerminalEvent = eventType
			sendCustomWebSocketFinalEvents(eventChan, s.handshakeHeader, &collector, result.ResponseModel)
			if eventType == "response.completed" || eventType == "response.incomplete" {
				return result, nil
			}
			proxyErr := domain.NewProxyErrorWithMessage(errors.New(eventType), false, "upstream Codex websocket terminal error")
			proxyErr.Scope = domain.ScopeProvider
			proxyErr.Code = "upstream_websocket_error"
			proxyErr.HTTPStatusCode = http.StatusBadGateway
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

func newCustomWebSocketAttemptError(err error, capabilityFailure bool) *domain.ResponsesWebSocketAttemptError {
	return &domain.ResponsesWebSocketAttemptError{
		Err:               err,
		CapabilityFailure: capabilityFailure,
	}
}

func customSupportsCodex(provider *domain.Provider) bool {
	return domain.ProviderNativelySupports(provider, domain.ClientTypeCodex)
}

func customCodexBaseURL(cfg *domain.ProviderConfigCustom) string {
	if cfg == nil {
		return ""
	}
	if url, ok := cfg.ClientBaseURL[domain.ClientTypeCodex]; ok && strings.TrimSpace(url) != "" {
		return strings.TrimSpace(url)
	}
	return strings.TrimSpace(cfg.BaseURL)
}

func joinCustomResponsesWebSocketURL(base string) (string, error) {
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

func buildCustomCodexWebSocketHeaders(c *flow.Ctx, apiKey string, frame []byte) http.Header {
	headers := make(http.Header)
	if c != nil && c.Request != nil {
		for key, values := range c.Request.Header {
			if isForbiddenCustomWebSocketHeader(key) {
				continue
			}
			for _, value := range values {
				headers.Add(key, value)
			}
		}
	}
	headers.Set("Authorization", "Bearer "+apiKey)
	headers.Set("OpenAI-Beta", mergeCommaSeparatedHeader(headers.Get("OpenAI-Beta"), customResponsesWebSocketBetaHeader))
	if c != nil {
		headers.Set("User-Agent", flow.ResolveUpstreamUserAgent(c, customCodexUserAgent))
	}
	if promptCacheKey := strings.TrimSpace(gjson.GetBytes(frame, "prompt_cache_key").String()); promptCacheKey != "" {
		if headers.Get("Session_id") == "" {
			headers.Set("Session_id", promptCacheKey)
		}
		if headers.Get("Conversation_id") == "" {
			headers.Set("Conversation_id", promptCacheKey)
		}
	}
	return headers
}

func isForbiddenCustomWebSocketHeader(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "authorization" || lower == "host" || lower == "accept" || lower == "content-type" ||
		lower == "content-length" || lower == "transfer-encoding" || lower == "connection" || lower == "upgrade" ||
		lower == "cookie" || lower == "origin" || lower == "x-api-key" || lower == "api-key" ||
		strings.HasPrefix(lower, "x-maxx-") ||
		strings.HasPrefix(lower, "sec-websocket-") {
		return true
	}
	return false
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

func customWebSocketFingerprint(providerID uint64, target string, headers http.Header) string {
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

func dialCustomResponsesWebSocket(ctx context.Context, target string, headers http.Header, fingerprint string) (*customWebSocketSession, error) {
	dialer := &websocket.Dialer{
		Proxy:             http.ProxyFromEnvironment,
		HandshakeTimeout:  customResponsesWebSocketHandshake,
		EnableCompression: true,
		NetDialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}
	conn, response, err := dialer.DialContext(ctx, target, headers)
	if err != nil {
		if response != nil && response.Body != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
			_ = response.Body.Close()
		}
		return nil, err
	}
	responseHeaders := make(http.Header)
	if response != nil {
		responseHeaders = response.Header.Clone()
		if response.Body != nil {
			_ = response.Body.Close()
		}
	}
	return newCustomWebSocketSession(conn, responseHeaders, fingerprint), nil
}

func validateOutboundCustomCodexWebSocketFrame(frame []byte) error {
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

func isCustomCodexWebSocketTerminalEvent(eventType string) bool {
	switch eventType {
	case "response.completed", "response.failed", "response.incomplete", "error":
		return true
	default:
		return false
	}
}

func flattenCustomHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for key, values := range h {
		if len(values) > 0 {
			out[key] = values[0]
		}
	}
	return out
}

func sendCustomWebSocketFinalEvents(
	eventChan domain.AdapterEventChan,
	headers http.Header,
	collector *usage.StreamCollector,
	model string,
) {
	if eventChan == nil {
		return
	}
	eventChan.SendResponseInfo(&domain.ResponseInfo{
		Status:  http.StatusSwitchingProtocols,
		Headers: flattenCustomHeaders(headers),
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
