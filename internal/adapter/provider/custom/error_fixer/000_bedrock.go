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
	// Only match when the error is clearly from AWS Bedrock.
	// "Bedrock Runtime" appears in the AWS SDK error chain.
	// "InvokeModel" / "InvokeModelWithResponseStream" are Bedrock API operations.
	return bytes.Contains(body, []byte("Bedrock Runtime")) ||
		bytes.Contains(body, []byte("InvokeModel"))
}

func (f *bedrockFixer) FixRequest(req *http.Request, body []byte) (*http.Request, []byte) {
	// 1. Strip cache_control from system, tools, messages
	body = stripAllCacheControl(body)

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

	// 4. Filter anthropic-beta header (Bedrock rejects unknown betas too)
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
