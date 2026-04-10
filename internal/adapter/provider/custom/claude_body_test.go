package custom

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/tidwall/gjson"
)

// noneDisguiseCustomCfg returns a custom config whose Disguise.Type explicitly
// disables any body-level disguise transformation. Used by tests that exercise
// processClaudeRequestBody steps OTHER than cloaking.
func noneDisguiseCustomCfg() *domain.ProviderConfigCustom {
	return &domain.ProviderConfigCustom{
		Disguise: &domain.ProviderConfigCustomDisguise{Type: "none"},
	}
}

func TestSystemPromptInjection(t *testing.T) {
	// Test case: empty body
	body := []byte(`{"model":"claude-3-5-sonnet","messages":[]}`)
	result := injectClaudeCodeSystemPrompt(body)

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}

	// Check system field exists and is array
	system, ok := parsed["system"].([]interface{})
	if !ok {
		t.Fatalf("system field is not an array: %T", parsed["system"])
	}

	// Should have 1 entry: Claude Code prompt
	if len(system) != 1 {
		t.Fatalf("Expected 1 system entry, got %d", len(system))
	}

	// Check first entry is Claude Code prompt
	entry0, ok := system[0].(map[string]interface{})
	if !ok {
		t.Fatalf("system entry 0 is not a map: %T", system[0])
	}
	if entry0["type"] != "text" {
		t.Errorf("Expected entry 0 type='text', got %v", entry0["type"])
	}
	if entry0["text"] != claudeCodeSystemPrompt {
		t.Errorf("Expected entry 0 text='%s', got %v", claudeCodeSystemPrompt, entry0["text"])
	}
}

func TestUserIDGeneration(t *testing.T) {
	userID := generateFakeUserID()

	// Check format matches expected regex
	if !isValidUserID(userID) {
		t.Errorf("Generated user_id doesn't match expected format: %s", userID)
	}
}

func TestCloakingForNonClaudeClient(t *testing.T) {
	body := []byte(`{"model":"claude-3-5-sonnet","messages":[{"role":"user","content":"hello"}]}`)

	// Non-Claude Code client (e.g., curl)
	result := applyCloaking(body, "curl/7.68.0", "claude-3-5-sonnet", nil)

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}

	// Should have system prompt injected
	system, ok := parsed["system"].([]interface{})
	if !ok || len(system) == 0 {
		t.Error("System prompt was not injected for non-Claude client")
	}

	// Should have metadata.user_id injected
	metadata, ok := parsed["metadata"].(map[string]interface{})
	if !ok {
		t.Error("metadata was not created")
	}

	userID, ok := metadata["user_id"].(string)
	if !ok || userID == "" {
		t.Error("user_id was not injected")
	}

	if !isValidUserID(userID) {
		t.Errorf("Injected user_id doesn't match expected format: %s", userID)
	}
}

func TestNoCloakingForClaudeClient(t *testing.T) {
	body := []byte(`{"model":"claude-3-5-sonnet","messages":[{"role":"user","content":"hello"}]}`)

	// Claude Code client
	result := applyCloaking(body, "claude-cli/2.1.23 (external, cli)", "claude-3-5-sonnet", nil)

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}

	// Should NOT have system prompt injected
	if _, ok := parsed["system"]; ok {
		t.Error("System prompt was injected for Claude Code client (should not)")
	}

	// Should NOT have metadata injected
	if _, ok := parsed["metadata"]; ok {
		t.Error("metadata was injected for Claude Code client (should not)")
	}
}

func TestShouldCloakModes(t *testing.T) {
	if !shouldCloak("", "curl/7.68.0") {
		t.Error("default mode should cloak non-claude clients")
	}
	if shouldCloak("", "claude-cli/2.1.17 (external, cli)") {
		t.Error("default mode should not cloak claude-cli clients")
	}
	if !shouldCloak("always", "claude-cli/2.1.17 (external, cli)") {
		t.Error("always mode should cloak all clients")
	}
	if shouldCloak("never", "curl/7.68.0") {
		t.Error("never mode should cloak none")
	}
	if !shouldCloak("", "claude-cli/dev") {
		t.Error("default mode should cloak non-official claude-cli UA")
	}
	if shouldCloak("", "Claude-CLI/2.1.17 (external, cli)") {
		t.Error("default mode should not cloak case-insensitive official claude-cli UA")
	}
}

