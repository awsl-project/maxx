package error_fixer

import (
	"net/http"
	"testing"

	"github.com/awsl-project/maxx/internal/adapter/provider/bedrock"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/tidwall/gjson"
)

func TestThinkingEnvelopeFixer_MatchResponse(t *testing.T) {
	f := &thinkingEnvelopeFixer{}

	cases := []struct {
		name       string
		status     int
		nilResp    bool
		body       string
		clientType domain.ClientType
		want       bool
	}{
		{
			name:       "match on thinking signature error",
			status:     400,
			body:       "{\"error\":{\"message\":\"messages.0.content.0: Invalid `signature` in `thinking` block (request id: 202604230718)\"}}",
			clientType: domain.ClientTypeClaude,
			want:       true,
		},
		{
			name:       "match on redacted_thinking data error",
			status:     400,
			body:       "{\"message\":\"messages.1.content.2: Invalid `data` in `redacted_thinking` block\"}",
			clientType: domain.ClientTypeClaude,
			want:       true,
		},
		{
			// The whole point of the regex over literal phrases:
			// hypothetical future fields on the same block types are
			// covered without code changes.
			name:       "match on hypothetical future field on thinking block",
			status:     400,
			body:       "{\"message\":\"Invalid `encryption_key` in `thinking` block\"}",
			clientType: domain.ClientTypeClaude,
			want:       true,
		},
		{
			// Stripping thinking blocks would not help for an error on
			// an unrelated block type, so we must not match it.
			name:       "no match on unrelated block type (tool_result)",
			status:     400,
			body:       "{\"message\":\"Invalid `tool_use_id` in `tool_result` block\"}",
			clientType: domain.ClientTypeClaude,
			want:       false,
		},
		{
			name:       "no match on AWS SigV4 signature error",
			status:     400,
			body:       `{"error":"The request signature we calculated does not match the signature you provided"}`,
			clientType: domain.ClientTypeClaude,
			want:       false,
		},
		{
			name:       "no match on thinking budget validation error",
			status:     400,
			body:       `{"error":"thinking.budget_tokens must be >= 1024"}`,
			clientType: domain.ClientTypeClaude,
			want:       false,
		},
		{
			name:       "no match without backticks",
			status:     400,
			body:       `{"error":"signature field is required on thinking blocks"}`,
			clientType: domain.ClientTypeClaude,
			want:       false,
		},
		{
			name:       "no match on wrong status",
			status:     500,
			body:       "Invalid `signature` in `thinking` block",
			clientType: domain.ClientTypeClaude,
			want:       false,
		},
		{
			name:       "no match on non-Claude client",
			status:     400,
			body:       "Invalid `signature` in `thinking` block",
			clientType: domain.ClientTypeGemini,
			want:       false,
		},
		{
			name:       "match on nil response (SSE error path)",
			nilResp:    true,
			body:       "event: error\ndata: {\"message\":\"Invalid `data` in `redacted_thinking` block\"}",
			clientType: domain.ClientTypeClaude,
			want:       true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var resp *http.Response
			if !c.nilResp {
				resp = &http.Response{StatusCode: c.status}
			}
			if got := f.MatchResponse(resp, []byte(c.body), c.clientType); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestThinkingEnvelopeFixer_FixRequest(t *testing.T) {
	f := &thinkingEnvelopeFixer{}
	req := &http.Request{}
	input := []byte(`{"messages":[
		{"role":"assistant","content":[
			{"type":"thinking","thinking":"...","signature":"abc"},
			{"type":"redacted_thinking","data":"opaque"},
			{"type":"text","text":"hi"}
		]}
	]}`)
	orig := make([]byte, len(input))
	copy(orig, input)

	gotReq, gotBody := f.FixRequest(req, input)
	if gotReq != req {
		t.Errorf("FixRequest should return the same request object")
	}
	if string(input) != string(orig) {
		t.Errorf("input slice was mutated: before=%s after=%s", orig, input)
	}
	if gjson.GetBytes(gotBody, "messages.0.content.#").Int() != 1 {
		t.Errorf("expected both thinking blocks stripped, got %s", gotBody)
	}
	if got := gjson.GetBytes(gotBody, "messages.0.content.0.type").String(); got != "text" {
		t.Errorf("remaining block type = %q, want text", got)
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
				]},
				{"role":"user","content":[
					{"type":"tool_result","tool_use_id":"t1","content":"ok"}
				]}
			]}`,
			check: func(t *testing.T, out []byte) {
				ac := gjson.GetBytes(out, "messages.0.content")
				if n := ac.Get("#").Int(); n != 2 {
					t.Fatalf("assistant content len = %d, want 2 (text+tool_use)", n)
				}
				if got := ac.Get("0.type").String(); got != "text" {
					t.Errorf("kept content[0].type = %q, want text", got)
				}
				if got := ac.Get("1.type").String(); got != "tool_use" {
					t.Errorf("kept content[1].type = %q, want tool_use", got)
				}
				uc := gjson.GetBytes(out, "messages.1.content")
				if n := uc.Get("#").Int(); n != 1 {
					t.Errorf("user content len = %d, want 1 (unchanged)", n)
				}
			},
		},
		{
			name: "string content is left untouched",
			input: `{"messages":[
				{"role":"user","content":"plain text"}
			]}`,
			check: func(t *testing.T, out []byte) {
				if got := gjson.GetBytes(out, "messages.0.content").String(); got != "plain text" {
					t.Errorf("string content mangled: %q", got)
				}
			},
		},
		{
			// Dropping to content:[] would retry-fail ("at least one
			// block required") and trigger an endless fix-retry loop;
			// dropping the whole message would merge the surrounding
			// user turns. Placeholder text keeps the retry valid.
			name: "assistant message of only thinking gets placeholder text",
			input: `{"messages":[
				{"role":"user","content":[{"type":"text","text":"hi"}]},
				{"role":"assistant","content":[
					{"type":"thinking","thinking":"...","signature":"abc"}
				]},
				{"role":"user","content":[{"type":"text","text":"next"}]}
			]}`,
			check: func(t *testing.T, out []byte) {
				ac := gjson.GetBytes(out, "messages.1.content")
				if n := ac.Get("#").Int(); n != 1 {
					t.Fatalf("assistant content len = %d, want 1 (placeholder)", n)
				}
				if got := ac.Get("0.type").String(); got != "text" {
					t.Errorf("placeholder type = %q, want text", got)
				}
				if got := ac.Get("0.text").String(); got == "" {
					t.Errorf("placeholder text must be non-empty")
				}
				if got := gjson.GetBytes(out, "messages.0.content.0.text").String(); got != "hi" {
					t.Errorf("preceding user turn mangled: %q", got)
				}
				if got := gjson.GetBytes(out, "messages.2.content.0.text").String(); got != "next" {
					t.Errorf("following user turn mangled: %q", got)
				}
			},
		},
		{
			// Non-assistant roles never legitimately carry thinking
			// blocks, but if one slips through we leave the empty
			// array rather than fabricate content for a role that
			// did not produce it.
			name: "non-assistant empty content after strip is left empty",
			input: `{"messages":[
				{"role":"user","content":[
					{"type":"redacted_thinking","data":"xxx"}
				]}
			]}`,
			check: func(t *testing.T, out []byte) {
				ac := gjson.GetBytes(out, "messages.0.content")
				if !ac.IsArray() {
					t.Fatalf("expected array, got %v", ac.Type)
				}
				if n := ac.Get("#").Int(); n != 0 {
					t.Errorf("expected empty array for non-assistant, got len %d", n)
				}
			},
		},
		{
			// Running the fix twice must be a no-op on the second
			// pass — otherwise a retry loop with no new thinking to
			// strip would keep re-triggering this fixer forever.
			name: "idempotent on already-stripped body",
			input: `{"messages":[
				{"role":"assistant","content":[
					{"type":"thinking","thinking":"t","signature":"s"},
					{"type":"text","text":"hello"}
				]}
			]}`,
			check: func(t *testing.T, out []byte) {
				twice := bedrock.StripThinkingBlocks(out)
				if string(twice) != string(out) {
					t.Errorf("second strip must be a no-op; before=%s after=%s", out, twice)
				}
			},
		},
		{
			name:  "no messages field is a no-op",
			input: `{"model":"claude-foo"}`,
			check: func(t *testing.T, out []byte) {
				if got := gjson.GetBytes(out, "model").String(); got != "claude-foo" {
					t.Errorf("unrelated fields mutated: %s", string(out))
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := bedrock.StripThinkingBlocks([]byte(c.input))
			c.check(t, out)
		})
	}
}
