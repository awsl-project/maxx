package codex

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
)

type recordingWebSocketSink struct {
	mu     sync.Mutex
	frames [][]byte
}

type failingWebSocketSink struct{ err error }

func (s failingWebSocketSink) WriteTextFrame([]byte) error { return s.err }

func (s *recordingWebSocketSink) WriteTextFrame(payload []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.frames = append(s.frames, bytes.Clone(payload))
	return nil
}

func TestExecuteResponsesWebSocket_PreservesOfficialWirePayload(t *testing.T) {
	clearCodexWebSocketUnsupportedForTests()
	var connectionCount atomic.Int32
	requests := make(chan []byte, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("path = %q, want /v1/responses", r.URL.Path)
		}
		if got := r.Header.Get("OpenAI-Beta"); !strings.Contains(got, codexResponsesWebSocketBetaHeader) {
			t.Errorf("OpenAI-Beta = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != "codex-test/1.0" {
			t.Errorf("User-Agent = %q", got)
		}
		conn, err := websocket.Upgrade(w, r, nil, 4096, 4096)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		connectionCount.Add(1)
		for turn := 1; turn <= 2; turn++ {
			_, payload, errRead := conn.ReadMessage()
			if errRead != nil {
				t.Errorf("read turn %d: %v", turn, errRead)
				return
			}
			requests <- payload
			responseID := "resp_" + string(rune('0'+turn))
			completed := `{"type":"response.completed","response":{"id":"` + responseID + `","model":"gpt-test","output":[],"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}}`
			if errWrite := conn.WriteMessage(websocket.TextMessage, []byte(completed)); errWrite != nil {
				t.Errorf("write turn %d: %v", turn, errWrite)
				return
			}
		}
	}))
	defer server.Close()

	provider := &domain.Provider{
		ID:   42,
		Type: "codex",
		Config: &domain.ProviderConfig{Codex: &domain.ProviderConfigCodex{
			AccessToken: "test-token",
			BaseURL:     server.URL + "/v1",
		}},
	}
	adapter := &CodexAdapter{
		provider:   provider,
		tokenCache: &TokenCache{AccessToken: "test-token"},
		httpClient: newUpstreamHTTPClient(),
	}
	connectionID := uuid.NewString()
	t.Cleanup(func() { adapter.CloseResponsesWebSocketConnection(connectionID) })
	sink := &recordingWebSocketSink{}

	firstRaw := []byte(`{"type":"response.create","model":"gpt-test","generate":false,"stream":true,"store":true,"background":true,"stream_options":{"include_usage":true},"client_metadata":{"source":"test"},"unknown_field":"preserve","input":[]}`)
	firstCtx := newCodexWebSocketTestContext(t)
	firstExchange := &domain.ResponsesWebSocketExchange{
		ConnectionID: connectionID,
		Frame:        firstRaw,
		Sink:         sink,
	}
	result, err := adapter.ExecuteResponsesWebSocket(firstCtx, provider, firstExchange)
	if err != nil {
		t.Fatalf("first execute: %v", err)
	}
	if result.Reused {
		t.Fatal("first turn unexpectedly reused a connection")
	}
	firstUpstream := <-requests
	if !bytes.Equal(firstUpstream, firstRaw) {
		t.Fatalf("upstream frame mutated:\n got %s\nwant %s", firstUpstream, firstRaw)
	}
	for _, field := range []string{"stream", "store", "generate", "stream_options", "client_metadata", "unknown_field"} {
		if !gjson.GetBytes(firstUpstream, field).Exists() {
			t.Fatalf("field %q was removed: %s", field, firstUpstream)
		}
	}

	secondRaw := []byte(`{"type":"response.create","model":"gpt-test","previous_response_id":"resp_1","generate":true,"input":[{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`)
	secondExchange := &domain.ResponsesWebSocketExchange{
		ConnectionID:       connectionID,
		Frame:              secondRaw,
		PreviousResponseID: "resp_1",
		PinnedProviderID:   provider.ID,
		Sink:               sink,
	}
	result, err = adapter.ExecuteResponsesWebSocket(newCodexWebSocketTestContext(t), provider, secondExchange)
	if err != nil {
		t.Fatalf("second execute: %v", err)
	}
	if !result.Reused {
		t.Fatal("continuation did not reuse the upstream connection")
	}
	secondUpstream := <-requests
	if !bytes.Equal(secondUpstream, secondRaw) {
		t.Fatalf("continuation frame mutated:\n got %s\nwant %s", secondUpstream, secondRaw)
	}
	if connectionCount.Load() != 1 {
		t.Fatalf("connections = %d, want 1", connectionCount.Load())
	}
	if len(sink.frames) != 2 {
		t.Fatalf("forwarded frames = %d, want 2", len(sink.frames))
	}
}