func TestSystemInjectionForHaikuWhenCloaked(t *testing.T) {
	body := []byte(`{"model":"claude-3-5-haiku-20241022","messages":[{"role":"user","content":"hello"}]}`)

	result := applyCloaking(body, "curl/7.68.0", "claude-3-5-haiku-20241022", nil)

	if !gjson.GetBytes(result, "system").Exists() {
		t.Error("system prompt should be injected for cloaked haiku requests")
	}
	if !gjson.GetBytes(result, "metadata.user_id").Exists() {
		t.Error("user_id should be injected for haiku models")
	}
}

func TestFullBodyProcessingAddsCacheControlAndExtractsBetas(t *testing.T) {
	body := []byte(`{
		"model":"claude-3-5-sonnet",
		"betas":["custom-beta-1"],
		"system":[{"type":"text","text":"You are helpful"}],
		"tools":[{"name":"test_tool","description":"A test tool"}],
		"messages":[
			{"role":"user","content":"hello"},
			{"role":"assistant","content":"ok"},
			{"role":"user","content":"again"}
		]
	}`)

	result, betas := processClaudeRequestBody(body, "curl/7.68.0", nil)

	if len(betas) != 1 || betas[0] != "custom-beta-1" {
		t.Fatalf("expected betas to be extracted, got %v", betas)
	}
	if gjson.GetBytes(result, "betas").Exists() {
		t.Error("betas should be removed from body")
	}

	if !gjson.GetBytes(result, "tools.0.cache_control").Exists() {
		t.Error("cache_control should be injected into tools")
	}
	system := gjson.GetBytes(result, "system")
	if !system.IsArray() || len(system.Array()) == 0 {
		t.Fatal("system should be an array with at least one entry")
	}
	lastIdx := len(system.Array()) - 1
	if !gjson.GetBytes(result, fmt.Sprintf("system.%d.cache_control", lastIdx)).Exists() {
		t.Error("cache_control should be injected into the last system entry")
	}
	if !gjson.GetBytes(result, "messages.0.content.0.cache_control").Exists() {
		t.Error("cache_control should be injected into second-to-last user message")
	}
}

func TestSensitiveWordObfuscation(t *testing.T) {
	body := []byte(`{"model":"claude-3-5-sonnet","messages":[{"role":"user","content":"this is secret"}]}`)
	cfg := &domain.DisguiseClaudeCodeOptions{
		Mode:           "always",
		SensitiveWords: []string{"secret"},
	}

	result := applyCloaking(body, "curl/7.68.0", "claude-3-5-sonnet", cfg)

	const zwsp = "\u200B"
	if strings.Contains(string(result), "secret") {
		t.Error("sensitive word should be obfuscated")
	}
	if !strings.Contains(string(result), "s"+zwsp+"ecret") {
		t.Error("obfuscated word should include zero-width space")
	}
}

func TestStrictCloakingReplacesSystem(t *testing.T) {
	body := []byte(`{
		"model":"claude-3-5-sonnet",
		"system":[
			{"type":"text","text":"Original system"},
			{"type":"text","text":"More system"}
		],
		"messages":[{"role":"user","content":"hello"}]
	}`)
	cfg := &domain.DisguiseClaudeCodeOptions{
		Mode:       "always",
		StrictMode: true,
	}

	result := applyCloaking(body, "curl/7.68.0", "claude-3-5-sonnet", cfg)

	system := gjson.GetBytes(result, "system")
	if !system.IsArray() || len(system.Array()) != 1 {
		t.Fatalf("strict mode should replace system with single entry, got %s", system.Raw)
	}
	if system.Array()[0].Get("text").String() != claudeCodeSystemPrompt {
		t.Errorf("strict mode system text mismatch: %s", system.Array()[0].Get("text").String())
	}
}

func TestSensitiveWordObfuscationInSystem(t *testing.T) {
	body := []byte(`{
		"model":"claude-3-5-sonnet",
		"system":[{"type":"text","text":"keep secret here"}],
		"messages":[{"role":"user","content":"hello"}]
	}`)
	cfg := &domain.DisguiseClaudeCodeOptions{
		Mode:           "always",
		SensitiveWords: []string{"secret"},
	}

	result := applyCloaking(body, "curl/7.68.0", "claude-3-5-sonnet", cfg)

	const zwsp = "\u200B"
	if strings.Contains(string(result), "secret") {
		t.Error("sensitive word in system should be obfuscated")
	}
	if !strings.Contains(string(result), "s"+zwsp+"ecret") {
		t.Error("obfuscated system word should include zero-width space")
	}
}

