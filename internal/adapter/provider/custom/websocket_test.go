package custom

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	provideradapter "github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/gorilla/websocket"
	"github.com/google/uuid"
)

type recordingCustomWSSink struct {
	frames [][]byte
}

func (s *recordingCustomWSSink) WriteTextFrame(payload []byte) error {
	s.frames = append(s.frames, append([]byte(nil), payload...))
	return nil
}

func newCustomWSTestContext(t *testing.T) *flow.Ctx {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/responses", nil).WithContext(context.Background())
	return flow.NewCtx(httptest.NewRecorder(), req)
}

func boolPtr(v bool) *bool { return &v }

func TestCustomAdapterImplementsResponsesWebSocket(t *testing.T) {
	var a provideradapter.ProviderAdapter = &CustomAdapter{
		provider: &domain.Provider{
			Type:                 "custom",
			SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex},
			Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
				BaseURL:            "https://example.invalid/v1",
				APIKey:             "sk-test",
				ResponsesWebSocket: boolPtr(true),
			}},
		},
	}
	if _, ok := a.(provideradapter.ResponsesWebSocketAdapter); !ok {
		t.Fatal("CustomAdapter must implement ResponsesWebSocketAdapter")
	}
}

func TestJoinCustomResponsesWebSocketURL(t *testing.T) {
	got, err := joinCustomResponsesWebSocketURL("https://codex.example/v1/")
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	if got != "wss://codex.example/v1/responses" {
		t.Fatalf("url = %q", got)
	}
}

func TestExecuteResponsesWebSocket_CustomCodexGateway(t *testing.T) {
	var dials atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dials.Add(1)
		if r.Header.Get("Authorization") != "Bearer sk-live" {
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/v1/responses" {
			t.Errorf("path = %q", r.URL.Path)
		}
		conn, err := websocket.Upgrade(w, r, nil, 4096, 4096)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer conn.Close()
		_, frame, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("read: %v", err)
			return
		}
		if string(frame) == "" {
			t.Error("empty frame")
		}
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"id":"resp_1","model":"gpt-test","output":[]}}`))
	}))
	defer upstream.Close()

	provider := &domain.Provider{
		ID:                   42,
		Type:                 "custom",
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex},
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL:            upstream.URL + "/v1",
			APIKey:             "sk-live",
			ResponsesWebSocket: boolPtr(true),
		}},
	}
	adapter := &CustomAdapter{provider: provider}
	connectionID := uuid.NewString()
	t.Cleanup(func() { adapter.CloseResponsesWebSocketConnection(connectionID) })
	sink := &recordingCustomWSSink{}
	result, err := adapter.ExecuteResponsesWebSocket(newCustomWSTestContext(t), provider, &domain.ResponsesWebSocketExchange{
		ConnectionID: connectionID,
		Frame:        []byte(`{"type":"response.create","model":"gpt-test","stream":true,"input":[]}`),
		Sink:         sink,
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result == nil || result.TerminalEvent != "response.completed" {
		t.Fatalf("result = %#v", result)
	}
	if dials.Load() != 1 {
		t.Fatalf("dials = %d", dials.Load())
	}
	if len(sink.frames) != 1 {
		t.Fatalf("frames = %d", len(sink.frames))
	}
}

func TestExecuteResponsesWebSocket_RejectsNonCodexCustom(t *testing.T) {
	provider := &domain.Provider{
		ID:                   7,
		Type:                 "custom",
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeOpenAI},
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL:            "https://example.invalid/v1",
			APIKey:             "sk",
			ResponsesWebSocket: boolPtr(true),
		}},
	}
	adapter := &CustomAdapter{provider: provider}
	_, err := adapter.ExecuteResponsesWebSocket(newCustomWSTestContext(t), provider, &domain.ResponsesWebSocketExchange{
		ConnectionID: "c",
		Frame:        []byte(`{"type":"response.create","model":"gpt-test","input":[]}`),
		Sink:         &recordingCustomWSSink{},
	})
	var wsErr *domain.ResponsesWebSocketAttemptError
	if !errors.As(err, &wsErr) || !wsErr.CapabilityFailure {
		t.Fatalf("err = %#v, want capability failure", err)
	}
}

func TestExecuteResponsesWebSocket_RejectsWhenFlagDisabled(t *testing.T) {
	provider := &domain.Provider{
		ID:                   8,
		Type:                 "custom",
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex},
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL: "https://example.invalid/v1",
			APIKey:  "sk",
			// ResponsesWebSocket unset → default false
		}},
	}
	adapter := &CustomAdapter{provider: provider}
	_, err := adapter.ExecuteResponsesWebSocket(newCustomWSTestContext(t), provider, &domain.ResponsesWebSocketExchange{
		ConnectionID: "c",
		Frame:        []byte(`{"type":"response.create","model":"gpt-test","input":[]}`),
		Sink:         &recordingCustomWSSink{},
	})
	var wsErr *domain.ResponsesWebSocketAttemptError
	if !errors.As(err, &wsErr) || !wsErr.CapabilityFailure {
		t.Fatalf("err = %#v, want capability failure when flag disabled", err)
	}
}

