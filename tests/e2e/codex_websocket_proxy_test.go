package e2e_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestCodexWebSocketE2E_UsesNativeUpstreamAndReusesConnection(t *testing.T) {
	var websocketConnections atomic.Int32
	var codexHTTPPosts atomic.Int32
	upstreamFrames := make(chan []byte, 2)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !websocket.IsWebSocketUpgrade(r) {
			codexHTTPPosts.Add(1)
			http.Error(w, "HTTP fallback is forbidden", http.StatusInternalServerError)
			return
		}
		if r.URL.Path != "/v1/responses" {
			t.Errorf("upstream path = %q, want /v1/responses", r.URL.Path)
		}
		conn, err := websocket.Upgrade(w, r, nil, 4096, 4096)
		if err != nil {
			t.Errorf("upgrade upstream: %v", err)
			return
		}
		defer conn.Close()
		websocketConnections.Add(1)
		for turn := 1; turn <= 2; turn++ {
			messageType, frame, readErr := conn.ReadMessage()
			if readErr != nil {
				t.Errorf("read upstream turn %d: %v", turn, readErr)
				return
			}
			if messageType != websocket.TextMessage {
				t.Errorf("upstream message type = %d, want text", messageType)
				return
			}
			upstreamFrames <- bytes.Clone(frame)
			responseID := "resp_1"
			if turn == 2 {
				responseID = "resp_2"
			}
			response := `{"type":"response.completed","response":{"id":"` + responseID + `","model":"gpt-test","output":[]}}`
			if writeErr := conn.WriteMessage(websocket.TextMessage, []byte(response)); writeErr != nil {
				t.Errorf("write upstream turn %d: %v", turn, writeErr)
				return
			}
		}
	}))
	defer upstream.Close()

	var customHTTPCalls atomic.Int32
	customHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		customHTTPCalls.Add(1)
		http.Error(w, "HTTP-only provider must not be selected", http.StatusInternalServerError)
	}))
	defer customHTTP.Close()

	env := NewProxyTestEnv(t)
	codexProviderID := createCodexProvider(t, env, map[string]any{
		"accessToken": "static-token",
		"baseURL":     upstream.URL + "/v1",
	})
	customProviderID := createProvider(t, env, "HTTP-only Codex candidate", customHTTP.URL, []string{"codex"})
	createNativeRoute(t, env, "codex", customProviderID, 1)
	createNativeRoute(t, env, "codex", codexProviderID, 2)

	downstreamURL := "ws" + strings.TrimPrefix(env.Server.URL, "http") + "/v1/responses"
	downstream, response, err := websocket.DefaultDialer.Dial(downstreamURL, nil)
	if err != nil {
		if response != nil && response.Body != nil {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			t.Fatalf("dial downstream: %v: %s", err, body)
		}
		t.Fatalf("dial downstream: %v", err)
	}
	defer downstream.Close()

	first := []byte(`{"type":"response.create","model":"gpt-test","stream":true,"store":true,"generate":false,"unknown_field":"preserve","input":[]}`)
	second := []byte(`{"type":"response.create","model":"gpt-test","stream":true,"previous_response_id":"resp_1","input":[{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`)
	for turn, frame := range [][]byte{first, second} {
		if err := downstream.WriteMessage(websocket.TextMessage, frame); err != nil {
			t.Fatalf("write downstream turn %d: %v", turn+1, err)
		}
		messageType, event, err := downstream.ReadMessage()
		if err != nil {
			t.Fatalf("read downstream turn %d: %v", turn+1, err)
		}
		if messageType != websocket.TextMessage || !bytes.Contains(event, []byte(`"type":"response.completed"`)) {
			t.Fatalf("downstream turn %d event = %s", turn+1, event)
		}
	}

	if got := <-upstreamFrames; !bytes.Equal(got, first) {
		t.Fatalf("first upstream frame mutated:\n got %s\nwant %s", got, first)
	}
	if got := <-upstreamFrames; !bytes.Equal(got, second) {
		t.Fatalf("second upstream frame mutated:\n got %s\nwant %s", got, second)
	}
	if got := websocketConnections.Load(); got != 1 {
		t.Fatalf("upstream websocket connections = %d, want 1", got)
	}
	if got := codexHTTPPosts.Load(); got != 0 {
		t.Fatalf("Codex HTTP POST calls = %d, want 0", got)
	}
	if got := customHTTPCalls.Load(); got != 0 {
		t.Fatalf("HTTP-only provider calls = %d, want 0", got)
	}
}