func TestEnsureCacheControlWithSystemString(t *testing.T) {
	body := []byte(`{
		"model":"claude-3-5-sonnet",
		"system":"You are helpful",
		"tools":[{"name":"test_tool","description":"A test tool"}],
		"messages":[
			{"role":"user","content":"hello"},
			{"role":"assistant","content":"ok"},
			{"role":"user","content":"again"}
		]
	}`)
	result, _ := processClaudeRequestBody(body, "curl/7.68.0", noneDisguiseCustomCfg())

	if !gjson.GetBytes(result, "system.0.cache_control").Exists() {
		t.Error("cache_control should be injected into system string")
	}
	if gjson.GetBytes(result, "system").Type != gjson.JSON {
		t.Error("system should be converted to array when injecting cache_control")
	}
}

func TestEnsureCacheControlDoesNotOverrideExistingTools(t *testing.T) {
	body := []byte(`{
		"model":"claude-3-5-sonnet",
		"tools":[
			{"name":"tool1","cache_control":{"type":"ephemeral"}},
			{"name":"tool2"}
		],
		"messages":[
			{"role":"user","content":"hello"},
			{"role":"assistant","content":"ok"},
			{"role":"user","content":"again"}
		]
	}`)
	result, _ := processClaudeRequestBody(body, "curl/7.68.0", noneDisguiseCustomCfg())

	if gjson.GetBytes(result, "tools.1.cache_control").Exists() {
		t.Error("cache_control should not be added when tools already have cache_control")
	}
}

func TestDisableThinkingIfToolChoiceForced(t *testing.T) {
	body := []byte(`{
		"model":"claude-3-5-sonnet",
		"thinking":{"type":"enabled","budget_tokens":1000},
		"tool_choice":{"type":"any"}
	}`)

	result := disableThinkingIfToolChoiceForced(body)
	if gjson.GetBytes(result, "thinking").Exists() {
		t.Error("thinking should be removed when tool_choice.type=any")
	}

	bodyAuto := []byte(`{
		"model":"claude-3-5-sonnet",
		"thinking":{"type":"enabled","budget_tokens":1000},
		"tool_choice":{"type":"auto"}
	}`)
	resultAuto := disableThinkingIfToolChoiceForced(bodyAuto)
	if !gjson.GetBytes(resultAuto, "thinking").Exists() {
		t.Error("thinking should remain when tool_choice.type=auto")
	}
}

func TestProcessClaudeRequestBodyDoesNotForceStream(t *testing.T) {
	body := []byte(`{
		"model":"claude-3-5-sonnet",
		"stream":false,
		"messages":[{"role":"user","content":"hello"}]
	}`)
	result, _ := processClaudeRequestBody(body, "curl/7.68.0", noneDisguiseCustomCfg())
	if gjson.GetBytes(result, "stream").Type != gjson.False {
		t.Error("stream flag should not be forced to true")
	}
}

func TestClaudeToolPrefixApplyAndStrip(t *testing.T) {
	body := []byte(`{
		"model":"claude-3-5-sonnet",
		"tools":[{"name":"t1"},{"type":"web_search","name":"web_search"}],
		"tool_choice":{"type":"tool","name":"t1"},
		"messages":[
			{"role":"assistant","content":[{"type":"tool_use","name":"t1","input":{}}]}
		],
		"content":[{"type":"tool_use","name":"t1"}]
	}`)

	updated := applyClaudeToolPrefix(body, "proxy_")
	if gjson.GetBytes(updated, "tools.0.name").String() != "proxy_t1" {
		t.Error("tool name should be prefixed")
	}
	if gjson.GetBytes(updated, "tools.1.name").String() != "web_search" {
		t.Error("built-in tool name should not be prefixed")
	}
	if gjson.GetBytes(updated, "tool_choice.name").String() != "proxy_t1" {
		t.Error("tool_choice name should be prefixed")
	}
	if gjson.GetBytes(updated, "messages.0.content.0.name").String() != "proxy_t1" {
		t.Error("tool_use name should be prefixed in messages")
	}

	// Simulate response stripping
	responseBody := []byte(`{"content":[{"type":"tool_use","name":"proxy_t1"}]}`)
	stripped := stripClaudeToolPrefixFromResponse(responseBody, "proxy_")
	if gjson.GetBytes(stripped, "content.0.name").String() != "t1" {
		t.Error("tool_use name should be stripped in response content")
	}
}

