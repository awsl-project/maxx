package error_fixer

// Upstream rejects thinking-block envelopes it cannot verify. Two
// concrete phrases appear in the wild today, both produced by the
// same Anthropic-style validator and both fixable the same way:
//
//   400 {"error":{"message":"... Invalid `signature` in `thinking` block ..."}}
//   400 {"message":"... Invalid `data` in `redacted_thinking` block ..."}
//
// The envelopes (`signature` on `thinking`, `data` on
// `redacted_thinking`) are scoped to the deployment that produced
// them, so neither survives a cross-deployment replay (router
// switch, multi-account rotation, client restoring history into a
// different backend, etc.).
//
// We match the family of errors with a single regex rather than the
// two literal phrases so that future field additions on the same two
// block types (e.g. a new `Invalid \`encryption_key\` in \`thinking\`
// block`) are picked up automatically. The block-type group is
// pinned to thinking / redacted_thinking so we never strip thinking
// blocks in response to an error from an unrelated block type.

import (
	"net/http"

	"github.com/awsl-project/maxx/internal/adapter/provider/bedrock"
	"github.com/awsl-project/maxx/internal/domain"
)

var _ ErrorFixer = (*thinkingEnvelopeFixer)(nil)

func init() {
	Register(&thinkingEnvelopeFixer{})
}

type thinkingEnvelopeFixer struct{}

func (f *thinkingEnvelopeFixer) Name() string  { return "thinking_envelope" }
func (f *thinkingEnvelopeFixer) Priority() int { return 100 }

func (f *thinkingEnvelopeFixer) MatchResponse(resp *http.Response, body []byte, clientType domain.ClientType) bool {
	if clientType != domain.ClientTypeClaude {
		return false
	}
	if resp != nil && resp.StatusCode != 400 {
		return false
	}
	return bedrock.IsThinkingBlockEnvelopeError(body)
}

func (f *thinkingEnvelopeFixer) FixRequest(req *http.Request, body []byte) (*http.Request, []byte) {
	return req, bedrock.StripThinkingBlocks(body)
}
