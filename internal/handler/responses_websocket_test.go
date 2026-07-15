package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/gorilla/websocket"
)

func TestResponsesWebSocket_ReplaysTranscriptThroughHTTPProxy(t *testing.T) {
	requestBodies := make(chan []byte, 2)
	callCount := 0
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Upgrade"); got != "" {
			t.Errorf("Upgrade header leaked to HTTP proxy: %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			return
		}
		requestBodies <- body
		callCount++

		w.Header().Set("Content-Type", "text/event-stream")
		if callCount == 1 {
			writeTestSSEEvent(t, w, `{"type":"response.created","response":{"id":"resp_1"}}`)
			writeTestSSEEvent(t, w, `{"type":"response.output_item.done","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup","arguments":"{}"}}`)
			writeTestSSEEvent(t, w, `{"type":"response.completed","response":{"id":"resp_1","output":[]}}`)
			return
		}
		writeTestSSEEvent(t, w, `{"type":"response.completed","response":{"id":"resp_2","output":[{"type":"message","id":"msg_2","role":"assistant","content":[]}]}}`)
	})

	conn := dialResponsesWebSocket(t, backend)
	defer conn.Close()

	firstRequest := `{"type":"response.create","model":"gpt-test","instructions":"be concise","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"call the tool"}]}]}`
	if err := conn.WriteMessage(websocket.TextMessage, []byte(firstRequest)); err != nil {
		t.Fatalf("write first request: %v", err)
	}
	firstCompleted := readWebSocketEventType(t, conn, "response.completed")
	firstResponse := decodeTestObject(t, firstCompleted["response"])
	firstOutput := decodeTestArray(t, firstResponse["output"])
	if len(firstOutput) != 1 {
		t.Fatalf("first completion output length = %d, want 1", len(firstOutput))
	}
	firstBody := receiveTestBody(t, requestBodies)
	assertNormalizedWebSocketHTTPBody(t, firstBody, "gpt-test", 1)

	secondRequest := `{"type":"response.create","previous_response_id":"resp_1","input":[{"type":"function_call_output","call_id":"call_1","output":"tool result"}]}`
	if err := conn.WriteMessage(websocket.TextMessage, []byte(secondRequest)); err != nil {
		t.Fatalf("write second request: %v", err)
	}
	readWebSocketEventType(t, conn, "response.completed")
	secondBody := receiveTestBody(t, requestBodies)
	assertNormalizedWebSocketHTTPBody(t, secondBody, "gpt-test", 3)

	secondObject := decodeTestObject(t, secondBody)
	if _, exists := secondObject["previous_response_id"]; exists {
		t.Fatal("previous_response_id must be removed for transcript replay")
	}
	if got := jsonString(secondObject["instructions"]); got != "be concise" {
		t.Fatalf("instructions = %q, want inherited value", got)
	}
}

func TestResponsesWebSocket_PreservesNativePayloadAndSession(t *testing.T) {
	type capturedRequest struct {
		body     []byte
		metadata maxxctx.ResponsesWebSocketRequest
	}
	requests := make(chan capturedRequest, 2)
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		metadata, ok := maxxctx.GetResponsesWebSocketRequest(r.Context())
		if !ok {
			t.Error("missing responses websocket request metadata")
		}
		requests <- capturedRequest{body: body, metadata: metadata}
		w.Header().Set("Content-Type", "text/event-stream")
		writeTestSSEEvent(t, w, `{"type":"response.completed","response":{"id":"resp_test","output":[]}}`)
	})

	conn := dialResponsesWebSocket(t, backend)
	defer conn.Close()

	firstPayload := []byte(`{"type":"response.create","model":"gpt-test","generate":false,"input":[]}`)
	if err := conn.WriteMessage(websocket.TextMessage, firstPayload); err != nil {
		t.Fatalf("write first request: %v", err)
	}
	readWebSocketEventType(t, conn, "response.completed")
	first := <-requests
	if string(first.metadata.Payload) != string(firstPayload) {
		t.Fatalf("native payload = %s, want %s", first.metadata.Payload, firstPayload)
	}
	if first.metadata.SessionID == "" {
		t.Fatal("websocket session ID is empty")
	}
	firstBody := decodeTestObject(t, first.body)
	if _, ok := firstBody["generate"]; ok {
		t.Fatalf("fallback request leaked websocket-only generate field: %s", first.body)
	}

	secondPayload := []byte(`{"type":"response.create","previous_response_id":"resp_test","generate":false,"input":[]}`)
	if err := conn.WriteMessage(websocket.TextMessage, secondPayload); err != nil {
		t.Fatalf("write second request: %v", err)
	}
	readWebSocketEventType(t, conn, "response.completed")
	second := <-requests
	if second.metadata.SessionID != first.metadata.SessionID {
		t.Fatalf("session ID changed: %q -> %q", first.metadata.SessionID, second.metadata.SessionID)
	}
	if string(second.metadata.Payload) != string(secondPayload) {
		t.Fatalf("second native payload = %s, want %s", second.metadata.Payload, secondPayload)
	}
	secondBody := decodeTestObject(t, second.body)
	if _, ok := secondBody["generate"]; ok {
		t.Fatalf("subsequent fallback request leaked websocket-only generate field: %s", second.body)
	}
}