func TestExecuteResponsesWebSocket_MissingContinuationSessionFails(t *testing.T) {
	provider := &domain.Provider{
		ID: 7,
		Config: &domain.ProviderConfig{Codex: &domain.ProviderConfigCodex{
			AccessToken: "static-token",
			BaseURL:     "https://example.test/v1",
		}},
	}
	adapter := &CodexAdapter{provider: provider, tokenCache: &TokenCache{AccessToken: "static-token"}}
	exchange := &domain.ResponsesWebSocketExchange{
		ConnectionID:       uuid.NewString(),
		Frame:              []byte(`{"type":"response.create","model":"gpt-test","previous_response_id":"resp_missing","input":[]}`),
		PreviousResponseID: "resp_missing",
		PinnedProviderID:   provider.ID,
		Sink:               &recordingWebSocketSink{},
	}
	_, err := adapter.ExecuteResponsesWebSocket(newCodexWebSocketTestContext(t), provider, exchange)
	if !errors.Is(err, domain.ErrResponsesWebSocketSessionUnavailable) {
		t.Fatalf("error = %v, want session unavailable", err)
	}
}

func TestExecuteResponsesWebSocket_StaticToken401DoesNotGeneratePlaceholder(t *testing.T) {
	clearCodexWebSocketUnsupportedForTests()
	var handshakes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handshakes.Add(1)
		if got := r.Header.Get("Authorization"); got != "Bearer static-token" {
			t.Errorf("Authorization = %q", got)
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	provider := &domain.Provider{ID: 81, Config: &domain.ProviderConfig{Codex: &domain.ProviderConfigCodex{
		AccessToken: "static-token",
		BaseURL:     server.URL + "/v1",
	}}}
	adapter := &CodexAdapter{provider: provider, tokenCache: &TokenCache{AccessToken: "static-token"}}
	exchange := &domain.ResponsesWebSocketExchange{
		ConnectionID: uuid.NewString(),
		Frame:        []byte(`{"type":"response.create","model":"gpt-test","input":[]}`),
		Sink:         &recordingWebSocketSink{},
	}
	_, err := adapter.ExecuteResponsesWebSocket(newCodexWebSocketTestContext(t), provider, exchange)
	if err == nil {
		t.Fatal("expected unauthorized handshake error")
	}
	if handshakes.Load() != 1 {
		t.Fatalf("handshakes = %d, want 1 without refresh retry", handshakes.Load())
	}
	if got := adapter.tokenCache.AccessToken; got != "static-token" || isFallbackCodexAccessToken(got) {
		t.Fatalf("token cache changed to %q", got)
	}
	var wsErr *domain.ResponsesWebSocketAttemptError
	if !errors.As(err, &wsErr) || wsErr.RequestFrameMayHaveBeenSent {
		t.Fatalf("attempt error = %#v, want pre-write failure", wsErr)
	}
	var proxyErr *domain.ProxyError
	if !errors.As(err, &proxyErr) || proxyErr.HTTPStatusCode != http.StatusUnauthorized || proxyErr.Scope != domain.ScopeKey {
		t.Fatalf("proxy error = %#v, want 401 key scope", proxyErr)
	}
}

func TestExecuteResponsesWebSocket_Upgrade404IsCachedSafeBeforeWrite(t *testing.T) {
	clearCodexWebSocketUnsupportedForTests()
	var handshakes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handshakes.Add(1)
		http.NotFound(w, r)
	}))
	defer server.Close()

	provider := &domain.Provider{ID: 82, Config: &domain.ProviderConfig{Codex: &domain.ProviderConfigCodex{
		AccessToken: "static-token",
		BaseURL:     server.URL + "/v1",
	}}}
	adapter := &CodexAdapter{provider: provider, tokenCache: &TokenCache{AccessToken: "static-token"}}
	for turn := 0; turn < 2; turn++ {
		exchange := &domain.ResponsesWebSocketExchange{
			ConnectionID: uuid.NewString(),
			Frame:        []byte(`{"type":"response.create","model":"gpt-test","input":[]}`),
			Sink:         &recordingWebSocketSink{},
		}
		_, err := adapter.ExecuteResponsesWebSocket(newCodexWebSocketTestContext(t), provider, exchange)
		var wsErr *domain.ResponsesWebSocketAttemptError
		if !errors.As(err, &wsErr) || !wsErr.CapabilityFailure || !wsErr.CanTryNextProvider() {
			t.Fatalf("turn %d error = %#v, want cached safe capability failure", turn, wsErr)
		}
	}
	if handshakes.Load() != 1 {
		t.Fatalf("handshakes = %d, want 1 after unsupported cache hit", handshakes.Load())
	}
}

