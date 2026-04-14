package bedrock

import (
	"fmt"
	"math"
	"testing"

	"github.com/tidwall/gjson"
)

func TestSanitizeForBedrockCompatStripsRejectedTopLevelFields(t *testing.T) {
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"stream":true,
		"betas":["claude-code-20250219"],
		"output_config":{"foo":"bar"},
		"context_management":{"x":1},
		"reasoning":{"effort":"high"},
		"max_tokens":1024,
		"messages":[{"role":"user","content":"hi"}]
	}`)

	result := SanitizeForBedrockCompat(body)

	for _, f := range []string{"betas", "output_config", "context_management", "reasoning"} {
		if gjson.GetBytes(result, f).Exists() {
			t.Errorf("expected %s to be stripped, still present", f)
		}
	}

	// model and stream are NOT stripped — relay still needs them. Only the
	// direct-Bedrock sanitizeRequestBody removes those.
	if got := gjson.GetBytes(result, "model").String(); got != "claude-sonnet-4-5" {
		t.Errorf("model should be preserved, got %q", got)
	}
	if !gjson.GetBytes(result, "stream").Exists() {
		t.Error("stream should be preserved by SanitizeForBedrockCompat")
	}
}

func TestSanitizeForBedrockCompatConvertsAdaptiveThinking(t *testing.T) {
	body := []byte(`{
		"thinking":{"type":"adaptive"},
		"max_tokens":4096,
		"messages":[{"role":"user","content":"hi"}]
	}`)

	result := SanitizeForBedrockCompat(body)

	if got := gjson.GetBytes(result, "thinking.type").String(); got != "enabled" {
		t.Errorf("thinking.type = %q, want enabled", got)
	}
	if got := gjson.GetBytes(result, "thinking.budget_tokens").Int(); got != 4095 {
		t.Errorf("thinking.budget_tokens = %d, want 4095 (max_tokens-1)", got)
	}
}

func TestSanitizeForBedrockCompatRaisesMaxTokensWhenBudgetExceeds(t *testing.T) {
	body := []byte(`{
		"thinking":{"type":"enabled","budget_tokens":5000},
		"max_tokens":2048,
		"messages":[{"role":"user","content":"hi"}]
	}`)

	result := SanitizeForBedrockCompat(body)

	if got := gjson.GetBytes(result, "max_tokens").Int(); got != 5001 {
		t.Errorf("max_tokens = %d, want 5001 (budget+1)", got)
	}
}

func TestSanitizeForBedrockCompatStripsCacheControlScope(t *testing.T) {
	body := []byte(`{
		"system":[{"type":"text","text":"x","cache_control":{"type":"ephemeral","scope":"turn"}}],
		"tools":[{"name":"t","cache_control":{"type":"ephemeral","scope":"turn"}}],
		"messages":[{"role":"user","content":[{"type":"text","text":"hi","cache_control":{"type":"ephemeral","scope":"turn"}}]}]
	}`)

	result := SanitizeForBedrockCompat(body)

	for _, p := range []string{
		"system.0.cache_control.scope",
		"tools.0.cache_control.scope",
		"messages.0.content.0.cache_control.scope",
	} {
		if gjson.GetBytes(result, p).Exists() {
			t.Errorf("%s should be stripped", p)
		}
	}

	// type sub-field preserved everywhere
	for _, p := range []string{
		"system.0.cache_control.type",
		"tools.0.cache_control.type",
		"messages.0.content.0.cache_control.type",
	} {
		if got := gjson.GetBytes(result, p).String(); got != "ephemeral" {
			t.Errorf("%s = %q, want ephemeral", p, got)
		}
	}
}

func TestSanitizeForBedrockCompatStripsToolsCustom(t *testing.T) {
	body := []byte(`{
		"tools":[{"name":"a","custom":{"foo":"bar"}},{"name":"b"}]
	}`)

	result := SanitizeForBedrockCompat(body)

	if gjson.GetBytes(result, "tools.0.custom").Exists() {
		t.Error("tools[0].custom should be stripped")
	}
	if got := gjson.GetBytes(result, "tools.0.name").String(); got != "a" {
		t.Errorf("tools[0].name = %q, want a", got)
	}
}

// TestEnsureMaxTokensAboveThinkingBudgetGuardsOverflow asserts that a
// pathologically large budget_tokens (== math.MaxInt64) does not cause the
// `+1` increment to wrap to a negative integer when written back into
// max_tokens. The request will fail at Bedrock anyway, but we never want to
// silently corrupt the field with garbage.
func TestEnsureMaxTokensAboveThinkingBudgetGuardsOverflow(t *testing.T) {
	body := []byte(fmt.Sprintf(
		`{"thinking":{"type":"enabled","budget_tokens":%d},"max_tokens":1024}`,
		int64(math.MaxInt64),
	))
	out := EnsureMaxTokensAboveThinkingBudget(body)
	got := gjson.GetBytes(out, "max_tokens").Int()
	if got != math.MaxInt64 {
		t.Errorf("max_tokens = %d, want clamp to math.MaxInt64 (%d)", got, int64(math.MaxInt64))
	}
	if got < 0 {
		t.Errorf("max_tokens overflowed to negative value: %d", got)
	}
}

func TestEnsureMaxTokensAboveThinkingBudgetIdempotent(t *testing.T) {
	// Already-valid input should be unchanged.
	body := []byte(`{"thinking":{"type":"enabled","budget_tokens":1024},"max_tokens":2048}`)
	out := EnsureMaxTokensAboveThinkingBudget(body)
	if got := gjson.GetBytes(out, "max_tokens").Int(); got != 2048 {
		t.Errorf("max_tokens = %d, want 2048 (unchanged)", got)
	}

	// Budget == max should be raised so max_tokens > budget.
	body = []byte(`{"thinking":{"type":"enabled","budget_tokens":1024},"max_tokens":1024}`)
	out = EnsureMaxTokensAboveThinkingBudget(body)
	if got := gjson.GetBytes(out, "max_tokens").Int(); got != 1025 {
		t.Errorf("max_tokens = %d, want 1025", got)
	}

	// thinking disabled — leave alone even if max_tokens is small.
	body = []byte(`{"thinking":{"type":"disabled","budget_tokens":2000},"max_tokens":100}`)
	out = EnsureMaxTokensAboveThinkingBudget(body)
	if got := gjson.GetBytes(out, "max_tokens").Int(); got != 100 {
		t.Errorf("disabled-thinking max_tokens = %d, want 100", got)
	}

	// max_tokens unset (== 0) — never invent a ceiling.
	body = []byte(`{"thinking":{"type":"enabled","budget_tokens":2000}}`)
	out = EnsureMaxTokensAboveThinkingBudget(body)
	if gjson.GetBytes(out, "max_tokens").Exists() {
		t.Error("max_tokens should not be invented when caller didn't set one")
	}
}

func TestRemoveOrphanedToolResults(t *testing.T) {
	t.Run("removes orphaned tool_result", func(t *testing.T) {
		body := []byte(`{
			"messages":[
				{"role":"user","content":"hi"},
				{"role":"assistant","content":[
					{"type":"tool_use","id":"toolu_aaa","name":"get_weather","input":{"city":"Tokyo"}}
				]},
				{"role":"user","content":[
					{"type":"tool_result","tool_use_id":"toolu_aaa","content":"sunny"},
					{"type":"tool_result","tool_use_id":"toolu_orphan","content":"stale"},
					{"type":"text","text":"thanks"}
				]}
			]
		}`)

		result := RemoveOrphanedToolResults(body)

		msgs := gjson.GetBytes(result, "messages").Array()
		userContent := msgs[2].Get("content").Array()

		if len(userContent) != 2 {
			t.Fatalf("expected 2 content blocks, got %d", len(userContent))
		}
		// Valid tool_result kept
		if userContent[0].Get("tool_use_id").String() != "toolu_aaa" {
			t.Error("valid tool_result should be kept")
		}
		// Text block kept
		if userContent[1].Get("type").String() != "text" {
			t.Error("text block should be kept")
		}
	})

	t.Run("replaces with empty text when all tool_results orphaned", func(t *testing.T) {
		body := []byte(`{
			"messages":[
				{"role":"assistant","content":[
					{"type":"tool_use","id":"toolu_aaa","name":"f","input":{}}
				]},
				{"role":"user","content":[
					{"type":"tool_result","tool_use_id":"toolu_bad","content":"x"}
				]}
			]
		}`)

		result := RemoveOrphanedToolResults(body)

		msgs := gjson.GetBytes(result, "messages").Array()
		userContent := msgs[1].Get("content").Array()
		if len(userContent) != 1 {
			t.Fatalf("expected 1 content block, got %d", len(userContent))
		}
		if userContent[0].Get("type").String() != "text" {
			t.Error("expected fallback text block")
		}
	})

	t.Run("no-op when all tool_results match", func(t *testing.T) {
		body := []byte(`{
			"messages":[
				{"role":"assistant","content":[
					{"type":"tool_use","id":"toolu_1","name":"f","input":{}}
				]},
				{"role":"user","content":[
					{"type":"tool_result","tool_use_id":"toolu_1","content":"ok"}
				]}
			]
		}`)

		result := RemoveOrphanedToolResults(body)

		// Should be unchanged (byte-exact)
		if gjson.GetBytes(result, "messages.1.content.0.tool_use_id").String() != "toolu_1" {
			t.Error("matching tool_result should be preserved")
		}
	})

	t.Run("no-op when no tool_results", func(t *testing.T) {
		body := []byte(`{
			"messages":[
				{"role":"user","content":"hi"},
				{"role":"assistant","content":"hello"}
			]
		}`)

		result := RemoveOrphanedToolResults(body)

		if string(result) != string(body) {
			t.Error("should be unchanged when no tool_results")
		}
	})

	t.Run("removes tool_result when preceding assistant has only text", func(t *testing.T) {
		body := []byte(`{
			"messages":[
				{"role":"assistant","content":[
					{"type":"text","text":"I don't need any tools for this."}
				]},
				{"role":"user","content":[
					{"type":"tool_result","tool_use_id":"toolu_stale","content":"leftover"},
					{"type":"text","text":"ok thanks"}
				]}
			]
		}`)

		result := RemoveOrphanedToolResults(body)

		msgs := gjson.GetBytes(result, "messages").Array()
		userContent := msgs[1].Get("content").Array()
		if len(userContent) != 1 {
			t.Fatalf("expected 1 content block, got %d", len(userContent))
		}
		if userContent[0].Get("type").String() != "text" {
			t.Error("expected text block preserved")
		}
		if userContent[0].Get("text").String() != "ok thanks" {
			t.Errorf("text = %q, want 'ok thanks'", userContent[0].Get("text").String())
		}
	})

	t.Run("does not touch user message without preceding assistant", func(t *testing.T) {
		body := []byte(`{
			"messages":[
				{"role":"user","content":[
					{"type":"tool_result","tool_use_id":"toolu_x","content":"y"}
				]}
			]
		}`)

		result := RemoveOrphanedToolResults(body)

		// First message has no preceding assistant, should be untouched
		if gjson.GetBytes(result, "messages.0.content.0.tool_use_id").String() != "toolu_x" {
			t.Error("first user message should not be modified")
		}
	})
}

func TestSanitizeForBedrockCompatStripsEmptyArrays(t *testing.T) {
	body := []byte(`{
		"system":[],
		"tools":[],
		"messages":[{"role":"user","content":"hi"}]
	}`)

	result := SanitizeForBedrockCompat(body)

	if gjson.GetBytes(result, "system").Exists() {
		t.Error("empty system[] should be stripped")
	}
	if gjson.GetBytes(result, "tools").Exists() {
		t.Error("empty tools[] should be stripped")
	}
	// messages should be preserved (it's not empty)
	if !gjson.GetBytes(result, "messages").Exists() {
		t.Error("non-empty messages should be preserved")
	}
}

func TestSanitizeForBedrockCompatPreservesNonEmptyArrays(t *testing.T) {
	body := []byte(`{
		"system":[{"type":"text","text":"hello"}],
		"tools":[{"name":"tool1"}],
		"messages":[{"role":"user","content":"hi"}]
	}`)

	result := SanitizeForBedrockCompat(body)

	if !gjson.GetBytes(result, "system").Exists() {
		t.Error("non-empty system should be preserved")
	}
	if !gjson.GetBytes(result, "tools").Exists() {
		t.Error("non-empty tools should be preserved")
	}
}

func TestSanitizeRequestBodyRemovesModelAndStream(t *testing.T) {
	// The direct-Bedrock helper should strip both fields and set anthropic_version.
	body := []byte(`{
		"model":"claude-sonnet-4-5",
		"stream":true,
		"messages":[{"role":"user","content":"hi"}]
	}`)

	result := sanitizeRequestBody(body)

	if gjson.GetBytes(result, "model").Exists() {
		t.Error("model should be stripped for direct Bedrock")
	}
	if gjson.GetBytes(result, "stream").Exists() {
		t.Error("stream should be stripped for direct Bedrock")
	}
	if got := gjson.GetBytes(result, "anthropic_version").String(); got != BedrockAPIVersion {
		t.Errorf("anthropic_version = %q, want %q", got, BedrockAPIVersion)
	}
}
