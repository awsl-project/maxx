package error_fixer

import (
	"bytes"
	"net/http"

	"github.com/awsl-project/maxx/internal/adapter/provider/bedrock"
	"github.com/awsl-project/maxx/internal/domain"
)

var _ ErrorFixer = (*bedrockClassicThinkingFixer)(nil)

func init() {
	Register(&bedrockClassicThinkingFixer{})
}

type bedrockClassicThinkingFixer struct{}

func (f *bedrockClassicThinkingFixer) Name() string { return "bedrock_classic_thinking" }
func (f *bedrockClassicThinkingFixer) Priority() int {
	return 0
}

func (f *bedrockClassicThinkingFixer) MatchResponse(resp *http.Response, body []byte, clientType domain.ClientType) bool {
	if resp == nil || resp.StatusCode != http.StatusBadRequest || clientType != domain.ClientTypeClaude {
		return false
	}
	if !bytes.Contains(body, []byte("Bedrock Runtime")) &&
		!bytes.Contains(body, []byte("InvokeModel")) {
		return false
	}
	return bedrock.IsAdaptiveThinkingRejectedError(body)
}

func (f *bedrockClassicThinkingFixer) FixRequest(req *http.Request, body []byte) (*http.Request, []byte) {
	rewritten := bedrock.RewriteAdaptiveThinkingToClassic(body)
	return (&bedrockFixer{}).FixRequest(req, rewritten)
}
