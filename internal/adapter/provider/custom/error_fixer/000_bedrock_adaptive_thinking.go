package error_fixer

import (
	"bytes"
	"net/http"

	"github.com/awsl-project/maxx/internal/adapter/provider/bedrock"
	"github.com/awsl-project/maxx/internal/domain"
)

var _ ErrorFixer = (*bedrockAdaptiveThinkingFixer)(nil)

func init() {
	Register(&bedrockAdaptiveThinkingFixer{})
}

type bedrockAdaptiveThinkingFixer struct{}

func (f *bedrockAdaptiveThinkingFixer) Name() string { return "bedrock_adaptive_thinking" }
func (f *bedrockAdaptiveThinkingFixer) Priority() int {
	return 0
}

func (f *bedrockAdaptiveThinkingFixer) MatchResponse(resp *http.Response, body []byte, clientType domain.ClientType) bool {
	if resp == nil || resp.StatusCode != http.StatusBadRequest || clientType != domain.ClientTypeClaude {
		return false
	}
	if !bytes.Contains(body, []byte("Bedrock Runtime")) &&
		!bytes.Contains(body, []byte("InvokeModel")) {
		return false
	}
	return bedrock.IsClassicThinkingRejectedError(body)
}

func (f *bedrockAdaptiveThinkingFixer) FixRequest(req *http.Request, body []byte) (*http.Request, []byte) {
	body = bedrock.StripSamplingParams(body)
	body = bedrock.RewriteClassicThinkingToAdaptive(body)
	return req, body
}
