package error_fixer

// Upstream rejects certain anthropic-beta header values (e.g. Vertex AI).
// Error examples:
//   400 "Unexpected value(s) 'prompt-caching-scope-2026-01-05' for the 'anthropic-beta' header"
//   400 "invalid beta flag"
//
// Note: Bedrock beta header issues are handled by bedrock.go alongside body fields.
// This fixer handles non-Bedrock upstreams that only reject specific betas.
//
// Fix: remove the rejected beta value(s) from the anthropic-beta header.

import (
	"bytes"
	"net/http"
	"strings"

	"github.com/awsl-project/maxx/internal/domain"
)

var _ ErrorFixer = (*betaHeaderFixer)(nil)

func init() {
	Register(&betaHeaderFixer{})
}

type betaHeaderFixer struct{}

func (f *betaHeaderFixer) Name() string    { return "beta_header" }
func (f *betaHeaderFixer) Priority() int { return 100 }

func (f *betaHeaderFixer) MatchResponse(resp *http.Response, body []byte, clientType domain.ClientType) bool {
	if clientType != domain.ClientTypeClaude {
		return false
	}
	if resp != nil && resp.StatusCode != 400 {
		return false
	}
	return bytes.Contains(body, []byte("anthropic-beta")) ||
		bytes.Contains(body, []byte("beta flag"))
}

func (f *betaHeaderFixer) FixRequest(req *http.Request, body []byte) (*http.Request, []byte) {
	filterRejectedBetas(req)
	return req, body
}

// filterRejectedBetas removes known-rejected beta values from the anthropic-beta header.
// Shared by betaHeaderFixer and bedrockFixer.
func filterRejectedBetas(req *http.Request) {
	betaHeader := req.Header.Get("anthropic-beta")
	if betaHeader == "" {
		return
	}

	parts := strings.Split(betaHeader, ",")
	var kept []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" && !rejectedBetas[trimmed] {
			kept = append(kept, trimmed)
		}
	}

	if len(kept) == 0 {
		req.Header.Del("anthropic-beta")
	} else {
		req.Header.Set("anthropic-beta", strings.Join(kept, ", "))
	}
}