func TestStripClaudeToolPrefixFromStreamLine(t *testing.T) {
	line := "data: {\"type\":\"content_block_start\",\"content_block\":{\"type\":\"tool_use\",\"name\":\"proxy_t1\"}}\n"
	out := stripClaudeToolPrefixFromStreamLine([]byte(line), "proxy_")
	if !strings.Contains(string(out), "\"name\":\"t1\"") {
		t.Error("stream line tool name should be stripped")
	}
}

func TestNoDuplicateSystemPromptInjection(t *testing.T) {
	// Body that already has Claude Code system prompt
	body := []byte(`{
		"model":"claude-3-5-sonnet",
		"messages":[{"role":"user","content":"hello"}],
		"system":[{"type":"text","text":"Additional instructions"},{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude."}]
	}`)

	result := injectClaudeCodeSystemPrompt(body)

	// Count occurrences of "Claude Code"
	count := strings.Count(string(result), "Claude Code")
	if count != 1 {
		t.Errorf("Expected 1 occurrence of 'Claude Code', got %d", count)
	}
}

func TestEnsureMinThinkingBudget(t *testing.T) {
	body := []byte(`{"thinking":{"type":"enabled","budget_tokens":512}}`)
	updated := ensureMinThinkingBudget(body)
	if got := gjson.GetBytes(updated, "thinking.budget_tokens").Int(); got != 1024 {
		t.Fatalf("budget_tokens = %d, want 1024", got)
	}

	body = []byte(`{"thinking":{"type":"enabled","budget_tokens":2048}}`)
	updated = ensureMinThinkingBudget(body)
	if got := gjson.GetBytes(updated, "thinking.budget_tokens").Int(); got != 2048 {
		t.Fatalf("budget_tokens = %d, want 2048", got)
	}

	body = []byte(`{"thinking":{"type":"enabled","budget_tokens":"oops"}}`)
	updated = ensureMinThinkingBudget(body)
	if gjson.GetBytes(updated, "thinking.budget_tokens").String() != "oops" {
		t.Fatalf("non-numeric budget_tokens should be unchanged")
	}

	// thinking disabled — should not touch budget_tokens
	body = []byte(`{"thinking":{"type":"disabled","budget_tokens":100}}`)
	updated = ensureMinThinkingBudget(body)
	if got := gjson.GetBytes(updated, "thinking.budget_tokens").Int(); got != 100 {
		t.Fatalf("disabled thinking budget_tokens = %d, want 100 (unchanged)", got)
	}
}

func TestCloakingPreservesSystemStringInNonStrictMode(t *testing.T) {
	body := []byte(`{
		"model":"claude-3-5-sonnet",
		"system":"Keep this instruction",
		"messages":[{"role":"user","content":"hello"}]
	}`)
	cfg := &domain.DisguiseClaudeCodeOptions{Mode: "always", StrictMode: false}

	result := applyCloaking(body, "curl/7.68.0", "claude-3-5-sonnet", cfg)
	if got := gjson.GetBytes(result, "system.0.text").String(); got != claudeCodeSystemPrompt {
		t.Fatalf("expected Claude Code prompt prepended, got %q", got)
	}
	if got := gjson.GetBytes(result, "system.1.text").String(); got != "Keep this instruction" {
		t.Fatalf("expected original system string preserved, got %q", got)
	}
}

func TestProcessClaudeRequestBodyStripsVolatileBillingCCH(t *testing.T) {
	body := []byte(`{
		"model":"claude-3-5-sonnet",
		"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=20250219; cc_entrypoint=cli; cch=abc123; cc_env=prod; cch=def456;"}],
		"messages":[
			{"role":"user","content":"hello"},
			{"role":"assistant","content":"ok"},
			{"role":"user","content":"again"}
		]
	}`)

	result, _ := processClaudeRequestBody(body, "claude-cli/2.1.23 (external, cli)", noneDisguiseCustomCfg())

	systemText := gjson.GetBytes(result, "system.0.text").String()
	if strings.Contains(systemText, "cch=") {
		t.Fatalf("expected volatile cch fields removed, got %q", systemText)
	}
	for _, key := range []string{"cc_version=20250219;", "cc_entrypoint=cli;", "cc_env=prod;"} {
		if !strings.Contains(systemText, key) {
			t.Fatalf("expected %s to be preserved, got %q", key, systemText)
		}
	}
}

