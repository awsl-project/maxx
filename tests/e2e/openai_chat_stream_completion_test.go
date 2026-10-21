package e2e_test

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/repository/sqlite"
)

func TestOpenAIChatStreamDisconnectAfterTerminalEvent(t *testing.T) {
	for _, timeouts := range []bool{false, true} {
		for _, reverseProxy := range []bool{false, true} {
			for _, tc := range []struct {
				name, tail, stopAt, status, errorText string
			}{
				{"before_finish", "", `"content":"hello"`, "FAILED", "client disconnected"},
				{"finish_without_done", "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":5,\"total_tokens\":17}}\n\n", `"finish_reason":"stop"`, "FAILED", "client disconnected"},
				{"after_done", "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":5,\"total_tokens\":17}}\n\ndata: [DONE]\n\n", "data: [DONE]", "COMPLETED", ""},
				{"error_then_done", "data: {\"error\":{\"message\":\"upstream rejected stream\",\"code\":503}}\n\ndata: [DONE]\n\n", "data: [DONE]", "FAILED", "upstream rejected stream"},
			} {
				t.Run(fmt.Sprintf("%s/timeouts=%t/reverse_proxy=%t", tc.name, timeouts, reverseProxy), func(t *testing.T) {
					env := NewProxyTestEnv(t)
					upstreamCanceled := make(chan struct{})
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: {\"model\":\"deepseek/deepseek-v4.1-flash\",\"choices\":[{\"delta\":{\"content\":\"hello\"},\"finish_reason\":null}]}\n\n"+tc.tail)
						w.(http.Flusher).Flush()
						// Keep the HTTP response open until the proxy closes its upstream
						// request, whether due to protocol completion or a client disconnect.
						<-r.Context().Done()
						close(upstreamCanceled)
					}))
					defer upstream.Close()
					providerID := createOpenAIChatTimeoutProvider(t, env, "delayed-stream-close", upstream.URL)
					createOpenAIRouteAtPosition(t, env, providerID, 1)
					if timeouts {
						for key, value := range map[string]string{
							"openai_chat_stream_timeouts_enabled":       "true",
							"openai_chat_stream_first_event_timeout_ms": "5000",
							"openai_chat_stream_idle_timeout_ms":        "5000",
						} {
							AssertStatus(t, env.AdminPut("/api/admin/settings/"+key, map[string]any{"value": value}), http.StatusOK)
						}
					}
					requestURL := env.URL("/v1/chat/completions")
					if reverseProxy {
						target, err := url.Parse(env.Server.URL)
						if err != nil {
							t.Fatal(err)
						}
						proxy := httptest.NewServer(httputil.NewSingleHostReverseProxy(target))
						defer proxy.Close()
						requestURL = proxy.URL + "/v1/chat/completions"
					}
					req, err := http.NewRequest(http.MethodPost, requestURL, strings.NewReader(`{"model":"deepseek-v4.1-flash","stream":true,"messages":[{"role":"user","content":"hello"}]}`))
					if err != nil {
						t.Fatal(err)
					}
					req.Header.Set("Content-Type", "application/json")
					client := &http.Client{Timeout: 10 * time.Second}
					resp, err := client.Do(req)
					if err != nil {
						t.Fatal(err)
					}
					defer resp.Body.Close()
					if resp.StatusCode != http.StatusOK {
						t.Fatalf("HTTP status = %d, want 200", resp.StatusCode)
					}
					reader := bufio.NewReader(resp.Body)
					found := false
					for {
						line, err := reader.ReadString('\n')
						if err != nil {
							t.Fatalf("read SSE: %v", err)
						}
						found = found || strings.Contains(line, tc.stopAt)
						if found && strings.TrimSpace(line) == "" {
							break
						}
					}
					// Behave like a client that has consumed the selected SSE event and closes
					// without waiting for the upstream's HTTP response to end.
					if err := resp.Body.Close(); err != nil {
						t.Fatal(err)
					}
					select {
					case <-upstreamCanceled:
					case <-time.After(5 * time.Second):
						t.Fatal("downstream close did not cancel the upstream request")
					}
					var requests []sqlite.ProxyRequest
					deadline := time.Now().Add(5 * time.Second)
					for {
						if err := env.DB.GormDB().Where("status IN ?", []string{"COMPLETED", "FAILED", "CANCELLED"}).Find(&requests).Error; err != nil {
							t.Fatal(err)
						}
						if len(requests) == 1 {
							break
						}
						if time.Now().After(deadline) {
							t.Fatal("request did not reach a final state")
						}
						time.Sleep(10 * time.Millisecond)
					}
					request := requests[0]
					if request.Status != tc.status {
						t.Fatalf("recorded status = %s, want %s; error=%s", request.Status, tc.status, request.Error)
					}
					if tc.errorText == "" && request.Error != "" || tc.errorText != "" && !strings.Contains(string(request.Error), tc.errorText) {
						t.Fatalf("recorded error = %q, want %q", request.Error, tc.errorText)
					}
					if request.StatusCode != http.StatusOK {
						t.Fatalf("recorded HTTP status = %d", request.StatusCode)
					}
					var attempts []sqlite.ProxyUpstreamAttempt
					if err := env.DB.GormDB().Where("proxy_request_id = ?", request.ID).Find(&attempts).Error; err != nil {
						t.Fatal(err)
					}
					attemptStatus := tc.status
					if tc.errorText == "client disconnected" {
						attemptStatus = "CANCELLED"
					}
					if len(attempts) != 1 || attempts[0].Status != attemptStatus {
						t.Fatalf("unexpected attempts: %+v", attempts)
					}
					if tc.status == "COMPLETED" && (attempts[0].InputTokenCount != 12 || attempts[0].OutputTokenCount != 5) {
						t.Fatalf("usage lost: input=%d output=%d", attempts[0].InputTokenCount, attempts[0].OutputTokenCount)
					}
				})
			}
		}
	}
}