func TestCodexWebSocketE2E_DeltaThenCloseSendsTerminalError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Upgrade(w, r, nil, 4096, 4096)
		if err != nil {
			t.Errorf("upgrade upstream: %v", err)
			return
		}
		defer conn.Close()
		if _, _, err = conn.ReadMessage(); err != nil {
			t.Errorf("read upstream request: %v", err)
			return
		}
		if err = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.output_text.delta","delta":"partial"}`)); err != nil {
			t.Errorf("write upstream delta: %v", err)
		}
	}))
	defer upstream.Close()

	env := NewProxyTestEnv(t)
	providerID := createCodexProvider(t, env, map[string]any{
		"accessToken": "static-token",
		"baseURL":     upstream.URL + "/v1",
	})
	createNativeRoute(t, env, "codex", providerID, 1)

	downstreamURL := "ws" + strings.TrimPrefix(env.Server.URL, "http") + "/v1/responses"
	downstream, _, err := websocket.DefaultDialer.Dial(downstreamURL, nil)
	if err != nil {
		t.Fatalf("dial downstream: %v", err)
	}
	defer downstream.Close()
	_ = downstream.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err = downstream.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-test","input":[]}`)); err != nil {
		t.Fatalf("write downstream request: %v", err)
	}

	_, delta, err := downstream.ReadMessage()
	if err != nil {
		t.Fatalf("read downstream delta: %v", err)
	}
	if !bytes.Contains(delta, []byte(`"type":"response.output_text.delta"`)) {
		t.Fatalf("first downstream event = %s, want delta", delta)
	}
	_, terminal, err := downstream.ReadMessage()
	if err != nil {
		t.Fatalf("read downstream terminal error: %v", err)
	}
	if !bytes.Contains(terminal, []byte(`"type":"error"`)) ||
		!bytes.Contains(terminal, []byte(`"code":"upstream_websocket_closed_before_terminal"`)) {
		t.Fatalf("terminal downstream event = %s", terminal)
	}
	if _, _, err = downstream.ReadMessage(); err == nil {
		t.Fatal("downstream connection remained open after committed upstream turn failure")
	}
}

// Client-supplied isNative is ignored; server derives true for native Codex providers.
func TestCodexWebSocketDirectsThroughCanonicalNativeRoute(t *testing.T) {
	var websocketConnections atomic.Int32
	var codexHTTPPosts atomic.Int32
	upstreamFrames := make(chan []byte, 1)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !websocket.IsWebSocketUpgrade(r) {
			codexHTTPPosts.Add(1)
			http.Error(w, "HTTP fallback is forbidden", http.StatusInternalServerError)
			return
		}
		if r.URL.Path != "/v1/responses" {
			t.Errorf("upstream path = %q, want /v1/responses", r.URL.Path)
		}
		conn, err := websocket.Upgrade(w, r, nil, 4096, 4096)
		if err != nil {
			t.Errorf("upgrade upstream: %v", err)
			return
		}
		defer conn.Close()
		websocketConnections.Add(1)
		messageType, frame, readErr := conn.ReadMessage()
		if readErr != nil {
			t.Errorf("read upstream: %v", readErr)
			return
		}
		if messageType != websocket.TextMessage {
			t.Errorf("upstream message type = %d, want text", messageType)
			return
		}
		upstreamFrames <- bytes.Clone(frame)
		response := `{"type":"response.completed","response":{"id":"resp_canonical","model":"gpt-test","output":[]}}`
		if writeErr := conn.WriteMessage(websocket.TextMessage, []byte(response)); writeErr != nil {
			t.Errorf("write upstream: %v", writeErr)
		}
	}))
	defer upstream.Close()

	var openaiHTTPCalls atomic.Int32
	openaiHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		openaiHTTPCalls.Add(1)
		http.Error(w, "OpenAI conversion route must not be selected", http.StatusInternalServerError)
	}))
	defer openaiHTTP.Close()

	env := NewProxyTestEnv(t)
	codexProviderID := createCodexProvider(t, env, map[string]any{
		"accessToken": "static-token",
		"baseURL":     upstream.URL + "/v1",
	})
	// Deliberately submit isNative=false; server must recompute to true for codex→codex.
	createResp := env.AdminPost("/api/admin/routes", map[string]any{
		"isEnabled":  true,
		"isNative":   false,
		"clientType": "codex",
		"providerID": codexProviderID,
		"position":   1,
	})
	if createResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(createResp.Body)
		createResp.Body.Close()
		t.Fatalf("create route with isNative=false: status=%d body=%s", createResp.StatusCode, body)
	}
	var created struct {
		ID       uint64 `json:"id"`
		IsNative bool   `json:"isNative"`
	}
	DecodeJSON(t, createResp, &created)
	if !created.IsNative {
		t.Fatalf("create response isNative = false, want true (server-derived)")
	}

	getResp := env.AdminGet("/api/admin/routes/" + itoa(created.ID))
	if getResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(getResp.Body)
		getResp.Body.Close()
		t.Fatalf("GET route: status=%d body=%s", getResp.StatusCode, body)
	}
	var fetched struct {
		ID       uint64 `json:"id"`
		IsNative bool   `json:"isNative"`
	}
	DecodeJSON(t, getResp, &fetched)
	if !fetched.IsNative {
		t.Fatalf("GET route isNative = false, want true")
	}

	// Competing non-native conversion candidate must not receive traffic.
	openaiProviderID := createProvider(t, env, "OpenAI conversion candidate", openaiHTTP.URL, []string{"openai"})
	createNativeRoute(t, env, "codex", openaiProviderID, 2)

	downstreamURL := "ws" + strings.TrimPrefix(env.Server.URL, "http") + "/v1/responses"
	downstream, response, err := websocket.DefaultDialer.Dial(downstreamURL, nil)
	if err != nil {
		if response != nil && response.Body != nil {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			t.Fatalf("dial downstream: %v: %s", err, body)
		}
		t.Fatalf("dial downstream: %v", err)
	}
	defer downstream.Close()

	frame := []byte(`{"type":"response.create","model":"gpt-test","stream":true,"store":true,"input":[]}`)
	if err := downstream.WriteMessage(websocket.TextMessage, frame); err != nil {
		t.Fatalf("write downstream: %v", err)
	}
	messageType, event, err := downstream.ReadMessage()
	if err != nil {
		t.Fatalf("read downstream: %v", err)
	}
	if messageType != websocket.TextMessage || !bytes.Contains(event, []byte(`"type":"response.completed"`)) {
		t.Fatalf("downstream event = %s", event)
	}
	if got := <-upstreamFrames; !bytes.Equal(got, frame) {
		t.Fatalf("upstream frame mutated:\n got %s\nwant %s", got, frame)
	}
	if got := websocketConnections.Load(); got != 1 {
		t.Fatalf("upstream websocket connections = %d, want 1", got)
	}
	if got := codexHTTPPosts.Load(); got != 0 {
		t.Fatalf("Codex HTTP POST calls = %d, want 0", got)
	}
	if got := openaiHTTPCalls.Load(); got != 0 {
		t.Fatalf("OpenAI conversion HTTP calls = %d, want 0", got)
	}
}

