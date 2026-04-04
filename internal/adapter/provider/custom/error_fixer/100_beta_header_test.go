package error_fixer

import (
	"net/http"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestBetaHeaderFixer_MatchResponse(t *testing.T) {
	f := &betaHeaderFixer{}

	tests := []struct {
		name       string
		resp       *http.Response
		body       string
		clientType domain.ClientType
		want       bool
	}{
		{
			name:       "anthropic-beta rejected",
			resp:       &http.Response{StatusCode: 400},
			body:       `{"error":{"message":"Unexpected value(s) 'prompt-caching-scope-2026-01-05' for the 'anthropic-beta' header"}}`,
			clientType: domain.ClientTypeClaude,
			want:       true,
		},
		{
			name:       "invalid beta flag",
			resp:       &http.Response{StatusCode: 400},
			body:       `{"error":{"message":"invalid beta flag"}}`,
			clientType: domain.ClientTypeClaude,
			want:       true,
		},
		{
			name:       "unrelated error",
			resp:       &http.Response{StatusCode: 400},
			body:       `{"error":{"message":"invalid model"}}`,
			clientType: domain.ClientTypeClaude,
			want:       false,
		},
		{
			name:       "nil response SSE path",
			resp:       nil,
			body:       `anthropic-beta header rejected`,
			clientType: domain.ClientTypeClaude,
			want:       true,
		},
		{
			name:       "wrong client type",
			resp:       &http.Response{StatusCode: 400},
			body:       `anthropic-beta`,
			clientType: domain.ClientTypeGemini,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.MatchResponse(tt.resp, []byte(tt.body), tt.clientType)
			if got != tt.want {
				t.Errorf("MatchResponse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBetaHeaderFixer_FixRequest(t *testing.T) {
	f := &betaHeaderFixer{}

	tests := []struct {
		name       string
		betaValue  string
		wantHeader string
		wantEmpty  bool
	}{
		{
			name:       "strip single rejected beta",
			betaValue:  "prompt-caching-scope-2026-01-05",
			wantHeader: "",
			wantEmpty:  true,
		},
		{
			name:       "keep supported, strip rejected",
			betaValue:  "max-tokens-3-5-sonnet-2024-07-15,prompt-caching-scope-2026-01-05,output-128k-2025-02-19",
			wantHeader: "max-tokens-3-5-sonnet-2024-07-15, output-128k-2025-02-19",
		},
		{
			name:       "strip multiple rejected betas",
			betaValue:  "tmp-preserve-thinking-2025-10-01,advanced-tool-use-2025-11-20",
			wantHeader: "",
			wantEmpty:  true,
		},
		{
			name:       "keep all if none rejected",
			betaValue:  "max-tokens-3-5-sonnet-2024-07-15, output-128k-2025-02-19",
			wantHeader: "max-tokens-3-5-sonnet-2024-07-15, output-128k-2025-02-19",
		},
		{
			name:       "no header at all",
			betaValue:  "",
			wantHeader: "",
			wantEmpty:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", "https://example.com", nil)
			if tt.betaValue != "" {
				req.Header.Set("anthropic-beta", tt.betaValue)
			}

			fixedReq, _ := f.FixRequest(req, []byte(`{}`))

			got := fixedReq.Header.Get("anthropic-beta")
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("expected empty anthropic-beta header, got %q", got)
				}
			} else {
				if got != tt.wantHeader {
					t.Errorf("anthropic-beta = %q, want %q", got, tt.wantHeader)
				}
			}
		})
	}
}

func TestBetaHeaderFixer_FixRequest_PreservesBody(t *testing.T) {
	f := &betaHeaderFixer{}

	body := []byte(`{"model":"claude-sonnet-4-20250514","messages":[]}`)
	req, _ := http.NewRequest("POST", "https://example.com", nil)
	req.Header.Set("anthropic-beta", "prompt-caching-scope-2026-01-05")

	_, result := f.FixRequest(req, body)

	if string(result) != string(body) {
		t.Error("body should not be modified by beta header fixer")
	}
}