func TestExecuteResponsesWebSocket_ResponseIncompleteTerminates(t *testing.T) {
	clearCodexWebSocketUnsupportedForTests()
	incomplete := []byte(`{"type":"response.incomplete","response":{"id":"resp_incomplete","model":"gpt-test","output":[]}}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Upgrade(w, r, nil, 4096, 4096)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		if _, _, err = conn.ReadMessage(); err != nil {
			t.Errorf("read request: %v", err)
			return
		}
		if err = conn.WriteMessage(websocket.TextMessage, incomplete); err != nil {
			t.Errorf("write incomplete: %v", err)
		}
	}))
	defer server.Close()

	provider := &domain.Provider{ID: 83, Config: &domain.ProviderConfig{Codex: &domain.ProviderConfigCodex{
		AccessToken: "static-token",
		BaseURL:     server.URL + "/v1",
	}}}
	adapter := &CodexAdapter{provider: provider, tokenCache: &TokenCache{AccessToken: "static-token"}}
	connectionID := uuid.NewString()
	t.Cleanup(func() { adapter.CloseResponsesWebSocketConnection(connectionID) })
	sink := &recordingWebSocketSink{}
	result, err := adapter.ExecuteResponsesWebSocket(newCodexWebSocketTestContext(t), provider, &domain.ResponsesWebSocketExchange{
		ConnectionID: connectionID,
		Frame:        []byte(`{"type":"response.create","model":"gpt-test","input":[]}`),
		Sink:         sink,
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.TerminalEvent != "response.incomplete" {
		t.Fatalf("terminal event = %q", result.TerminalEvent)
	}
	if len(sink.frames) != 1 || !bytes.Equal(sink.frames[0], incomplete) {
		t.Fatalf("forwarded frames = %q, want raw incomplete frame", sink.frames)
	}
}

func TestExecuteResponsesWebSocket_SinkWriteErrorIsNotReplayable(t *testing.T) {
	clearCodexWebSocketUnsupportedForTests()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Upgrade(w, r, nil, 4096, 4096)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		if _, _, err = conn.ReadMessage(); err != nil {
			return
		}
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"model":"gpt-test","output":[]}}`))
	}))
	defer server.Close()

	provider := &domain.Provider{ID: 84, Config: &domain.ProviderConfig{Codex: &domain.ProviderConfigCodex{
		AccessToken: "static-token",
		BaseURL:     server.URL + "/v1",
	}}}
	adapter := &CodexAdapter{provider: provider, tokenCache: &TokenCache{AccessToken: "static-token"}}
	sinkErr := errors.New("downstream write failed")
	_, err := adapter.ExecuteResponsesWebSocket(newCodexWebSocketTestContext(t), provider, &domain.ResponsesWebSocketExchange{
		ConnectionID: uuid.NewString(),
		Frame:        []byte(`{"type":"response.create","model":"gpt-test","input":[]}`),
		Sink:         failingWebSocketSink{err: sinkErr},
	})
	var wsErr *domain.ResponsesWebSocketAttemptError
	if !errors.As(err, &wsErr) || !errors.Is(err, sinkErr) {
		t.Fatalf("error = %#v, want downstream write error", err)
	}
	if wsErr.CanTryNextProvider() || !wsErr.RequestFrameMayHaveBeenSent || !wsErr.FirstEventReceived {
		t.Fatalf("attempt flags = %#v, want committed non-replayable failure", wsErr)
	}
}

