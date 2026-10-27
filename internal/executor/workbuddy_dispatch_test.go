package executor

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/awsl-project/maxx/internal/adapter/provider/workbuddyai"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
)

func TestWorkBuddyClaudeDispatchMockUpstream(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/v2/chat/completions" {
					t.Errorf("upstream path %s", r.URL.Path)
				}
				if r.Header.Get("Authorization") != "Bearer dummy" {
					t.Error("auth mismatch")
				}
				var b map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
					t.Error(err)
				}
				if b["model"] != "workbuddy-target" {
					t.Errorf("outbound model = %v, want mapped target", b["model"])
				}
				if b["stream"] != true {
					t.Error("must request stream")
				}
				msgs, ok := b["messages"].([]interface{})
				if !ok || len(msgs) == 0 {
					t.Error("not OpenAI messages")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"id\":\"chat-test\",\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"converted reply\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"chat-test\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2}}\n\ndata: [DONE]\n\n")
			}))
			defer upstream.Close()
			// The adapter owns its transport, so point its provider proxy at the mock forward proxy.
			p := &domain.Provider{ID: 40, Type: "workbuddy-ai", Name: "workbuddy-test", SupportedClientTypes: []domain.ClientType{domain.ClientTypeOpenAI}, Config: &domain.ProviderConfig{WorkBuddyAI: &domain.ProviderConfigWorkBuddyAI{AccessToken: "dummy"}}}
			// A CONNECT-free test transport is installed via the package's explicit test constructor.
			adapter, err := workbuddyai.NewAdapterWithClient(p, upstream.URL, upstream.Client())
			if err != nil {
				t.Fatal(err)
			}
			body := fmt.Sprintf(`{"model":"claude-3-5-sonnet","max_tokens":64,"messages":[{"role":"user","content":"hello"}],"stream":%t}`, stream)
			c, proxyRepo, attemptRepo := newClaudeOpenAIConversionDispatchCtx(t, body, &openAIOnlyConversionAdapter{})
			state := c.Keys[flow.KeyExecutorState].(*execState)
			state.isStream = stream
			state.routes[0].Provider = p
			state.routes[0].ProviderAdapter = adapter
			e := newOpenAIClaudeConversionTestExecutor(proxyRepo, attemptRepo)
			e.modelMappingRepo = &staticModelMappingRepo{mappings: []*domain.ModelMapping{{Pattern: "claude-3-5-sonnet", Target: "workbuddy-target", IsEnabled: true}}}
			e.dispatch(c)
			if c.Err != nil {
				t.Fatal(c.Err)
			}
			if calls != 1 {
				t.Fatalf("calls %d", calls)
			}
			out := c.Writer.(*httptest.ResponseRecorder).Body.String()
			if !strings.Contains(out, "converted reply") {
				t.Fatal(out)
			}
			if stream {
				if !strings.Contains(out, "message_stop") {
					t.Fatal(out)
				}
			} else if !strings.Contains(out, `"type":"message"`) {
				t.Fatal(out)
			}
		})
	}
}
