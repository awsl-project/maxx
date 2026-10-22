package workbuddyai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testProvider() *domain.Provider {
	return &domain.Provider{ID: 1, Type: "workbuddy-ai", Config: &domain.ProviderConfig{WorkBuddyAI: &domain.ProviderConfigWorkBuddyAI{AccessToken: "dummy", UID: "test-user", EnterpriseID: "test-team"}}}
}
func executeTest(t *testing.T, server *httptest.Server, body string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	p := testProvider()
	a := &Adapter{client: server.Client(), endpoint: server.URL}
	r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	w := httptest.NewRecorder()
	c := flow.NewCtx(w, r)
	c.Set(flow.KeyClientType, domain.ClientTypeOpenAI)
	c.Set(flow.KeyRequestURI, "/v1/chat/completions")
	c.Set(flow.KeyRequestBody, []byte(body))
	return w, a.Execute(c, p)
}

const textFrames = "data: {\"id\":\"test\",\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hello\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"test\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\" world\"},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3}}\n\ndata: [DONE]\n\n"

func TestChatStreamAndAggregate(t *testing.T) {
	for _, stream := range []bool{true, false} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v2/chat/completions" {
					t.Errorf("path %s", r.URL.Path)
				}
				var b map[string]interface{}
				_ = json.NewDecoder(r.Body).Decode(&b)
				if b["stream"] != true {
					t.Error("upstream must stream")
				}
				if b["messages"].([]interface{})[0].(map[string]interface{})["content"] != "You are Claude. Preserve this." {
					t.Error("user semantics changed")
				}
				for _, h := range []string{"Authorization", "X-User-Id", "User-Agent", "X-Agent-Type", "X-Agent-Purpose", "X-IDE-Name", "X-Enterprise-Id", "X-Tenant-Id", "X-Conversation-Request-ID"} {
					if r.Header.Get(h) == "" {
						t.Errorf("missing %s", h)
					}
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, textFrames)
			}))
			defer s.Close()
			w, err := executeTest(t, s, fmt.Sprintf(`{"model":"test-model","stream":%t,"messages":[{"role":"user","content":"You are Claude. Preserve this."}]}`, stream))
			if err != nil {
				t.Fatal(err)
			}
			if stream {
				if !strings.Contains(w.Body.String(), "[DONE]") {
					t.Fatal(w.Body.String())
				}
			} else {
				var b map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
					t.Fatal(err)
				}
				if b["object"] != "chat.completion" || !strings.Contains(w.Body.String(), "hello world") {
					t.Fatal(w.Body.String())
				}
			}
		})
	}
}
func TestUpstreamRefusalDoesNotRetryOrCommit(t *testing.T) {
	for _, status := range []int{200, 403} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(status)
				fmt.Fprint(w, `{"code":10085,"msg":"unapproved channel"}`)
			}))
			defer s.Close()
			w, err := executeTest(t, s, `{"messages":[],"stream":true}`)
			var pe *domain.ProxyError
			if !errors.As(err, &pe) || pe.Retryable || !strings.Contains(err.Error(), "10085") || !strings.Contains(err.Error(), "unapproved channel") {
				t.Fatalf("error %v", err)
			}
			if calls != 1 || w.Body.Len() != 0 {
				t.Fatalf("calls %d body %s", calls, w.Body.String())
			}
		})
	}
}
func TestPrematureEOFAndSSEError(t *testing.T) {
	for _, data := range []string{strings.Replace(textFrames, "data: [DONE]\n\n", "", 1), "data: {\"error\":{\"message\":\"quota exhausted\"}}\n\n"} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, data)
		}))
		w, err := executeTest(t, s, `{"messages":[],"stream":false}`)
		s.Close()
		if err == nil || w.Body.Len() != 0 {
			t.Fatalf("err %v body %s", err, w.Body.String())
		}
	}
}
func TestChatCancellation(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil).WithContext(ctx)
	c := flow.NewCtx(httptest.NewRecorder(), r)
	c.Set(flow.KeyClientType, domain.ClientTypeOpenAI)
	c.Set(flow.KeyRequestURI, "/v1/chat/completions")
	c.Set(flow.KeyRequestBody, []byte(`{"messages":[],"stream":true}`))
	a := &Adapter{client: s.Client(), endpoint: s.URL}
	if err := a.Execute(c, testProvider()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error %v", err)
	}
}
func TestToolAggregationAndTurnIDs(t *testing.T) {
	a := newChatAggregate()
	for _, s := range []string{`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call-test","type":"function","function":{"name":"lookup","arguments":"{\"x\":"}}]}}]}`, `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"1}"}}]},"finish_reason":"tool_calls"}]}`} {
		var f map[string]json.RawMessage
		if err := json.Unmarshal([]byte(s), &f); err != nil {
			t.Fatal(err)
		}
		if err := a.add(f); err != nil {
			t.Fatal(err)
		}
	}
	out, err := a.finish()
	if err != nil || !strings.Contains(string(out), `"arguments":"{\"x\":1}"`) {
		t.Fatalf("%s %v", out, err)
	}
	var b map[string]json.RawMessage
	_ = json.Unmarshal([]byte(`{"messages":[{"role":"user","content":"hi"}]}`), &b)
	h1 := http.Header{}
	attend(h1, b, "session")
	_ = json.Unmarshal([]byte(`{"messages":[{"role":"user","content":"hi"},{"role":"assistant","tool_calls":[{"id":"call-test"}]},{"role":"tool","content":"result"}]}`), &b)
	h2 := http.Header{}
	attend(h2, b, "session")
	if h1.Get("X-Conversation-Request-ID") != h2.Get("X-Conversation-Request-ID") || h1.Get("X-Request-ID") == h2.Get("X-Request-ID") {
		t.Fatal("tool continuation attribution")
	}
}

func TestExpiredRefreshFailureIsNotHidden(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401); fmt.Fprint(w, "{}") }))
	defer s.Close()
	_, err := NewClient(s.URL, s.Client()).Fresh(context.Background(), Config{AccessToken: "dummy", RefreshToken: "dummy-refresh", ExpiresAt: time.Now().Add(-time.Minute).UnixMilli()}, nil)
	if err == nil {
		t.Fatal("expired credential refresh failure hidden")
	}
}
