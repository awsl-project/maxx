package bedrock

import (
	"fmt"
	"math"
	"strings"
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

func TestSanitizeForBedrockCompatStripsSamplingParamsWhenThinking(t *testing.T) {
	cases := []struct {
		name         string
		body         string
		wantStripped bool
	}{
		{
			name:         "thinking enabled strips sampling",
			body:         `{"thinking":{"type":"enabled","budget_tokens":2048},"max_tokens":4096,"temperature":0.7,"top_p":0.9,"top_k":40,"messages":[{"role":"user","content":"hi"}]}`,
			wantStripped: true,
		},
		{
			name:         "thinking adaptive strips sampling",
			body:         `{"thinking":{"type":"adaptive"},"temperature":0.5,"top_p":0.8,"messages":[{"role":"user","content":"hi"}]}`,
			wantStripped: true,
		},
		{
			name:         "no thinking preserves sampling",
			body:         `{"temperature":0.7,"top_p":0.9,"top_k":40,"messages":[{"role":"user","content":"hi"}]}`,
			wantStripped: false,
		},
		{
			name:         "thinking disabled preserves sampling",
			body:         `{"thinking":{"type":"disabled"},"temperature":0.7,"messages":[{"role":"user","content":"hi"}]}`,
			wantStripped: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := SanitizeForBedrockCompat([]byte(c.body))
			for _, f := range []string{"temperature", "top_p", "top_k"} {
				exists := gjson.GetBytes(result, f).Exists()
				inInput := gjson.GetBytes([]byte(c.body), f).Exists()
				if !inInput {
					continue
				}
				if c.wantStripped && exists {
					t.Errorf("%s should be stripped, still present", f)
				}
				if !c.wantStripped && !exists {
					t.Errorf("%s should be preserved, was stripped", f)
				}
			}
		})
	}
}

func TestIsSamplingParamRejectedError(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "anthropic-style (with backticks) temperature",
			body: `{"message":"` + "`temperature`" + ` may only be set to 1 when thinking is enabled"}`,
			want: true,
		},
		{
			name: "anthropic-style (with backticks) top_p",
			body: `{"message":"` + "`top_p`" + ` is not supported when thinking is enabled"}`,
			want: true,
		},
		{
			name: "bedrock-native (no backticks) temperature",
			body: `{"message":"temperature may only be set to 1 when thinking is enabled"}`,
			want: true,
		},
		{
			name: "bedrock-native (no backticks) top_k",
			body: `{"message":"top_k is not supported when thinking is enabled"}`,
			want: true,
		},
		{
			name: "thinking mentioned first, then field",
			body: `{"message":"thinking mode is active so top_p is not allowed"}`,
			want: true,
		},
		{
			name: "with extended thinking phrasing",
			body: `{"message":"temperature cannot be set with extended thinking"}`,
			want: true,
		},
		{
			name: "unrelated 400 (no thinking mention)",
			body: `{"message":"temperature must be between 0 and 1"}`,
			want: false,
		},
		{
			name: "unrelated 400 (thinking mentioned but no sampling field)",
			body: `{"message":"thinking budget exceeds max_tokens"}`,
			want: false,
		},
		{
			name: "false-positive guard: bare co-occurrence is not rejection",
			body: `{"message":"temperature must be between 0 and 1; thinking budget exceeds max_tokens"}`,
			want: false,
		},
		{
			name: "false-positive guard: thinking as noun modifier near field",
			body: `{"message":"thinking budget too low; temperature was 0.7"}`,
			want: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsSamplingParamRejectedError([]byte(c.body)); got != c.want {
				t.Errorf("IsSamplingParamRejectedError = %v, want %v", got, c.want)
			}
		})
	}
}