func TestStripVolatileClaudeBillingCCHSupportsMessageEnvelope(t *testing.T) {
	body := []byte(`{
		"message":{
			"system":[
				{"type":"text","text":"cc_version=20250219; cch=r1; cc_entrypoint=cli;"}
			]
		}
	}`)

	result := stripVolatileClaudeBillingCCH(body)
	systemText := gjson.GetBytes(result, "message.system.0.text").String()
	if strings.Contains(systemText, "cch=") {
		t.Fatalf("expected envelope message.system billing cch removed, got %q", systemText)
	}
	if !strings.Contains(systemText, "cc_version=20250219;") || !strings.Contains(systemText, "cc_entrypoint=cli;") {
		t.Fatalf("expected stable billing keys preserved, got %q", systemText)
	}
}

func TestSanitizeClaudeMessagesRemovesEmptyTextBlocks(t *testing.T) {
	body := []byte(`{
		"model":"claude-3-5-sonnet",
		"messages":[
			{
				"role":"user",
				"content":[
					{"type":"text","text":"   "},
					{"type":"tool_result","tool_use_id":"tool_1","content":"ok"},
					{"type":"text","text":"hello"}
				]
			}
		]
	}`)

	result := sanitizeClaudeMessages(body)

	if got := gjson.GetBytes(result, "messages.0.content.#").Int(); got != 2 {
		t.Fatalf("content block count = %d, want 2", got)
	}
	if gjson.GetBytes(result, "messages.0.content.0.type").String() != "tool_result" {
		t.Fatalf("first block type = %q, want tool_result", gjson.GetBytes(result, "messages.0.content.0.type").String())
	}
	if gjson.GetBytes(result, "messages.0.content.1.text").String() != "hello" {
		t.Fatalf("remaining text block = %q, want hello", gjson.GetBytes(result, "messages.0.content.1.text").String())
	}
}

func TestSanitizeClaudeMessagesRemovesEmptyTextBlocksInToolResult(t *testing.T) {
	body := []byte(`{
		"model":"claude-3-5-sonnet",
		"messages":[
			{
				"role":"user",
				"content":[
					{
						"type":"tool_result",
						"tool_use_id":"tool_1",
						"content":[
							{"type":"text","text":""},
							{"type":"text","text":"real content"}
						]
					}
				]
			}
		]
	}`)

	result := sanitizeClaudeMessages(body)

	nested := gjson.GetBytes(result, "messages.0.content.0.content")
	if !nested.IsArray() {
		t.Fatalf("expected nested content to be array, got %s", nested.Raw)
	}
	if got := nested.Get("#").Int(); got != 1 {
		t.Fatalf("nested content block count = %d, want 1", got)
	}
	if got := nested.Get("0.text").String(); got != "real content" {
		t.Fatalf("nested text = %q, want 'real content'", got)
	}
}

func TestSanitizeClaudeMessagesReplacesAllEmptyToolResultWithPlaceholder(t *testing.T) {
	body := []byte(`{
		"model":"claude-3-5-sonnet",
		"messages":[
			{
				"role":"user",
				"content":[
					{
						"type":"tool_result",
						"tool_use_id":"tool_1",
						"content":[{"type":"text","text":"   "}]
					},
					{"type":"text","text":"hello"}
				]
			}
		]
	}`)

	result := sanitizeClaudeMessages(body)

	// tool_result must be preserved (not dropped) to maintain tool_use/tool_result correspondence.
	if got := gjson.GetBytes(result, "messages.0.content.#").Int(); got != 2 {
		t.Fatalf("content block count = %d, want 2 (tool_result should be preserved with placeholder)", got)
	}
	if gjson.GetBytes(result, "messages.0.content.0.type").String() != "tool_result" {
		t.Fatalf("first block type = %q, want tool_result", gjson.GetBytes(result, "messages.0.content.0.type").String())
	}
	nested := gjson.GetBytes(result, "messages.0.content.0.content")
	if !nested.IsArray() || nested.Get("#").Int() != 1 {
		t.Fatalf("tool_result nested content should have 1 placeholder block, got %s", nested.Raw)
	}
	if got := nested.Get("0.text").String(); got != "[empty]" {
		t.Fatalf("placeholder text = %q, want '[empty]'", got)
	}
}