func TestCodexResponsesWebSocketURL(t *testing.T) {
	tests := map[string]string{
		"https://host.example/v1":               "wss://host.example/v1/responses",
		"https://host.example/v1/responses":     "wss://host.example/v1/responses",
		"https://chatgpt.com/backend-api/codex": "wss://chatgpt.com/backend-api/codex/responses",
		"ws://host.example/api?source=test":     "ws://host.example/api/responses?source=test",
	}
	for base, want := range tests {
		t.Run(base, func(t *testing.T) {
			got, err := joinResponsesWebSocketURL(base)
			if err != nil {
				t.Fatalf("join URL: %v", err)
			}
			if got != want {
				t.Fatalf("URL = %q, want %q", got, want)
			}
		})
	}
}

func TestBuildCodexWebSocketHeaders_StripsHopByHopAndMergesBeta(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses", nil)
	request.Header.Set("Authorization", "Bearer client-token")
	request.Header.Set("Connection", "upgrade")
	request.Header.Set("Sec-WebSocket-Key", "client-key")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("OpenAI-Beta", "feature=1")
	request.Header.Set("User-Agent", "codex-test/1.0")
	request.Header.Set("X-OpenAI-Internal-Codex-Responses-Lite", "true")
	request.Header.Set("Cookie", "session=client-secret")
	request.Header.Set("Origin", "https://client.example")
	request.Header.Set("X-Api-Key", "client-api-key")
	request.Header.Set("X-Goog-Api-Key", "client-google-key")
	request.Header.Set("Api-Key", "client-azure-key")
	request.Header.Set("X-Maxx-Route", "internal-route")
	c := flow.NewCtx(httptest.NewRecorder(), request)

	headers := buildCodexWebSocketHeaders(c, "provider-token", "account-1", []byte(`{"prompt_cache_key":"session-1"}`))
	if got := headers.Get("Authorization"); got != "Bearer provider-token" {
		t.Fatalf("Authorization = %q", got)
	}
	for _, key := range []string{
		"Connection", "Sec-WebSocket-Key", "Content-Type", "Cookie", "Origin",
		"X-Api-Key", "X-Goog-Api-Key", "Api-Key", "X-Maxx-Route",
	} {
		if got := headers.Get(key); got != "" {
			t.Fatalf("forbidden %s = %q", key, got)
		}
	}
	if got := headers.Get("OpenAI-Beta"); !strings.Contains(got, "feature=1") || !strings.Contains(got, codexResponsesWebSocketBetaHeader) {
		t.Fatalf("OpenAI-Beta = %q", got)
	}
	if got := headers.Get("Session_id"); got != "session-1" {
		t.Fatalf("Session_id = %q", got)
	}
	if got := headers.Get("X-OpenAI-Internal-Codex-Responses-Lite"); got != "true" {
		t.Fatalf("Responses Lite header = %q", got)
	}
}

