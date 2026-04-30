package bedrock

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestIsThinkingBlockEnvelopeError(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "thinking signature error",
			body: "{\"error\":{\"message\":\"messages.0.content.0: Invalid `signature` in `thinking` block\"}}",
			want: true,
		},
		{
			name: "redacted_thinking data error (Bedrock-style)",
			body: "{\"message\":\"messages.1.content.2: Invalid `data` in `redacted_thinking` block\"}",
			want: true,
		},
		{
			// Future-proofing: the regex is generic over the field
			// name, so a hypothetical new envelope field on the same
			// block types is matched without code changes.
			name: "hypothetical future field on thinking block",
			body: "{\"message\":\"Invalid `encryption_key` in `thinking` block\"}",
			want: true,
		},
		{
			// Stripping thinking blocks would not help for an error
			// on an unrelated block type, so we must not match it.
			name: "unrelated block type (tool_result) is not matched",
			body: "{\"message\":\"Invalid `tool_use_id` in `tool_result` block\"}",
			want: false,
		},
		{
			name: "AWS SigV4 signature mismatch is unrelated",
			body: `{"message":"The request signature we calculated does not match the signature you provided"}`,
			want: false,
		},
		{
			name: "thinking budget validation error is unrelated",
			body: `{"message":"thinking.budget_tokens must be >= 1024"}`,
			want: false,
		},
		{
			name: "without backticks does not match",
			body: `{"message":"signature field is required on thinking blocks"}`,
			want: false,
		},
		{
			name: "empty body",
			body: "",
			want: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsThinkingBlockEnvelopeError([]byte(c.body)); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestStripThinkingBlocks(t *testing.T) {
	cases := []struct {
		name  string
		input string
		check func(t *testing.T, out []byte)
	}{
		{
			name: "removes thinking and redacted_thinking, preserves text and tool_use",
			input: `{"messages":[
				{"role":"assistant","content":[
					{"type":"thinking","thinking":"...","signature":"abc"},
					{"type":"text","text":"hello"},
					{"type":"redacted_thinking","data":"xxx"},
					{"type":"tool_use","id":"t1","name":"x","input":{}}
				]}
			]}`,
			check: func(t *testing.T, out []byte) {
				ac := gjson.GetBytes(out, "messages.0.content")
				if n := ac.Get("#").Int(); n != 2 {
					t.Fatalf("len = %d, want 2", n)
				}
				if got := ac.Get("0.type").String(); got != "text" {
					t.Errorf("[0].type = %q, want text", got)
				}
				if got := ac.Get("1.type").String(); got != "tool_use" {
					t.Errorf("[1].type = %q, want tool_use", got)
				}
			},
		},
		{
			name:  "string content is left untouched",
			input: `{"messages":[{"role":"user","content":"plain"}]}`,
			check: func(t *testing.T, out []byte) {
				if got := gjson.GetBytes(out, "messages.0.content").String(); got != "plain" {
					t.Errorf("string content mangled: %q", got)
				}
			},
		},
		{
			name: "assistant-only-thinking gets placeholder text",
			input: `{"messages":[
				{"role":"assistant","content":[{"type":"thinking","thinking":"t","signature":"s"}]}
			]}`,
			check: func(t *testing.T, out []byte) {
				ac := gjson.GetBytes(out, "messages.0.content")
				if n := ac.Get("#").Int(); n != 1 {
					t.Fatalf("len = %d, want 1 (placeholder)", n)
				}
				if got := ac.Get("0.type").String(); got != "text" {
					t.Errorf("placeholder type = %q, want text", got)
				}
			},
		},
		{
			name: "non-assistant thinking-only message is left empty",
			input: `{"messages":[
				{"role":"user","content":[{"type":"redacted_thinking","data":"x"}]}
			]}`,
			check: func(t *testing.T, out []byte) {
				ac := gjson.GetBytes(out, "messages.0.content")
				if n := ac.Get("#").Int(); n != 0 {
					t.Errorf("len = %d, want 0", n)
				}
			},
		},
		{
			name: "idempotent",
			input: `{"messages":[
				{"role":"assistant","content":[
					{"type":"thinking","thinking":"t","signature":"s"},
					{"type":"text","text":"hi"}
				]}
			]}`,
			check: func(t *testing.T, out []byte) {
				twice := StripThinkingBlocks(out)
				if string(twice) != string(out) {
					t.Errorf("not idempotent: before=%s after=%s", out, twice)
				}
			},
		},
		{
			name:  "no messages field is no-op",
			input: `{"model":"claude-foo"}`,
			check: func(t *testing.T, out []byte) {
				if got := gjson.GetBytes(out, "model").String(); got != "claude-foo" {
					t.Errorf("unrelated mutated: %s", string(out))
				}
			},
		},
		{
			// Exercises the bedrock-adapter fall-through path: matcher
			// fired (e.g. error message mentioned a thinking block on
			// some echoed system content) but the body has nothing to
			// strip, so output equals input and the caller's
			// `bytes.Equal(stripped, requestBody)` guard prevents a
			// pointless retry.
			name: "no thinking blocks present is a no-op",
			input: `{"messages":[
				{"role":"user","content":[{"type":"text","text":"hi"}]},
				{"role":"assistant","content":[{"type":"text","text":"hello"}]}
			]}`,
			check: func(t *testing.T, out []byte) {
				if u := gjson.GetBytes(out, "messages.0.content.0.text").String(); u != "hi" {
					t.Errorf("user text changed: %q", u)
				}
				if a := gjson.GetBytes(out, "messages.1.content.0.text").String(); a != "hello" {
					t.Errorf("assistant text changed: %q", a)
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			input := []byte(c.input)
			orig := append([]byte(nil), input...)
			out := StripThinkingBlocks(input)
			if string(input) != string(orig) {
				t.Errorf("input slice mutated; before=%s after=%s", orig, input)
			}
			c.check(t, out)
		})
	}
}