func TestProcessClaudeRequestBodyReplacesEmptyContentWithPlaceholder(t *testing.T) {
	body := []byte(`{
		"model":"claude-3-5-sonnet",
		"messages":[
			{"role":"user","content":[{"type":"text","text":"   "}]},
			{"role":"assistant","content":"ok"},
			{"role":"user","content":"next"}
		]
	}`)

	result, _ := processClaudeRequestBody(body, "curl/7.68.0", noneDisguiseCustomCfg())

	// Message count is preserved: empty content is replaced with placeholder,
	// never dropped, to maintain user/assistant alternation and tool correspondence.
	if got := gjson.GetBytes(result, "messages.#").Int(); got != 3 {
		t.Fatalf("message count = %d, want 3", got)
	}
	if gjson.GetBytes(result, "messages.0.role").String() != "user" {
		t.Fatalf("first message role = %q, want user", gjson.GetBytes(result, "messages.0.role").String())
	}
	if got := gjson.GetBytes(result, "messages.0.content.0.text").String(); got != "[empty]" {
		t.Fatalf("placeholder text = %q, want '[empty]'", got)
	}
}

func TestEnsureToolResultCorrespondenceInjectsMissing(t *testing.T) {
	body := []byte(`{
		"model":"claude-3-5-sonnet",
		"messages":[
			{"role":"user","content":"hello"},
			{"role":"assistant","content":[
				{"type":"tool_use","id":"tool_1","name":"bash","input":{"cmd":"ls"}},
				{"type":"tool_use","id":"tool_2","name":"read","input":{"path":"a.txt"}}
			]},
			{"role":"user","content":[
				{"type":"tool_result","tool_use_id":"tool_1","content":"file1"},
				{"type":"text","text":"here you go"}
			]}
		]
	}`)

	result := ensureToolResultCorrespondence(body)

	userContent := gjson.GetBytes(result, "messages.2.content")
	if !userContent.IsArray() {
		t.Fatalf("expected array content, got %s", userContent.Raw)
	}

	// tool_2 was missing, should be prepended.
	first := userContent.Array()[0]
	if first.Get("type").String() != "tool_result" {
		t.Fatalf("first block type = %q, want tool_result", first.Get("type").String())
	}
	if first.Get("tool_use_id").String() != "tool_2" {
		t.Fatalf("injected tool_use_id = %q, want tool_2", first.Get("tool_use_id").String())
	}
	if first.Get("content.0.text").String() != "[empty]" {
		t.Fatalf("injected content text = %q, want '[empty]'", first.Get("content.0.text").String())
	}

	// Original blocks should still be there after the injected one.
	if got := userContent.Get("#").Int(); got != 3 {
		t.Fatalf("content block count = %d, want 3", got)
	}
}

func TestEnsureToolResultCorrespondenceNoop(t *testing.T) {
	// All tool_results present: no modification.
	body := []byte(`{
		"model":"claude-3-5-sonnet",
		"messages":[
			{"role":"user","content":"hello"},
			{"role":"assistant","content":[
				{"type":"tool_use","id":"t1","name":"bash","input":{}}
			]},
			{"role":"user","content":[
				{"type":"tool_result","tool_use_id":"t1","content":"ok"}
			]}
		]
	}`)

	result := ensureToolResultCorrespondence(body)
	if string(result) != string(body) {
		t.Fatal("expected body to be unchanged when all tool_results present")
	}
}

