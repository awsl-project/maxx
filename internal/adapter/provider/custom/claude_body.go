package custom

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/awsl-project/maxx/internal/adapter/provider/bedrock"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Claude Code system prompt for cloaking
const claudeCodeSystemPrompt = `You are Claude Code, Anthropic's official CLI for Claude.`

const claudeToolPrefix = "proxy_"

// userIDPattern matches Claude Code format: user_[64-hex]_account__session_[uuid-v4]
var userIDPattern = regexp.MustCompile(`^user_[a-fA-F0-9]{64}_account__session_[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// claudeCLIUserAgentPattern matches official Claude CLI user agent pattern.
// Aligns with sub2api/claude-relay-service detection: claude-cli/x.y.z
var claudeCLIUserAgentPattern = regexp.MustCompile(`(?i)^claude-cli/\d+\.\d+\.\d+`)

// claudeBillingCCHPattern matches volatile cch fields injected in billing header text.
var claudeBillingCCHPattern = regexp.MustCompile(`(?i)\bcch=[^;]*;\s*`)

// processClaudeRequestBody processes Claude request body before sending to upstream.
// The Disguise field on the custom config selects the body transformation mode:
//   - nil / Type=="" / Type=="claude-code" — applyCloaking (legacy default; auto-detects
//     Claude Code clients via UA and injects system prompt + fake user_id when needed)
//   - Type=="none"                         — no disguise body transform (structural
//     fixes for cache_control / tool_results / etc still run)
//   - Type=="bedrock"                      — strip Claude Code identity via
//     bedrock.SanitizeForBedrockCompat so relays whose backend is AWS Bedrock accept the body
//
// Following CLIProxyAPI order:
//  1. strip volatile billing cch fields from system text
//  2. apply disguise (claude-code OR bedrock OR none)
//  3. disableThinkingIfToolChoiceForced
//  4. ensureMinThinkingBudget
//  5. ensureCacheControl (auto-inject if missing)
//  6. sanitizeClaudeMessages (fix empty text content blocks)
//  7. ensureToolResultCorrespondence (inject missing tool_results)
//  8. extractAndRemoveBetas
//
// Returns processed body and extra betas for header.
func processClaudeRequestBody(body []byte, clientUserAgent string, customCfg *domain.ProviderConfigCustom) ([]byte, []string) {
	modelName := gjson.GetBytes(body, "model").String()

	// 1. Strip volatile billing cch fields to keep cache keys stable.
	body = stripVolatileClaudeBillingCCH(body)

	// 2. Apply disguise transformation.
	disguise := disguiseFromCustomConfig(customCfg)
	bedrockMode := false
	switch strings.ToLower(strings.TrimSpace(disguiseType(disguise))) {
	case domain.DisguiseTypeBedrock:
		// Strip Claude Code identifying fields so the upstream relay's Bedrock
		// backend won't reject the request with "invalid beta flag" etc.
		body = bedrock.SanitizeForBedrockCompat(body)
		// Model-aware adaptive thinking: Opus 4.7 and other adaptive-only
		// SKUs treat every request as a thinking request and reject sampling
		// params even when the caller never set thinking.type. The `model`
		// field may already carry a Bedrock-qualified ID after the custom
		// provider's model-mapping rewrite (e.g.
		// "us.anthropic.claude-opus-4-7-20260115-v1:0"), so normalize it to
		// the Anthropic short name first — AdaptThinkingForModel keys on
		// "claude-opus-4-7", not on inference-profile IDs.
		body = bedrock.AdaptThinkingForModel(body, bedrock.ShortNameForModel(modelName))
		bedrockMode = true
	case domain.DisguiseTypeNone:
		// Explicit opt-out: no body transformation.
	default:
		// "" / "claude-code" / unknown / nil — keep legacy claude-code cloak default.
		var ccOpts *domain.DisguiseClaudeCodeOptions
		if disguise != nil {
			ccOpts = disguise.ClaudeCode
		}
		body = applyCloaking(body, clientUserAgent, modelName, ccOpts)
	}

	// 3. Disable thinking if tool_choice forces tool use
	body = disableThinkingIfToolChoiceForced(body)

	// 4. Ensure minimum thinking budget if present
	body = ensureMinThinkingBudget(body)

	// 4b. For bedrock disguise: re-check the `max_tokens > thinking.budget_tokens`
	// invariant. ensureMinThinkingBudget may have just raised budget_tokens to 1024,
	// which can violate Bedrock's constraint even though the body looked valid at step 2.
	if bedrockMode {
		body = bedrock.EnsureMaxTokensAboveThinkingBudget(body)
	}

	// 5. Auto-inject cache_control if missing (CLIProxyAPI behavior)
	if countCacheControls(body) == 0 {
		body = ensureCacheControl(body)
	}

	// 6. Remove empty text content blocks to satisfy Anthropic validation.
	body = sanitizeClaudeMessages(body)

	// 7. Fix tool_use/tool_result correspondence:
	//    a) Remove orphaned tool_results (tool_use_id not in preceding assistant message)
	//    b) Inject missing tool_results (tool_use without corresponding tool_result)
	body = bedrock.RemoveOrphanedToolResults(body)
	body = ensureToolResultCorrespondence(body)

	// 8. Extract betas from body (to be added to header)
	var extraBetas []string
	extraBetas, body = extractAndRemoveBetas(body)

	return body, extraBetas
}

// disguiseFromCustomConfig returns the effective disguise for a custom config,
// migrating any legacy `cloak` field via ResolveDisguise().
func disguiseFromCustomConfig(cfg *domain.ProviderConfigCustom) *domain.ProviderConfigCustomDisguise {
	return cfg.ResolveDisguise()
}

// disguiseType returns the Type field of a Disguise or "" if nil.
func disguiseType(d *domain.ProviderConfigCustomDisguise) string {
	if d == nil {
		return ""
	}
	return d.Type
}

// sanitizeClaudeMessages removes invalid empty text blocks from messages content.
// Anthropic rejects requests containing message content blocks like:
// {"type":"text","text":""}
// Messages are never dropped; empty content is replaced with a placeholder to
// preserve conversation structure and tool_use/tool_result correspondence.
func sanitizeClaudeMessages(body []byte) []byte {
	messages := gjson.GetBytes(body, "messages")
	if !messages.Exists() || !messages.IsArray() {
		return body
	}

	msgArr := messages.Array()
	filteredMessages := make([]string, 0, len(msgArr))
	modified := false

	for _, msg := range msgArr {
		content := msg.Get("content")

		// Replace empty string content with a placeholder.
		if content.Type == gjson.String && strings.TrimSpace(content.String()) == "" {
			if updatedMsg, err := sjson.SetRaw(msg.Raw, "content", `[{"type":"text","text":"[empty]"}]`); err == nil {
				filteredMessages = append(filteredMessages, updatedMsg)
				modified = true
			} else {
				filteredMessages = append(filteredMessages, msg.Raw)
			}
			continue
		}

		// Remove empty text blocks from array content.
		if content.IsArray() {
			blocks := content.Array()
			filteredBlocks := make([]string, 0, len(blocks))
			blockModified := false

			for _, block := range blocks {
				blockType := block.Get("type").String()

				// Sanitize nested content inside tool_result blocks (one level deep).
				if blockType == "tool_result" {
					nestedContent := block.Get("content")
					if nestedContent.IsArray() {
						nestedBlocks := nestedContent.Array()
						filteredNested := make([]string, 0, len(nestedBlocks))
						nestedModified := false
						for _, nb := range nestedBlocks {
							if nb.Get("type").String() == "text" && strings.TrimSpace(nb.Get("text").String()) == "" {
								nestedModified = true
								continue
							}
							filteredNested = append(filteredNested, nb.Raw)
						}
						if nestedModified {
							var newContent string
							if len(filteredNested) == 0 {
								// All nested blocks were empty; replace with a placeholder to
								// preserve the required tool_use/tool_result correspondence.
								newContent = `[{"type":"text","text":"[empty]"}]`
							} else {
								newContent = "[" + strings.Join(filteredNested, ",") + "]"
							}
							updated, err := sjson.SetRaw(block.Raw, "content", newContent)
							if err == nil {
								// Only mark modified after a successful update.
								blockModified = true
								modified = true
								filteredBlocks = append(filteredBlocks, updated)
								continue
							}
							// SetRaw failed; keep the original block unchanged.
						}
					}
					filteredBlocks = append(filteredBlocks, block.Raw)
					continue
				}

				if blockType != "text" {
					filteredBlocks = append(filteredBlocks, block.Raw)
					continue
				}
				if strings.TrimSpace(block.Get("text").String()) == "" {
					blockModified = true
					modified = true
					continue
				}
				filteredBlocks = append(filteredBlocks, block.Raw)
			}

			// If all content blocks were removed, replace with a placeholder rather than
			// dropping the message, to preserve user/assistant alternation and
			// tool_use/tool_result correspondence.
			if len(filteredBlocks) == 0 {
				if updatedMsg, err := sjson.SetRaw(msg.Raw, "content", `[{"type":"text","text":"[empty]"}]`); err == nil {
					filteredMessages = append(filteredMessages, updatedMsg)
					modified = true
				} else {
					filteredMessages = append(filteredMessages, msg.Raw)
				}
				continue
			}

			if blockModified {
				updatedMsg, err := sjson.SetRaw(msg.Raw, "content", "["+strings.Join(filteredBlocks, ",")+"]")
				if err == nil {
					filteredMessages = append(filteredMessages, updatedMsg)
					continue
				}
			}
		}

		filteredMessages = append(filteredMessages, msg.Raw)
	}

	if !modified {
		return body
	}

	updatedBody, err := sjson.SetRawBytes(body, "messages", []byte("["+strings.Join(filteredMessages, ",")+"]"))
	if err != nil {
		return body
	}
	return updatedBody
}

// ensureToolResultCorrespondence ensures every tool_use block in an assistant
// message has a matching tool_result in the immediately following user message.
// Missing tool_results are injected with placeholder content so the request
// satisfies Anthropic's requirement that "each tool_use block must have a
// corresponding tool_result block in the next message".
// If the assistant message is the last message or the next message is not a user
// message, a synthetic user message is inserted with all required tool_results.
func ensureToolResultCorrespondence(body []byte) []byte {
	messages := gjson.GetBytes(body, "messages")
	if !messages.Exists() || !messages.IsArray() {
		return body
	}

	msgArr := messages.Array()
	if len(msgArr) == 0 {
		return body
	}

	rebuilt := make([]string, 0, len(msgArr)+4)
	modified := false

	for i := 0; i < len(msgArr); i++ {
		msg := msgArr[i]
		rebuilt = append(rebuilt, msg.Raw)

		if msg.Get("role").String() != "assistant" {
			continue
		}
		content := msg.Get("content")
		if !content.IsArray() {
			continue
		}

		// Collect tool_use IDs from this assistant message.
		var toolUseIDs []string
		content.ForEach(func(_, block gjson.Result) bool {
			if block.Get("type").String() == "tool_use" {
				if id := block.Get("id").String(); id != "" {
					toolUseIDs = append(toolUseIDs, id)
				}
			}
			return true
		})
		if len(toolUseIDs) == 0 {
			continue
		}

		// Check if the next message is a user message with the required tool_results.
		hasNextUser := i+1 < len(msgArr) && msgArr[i+1].Get("role").String() == "user"

		if hasNextUser {
			nextMsg := msgArr[i+1]
			nextContent := nextMsg.Get("content")

			existing := make(map[string]bool)
			if nextContent.IsArray() {
				nextContent.ForEach(func(_, block gjson.Result) bool {
					if block.Get("type").String() == "tool_result" {
						existing[block.Get("tool_use_id").String()] = true
					}
					return true
				})
			}

			var missingIDs []string
			for _, id := range toolUseIDs {
				if !existing[id] {
					missingIDs = append(missingIDs, id)
				}
			}

			if len(missingIDs) == 0 {
				continue // all tool_results present
			}

			// Prepend missing tool_results to the existing user message content.
			var newBlocks []string
			for _, id := range missingIDs {
				newBlocks = append(newBlocks,
					fmt.Sprintf(`{"type":"tool_result","tool_use_id":%s,"content":[{"type":"text","text":"[empty]"}]}`,
						strconv.Quote(id)))
			}
			if nextContent.IsArray() {
				nextContent.ForEach(func(_, block gjson.Result) bool {
					newBlocks = append(newBlocks, block.Raw)
					return true
				})
			} else if nextContent.Type == gjson.String && strings.TrimSpace(nextContent.String()) != "" {
				newBlocks = append(newBlocks,
					fmt.Sprintf(`{"type":"text","text":%s}`, strconv.Quote(nextContent.String())))
			}

			if updatedNext, err := sjson.SetRaw(nextMsg.Raw, "content", "["+strings.Join(newBlocks, ",")+"]"); err == nil {
				rebuilt = append(rebuilt, updatedNext)
				i++ // skip original next message
				modified = true
			}
		} else {
			// No user message follows — inject a synthetic one with all tool_results.
			var blocks []string
			for _, id := range toolUseIDs {
				blocks = append(blocks,
					fmt.Sprintf(`{"type":"tool_result","tool_use_id":%s,"content":[{"type":"text","text":"[empty]"}]}`,
						strconv.Quote(id)))
			}
			rebuilt = append(rebuilt, `{"role":"user","content":[`+strings.Join(blocks, ",")+`]}`)
			modified = true
		}
	}

	if !modified {
		return body
	}

	updatedBody, err := sjson.SetRawBytes(body, "messages", []byte("["+strings.Join(rebuilt, ",")+"]"))
	if err != nil {
		return body
	}
	return updatedBody
}

func stripVolatileClaudeBillingCCH(body []byte) []byte {
	body = stripVolatileClaudeBillingCCHAtPath(body, "system.0.text")
	body = stripVolatileClaudeBillingCCHAtPath(body, "message.system.0.text")
	return body
}

func stripVolatileClaudeBillingCCHAtPath(body []byte, path string) []byte {
	current := gjson.GetBytes(body, path)
	if current.Type != gjson.String {
		return body
	}

	updatedText := claudeBillingCCHPattern.ReplaceAllString(current.String(), "")
	if updatedText == current.String() {
		return body
	}

	updatedBody, err := sjson.SetBytes(body, path, updatedText)
	if err != nil {
		return body
	}
	return updatedBody
}

// applyCloaking applies cloaking transformations based on config and client.
// Cloaking includes: system prompt injection, fake user ID, sensitive word obfuscation.
func applyCloaking(body []byte, clientUserAgent string, model string, opts *domain.DisguiseClaudeCodeOptions) []byte {
	var cloakMode string
	var strictMode bool
	var sensitiveWords []string

	if opts != nil {
		cloakMode = strings.TrimSpace(opts.Mode)
		strictMode = opts.StrictMode
		sensitiveWords = opts.SensitiveWords
	}

	// Default mode is "auto"
	if !shouldCloak(cloakMode, clientUserAgent) {
		return body
	}

	// Always ensure Claude Code system prompt for cloaked requests.
	// This keeps messages-path requests compatible with strict Claude client validators.
	body = checkSystemInstructionsWithMode(body, strictMode)

	// Inject fake user_id
	body = injectFakeUserID(body)

	// Apply sensitive word obfuscation
	if len(sensitiveWords) > 0 {
		matcher := buildSensitiveWordMatcher(sensitiveWords)
		body = obfuscateSensitiveWords(body, matcher)
	}

	return body
}

// isClaudeCodeClient checks if the User-Agent indicates a Claude Code client.
func isClaudeCodeClient(userAgent string) bool {
	return claudeCLIUserAgentPattern.MatchString(strings.TrimSpace(userAgent))
}

func isClaudeOAuthToken(apiKey string) bool {
	return strings.Contains(apiKey, "sk-ant-oat")
}

func ensureMinThinkingBudget(body []byte) []byte {
	const minBudget = 1024
	// Claude API format: {"thinking": {"type": "enabled", "budget_tokens": N}}
	if gjson.GetBytes(body, "thinking.type").String() != "enabled" {
		return body
	}
	result := gjson.GetBytes(body, "thinking.budget_tokens")
	if result.Type != gjson.Number {
		return body
	}
	if result.Int() >= minBudget {
		return body
	}
	updated, err := sjson.SetBytes(body, "thinking.budget_tokens", minBudget)
	if err != nil {
		return body
	}
	return updated
}

func applyClaudeToolPrefix(body []byte, prefix string) []byte {
	if prefix == "" {
		return body
	}

	if tools := gjson.GetBytes(body, "tools"); tools.Exists() && tools.IsArray() {
		tools.ForEach(func(index, tool gjson.Result) bool {
			// Skip built-in tools (web_search, code_execution, etc.) which have
			// a "type" field and require their name to remain unchanged.
			if tool.Get("type").Exists() && tool.Get("type").String() != "" {
				return true
			}
			name := tool.Get("name").String()
			if name == "" || strings.HasPrefix(name, prefix) {
				return true
			}
			path := fmt.Sprintf("tools.%d.name", index.Int())
			body, _ = sjson.SetBytes(body, path, prefix+name)
			return true
		})
	}

	if gjson.GetBytes(body, "tool_choice.type").String() == "tool" {
		name := gjson.GetBytes(body, "tool_choice.name").String()
		if name != "" && !strings.HasPrefix(name, prefix) {
			body, _ = sjson.SetBytes(body, "tool_choice.name", prefix+name)
		}
	}

	if messages := gjson.GetBytes(body, "messages"); messages.Exists() && messages.IsArray() {
		messages.ForEach(func(msgIndex, msg gjson.Result) bool {
			content := msg.Get("content")
			if !content.Exists() || !content.IsArray() {
				return true
			}
			content.ForEach(func(contentIndex, part gjson.Result) bool {
				if part.Get("type").String() != "tool_use" {
					return true
				}
				name := part.Get("name").String()
				if name == "" || strings.HasPrefix(name, prefix) {
					return true
				}
				path := fmt.Sprintf("messages.%d.content.%d.name", msgIndex.Int(), contentIndex.Int())
				body, _ = sjson.SetBytes(body, path, prefix+name)
				return true
			})
			return true
		})
	}

	return body
}

func stripClaudeToolPrefixFromResponse(body []byte, prefix string) []byte {
	if prefix == "" {
		return body
	}
	content := gjson.GetBytes(body, "content")
	if !content.Exists() || !content.IsArray() {
		return body
	}
	content.ForEach(func(index, part gjson.Result) bool {
		if part.Get("type").String() != "tool_use" {
			return true
		}
		name := part.Get("name").String()
		if !strings.HasPrefix(name, prefix) {
			return true
		}
		path := fmt.Sprintf("content.%d.name", index.Int())
		body, _ = sjson.SetBytes(body, path, strings.TrimPrefix(name, prefix))
		return true
	})
	return body
}

func stripClaudeToolPrefixFromStreamLine(line []byte, prefix string) []byte {
	if prefix == "" {
		return line
	}
	payload := jsonPayload(line)
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return line
	}
	contentBlock := gjson.GetBytes(payload, "content_block")
	if !contentBlock.Exists() || contentBlock.Get("type").String() != "tool_use" {
		return line
	}
	name := contentBlock.Get("name").String()
	if !strings.HasPrefix(name, prefix) {
		return line
	}
	updated, err := sjson.SetBytes(payload, "content_block.name", strings.TrimPrefix(name, prefix))
	if err != nil {
		return line
	}

	trimmed := bytes.TrimSpace(line)
	if bytes.HasPrefix(trimmed, []byte("data:")) {
		return append([]byte("data: "), updated...)
	}
	return updated
}

func jsonPayload(line []byte) []byte {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return nil
	}
	if bytes.Equal(trimmed, []byte("[DONE]")) {
		return nil
	}
	if bytes.HasPrefix(trimmed, []byte("event:")) {
		return nil
	}
	if bytes.HasPrefix(trimmed, []byte("data:")) {
		trimmed = bytes.TrimSpace(trimmed[len("data:"):])
	}
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil
	}
	return trimmed
}

// injectClaudeCodeSystemPrompt injects Claude Code system prompt into the request.
// This is the non-strict cloaking behavior (prepend prompt).
func injectClaudeCodeSystemPrompt(body []byte) []byte {
	return checkSystemInstructionsWithMode(body, false)
}

// injectFakeUserID generates and injects a fake user_id into the request metadata.
// Only injects if user_id is missing or invalid.
func injectFakeUserID(body []byte) []byte {
	existingUserID := gjson.GetBytes(body, "metadata.user_id").String()
	if existingUserID != "" && isValidUserID(existingUserID) {
		return body
	}

	// Generate and inject fake user_id
	body, _ = sjson.SetBytes(body, "metadata.user_id", generateFakeUserID())
	return body
}

// shouldCloak determines if request should be cloaked based on config and client User-Agent.
// Returns true if cloaking should be applied.
func shouldCloak(cloakMode string, userAgent string) bool {
	switch strings.ToLower(cloakMode) {
	case "always":
		return true
	case "never":
		return false
	default: // "auto" or empty
		return !isClaudeCodeClient(userAgent)
	}
}

// isValidUserID checks if a user_id matches Claude Code format.
func isValidUserID(userID string) bool {
	return userIDPattern.MatchString(userID)
}

// generateFakeUserID generates a fake user_id in Claude Code format.
// Format: user_{64-hex}_account__session_{uuid}
func generateFakeUserID() string {
	// Generate 32 random bytes (64 hex chars)
	randomBytes := make([]byte, 32)
	_, _ = rand.Read(randomBytes)
	hexPart := hex.EncodeToString(randomBytes)

	// Generate UUID for session
	sessionUUID := uuid.New().String()

	return "user_" + hexPart + "_account__session_" + sessionUUID
}

// disableThinkingIfToolChoiceForced checks if tool_choice forces tool use and disables thinking.
// Anthropic API does not allow thinking when tool_choice is set to "any" or "tool".
// See: https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking#important-considerations
func disableThinkingIfToolChoiceForced(body []byte) []byte {
	toolChoiceType := gjson.GetBytes(body, "tool_choice.type").String()
	// "auto" is allowed with thinking, but "any" or "tool" (specific tool) are not
	if toolChoiceType == "any" || toolChoiceType == "tool" {
		// Remove thinking configuration entirely to avoid API error
		body, _ = sjson.DeleteBytes(body, "thinking")
	}
	return body
}

// extractAndRemoveBetas extracts betas array from request body and removes it.
// Returns the extracted betas and the modified body.
func extractAndRemoveBetas(body []byte) ([]string, []byte) {
	betasResult := gjson.GetBytes(body, "betas")
	if !betasResult.Exists() {
		return nil, body
	}

	var betas []string
	if betasResult.IsArray() {
		for _, item := range betasResult.Array() {
			if s := strings.TrimSpace(item.String()); s != "" {
				betas = append(betas, s)
			}
		}
	} else if s := strings.TrimSpace(betasResult.String()); s != "" {
		betas = append(betas, s)
	}

	body, _ = sjson.DeleteBytes(body, "betas")
	return betas, body
}

// checkSystemInstructionsWithMode injects Claude Code system prompt.
// In strict mode, it replaces all user system messages.
// In non-strict mode (default), it prepends to existing system messages.
func checkSystemInstructionsWithMode(body []byte, strictMode bool) []byte {
	if hasClaudeCodeSystemPrompt(body) {
		return body
	}

	claudeCodeInstructions := `[{"type":"text","text":"` + claudeCodeSystemPrompt + `"}]`

	if strictMode {
		body, _ = sjson.SetRawBytes(body, "system", []byte(claudeCodeInstructions))
		return body
	}

	system := gjson.GetBytes(body, "system")
	if system.IsArray() {
		system.ForEach(func(_, part gjson.Result) bool {
			if part.Get("type").String() == "text" {
				claudeCodeInstructions, _ = sjson.SetRaw(claudeCodeInstructions, "-1", part.Raw)
			}
			return true
		})
		body, _ = sjson.SetRawBytes(body, "system", []byte(claudeCodeInstructions))
		return body
	}

	if system.Type == gjson.String && strings.TrimSpace(system.String()) != "" {
		existingBlock := `{"type":"text","text":` + system.Raw + `}`
		claudeCodeInstructions, _ = sjson.SetRaw(claudeCodeInstructions, "-1", existingBlock)
	}
	body, _ = sjson.SetRawBytes(body, "system", []byte(claudeCodeInstructions))
	return body
}

func hasClaudeCodeSystemPrompt(body []byte) bool {
	system := gjson.GetBytes(body, "system")
	if !system.Exists() {
		return false
	}

	if system.IsArray() {
		found := false
		system.ForEach(func(_, part gjson.Result) bool {
			if strings.TrimSpace(part.Get("text").String()) == claudeCodeSystemPrompt {
				found = true
				return false
			}
			if part.Type == gjson.String && strings.TrimSpace(part.String()) == claudeCodeSystemPrompt {
				found = true
				return false
			}
			return true
		})
		return found
	}

	if system.Type == gjson.String {
		return strings.TrimSpace(system.String()) == claudeCodeSystemPrompt
	}

	if system.IsObject() {
		return strings.TrimSpace(system.Get("text").String()) == claudeCodeSystemPrompt
	}

	return false
}

// ===== Sensitive word obfuscation (CLIProxyAPI-aligned) =====

// zeroWidthSpace is the Unicode zero-width space character used for obfuscation.
const zeroWidthSpace = "\u200B"

// SensitiveWordMatcher holds the compiled regex for matching sensitive words.
type SensitiveWordMatcher struct {
	regex *regexp.Regexp
}

// buildSensitiveWordMatcher compiles a regex from the word list.
// Words are sorted by length (longest first) for proper matching.
func buildSensitiveWordMatcher(words []string) *SensitiveWordMatcher {
	if len(words) == 0 {
		return nil
	}

	var validWords []string
	for _, w := range words {
		w = strings.TrimSpace(w)
		if utf8.RuneCountInString(w) >= 2 && !strings.Contains(w, zeroWidthSpace) {
			validWords = append(validWords, w)
		}
	}
	if len(validWords) == 0 {
		return nil
	}

	sort.Slice(validWords, func(i, j int) bool {
		return len(validWords[i]) > len(validWords[j])
	})

	escaped := make([]string, len(validWords))
	for i, w := range validWords {
		escaped[i] = regexp.QuoteMeta(w)
	}

	pattern := "(?i)" + strings.Join(escaped, "|")
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}

	return &SensitiveWordMatcher{regex: re}
}

// obfuscateWord inserts a zero-width space after the first grapheme.
func obfuscateWord(word string) string {
	if strings.Contains(word, zeroWidthSpace) {
		return word
	}
	r, size := utf8.DecodeRuneInString(word)
	if r == utf8.RuneError || size >= len(word) {
		return word
	}
	return string(r) + zeroWidthSpace + word[size:]
}

// obfuscateText replaces all sensitive words in the text.
func (m *SensitiveWordMatcher) obfuscateText(text string) string {
	if m == nil || m.regex == nil {
		return text
	}
	return m.regex.ReplaceAllStringFunc(text, obfuscateWord)
}

// obfuscateSensitiveWords processes the payload and obfuscates sensitive words
// in system blocks and message content.
func obfuscateSensitiveWords(payload []byte, matcher *SensitiveWordMatcher) []byte {
	if matcher == nil || matcher.regex == nil {
		return payload
	}
	payload = obfuscateSystemBlocks(payload, matcher)
	payload = obfuscateMessages(payload, matcher)
	return payload
}

// obfuscateSystemBlocks obfuscates sensitive words in system blocks.
func obfuscateSystemBlocks(payload []byte, matcher *SensitiveWordMatcher) []byte {
	system := gjson.GetBytes(payload, "system")
	if !system.Exists() {
		return payload
	}

	if system.IsArray() {
		modified := false
		system.ForEach(func(key, value gjson.Result) bool {
			if value.Get("type").String() == "text" {
				text := value.Get("text").String()
				obfuscated := matcher.obfuscateText(text)
				if obfuscated != text {
					path := "system." + key.String() + ".text"
					payload, _ = sjson.SetBytes(payload, path, obfuscated)
					modified = true
				}
			}
			return true
		})
		if modified {
			return payload
		}
	} else if system.Type == gjson.String {
		text := system.String()
		obfuscated := matcher.obfuscateText(text)
		if obfuscated != text {
			payload, _ = sjson.SetBytes(payload, "system", obfuscated)
		}
	}

	return payload
}

// obfuscateMessages obfuscates sensitive words in message content.
func obfuscateMessages(payload []byte, matcher *SensitiveWordMatcher) []byte {
	messages := gjson.GetBytes(payload, "messages")
	if !messages.Exists() || !messages.IsArray() {
		return payload
	}

	messages.ForEach(func(msgKey, msg gjson.Result) bool {
		content := msg.Get("content")
		if !content.Exists() {
			return true
		}

		msgPath := "messages." + msgKey.String()

		if content.Type == gjson.String {
			text := content.String()
			obfuscated := matcher.obfuscateText(text)
			if obfuscated != text {
				payload, _ = sjson.SetBytes(payload, msgPath+".content", obfuscated)
			}
		} else if content.IsArray() {
			content.ForEach(func(blockKey, block gjson.Result) bool {
				if block.Get("type").String() == "text" {
					text := block.Get("text").String()
					obfuscated := matcher.obfuscateText(text)
					if obfuscated != text {
						path := msgPath + ".content." + blockKey.String() + ".text"
						payload, _ = sjson.SetBytes(payload, path, obfuscated)
					}
				}
				return true
			})
		}

		return true
	})

	return payload
}

// ===== Cache control injection (CLIProxyAPI-aligned) =====

// ensureCacheControl injects cache_control breakpoints into the payload for optimal prompt caching.
// According to Anthropic's documentation, cache prefixes are created in order: tools -> system -> messages.
func ensureCacheControl(payload []byte) []byte {
	payload = injectToolsCacheControl(payload)
	payload = injectSystemCacheControl(payload)
	payload = injectMessagesCacheControl(payload)
	return payload
}

func countCacheControls(payload []byte) int {
	count := 0

	system := gjson.GetBytes(payload, "system")
	if system.IsArray() {
		system.ForEach(func(_, item gjson.Result) bool {
			if item.Get("cache_control").Exists() {
				count++
			}
			return true
		})
	}

	tools := gjson.GetBytes(payload, "tools")
	if tools.IsArray() {
		tools.ForEach(func(_, item gjson.Result) bool {
			if item.Get("cache_control").Exists() {
				count++
			}
			return true
		})
	}

	messages := gjson.GetBytes(payload, "messages")
	if messages.IsArray() {
		messages.ForEach(func(_, msg gjson.Result) bool {
			content := msg.Get("content")
			if content.IsArray() {
				content.ForEach(func(_, item gjson.Result) bool {
					if item.Get("cache_control").Exists() {
						count++
					}
					return true
				})
			}
			return true
		})
	}

	return count
}

// injectMessagesCacheControl adds cache_control to the second-to-last user turn for multi-turn caching.
func injectMessagesCacheControl(payload []byte) []byte {
	messages := gjson.GetBytes(payload, "messages")
	if !messages.Exists() || !messages.IsArray() {
		return payload
	}

	hasCacheControlInMessages := false
	messages.ForEach(func(_, msg gjson.Result) bool {
		content := msg.Get("content")
		if content.IsArray() {
			content.ForEach(func(_, item gjson.Result) bool {
				if item.Get("cache_control").Exists() {
					hasCacheControlInMessages = true
					return false
				}
				return true
			})
		}
		return !hasCacheControlInMessages
	})
	if hasCacheControlInMessages {
		return payload
	}

	var userMsgIndices []int
	messages.ForEach(func(index gjson.Result, msg gjson.Result) bool {
		if msg.Get("role").String() == "user" {
			userMsgIndices = append(userMsgIndices, int(index.Int()))
		}
		return true
	})
	if len(userMsgIndices) < 2 {
		return payload
	}

	secondToLastUserIdx := userMsgIndices[len(userMsgIndices)-2]
	contentPath := fmt.Sprintf("messages.%d.content", secondToLastUserIdx)
	content := gjson.GetBytes(payload, contentPath)

	if content.IsArray() {
		contentCount := int(content.Get("#").Int())
		if contentCount > 0 {
			cacheControlPath := fmt.Sprintf("messages.%d.content.%d.cache_control", secondToLastUserIdx, contentCount-1)
			result, err := sjson.SetBytes(payload, cacheControlPath, map[string]string{"type": "ephemeral"})
			if err != nil {
				log.Printf("failed to inject cache_control into messages: %v", err)
				return payload
			}
			payload = result
		}
	} else if content.Type == gjson.String {
		text := content.String()
		newContent := []map[string]interface{}{
			{
				"type": "text",
				"text": text,
				"cache_control": map[string]string{
					"type": "ephemeral",
				},
			},
		}
		result, err := sjson.SetBytes(payload, contentPath, newContent)
		if err != nil {
			log.Printf("failed to inject cache_control into message string content: %v", err)
			return payload
		}
		payload = result
	}

	return payload
}

// injectToolsCacheControl adds cache_control to the last tool in the tools array.
func injectToolsCacheControl(payload []byte) []byte {
	tools := gjson.GetBytes(payload, "tools")
	if !tools.Exists() || !tools.IsArray() {
		return payload
	}

	toolCount := int(tools.Get("#").Int())
	if toolCount == 0 {
		return payload
	}

	hasCacheControlInTools := false
	tools.ForEach(func(_, tool gjson.Result) bool {
		if tool.Get("cache_control").Exists() {
			hasCacheControlInTools = true
			return false
		}
		return true
	})
	if hasCacheControlInTools {
		return payload
	}

	lastToolPath := fmt.Sprintf("tools.%d.cache_control", toolCount-1)
	result, err := sjson.SetBytes(payload, lastToolPath, map[string]string{"type": "ephemeral"})
	if err != nil {
		log.Printf("failed to inject cache_control into tools array: %v", err)
		return payload
	}

	return result
}

// injectSystemCacheControl adds cache_control to the last element in the system prompt.
func injectSystemCacheControl(payload []byte) []byte {
	system := gjson.GetBytes(payload, "system")
	if !system.Exists() {
		return payload
	}

	if system.IsArray() {
		count := int(system.Get("#").Int())
		if count == 0 {
			return payload
		}

		hasCacheControlInSystem := false
		system.ForEach(func(_, item gjson.Result) bool {
			if item.Get("cache_control").Exists() {
				hasCacheControlInSystem = true
				return false
			}
			return true
		})
		if hasCacheControlInSystem {
			return payload
		}

		lastSystemPath := fmt.Sprintf("system.%d.cache_control", count-1)
		result, err := sjson.SetBytes(payload, lastSystemPath, map[string]string{"type": "ephemeral"})
		if err != nil {
			log.Printf("failed to inject cache_control into system array: %v", err)
			return payload
		}
		payload = result
	} else if system.Type == gjson.String {
		text := system.String()
		newSystem := []map[string]interface{}{
			{
				"type": "text",
				"text": text,
				"cache_control": map[string]string{
					"type": "ephemeral",
				},
			},
		}
		result, err := sjson.SetBytes(payload, "system", newSystem)
		if err != nil {
			log.Printf("failed to inject cache_control into system string: %v", err)
			return payload
		}
		payload = result
	}

	return payload
}
