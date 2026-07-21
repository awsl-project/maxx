package handler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
)

func TestWriteResponsesWebSocketUpgradeRequired(t *testing.T) {
	rec := httptest.NewRecorder()
	writeResponsesWebSocketUpgradeRequired(rec)
	if rec.Code != http.StatusUpgradeRequired {
		t.Fatalf("status = %d, want 426", rec.Code)
	}
	body := rec.Body.String()
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"code":"websocket_not_supported"`)) {
		t.Fatalf("body = %s, want websocket_not_supported", body)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"fallback":"http_sse"`)) {
		t.Fatalf("body = %s, want fallback=http_sse", body)
	}
}

func TestResponsesWebSocketShouldForceReconnectForHTTPFallback(t *testing.T) {
	if !responsesWebSocketShouldForceReconnectForHTTPFallback(domain.ErrNoResponsesWebSocketProviders) {
		t.Fatal("expected force reconnect for no providers")
	}
	proxyErr := domain.NewProxyErrorWithMessage(domain.ErrNoResponsesWebSocketProviders, true, "no ws")
	proxyErr.Code = "websocket_not_supported"
	if !responsesWebSocketShouldForceReconnectForHTTPFallback(proxyErr) {
		t.Fatal("expected force reconnect for websocket_not_supported")
	}
	if responsesWebSocketShouldForceReconnectForHTTPFallback(errors.New("other")) {
		t.Fatal("did not expect force reconnect for unrelated error")
	}
}

func TestResponsesWebSocket_PreservesResponseCreateFrame(t *testing.T) {
	payload := []byte(`{"type":"response.create","model":"gpt-test","stream":true,"store":true,"generate":false,"previous_response_id":"resp_1","stream_options":{"include_usage":true},"client_metadata":{"source":"test"},"unknown":42,"input":[]}`)
	previous, err := validateResponsesWebSocketFrame(payload)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if previous != "resp_1" {
		t.Fatalf("previous response = %q", previous)
	}
	body, err := responsesWebSocketLogicalBody(payload)
	if err != nil {
		t.Fatalf("logical body: %v", err)
	}
	if gjson.GetBytes(body, "type").Exists() {
		t.Fatalf("logical body retained event type: %s", body)
	}
	for _, field := range []string{"model", "stream", "store", "generate", "previous_response_id", "stream_options", "client_metadata", "unknown", "input"} {
		if !gjson.GetBytes(body, field).Exists() {
			t.Fatalf("field %q removed from logical body: %s", field, body)
		}
	}
}

func TestResponsesWebSocket_RejectsResponseAppend(t *testing.T) {
	_, err := validateResponsesWebSocketFrame([]byte(`{"type":"response.append","model":"gpt-test","input":[]}`))
	if err == nil {
		t.Fatal("response.append was accepted")
	}
}

func TestResponsesWebSocket_ValidatesInputAndPreviousResponseID(t *testing.T) {
	tests := [][]byte{
		[]byte(`{"type":"response.create","model":"gpt-test","input":{}}`),
		[]byte(`{"type":"response.create","model":"gpt-test","previous_response_id":1,"input":[]}`),
		[]byte(`{"type":"response.create","model":"","input":[]}`),
	}
	for _, payload := range tests {
		if _, err := validateResponsesWebSocketFrame(payload); err == nil {
			t.Fatalf("invalid payload accepted: %s", payload)
		}
	}
}