func TestEnsureToolResultCorrespondenceInjectsSyntheticUserMessage(t *testing.T) {
	// Assistant with tool_use is the LAST message — no user message follows.
	body := []byte(`{
		"model":"claude-3-5-sonnet",
		"messages":[
			{"role":"user","content":"hello"},
			{"role":"assistant","content":[
				{"type":"tool_use","id":"tool_1","name":"bash","input":{"cmd":"ls"}}
			]}
		]
	}`)

	result := ensureToolResultCorrespondence(body)

	if got := gjson.GetBytes(result, "messages.#").Int(); got != 3 {
		t.Fatalf("message count = %d, want 3 (synthetic user message should be injected)", got)
	}
	injected := gjson.GetBytes(result, "messages.2")
	if injected.Get("role").String() != "user" {
		t.Fatalf("injected message role = %q, want user", injected.Get("role").String())
	}
	if injected.Get("content.0.type").String() != "tool_result" {
		t.Fatalf("injected block type = %q, want tool_result", injected.Get("content.0.type").String())
	}
	if injected.Get("content.0.tool_use_id").String() != "tool_1" {
		t.Fatalf("injected tool_use_id = %q, want tool_1", injected.Get("content.0.tool_use_id").String())
	}
}

func TestEnsureToolResultCorrespondenceNextIsNotUser(t *testing.T) {
	// Assistant with tool_use followed by another assistant (not user).
	body := []byte(`{
		"model":"claude-3-5-sonnet",
		"messages":[
			{"role":"user","content":"hello"},
			{"role":"assistant","content":[
				{"type":"tool_use","id":"t1","name":"bash","input":{}}
			]},
			{"role":"assistant","content":"continued"}
		]
	}`)

	result := ensureToolResultCorrespondence(body)

	// Should inject a user message between the two assistants.
	if got := gjson.GetBytes(result, "messages.#").Int(); got != 4 {
		t.Fatalf("message count = %d, want 4", got)
	}
	if gjson.GetBytes(result, "messages.2.role").String() != "user" {
		t.Fatalf("messages[2] role = %q, want user", gjson.GetBytes(result, "messages.2.role").String())
	}
	if gjson.GetBytes(result, "messages.2.content.0.tool_use_id").String() != "t1" {
		t.Fatalf("injected tool_use_id = %q, want t1", gjson.GetBytes(result, "messages.2.content.0.tool_use_id").String())
	}
	if gjson.GetBytes(result, "messages.3.role").String() != "assistant" {
		t.Fatalf("messages[3] role = %q, want assistant", gjson.GetBytes(result, "messages.3.role").String())
	}
}

// bedrockDisguiseCustomCfg returns a custom config wired for the "bedrock" disguise.
func bedrockDisguiseCustomCfg() *domain.ProviderConfigCustom {
	return &domain.ProviderConfigCustom{
		Disguise: &domain.ProviderConfigCustomDisguise{Type: "bedrock"},
	}
}

