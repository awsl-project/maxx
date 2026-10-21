package e2e_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider"
	cpaantigravity "github.com/awsl-project/maxx/internal/adapter/provider/cliproxyapi_antigravity"
	cpacodex "github.com/awsl-project/maxx/internal/adapter/provider/cliproxyapi_codex"
	cpagrok "github.com/awsl-project/maxx/internal/adapter/provider/cliproxyapi_grok"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
)

// Exercise the real CPA executors and translators against a local HTTP upstream.
// The request-scoped transport also keeps OAuth refresh calls inside this fixture.
func TestCPAExecutorsAgainstLocalUpstream(t *testing.T) {
	clients := []struct {
		name, model, path, body string
		clientType              domain.ClientType
		newAdapter              func(*domain.Provider) (provider.ProviderAdapter, error)
	}{
		{"codex", "gpt-5.4", "/v1/responses", `{"input":[{"role":"user","content":"hello"}]}`, domain.ClientTypeCodex, cpacodex.NewAdapter},
		{"antigravity-gemini", "gemini-2.5-flash", "/v1beta/models/gemini-2.5-flash:generateContent", `{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`, domain.ClientTypeGemini, cpaantigravity.NewAdapter},
		{"antigravity-claude", "claude-sonnet-4-5", "/v1/messages", `{"max_tokens":32,"messages":[{"role":"user","content":"hello"}]}`, domain.ClientTypeClaude, cpaantigravity.NewAdapter},
		{"grok", "grok-4", "/v1/chat/completions", `{"messages":[{"role":"user","content":"hello"}]}`, domain.ClientTypeOpenAI, cpagrok.NewAdapter},
	}
	for _, client := range clients {
		for _, stream := range []bool{false, true} {
			for _, status := range []int{http.StatusOK, http.StatusUnauthorized, http.StatusTooManyRequests} {
				t.Run(fmt.Sprintf("%s/stream=%t/status=%d", client.name, stream, status), func(t *testing.T) {
					var refreshes, generations atomic.Int32
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path == "/token" {
							refreshes.Add(1)
							if err := r.ParseForm(); err != nil || r.Form.Get("grant_type") != "refresh_token" {
								t.Errorf("invalid refresh request: %v", err)
							}
							w.Header().Set("Content-Type", "application/json")
							_, _ = io.WriteString(w, `{"access_token":"local-access-token","expires_in":3600,"token_type":"Bearer"}`)
							return
						}
						if !strings.Contains(r.URL.Path, "responses") && !strings.Contains(r.URL.Path, "GenerateContent") && !strings.Contains(r.URL.Path, "generateContent") {
							t.Errorf("unexpected upstream path: %s", r.URL.Path)
							http.Error(w, "unexpected upstream path", http.StatusNotFound)
							return
						}
						generations.Add(1)
						if got := r.Header.Get("Authorization"); got != "Bearer local-access-token" {
							t.Errorf("upstream authorization = %q", got)
						}
						if status != http.StatusOK {
							w.Header().Set("Content-Type", "application/json")
							w.WriteHeader(status)
							_, _ = io.WriteString(w, `{"error":{"message":"local upstream rejection"}}`)
							return
						}
						if strings.Contains(r.URL.Path, "responses") {
							w.Header().Set("Content-Type", "text/event-stream")
							message := `{"type":"message","id":"msg_local","role":"assistant","status":"completed","content":[{"type":"output_text","text":"cpa-v8-ok"}]}`
							_, _ = fmt.Fprintf(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_local\",\"model\":%q,\"output\":[]}}\n\n", client.model)
							_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_local\",\"output_index\":0,\"content_index\":0,\"delta\":\"cpa-v8-ok\"}\n\n")
							_, _ = fmt.Fprintf(w, "data: {\"type\":\"response.output_item.done\",\"output_index\":0,\"item\":%s}\n\n", message)
							_, _ = fmt.Fprintf(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_local\",\"object\":\"response\",\"status\":\"completed\",\"model\":%q,\"output\":[%s],\"usage\":{\"input_tokens\":3,\"output_tokens\":2,\"total_tokens\":5}}}\n\n", client.model, message)
							return
						}
						response := fmt.Sprintf(`{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"cpa-v8-ok"}]},"finishReason":"STOP","index":0}],"modelVersion":%q,"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":2,"totalTokenCount":5}}}`, client.model)
						if strings.Contains(r.URL.Path, "streamGenerateContent") {
							w.Header().Set("Content-Type", "text/event-stream")
							_, _ = fmt.Fprintf(w, "data: %s\n\n", response)
						} else {
							w.Header().Set("Content-Type", "application/json")
							_, _ = io.WriteString(w, response)
						}
					}))
					defer upstream.Close()
					target, err := url.Parse(upstream.URL)
					if err != nil {
						t.Fatal(err)
					}
					transport := &cpaLocalTransport{target: target, next: upstream.Client().Transport}
					cfg := &domain.ProviderConfig{
						Codex:                  &domain.ProviderConfigCodex{AccessToken: "local-access-token", ExpiresAt: time.Now().Add(time.Hour).Format(time.RFC3339), BaseURL: upstream.URL},
						CLIProxyAPIAntigravity: &domain.ProviderConfigCLIProxyAPIAntigravity{RefreshToken: upstream.URL, ProjectID: "local-project"},
						Grok:                   &domain.ProviderConfigGrok{AccessToken: "local-access-token", BaseURL: upstream.URL},
					}
					p := &domain.Provider{ID: 1, Name: client.name, Config: cfg}
					adapter, err := client.newAdapter(p)
					if err != nil {
						t.Fatal(err)
					}
					body := []byte(fmt.Sprintf(`{"model":%q,"stream":%t,%s`, client.model, stream, client.body[1:]))
					req := httptest.NewRequest(http.MethodPost, client.path, strings.NewReader(string(body)))
					ctx := context.WithValue(req.Context(), "cliproxy.roundtripper", http.RoundTripper(transport))
					rec := httptest.NewRecorder()
					c := flow.NewCtx(rec, req.WithContext(ctx))
					c.Set(flow.KeyClientType, client.clientType)
					c.Set(flow.KeyRequestBody, body)
					c.Set(flow.KeyRequestModel, client.model)
					c.Set(flow.KeyMappedModel, client.model)
					c.Set(flow.KeyRequestURI, client.path)
					c.Set(flow.KeyIsStream, stream)
					err = adapter.Execute(c, p)
					if generations.Load() == 0 {
						t.Fatalf("SDK did not reach local upstream: %v", err)
					}
					if strings.HasPrefix(client.name, "antigravity") && refreshes.Load() == 0 {
						t.Fatal("SDK did not refresh the Antigravity token")
					}
					if status != http.StatusOK {
						var proxyErr *domain.ProxyError
						if !errors.As(err, &proxyErr) || proxyErr.HTTPStatusCode != status {
							t.Fatalf("upstream error = %v, want HTTP status %d", err, status)
						}
						return
					}
					if err != nil || rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "cpa-v8-ok") {
						t.Fatalf("SDK response: error=%v status=%d body=%s", err, rec.Code, rec.Body.String())
					}
					if stream && !strings.Contains(rec.Header().Get("Content-Type"), "text/event-stream") {
						t.Fatalf("stream content type = %q", rec.Header().Get("Content-Type"))
					}
				})
			}
		}
	}
}

type cpaLocalTransport struct {
	target *url.URL
	next   http.RoundTripper
}

func (rt *cpaLocalTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	local := req.Clone(req.Context())
	local.URL.Scheme = rt.target.Scheme
	local.URL.Host = rt.target.Host
	local.Host = rt.target.Host
	return rt.next.RoundTrip(local)
}