func TestCodexWebSocketSessionStore_DoesNotEvictActiveSession(t *testing.T) {
	store := &codexWebSocketSessionStore{
		sessions:   make(map[codexWebSocketSessionKey]*codexWebSocketSession),
		maxEntries: 1,
	}
	active := newStoredCodexWebSocketSession(time.Now().Add(-time.Hour), 1)
	activeKey := codexWebSocketSessionKey{ConnectionID: "active", ProviderID: 1}
	store.sessions[activeKey] = active
	newSession := newStoredCodexWebSocketSession(time.Now(), 1)
	if store.put(codexWebSocketSessionKey{ConnectionID: "new", ProviderID: 2}, newSession) {
		t.Fatal("put succeeded while every stored session was active")
	}
	if store.sessions[activeKey] != active || active.isClosed() {
		t.Fatal("active session was evicted or closed")
	}
	if len(store.sessions) != 1 {
		t.Fatalf("stored sessions = %d, want 1", len(store.sessions))
	}
}

func TestCodexWebSocketSessionStore_PrunesOnlyIdleSessions(t *testing.T) {
	store := &codexWebSocketSessionStore{
		sessions:   make(map[codexWebSocketSessionKey]*codexWebSocketSession),
		maxEntries: 2,
	}
	now := time.Now()
	idle := newStoredCodexWebSocketSession(now.Add(-codexResponsesWebSocketSessionIdleTimeout-time.Minute), 0)
	active := newStoredCodexWebSocketSession(now.Add(-codexResponsesWebSocketSessionIdleTimeout-time.Minute), 1)
	store.sessions[codexWebSocketSessionKey{ConnectionID: "idle", ProviderID: 1}] = idle
	store.sessions[codexWebSocketSessionKey{ConnectionID: "active", ProviderID: 2}] = active

	store.pruneIdle(now)

	if !idle.isClosed() {
		t.Fatal("idle session was not closed")
	}
	if active.isClosed() {
		t.Fatal("active session was closed by idle pruning")
	}
	if len(store.sessions) != 1 {
		t.Fatalf("stored sessions = %d, want active session only", len(store.sessions))
	}
}

func TestClassifyCodexWebSocketEvent_RootQuotaError(t *testing.T) {
	proxyErr := classifyCodexWebSocketEvent(
		[]byte(`{"type":"error","code":"insufficient_quota","message":"quota exhausted"}`),
		"gpt-test",
	)
	if proxyErr.HTTPStatusCode != http.StatusTooManyRequests || proxyErr.Scope != domain.ScopeKey ||
		proxyErr.Reason != domain.CooldownReasonRateLimitExceeded || proxyErr.Message != "quota exhausted" {
		t.Fatalf("classified error = %#v", proxyErr)
	}
}

func newStoredCodexWebSocketSession(lastUsed time.Time, activeTurns int32) *codexWebSocketSession {
	session := &codexWebSocketSession{done: make(chan struct{})}
	session.lastUsedAt.Store(lastUsed.UnixNano())
	session.activeTurns.Store(activeTurns)
	return session
}

func newCodexWebSocketTestContext(t *testing.T) *flow.Ctx {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses", nil).WithContext(context.Background())
	request.Header.Set("User-Agent", "codex-test/1.0")
	c := flow.NewCtx(httptest.NewRecorder(), request)
	c.Set(flow.KeyClientType, domain.ClientTypeCodex)
	c.Set(flow.KeyOriginalClientType, domain.ClientTypeCodex)
	c.Set(flow.KeyMappedModel, "gpt-test")
	c.Set(flow.KeyIsStream, true)
	return c
}
