package error_fixer

// Upstream rejects thinking blocks whose signatures it cannot verify.
// Signatures are scoped to the deployment that produced them (account,
// model, sometimes private wrapper format), so a signature from one
// provider is not portable to another. When a client replays extended-
// thinking history across providers, the destination may reject it with
// messages like:
//
//   400 {"error":{"type":"<nil>","message":"messages.0.content.0: Invalid `signature` in `thinking` block (request id: ...) (request id: ...)"}}
//
// Fix: drop every content block whose type is "thinking" or
// "redacted_thinking" from all messages, then retry. Thinking context
// for the current turn is lost, but the rest of the conversation
// survives — preferable to a hard 400 that exhausts the retry budget.

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var _ ErrorFixer = (*thinkingSignatureFixer)(nil)

func init() {
	Register(&thinkingSignatureFixer{})
}

type thinkingSignatureFixer struct{}

func (f *thinkingSignatureFixer) Name() string  { return "thinking_signature" }
func (f *thinkingSignatureFixer) Priority() int { return 100 }

func (f *thinkingSignatureFixer) MatchResponse(resp *http.Response, body []byte, clientType domain.ClientType) bool {
	if clientType != domain.ClientTypeClaude {
		return false
	}
	if resp != nil && resp.StatusCode != 400 {
		return false
	}
	// Match the exact backticked phrase produced by Anthropic-style
	// validators: "Invalid `signature` in `thinking` block". The
	// backticks distinguish this from unrelated 400 bodies that happen
	// to mention "signature" (e.g. AWS SigV4 auth errors) or "thinking"
	// (e.g. budget / config validation errors).
	return bytes.Contains(body, []byte("Invalid `signature` in `thinking` block"))
}

func (f *thinkingSignatureFixer) FixRequest(req *http.Request, body []byte) (*http.Request, []byte) {
	return req, stripThinkingBlocks(body)
}

// stripThinkingBlocks removes every content block whose `type` is
// `thinking` or `redacted_thinking` from all messages. Non-array
// content (plain string) and non-thinking blocks are preserved. If
// stripping empties a message's content array, the message is left
// in place — downstream semantics (empty content validation) are
// upstream's responsibility, and dropping the whole message would
// alter turn ordering.
func stripThinkingBlocks(payload []byte) []byte {
	messages := gjson.GetBytes(payload, "messages")
	if !messages.IsArray() {
		return payload
	}
	for i := int(messages.Get("#").Int()) - 1; i >= 0; i-- {
		contentPath := fmt.Sprintf("messages.%d.content", i)
		content := gjson.GetBytes(payload, contentPath)
		if !content.IsArray() {
			continue
		}
		// Iterate in reverse so index-based deletes do not shift
		// remaining indices.
		for j := int(content.Get("#").Int()) - 1; j >= 0; j-- {
			blockType := gjson.GetBytes(payload, fmt.Sprintf("%s.%d.type", contentPath, j)).String()
			if blockType == "thinking" || blockType == "redacted_thinking" {
				payload, _ = sjson.DeleteBytes(payload, fmt.Sprintf("%s.%d", contentPath, j))
			}
		}
	}
	return payload
}