func TestProcessClaudeRequestBodyBedrockDisguiseStripsUnsupportedFields(t *testing.T) {
	// A representative Claude Code request body that includes every field Bedrock
	// rejects: betas, output_config, context_management, reasoning, tools[].custom,
	// cache_control.scope, and thinking.type=adaptive.
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"betas":["claude-code-20250219"],
		"output_config":{"foo":"bar"},
		"context_management":{"x":1},
		"reasoning":{"effort":"high"},
		"thinking":{"type":"adaptive"},
		"max_tokens":2048,
		"system":[
			{"type":"text","text":"You are Claude Code","cache_control":{"type":"ephemeral","scope":"turn"}}
		],
		"tools":[
			{"name":"bash","custom":{"foo":"bar"},"cache_control":{"type":"ephemeral","scope":"turn"}}
		],
		"messages":[
			{"role":"user","content":[{"type":"text","text":"hi","cache_control":{"type":"ephemeral","scope":"turn"}}]}
		]
	}`)

	result, betas := processClaudeRequestBody(body, "claude-cli/2.1.23 (external, cli)", bedrockDisguiseCustomCfg())

	// betas removed from body by the bedrock sanitizer (strictly before
	// extractAndRemoveBetas sees them), so extraBetas should be empty for the
	// bedrock disguise. The header path drops Anthropic-Beta entirely anyway.
	if gjson.GetBytes(result, "betas").Exists() {
		t.Error("betas should be removed from body")
	}
	if len(betas) != 0 {
		t.Errorf("expected no extra betas under bedrock disguise, got %v", betas)
	}

	// Bedrock-rejected top-level fields gone
	for _, f := range []string{"output_config", "context_management", "reasoning"} {
		if gjson.GetBytes(result, f).Exists() {
			t.Errorf("expected %s to be stripped", f)
		}
	}

	// thinking.type=adaptive → enabled
	if got := gjson.GetBytes(result, "thinking.type").String(); got != "enabled" {
		t.Errorf("thinking.type = %q, want enabled", got)
	}
	// budget_tokens auto-populated
	if got := gjson.GetBytes(result, "thinking.budget_tokens").Int(); got <= 0 {
		t.Errorf("thinking.budget_tokens should be > 0, got %d", got)
	}

	// cache_control.scope stripped from system / tools / messages, type preserved
	for _, p := range []string{
		"system.0.cache_control.scope",
		"tools.0.cache_control.scope",
		"messages.0.content.0.cache_control.scope",
	} {
		if gjson.GetBytes(result, p).Exists() {
			t.Errorf("expected %s to be stripped", p)
		}
	}
	for _, p := range []string{
		"system.0.cache_control.type",
		"tools.0.cache_control.type",
		"messages.0.content.0.cache_control.type",
	} {
		if got := gjson.GetBytes(result, p).String(); got != "ephemeral" {
			t.Errorf("expected %s = ephemeral, got %q", p, got)
		}
	}

	// tools[].custom stripped
	if gjson.GetBytes(result, "tools.0.custom").Exists() {
		t.Error("expected tools[].custom to be stripped")
	}

	// Importantly: model and stream NOT stripped (relay still needs them)
	if got := gjson.GetBytes(result, "model").String(); got != "claude-sonnet-4-5" {
		t.Errorf("model field should be preserved, got %q", got)
	}

	// And critically: NO Claude Code system prompt injected (bedrock disguise
	// is the OPPOSITE of cloaking — we strip Claude Code identity, not add it).
	systemText := gjson.GetBytes(result, "system.0.text").String()
	if strings.Contains(systemText, "Claude Code, Anthropic's official CLI for Claude") {
		t.Errorf("bedrock disguise should not inject Claude Code system prompt, got %q", systemText)
	}
	if gjson.GetBytes(result, "metadata.user_id").Exists() {
		t.Error("bedrock disguise should not inject fake user_id")
	}
}

// TestProcessClaudeRequestBodyBedrockDisguiseRecheckMaxTokensAfterMinBudget
// guards against a subtle ordering bug: SanitizeForBedrockCompat enforces
// `max_tokens > thinking.budget_tokens` early, then ensureMinThinkingBudget
// raises the budget to 1024 — which can put max_tokens BELOW the new budget.
// processClaudeRequestBody must re-run the constraint after the min-budget
// raise so Bedrock doesn't reject the request.
func TestProcessClaudeRequestBodyBedrockDisguiseRecheckMaxTokensAfterMinBudget(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-6",
		"max_tokens":200,
		"thinking":{"type":"enabled","budget_tokens":100},
		"messages":[{"role":"user","content":"hi"}]
	}`)

	result, _ := processClaudeRequestBody(body, "claude-cli/2.1.23 (external, cli)", bedrockDisguiseCustomCfg())

	// ensureMinThinkingBudget should have raised budget to 1024
	if got := gjson.GetBytes(result, "thinking.budget_tokens").Int(); got != 1024 {
		t.Errorf("thinking.budget_tokens = %d, want 1024 (min)", got)
	}
	// max_tokens MUST then be raised above the new budget
	maxT := gjson.GetBytes(result, "max_tokens").Int()
	if maxT <= 1024 {
		t.Errorf("max_tokens = %d, want > 1024 (above raised budget)", maxT)
	}
}

func TestProcessClaudeRequestBodyNoneDisguiseSkipsCloaking(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"messages":[{"role":"user","content":"hello"}]
	}`)

	// Non-Claude UA + nil cfg = legacy auto-cloak (test from baseline)
	resultLegacy, _ := processClaudeRequestBody(body, "curl/8.0.0", nil)
	if !gjson.GetBytes(resultLegacy, "metadata.user_id").Exists() {
		t.Error("nil disguise should preserve legacy auto-cloak default for non-Claude UA")
	}

	// Same UA + explicit Type=none = no cloaking
	result, _ := processClaudeRequestBody(body, "curl/8.0.0", noneDisguiseCustomCfg())
	if gjson.GetBytes(result, "metadata.user_id").Exists() {
		t.Error("Type=none should skip body cloaking")
	}
	if gjson.GetBytes(result, "system").Exists() {
		t.Error("Type=none should not inject system prompt")
	}
}