// OpenAI HTTP conversion routes are never eligible for Responses WebSocket.
func TestCodexWebSocketRejectsOpenAIConversionRoute(t *testing.T) {
	var openaiHTTPCalls atomic.Int32
	openaiHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		openaiHTTPCalls.Add(1)
		http.Error(w, "OpenAI HTTP upstream must not be called for WS", http.StatusInternalServerError)
	}))
	defer openaiHTTP.Close()

	env := NewProxyTestEnv(t)
	openaiProviderID := createProvider(t, env, "OpenAI conversion only", openaiHTTP.URL, []string{"openai"})
	// Codex client route targeting an OpenAI-only provider is non-native conversion.
	routeResp := env.AdminPost("/api/admin/routes", map[string]any{
		"isEnabled":  true,
		"isNative":   true, // client claim ignored; server derives false
		"clientType": "codex",
		"providerID": openaiProviderID,
		"position":   1,
	})
	if routeResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(routeResp.Body)
		routeResp.Body.Close()
		t.Fatalf("create conversion route: status=%d body=%s", routeResp.StatusCode, body)
	}
	var route struct {
		ID       uint64 `json:"id"`
		IsNative bool   `json:"isNative"`
	}
	DecodeJSON(t, routeResp, &route)
	if route.IsNative {
		t.Fatalf("conversion route isNative = true, want false")
	}

	downstreamURL := "ws" + strings.TrimPrefix(env.Server.URL, "http") + "/v1/responses"
	downstream, _, err := websocket.DefaultDialer.Dial(downstreamURL, nil)
	if err != nil {
		t.Fatalf("dial downstream: %v", err)
	}
	defer downstream.Close()
	_ = downstream.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err = downstream.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-test","input":[]}`)); err != nil {
		t.Fatalf("write downstream: %v", err)
	}
	_, event, err := downstream.ReadMessage()
	if err != nil {
		t.Fatalf("read downstream error event: %v", err)
	}
	if !bytes.Contains(event, []byte(`"type":"error"`)) ||
		!bytes.Contains(event, []byte(`"code":"websocket_transport_unavailable"`)) {
		t.Fatalf("downstream event = %s, want websocket_transport_unavailable", event)
	}
	if got := openaiHTTPCalls.Load(); got != 0 {
		t.Fatalf("OpenAI HTTP upstream calls = %d, want 0", got)
	}
}

func createNativeRoute(t *testing.T, env *ProxyTestEnv, clientType string, providerID uint64, position int) uint64 {
	t.Helper()
	resp := env.AdminPost("/api/admin/routes", map[string]any{
		"isEnabled":  true,
		"isNative":   true,
		"clientType": clientType,
		"providerID": providerID,
		"position":   position,
	})
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("create native route: status=%d body=%s", resp.StatusCode, body)
	}
	var route struct {
		ID uint64 `json:"id"`
	}
	DecodeJSON(t, resp, &route)
	return route.ID
}