func TestStripSamplingParams(t *testing.T) {
	body := []byte(`{"temperature":0.7,"top_p":0.9,"top_k":40,"max_tokens":1024}`)
	got := StripSamplingParams(body)
	for _, f := range []string{"temperature", "top_p", "top_k"} {
		if gjson.GetBytes(got, f).Exists() {
			t.Errorf("%s should be stripped", f)
		}
	}
	if !gjson.GetBytes(got, "max_tokens").Exists() {
		t.Error("max_tokens should be preserved")
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

func TestNormalizeToolIdentifiers(t *testing.T) {
	t.Run("rewrites tool_use.id and matching tool_result.tool_use_id", func(t *testing.T) {
		body := []byte(`{
			"messages":[
				{"role":"assistant","content":[
					{"type":"tool_use","id":"functions.foo:0","name":"functions.foo","input":{}}
				]},
				{"role":"user","content":[
					{"type":"tool_result","tool_use_id":"functions.foo:0","content":"ok"}
				]}
			]
		}`)

		result := NormalizeToolIdentifiers(body)

		gotID := gjson.GetBytes(result, "messages.0.content.0.id").String()
		if gotID != "functions_foo_0" {
			t.Errorf("tool_use.id = %q, want functions_foo_0", gotID)
		}
		gotName := gjson.GetBytes(result, "messages.0.content.0.name").String()
		if gotName != "functions_foo" {
			t.Errorf("tool_use.name = %q, want functions_foo", gotName)
		}
		gotRefID := gjson.GetBytes(result, "messages.1.content.0.tool_use_id").String()
		if gotRefID != gotID {
			t.Errorf("tool_result.tool_use_id = %q, want matching %q", gotRefID, gotID)
		}
	})

	t.Run("preserves already-valid identifiers", func(t *testing.T) {
		body := []byte(`{
			"messages":[
				{"role":"assistant","content":[
					{"type":"tool_use","id":"toolu_abc-123","name":"my_tool","input":{}}
				]}
			]
		}`)

		result := NormalizeToolIdentifiers(body)

		if gjson.GetBytes(result, "messages.0.content.0.id").String() != "toolu_abc-123" {
			t.Errorf("valid id mutated: %s", result)
		}
		if gjson.GetBytes(result, "messages.0.content.0.name").String() != "my_tool" {
			t.Errorf("valid name mutated: %s", result)
		}
	})

	t.Run("disambiguates colliding originals", func(t *testing.T) {
		body := []byte(`{
			"messages":[
				{"role":"assistant","content":[
					{"type":"tool_use","id":"foo_bar","name":"a","input":{}},
					{"type":"tool_use","id":"foo.bar","name":"b","input":{}}
				]},
				{"role":"user","content":[
					{"type":"tool_result","tool_use_id":"foo_bar","content":"r0"},
					{"type":"tool_result","tool_use_id":"foo.bar","content":"r1"}
				]}
			]
		}`)

		result := NormalizeToolIdentifiers(body)

		id0 := gjson.GetBytes(result, "messages.0.content.0.id").String()
		id1 := gjson.GetBytes(result, "messages.0.content.1.id").String()
		ref0 := gjson.GetBytes(result, "messages.1.content.0.tool_use_id").String()
		ref1 := gjson.GetBytes(result, "messages.1.content.1.tool_use_id").String()

		if id0 != "foo_bar" {
			t.Errorf("first id should keep canonical form, got %q", id0)
		}
		if id1 == id0 {
			t.Errorf("colliding ids must disambiguate, both got %q", id0)
		}
		if ref0 != id0 {
			t.Errorf("tool_result[0] should match tool_use[0]: %q vs %q", ref0, id0)
		}
		if ref1 != id1 {
			t.Errorf("tool_result[1] should match tool_use[1]: %q vs %q", ref1, id1)
		}
	})

	t.Run("handles multiple tool_use blocks in one message", func(t *testing.T) {
		body := []byte(`{
			"messages":[
				{"role":"assistant","content":[
					{"type":"text","text":"thinking..."},
					{"type":"tool_use","id":"a.1","name":"f.x","input":{}},
					{"type":"tool_use","id":"a.2","name":"f.y","input":{}},
					{"type":"tool_use","id":"a.3","name":"f.z","input":{}}
				]}
			]
		}`)

		result := NormalizeToolIdentifiers(body)

		for j, want := range []string{"a_1", "a_2", "a_3"} {
			path := fmt.Sprintf("messages.0.content.%d.id", j+1)
			if got := gjson.GetBytes(result, path).String(); got != want {
				t.Errorf("content.%d.id = %q, want %q", j+1, got, want)
			}
		}
	})

	t.Run("caps tool_use.name in messages content", func(t *testing.T) {
		longName := strings.Repeat("a.", 80) // 160 chars, also contains invalid `.`
		body := []byte(`{
			"messages":[
				{"role":"assistant","content":[
					{"type":"tool_use","id":"toolu_1","name":"` + longName + `","input":{}}
				]}
			]
		}`)

		result := NormalizeToolIdentifiers(body)

		got := gjson.GetBytes(result, "messages.0.content.0.name").String()
		if len(got) != 128 {
			t.Errorf("tool_use.name length = %d, want 128", len(got))
		}
		if toolIdentifierInvalidChar.MatchString(got) {
			t.Errorf("tool_use.name still contains invalid chars: %q", got)
		}
	})

	t.Run("suffix skips slots already taken by a valid id", func(t *testing.T) {
		// `foo_bar_1` is already valid; `foo_bar` is already valid;
		// `foo.bar` would collapse onto `foo_bar` → suffixed to `foo_bar_1` (taken)
		// → must bump to `foo_bar_2`.
		body := []byte(`{
			"messages":[
				{"role":"assistant","content":[
					{"type":"tool_use","id":"foo_bar_1","name":"a","input":{}},
					{"type":"tool_use","id":"foo_bar","name":"b","input":{}},
					{"type":"tool_use","id":"foo.bar","name":"c","input":{}}
				]}
			]
		}`)

		result := NormalizeToolIdentifiers(body)
		ids := []string{
			gjson.GetBytes(result, "messages.0.content.0.id").String(),
			gjson.GetBytes(result, "messages.0.content.1.id").String(),
			gjson.GetBytes(result, "messages.0.content.2.id").String(),
		}
		want := []string{"foo_bar_1", "foo_bar", "foo_bar_2"}
		for i, w := range want {
			if ids[i] != w {
				t.Errorf("ids[%d] = %q, want %q (all=%v)", i, ids[i], w, ids)
			}
		}
	})

	t.Run("repeated original id maps to same normalized value", func(t *testing.T) {
		body := []byte(`{
			"messages":[
				{"role":"assistant","content":[
					{"type":"tool_use","id":"x.1","name":"a","input":{}}
				]},
				{"role":"user","content":[
					{"type":"tool_result","tool_use_id":"x.1","content":"r1"},
					{"type":"tool_result","tool_use_id":"x.1","content":"r2"}
				]}
			]
		}`)

		result := NormalizeToolIdentifiers(body)
		id := gjson.GetBytes(result, "messages.0.content.0.id").String()
		ref0 := gjson.GetBytes(result, "messages.1.content.0.tool_use_id").String()
		ref1 := gjson.GetBytes(result, "messages.1.content.1.tool_use_id").String()
		if id == "" || id != ref0 || id != ref1 {
			t.Errorf("repeated original should map identically: id=%q ref0=%q ref1=%q", id, ref0, ref1)
		}
	})

	t.Run("ignores non-array message content", func(t *testing.T) {
		body := []byte(`{"messages":[{"role":"user","content":"plain string"}]}`)
		result := NormalizeToolIdentifiers(body)
		if gjson.GetBytes(result, "messages.0.content").String() != "plain string" {
			t.Errorf("plain-string content should be untouched: %s", result)
		}
	})

	t.Run("disambiguates colliding tool names across tools[] and tool_use", func(t *testing.T) {
		// Regression: two distinct tool definitions whose names collapse to
		// the same Bedrock-valid form must stay distinguishable on the wire,
		// and tool_use.name references must follow the same mapping so that
		// each tool_use points to the right (renamed) definition.
		body := []byte(`{
			"tools":[
				{"name":"functions.foo","description":"a","input_schema":{}},
				{"name":"functions/foo","description":"b","input_schema":{}}
			],
			"messages":[{"role":"assistant","content":[
				{"type":"tool_use","id":"id1","name":"functions.foo","input":{}},
				{"type":"tool_use","id":"id2","name":"functions/foo","input":{}}
			]}]
		}`)

		result := NormalizeToolIdentifiers(body)

		t0 := gjson.GetBytes(result, "tools.0.name").String()
		t1 := gjson.GetBytes(result, "tools.1.name").String()
		u0 := gjson.GetBytes(result, "messages.0.content.0.name").String()
		u1 := gjson.GetBytes(result, "messages.0.content.1.name").String()

		if t0 == t1 {
			t.Errorf("tools[] names must not collide, both = %q", t0)
		}
		if u0 == u1 {
			t.Errorf("tool_use names must not collide, both = %q", u0)
		}
		if t0 != u0 {
			t.Errorf("tool_use[0].name should match tools[0].name: %q vs %q", u0, t0)
		}
		if t1 != u1 {
			t.Errorf("tool_use[1].name should match tools[1].name: %q vs %q", u1, t1)
		}
		// And the first-occurrence original wins canonical form.
		if t0 != "functions_foo" {
			t.Errorf("tools[0].name should keep canonical form, got %q", t0)
		}
	})

	t.Run("suffix respects 128-char name cap", func(t *testing.T) {
		// Two long names that collapse to the same 128-char base. Suffix
		// allocation must re-truncate the base so `base + "_1"` stays ≤ 128.
		long := strings.Repeat("a", 130)
		body := []byte(`{
			"tools":[
				{"name":"` + long + `.x"},
				{"name":"` + long + `/x"}
			],
			"messages":[{"role":"user","content":"hi"}]
		}`)

		result := NormalizeToolIdentifiers(body)
		t0 := gjson.GetBytes(result, "tools.0.name").String()
		t1 := gjson.GetBytes(result, "tools.1.name").String()
		if len(t0) > 128 || len(t1) > 128 {
			t.Errorf("names exceed 128-char cap: %d, %d", len(t0), len(t1))
		}
		if t0 == t1 {
			t.Errorf("names should be distinct after suffix, both = %q", t0)
		}
	})

	t.Run("normalizes tools[].name", func(t *testing.T) {
		body := []byte(`{
			"tools":[{"name":"functions.bar","description":"d","input_schema":{}}],
			"messages":[{"role":"user","content":"hi"}]
		}`)

		result := NormalizeToolIdentifiers(body)

		if got := gjson.GetBytes(result, "tools.0.name").String(); got != "functions_bar" {
			t.Errorf("tools[0].name = %q, want functions_bar", got)
		}
	})

	t.Run("caps name at 128 chars", func(t *testing.T) {
		longName := strings.Repeat("a", 150)
		body := []byte(`{"tools":[{"name":"` + longName + `"}],"messages":[{"role":"user","content":"hi"}]}`)

		result := NormalizeToolIdentifiers(body)

		got := gjson.GetBytes(result, "tools.0.name").String()
		if len(got) != 128 {
			t.Errorf("tools[0].name length = %d, want 128", len(got))
		}
	})

	t.Run("integrates via SanitizeForBedrockCompat", func(t *testing.T) {
		body := []byte(`{
			"messages":[
				{"role":"assistant","content":[
					{"type":"tool_use","id":"functions.x:1","name":"functions.x","input":{}}
				]},
				{"role":"user","content":[
					{"type":"tool_result","tool_use_id":"functions.x:1","content":"ok"}
				]}
			]
		}`)

		result := SanitizeForBedrockCompat(body)

		if got := gjson.GetBytes(result, "messages.0.content.0.id").String(); got != "functions_x_1" {
			t.Errorf("SanitizeForBedrockCompat did not normalize tool_use.id: %q", got)
		}
		if got := gjson.GetBytes(result, "messages.1.content.0.tool_use_id").String(); got != "functions_x_1" {
			t.Errorf("SanitizeForBedrockCompat did not normalize tool_result.tool_use_id: %q", got)
		}
	})
}
