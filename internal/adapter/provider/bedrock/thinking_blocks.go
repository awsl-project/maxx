package bedrock

import (
	"fmt"
	"regexp"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Anthropic-style upstreams (Claude API and Bedrock alike) reject
// extended-thinking blocks whose cryptographic envelope they cannot
// verify:
//
//   thinking          — has a `signature` field; rejected as
//                       "Invalid `signature` in `thinking` block"
//   redacted_thinking — has an opaque `data` field; rejected as
//                       "Invalid `data` in `redacted_thinking` block"
//
// Both envelopes are scoped to the deployment that produced them
// (account, model, sometimes private wrapper format), so a block
// emitted by one provider is not portable to another. When a client
// replays history across providers (e.g. Claude Code CLI replaying a
// transcript captured against Anthropic against Bedrock), the
// destination rejects the offending block.
//
// The fix is symmetric in both cases: drop every thinking and
// redacted_thinking block. Thinking context for the current turn is
// lost, but the rest of the conversation survives — preferable to a
// hard 400 that exhausts the retry budget. Lives in the bedrock
// package because bedrock is already the canonical home for the
// Anthropic-shape sanitizers shared with custom/error_fixer.

// emptyAssistantPlaceholder is the content we leave on an assistant
// message that was nothing but thinking blocks. Upstreams reject both
// empty content arrays and zero-length text, and dropping the whole
// message would merge adjacent user turns (also rejected). A minimal
// text block keeps turn structure intact for the retry.
var emptyAssistantPlaceholder = []byte(`[{"type":"text","text":"[thinking omitted]"}]`)

// StripThinkingBlocks removes every content block whose `type` is
// `thinking` or `redacted_thinking` from all messages. Plain-string
// content and non-thinking blocks are preserved. When stripping
// empties an assistant message's content array, a single placeholder
// text block is inserted to keep the retry valid — dropping the
// message would create adjacent user turns and an empty array is
// itself a validation error on most upstreams. Idempotent.
//
// The caller's slice is not modified: sjson's mutation helpers may
// edit their input in place when there's room, so we defensively
// copy on entry. This is rarely on a hot path (error recovery only)
// so the extra allocation is negligible, and it lets callers reuse
// the original body unchanged on the retry path.
func StripThinkingBlocks(payload []byte) []byte {
	messages := gjson.GetBytes(payload, "messages")
	if !messages.IsArray() {
		return payload
	}
	payload = append([]byte(nil), payload...)
	for i := int(messages.Get("#").Int()) - 1; i >= 0; i-- {
		contentPath := fmt.Sprintf("messages.%d.content", i)
		content := gjson.GetBytes(payload, contentPath)
		if !content.IsArray() {
			continue
		}
		removed := false
		// Iterate in reverse so index-based deletes do not shift
		// remaining indices.
		for j := int(content.Get("#").Int()) - 1; j >= 0; j-- {
			blockType := gjson.GetBytes(payload, fmt.Sprintf("%s.%d.type", contentPath, j)).String()
			if blockType == "thinking" || blockType == "redacted_thinking" {
				payload, _ = sjson.DeleteBytes(payload, fmt.Sprintf("%s.%d", contentPath, j))
				removed = true
			}
		}
		if !removed {
			continue
		}
		if gjson.GetBytes(payload, contentPath+".#").Int() > 0 {
			continue
		}
		role := gjson.GetBytes(payload, fmt.Sprintf("messages.%d.role", i)).String()
		if role != "assistant" {
			// Only assistant turns legitimately carry thinking blocks.
			// If a non-assistant role somehow had only thinking
			// content, we leave the empty array as-is rather than
			// fabricate content for a role that never produced it.
			continue
		}
		payload, _ = sjson.SetRawBytes(payload, contentPath, emptyAssistantPlaceholder)
	}
	return payload
}

// thinkingEnvelopePattern matches the family of Anthropic-style
// validator phrases of the form "Invalid `<field>` in `<block>`
// block" where <block> is a thinking-block type. Generic over the
// field so future field validations on the same block types are
// covered without code changes; pinned on the block type so we never
// strip thinking blocks in response to an error from an unrelated
// block (e.g. tool_result). Backticks are required: they distinguish
// these from unrelated 400s that mention "signature" (AWS SigV4) or
// "thinking" (budget validation) in plain prose.
var thinkingEnvelopePattern = regexp.MustCompile("Invalid `[^`]+` in `(?:thinking|redacted_thinking)` block")

// IsThinkingBlockEnvelopeError reports whether the upstream error
// body is an Anthropic-style rejection of a thinking-block envelope
// that can be recovered from by stripping the offending blocks.
// Today that covers `Invalid \`signature\` in \`thinking\` block` and
// `Invalid \`data\` in \`redacted_thinking\` block`; the pattern is
// deliberately generic so a future field rejection on the same block
// types matches automatically.
func IsThinkingBlockEnvelopeError(body []byte) bool {
	return thinkingEnvelopePattern.Match(body)
}
