package bedrock

import "testing"

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
