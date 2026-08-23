package custom

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
)

// The async video-generation surface must be forwarded verbatim: the inbound
// HTTP method (POST submit / GET poll) is preserved, the request URI — including
// the /{task_id} suffix on a poll — is passed through unchanged, and the
// provider's bearer key is injected.
func TestCustomAdapterExecuteVideoSubmitAndPoll(t *testing.T) {
	cases := []struct {
		name       string
		method     string
		requestURI string
		body       []byte
		wantPath   string
	}{
		{
			name:       "submit",
			method:     http.MethodPost,
			requestURI: "/v1/video/generations",
			body:       []byte(`{"model":"doubao-seedance-2-0-260128","prompt":"a cat runs"}`),
			wantPath:   "/v1/video/generations",
		},
		{
			name:       "poll",
			method:     http.MethodGet,
			requestURI: "/v1/video/generations/task_abc123",
			body:       nil,
			wantPath:   "/v1/video/generations/task_abc123",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotMethod, gotPath, gotAuth, gotAPIKey, gotGoog, gotProxyAuth string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path
				gotAuth = r.Header.Get("Authorization")
				gotAPIKey = r.Header.Get("x-api-key")
				gotGoog = r.Header.Get("x-goog-api-key")
				gotProxyAuth = r.Header.Get("Proxy-Authorization")
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"task_id":"task_abc123","status":"queued"}`)
			}))
			defer server.Close()

			adapter, err := NewAdapter(&domain.Provider{
				Name:                 "code0-seedance",
				Type:                 "custom",
				SupportedClientTypes: []domain.ClientType{domain.ClientTypeVideo},
				Config: &domain.ProviderConfig{
					Custom: &domain.ProviderConfigCustom{
						BaseURL: server.URL,
						APIKey:  "sk-seedance",
					},
				},
			})
			if err != nil {
				t.Fatalf("NewAdapter error: %v", err)
			}

			req, _ := http.NewRequestWithContext(context.Background(), tc.method, "http://localhost"+tc.requestURI, nil)
			// The client may present its maxx token in any accepted auth header; none
			// of them must reach the upstream — only the provider key does.
			req.Header.Set("Authorization", "Bearer sk-maxx-client-token")
			req.Header.Set("x-api-key", "sk-maxx-client-token")
			req.Header.Set("x-goog-api-key", "sk-maxx-client-token")
			req.Header.Set("Proxy-Authorization", "Bearer sk-maxx-client-token")

			rec := httptest.NewRecorder()
			ctx := flow.NewCtx(rec, req)
			ctx.Set(flow.KeyClientType, domain.ClientTypeVideo)
			ctx.Set(flow.KeyRequestURI, tc.requestURI)
			ctx.Set(flow.KeyRequestBody, tc.body)

			if err := adapter.Execute(ctx, &domain.Provider{}); err != nil {
				t.Fatalf("Execute error: %v", err)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if gotMethod != tc.method {
				t.Fatalf("upstream method = %q, want %q", gotMethod, tc.method)
			}
			if gotPath != tc.wantPath {
				t.Fatalf("upstream path = %q, want %q", gotPath, tc.wantPath)
			}
			if gotAuth != "Bearer sk-seedance" {
				t.Fatalf("upstream Authorization = %q, want %q", gotAuth, "Bearer sk-seedance")
			}
			// The client's token must not leak through any alternate auth header.
			if gotAPIKey != "" || gotGoog != "" || gotProxyAuth != "" {
				t.Fatalf("client auth leaked upstream: x-api-key=%q x-goog-api-key=%q Proxy-Authorization=%q", gotAPIKey, gotGoog, gotProxyAuth)
			}
		})
	}
}