func TestResponsesWebSocket_MapsHTTPErrorToWebSocketError(t *testing.T) {
	backend := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"type":"authentication_error","message":"bad token"}}`))
	})

	conn := dialResponsesWebSocket(t, backend)
	defer conn.Close()

	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-test","input":[]}`)); err != nil {
		t.Fatalf("write request: %v", err)
	}
	event := readWebSocketEventType(t, conn, "error")
	var status int
	if err := json.Unmarshal(event["status"], &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", status, http.StatusUnauthorized)
	}
	errorBody := decodeTestObject(t, event["error"])
	if got := jsonString(errorBody["message"]); got != "bad token" {
		t.Fatalf("error message = %q, want bad token", got)
	}
}

func dialResponsesWebSocket(t *testing.T, backend http.Handler) *websocket.Conn {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveResponsesWebSocket(w, r, backend, 0)
	}))
	t.Cleanup(server.Close)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/v1/responses"
	conn, response, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		if response != nil {
			t.Fatalf("dial responses websocket: %v (status %s)", err, response.Status)
		}
		t.Fatalf("dial responses websocket: %v", err)
	}
	return conn
}

func writeTestSSEEvent(t *testing.T, w http.ResponseWriter, payload string) {
	t.Helper()
	if _, err := io.WriteString(w, "data: "+payload+"\n\n"); err != nil {
		t.Errorf("write SSE event: %v", err)
		return
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func readWebSocketEventType(t *testing.T, conn *websocket.Conn, wantedType string) map[string]json.RawMessage {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("set websocket read deadline: %v", err)
	}
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read websocket event %s: %v", wantedType, err)
		}
		event := decodeTestObject(t, payload)
		if jsonString(event["type"]) == wantedType {
			return event
		}
	}
}

func receiveTestBody(t *testing.T, bodies <-chan []byte) []byte {
	t.Helper()
	select {
	case body := <-bodies:
		return body
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for proxied request body")
		return nil
	}
}

func assertNormalizedWebSocketHTTPBody(t *testing.T, body []byte, model string, inputLength int) {
	t.Helper()
	object := decodeTestObject(t, body)
	if _, exists := object["type"]; exists {
		t.Fatal("websocket event type leaked into HTTP Responses body")
	}
	var stream bool
	if err := json.Unmarshal(object["stream"], &stream); err != nil || !stream {
		t.Fatalf("stream = %s, want true", object["stream"])
	}
	if got := jsonString(object["model"]); got != model {
		t.Fatalf("model = %q, want %q", got, model)
	}
	input := decodeTestArray(t, object["input"])
	if len(input) != inputLength {
		t.Fatalf("input length = %d, want %d; body=%s", len(input), inputLength, body)
	}
}

func decodeTestObject(t *testing.T, raw []byte) map[string]json.RawMessage {
	t.Helper()
	object, err := decodeJSONObject(raw)
	if err != nil {
		t.Fatalf("decode JSON object %s: %v", raw, err)
	}
	return object
}

func decodeTestArray(t *testing.T, raw []byte) []json.RawMessage {
	t.Helper()
	array, err := decodeJSONArray(raw)
	if err != nil {
		t.Fatalf("decode JSON array %s: %v", raw, err)
	}
	return array
}
