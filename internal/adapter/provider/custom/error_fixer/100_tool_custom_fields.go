package error_fixer

// Upstream rejects fields inside tools[].custom (e.g. AWS Bedrock).
// Error examples:
//   400 "tools.0.custom.defer_loading: Extra inputs are not permitted"
//   400 "tools.0.custom.eager_input_streaming: Extra inputs are not permitted"
//   400 "tools.3.custom.input_examples: Extra inputs are not permitted"
//
// Fix: strip the entire "custom" object from each tool definition,
// since Bedrock does not recognize any sub-fields in it.

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// toolCustomKeywords are substrings that indicate a tools[].custom rejection.
var toolCustomKeywords = []string{
	"defer_loading",
	"eager_input_streaming",
	"input_examples",
}

var _ ErrorFixer = (*toolCustomFieldsFixer)(nil)

func init() {
	Register(&toolCustomFieldsFixer{})
}

type toolCustomFieldsFixer struct{}

func (f *toolCustomFieldsFixer) Name() string    { return "tool_custom_fields" }
func (f *toolCustomFieldsFixer) Priority() int { return 100 }

func (f *toolCustomFieldsFixer) MatchResponse(resp *http.Response, body []byte, clientType domain.ClientType) bool {
	if clientType != domain.ClientTypeClaude {
		return false
	}
	if resp != nil && resp.StatusCode != 400 {
		return false
	}
	// Require "Extra inputs are not permitted" to avoid false positives.
	// Also match ".custom." for generic tool custom field errors.
	if !bytes.Contains(body, []byte("Extra inputs are not permitted")) {
		return false
	}
	for _, kw := range toolCustomKeywords {
		if bytes.Contains(body, []byte(kw)) {
			return true
		}
	}
	// Generic match: any custom sub-field error (e.g. new fields added by Anthropic)
	return bytes.Contains(body, []byte(".custom."))
}

func (f *toolCustomFieldsFixer) FixRequest(req *http.Request, body []byte) (*http.Request, []byte) {
	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return req, body
	}
	for i := int(tools.Get("#").Int()) - 1; i >= 0; i-- {
		path := fmt.Sprintf("tools.%d.custom", i)
		if gjson.GetBytes(body, path).Exists() {
			body, _ = sjson.DeleteBytes(body, path)
		}
	}
	return req, body
}
