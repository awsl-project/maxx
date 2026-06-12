package error_fixer

// AWS Bedrock has strict schema validation and rejects any unrecognized field.
// It only reports ONE field per error, so without aggressive stripping,
// fixing N fields would need N round-trips.
//
// This fixer detects Bedrock by error signature and strips ALL known
// unsupported fields at once.
//
// Error examples (real-world):
//   400 "InvokeModel: operation error Bedrock Runtime: InvokeModel, ...
//        ValidationException: output_config.effort: Extra inputs are not permitted"
//   400 "...ValidationException: ***.***.***.custom: Extra inputs are not permitted"
//        (some proxies mask field paths)

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/awsl-project/maxx/internal/adapter/provider/bedrock"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var _ ErrorFixer = (*bedrockFixer)(nil)

func init() {
	Register(&bedrockFixer{})
}

type bedrockFixer struct{}

func (f *bedrockFixer) Name() string    { return "bedrock" }
func (f *bedrockFixer) Priority() int { return 0 } // highest — covers everything

func (f *bedrockFixer) MatchResponse(resp *http.Response, body []byte, clientType domain.ClientType) bool {
	if resp == nil || resp.StatusCode != 400 || clientType != domain.ClientTypeClaude {
		return false
	}
	// Bedrock SDK errors carry "Bedrock Runtime" / "InvokeModel" in the
	// AWS SDK error chain. We match on either marker.
	if !bytes.Contains(body, []byte("Bedrock Runtime")) &&
		!bytes.Contains(body, []byte("InvokeModel")) {
		return false
	}
	// Decline when the body is actually a thinking-block envelope
	// rejection wrapped in an AWS SDK message — that case has its own
	// fixer (thinking_envelope) which strips the offending blocks.
	// Without this opt-out, priority-exclusive matching would hand the
	// request to bedrockFixer, which strips unrelated fields and
	// leaves the thinking blocks in place; the retry then fails the
	// same way and the thinking_envelope fixer never gets a chance.
	if bedrock.IsThinkingBlockEnvelopeError(body) {
		return false
	}
	return true
}

func (f *bedrockFixer) FixRequest(req *http.Request, body []byte) (*http.Request, []byte) {
	// 1. Handle cache_control:
	// Modern Bedrock supports cache_control.type but rejects sub-fields like "scope".
	// Only strip scope first; if cache_control itself is rejected (older regions/configs),
	// the error will contain "cache_control" without "scope" and the cache_control fixer
	// will handle it in a subsequent round.
	body = stripCacheControlScope(body)

	// 2. Strip known unsupported top-level fields
	for _, field := range []string{"output_config", "context_management", "reasoning"} {
		if gjson.GetBytes(body, field).Exists() {
			body, _ = sjson.DeleteBytes(body, field)
		}
	}

	// 3. Strip tools[].custom
	tools := gjson.GetBytes(body, "tools")
	if tools.IsArray() {
		for i := int(tools.Get("#").Int()) - 1; i >= 0; i-- {
			path := fmt.Sprintf("tools.%d.custom", i)
			if gjson.GetBytes(body, path).Exists() {
				body, _ = sjson.DeleteBytes(body, path)
			}
		}
	}

	// 4. Remove empty top-level arrays (Bedrock rejects empty system/tools)
	for _, field := range []string{"system", "tools"} {
		v := gjson.GetBytes(body, field)
		if v.IsArray() && v.Get("#").Int() == 0 {
			body, _ = sjson.DeleteBytes(body, field)
		}
	}

	// 5. Filter anthropic-beta header (Bedrock rejects unknown betas too)
	filterRejectedBetas(req)

	return req, body
}

// rejectedBetas lists beta values known to be rejected by strict upstreams.
// Shared by bedrockFixer and betaHeaderFixer.
var rejectedBetas = map[string]bool{
	"prompt-caching-scope-2026-01-05":  true,
	"tmp-preserve-thinking-2025-10-01": true,
	"advanced-tool-use-2025-11-20":     true,
	"web-search-2025-03-05":            true,
	"context-management-2025-06-27":    true,
}
