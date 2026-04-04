package error_fixer

// Upstream rejects top-level body fields not in its schema (e.g. AWS Bedrock).
// Error examples:
//   400 "output_config.effort: Extra inputs are not permitted"
//   400 "context_management: Extra inputs are not permitted"
//   400 "reasoning: Extra inputs are not permitted"
//
// Fix: strip the offending top-level field from the request body.

import (
	"bytes"
	"net/http"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// extraBodyFields lists top-level fields that strict upstreams may reject.
var extraBodyFields = []string{
	"output_config",
	"context_management",
	"reasoning",
}

var _ ErrorFixer = (*extraBodyFieldsFixer)(nil)

func init() {
	Register(&extraBodyFieldsFixer{})
}

type extraBodyFieldsFixer struct{}

func (f *extraBodyFieldsFixer) Name() string    { return "extra_body_fields" }
func (f *extraBodyFieldsFixer) Priority() int { return 100 }

func (f *extraBodyFieldsFixer) MatchResponse(resp *http.Response, body []byte, clientType domain.ClientType) bool {
	if clientType != domain.ClientTypeClaude {
		return false
	}
	if resp != nil && resp.StatusCode != 400 {
		return false
	}
	// Require "Extra inputs are not permitted" to avoid false positives
	// on errors that coincidentally contain words like "reasoning".
	if !bytes.Contains(body, []byte("Extra inputs are not permitted")) {
		return false
	}
	for _, field := range extraBodyFields {
		if bytes.Contains(body, []byte(field)) {
			return true
		}
	}
	return false
}

func (f *extraBodyFieldsFixer) FixRequest(req *http.Request, body []byte) (*http.Request, []byte) {
	for _, field := range extraBodyFields {
		if gjson.GetBytes(body, field).Exists() {
			body, _ = sjson.DeleteBytes(body, field)
		}
	}
	return req, body
}
