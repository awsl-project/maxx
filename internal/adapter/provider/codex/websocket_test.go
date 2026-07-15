package codex

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
)

func TestExecuteResponsesWebSocket_ReusesNativeUpstreamSession(t *testing.T) {
	var connectionCount atomic.Int32
	requests := make(chan []byte, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("path = %q, want /v1/responses", r.URL.Path)
		}
		if got := r.Header.Get("OpenAI-Beta"); got != codexResponsesWebSocketBetaHeader {
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
			BaseURL:     server.URL,
		}},
	}
	adapter := &CodexAdapter{
		provider:   provider,
		tokenCache: &TokenCache{AccessToken: "test-token"},
		httpClient: newUpstreamHTTPClient(),
	}
	sessionID := uuid.NewString()
	storeKey := "42:" + sessionID
	t.Cleanup(func() {
		if session := globalCodexWebSocketSessions.get(storeKey); session != nil {
			globalCodexWebSocketSessions.remove(storeKey, session)
		}
	})

	firstRaw := []byte(`{"type":"response.create","model":"client-model","generate":false,"stream":true,"background":true,"input":[]}`)
	firstCtx := newCodexWebSocketTestContext(t, firstRaw, sessionID)
	handled, err := adapter.executeResponsesWebSocket(firstCtx, provider)
	if !handled || err != nil {
		t.Fatalf("first execute handled=%v err=%v", handled, err)
	}
	firstUpstream := <-requests
	if got := gjson.GetBytes(firstUpstream, "type").String(); got != "response.create" {
		t.Fatalf("type = %q", got)
	}
	if got := gjson.GetBytes(firstUpstream, "model").String(); got != "gpt-test" {
		t.Fatalf("model = %q, want mapped model", got)
	}
	if gjson.GetBytes(firstUpstream, "stream").Exists() || gjson.GetBytes(firstUpstream, "background").Exists() {
		t.Fatalf("websocket-only implicit fields not removed: %s", firstUpstream)
	}
	generate := gjson.GetBytes(firstUpstream, "generate")
	if !generate.Exists() || generate.Bool() {
		t.Fatalf("native v2 prewarm generate = %s, want false", generate.Raw)
	}

	secondRaw := []byte(`{"type":"response.append","previous_response_id":"resp_1","input":[{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`)
	secondCtx := newCodexWebSocketTestContext(t, secondRaw, sessionID)
	handled, err = adapter.executeResponsesWebSocket(secondCtx, provider)
	if !handled || err != nil {
		t.Fatalf("second execute handled=%v err=%v", handled, err)
	}
	secondUpstream := <-requests
	if got := gjson.GetBytes(secondUpstream, "type").String(); got != "response.create" {
		t.Fatalf("second type = %q", got)
	}
	if got := gjson.GetBytes(secondUpstream, "previous_response_id").String(); got != "resp_1" {
		t.Fatalf("previous_response_id = %q", got)
	}
	if connectionCount.Load() != 1 {
		t.Fatalf("connections = %d, want 1", connectionCount.Load())
	}
}

func TestExecuteResponsesWebSocket_SkipsProtocolConversion(t *testing.T) {
	provider := &domain.Provider{ID: 1, Config: &domain.ProviderConfig{Codex: &domain.ProviderConfigCodex{}}}
	adapter := &CodexAdapter{provider: provider, tokenCache: &TokenCache{}}
	raw := []byte(`{"type":"response.create","model":"gpt-test","input":[]}`)
	request := httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses", strings.NewReader(`{"model":"gpt-test","stream":true,"input":[]}`))
	request = request.WithContext(maxxctx.WithResponsesWebSocketRequest(context.Background(), "session", raw))
	c := flow.NewCtx(httptest.NewRecorder(), request)
	c.Set(flow.KeyClientType, domain.ClientTypeCodex)
	c.Set(flow.KeyOriginalClientType, domain.ClientTypeClaude)

	handled, err := adapter.executeResponsesWebSocket(c, provider)
	if handled || err != nil {
		t.Fatalf("conversion handled=%v err=%v", handled, err)
	}
}

func newCodexWebSocketTestContext(t *testing.T, raw []byte, sessionID string) *flow.Ctx {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses", nil)
	request.Header.Set("User-Agent", "codex-test/1.0")
	request = request.WithContext(maxxctx.WithResponsesWebSocketRequest(context.Background(), sessionID, raw))
	c := flow.NewCtx(httptest.NewRecorder(), request)
	c.Set(flow.KeyClientType, domain.ClientTypeCodex)
	c.Set(flow.KeyOriginalClientType, domain.ClientTypeCodex)
	c.Set(flow.KeyMappedModel, "gpt-test")
	c.Set(flow.KeyIsStream, true)
	c.Set(flow.KeyResponsesClientPath, "/v1/responses")
	return c
}