func TestNewResponsesWebSocketTurnRequestReusesImmutableBodies(t *testing.T) {
	original := httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	original.Header.Set("Connection", "upgrade")
	original.Header.Set("Sec-WebSocket-Key", "secret")
	body := []byte(`{"model":"gpt-test","stream":true,"input":[]}`)
	payload := []byte(`{"type":"response.create","model":"gpt-test","stream":true,"input":[]}`)

	request := newResponsesWebSocketTurnRequest(original, context.Background(), body, "connection-test", payload)
	preloaded := maxxctx.GetRequestBody(request.Context())
	if len(preloaded) == 0 || &preloaded[0] != &body[0] {
		t.Fatal("internal request should reuse the logical body")
	}
	metadata, ok := maxxctx.GetResponsesWebSocketRequest(request.Context())
	if !ok || metadata.SessionID != "connection-test" {
		t.Fatalf("metadata = %#v, %v", metadata, ok)
	}
	if len(metadata.Payload) == 0 || &metadata.Payload[0] != &payload[0] {
		t.Fatal("metadata should reuse the immutable client frame")
	}
	readBody, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !bytes.Equal(readBody, body) {
		t.Fatalf("body = %s, want %s", readBody, body)
	}
	for _, key := range []string{"Connection", "Sec-WebSocket-Key"} {
		if got := request.Header.Get(key); got != "" {
			t.Fatalf("%s leaked: %q", key, got)
		}
	}
	if got := request.Header.Get("Accept"); got != "application/json" {
		t.Fatalf("Accept = %q", got)
	}
}

func TestResponsesWebSocket_OriginPolicy(t *testing.T) {
	t.Setenv("MAXX_CORS_ALLOW_ORIGINS", "https://allowed.example")
	req := httptest.NewRequest(http.MethodGet, "http://maxx.example/v1/responses", nil)
	req.Host = "maxx.example"
	if !checkResponsesWebSocketOrigin(req) {
		t.Fatal("CLI request without Origin was rejected")
	}
	req.Header.Set("Origin", "http://maxx.example")
	if !checkResponsesWebSocketOrigin(req) {
		t.Fatal("same-origin browser request was rejected")
	}
	req.Header.Set("Origin", "https://allowed.example")
	if !checkResponsesWebSocketOrigin(req) {
		t.Fatal("allowlisted browser request was rejected")
	}
	req.Header.Set("Origin", "https://evil.example")
	if checkResponsesWebSocketOrigin(req) {
		t.Fatal("cross-site browser request was accepted")
	}
}

func TestResponsesWebSocketClient_WriteTextFrame(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		client := newResponsesWebSocketClient(conn)
		defer client.close(websocket.CloseNormalClosure, "")
		_ = client.WriteTextFrame([]byte(`{"type":"response.completed"}`))
	}))
	defer server.Close()
	wsURL := "ws" + server.URL[len("http"):] + "/v1/responses"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	_, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(payload) != `{"type":"response.completed"}` {
		t.Fatalf("payload = %s", payload)
	}
}

func TestResponsesWebSocketReadLimit(t *testing.T) {
	for configured, want := range map[int64]int64{
		0:                              responsesWSMaxPendingBytes,
		responsesWSMaxPendingBytes + 1: responsesWSMaxPendingBytes,
		responsesWSMaxPendingBytes / 2: responsesWSMaxPendingBytes / 2,
	} {
		if got := responsesWebSocketReadLimit(configured); got != want {
			t.Fatalf("read limit for %d = %d, want %d", configured, got, want)
		}
	}
}

func TestResponsesWebSocketErrorAlreadySentRequiresTerminalError(t *testing.T) {
	partial := &domain.ResponsesWebSocketAttemptError{
		Err:             errors.New("upstream closed"),
		ClientEventSent: true,
	}
	if responsesWebSocketErrorAlreadySent(partial) {
		t.Fatal("ordinary upstream event suppressed the terminal client error")
	}
	if !responsesWebSocketTurnCommitted(partial) {
		t.Fatal("ordinary upstream event did not commit the turn")
	}
	terminal := &domain.ResponsesWebSocketAttemptError{
		Err:                    errors.New("upstream error event"),
		ClientEventSent:        true,
		TerminalErrorEventSent: true,
	}
	if !responsesWebSocketErrorAlreadySent(terminal) {
		t.Fatal("forwarded terminal error event was not recognized")
	}
}
