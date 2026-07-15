package requestmeta

import "testing"

func TestReasoningEffort(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "codex responses", body: `{"reasoning":{"effort":"high"}}`, want: "high"},
		{name: "normalizes whitespace and case", body: `{"reasoning":{"effort":" HIGH "}}`, want: "high"},
		{name: "responses takes precedence", body: `{"reasoning":{"effort":"high"},"reasoning_effort":"low"}`, want: "high"},
		{name: "openai chat", body: `{"reasoning_effort":"xhigh"}`, want: "xhigh"},
		{name: "claude adaptive", body: `{"thinking":{"type":"adaptive"},"output_config":{"effort":"medium"}}`, want: "medium"},
		{name: "claude adaptive default", body: `{"thinking":{"type":"adaptive"}}`, want: "auto"},
		{name: "claude classic automatic", body: `{"thinking":{"type":"enabled","budget_tokens":-1}}`, want: "auto"},
		{name: "claude classic none", body: `{"thinking":{"type":"enabled","budget_tokens":0}}`, want: "none"},
		{name: "claude classic low", body: `{"thinking":{"type":"enabled","budget_tokens":1024}}`, want: "low"},
		{name: "claude classic medium lower bound", body: `{"thinking":{"type":"enabled","budget_tokens":1025}}`, want: "medium"},
		{name: "claude classic medium", body: `{"thinking":{"type":"enabled","budget_tokens":8192}}`, want: "medium"},
		{name: "claude classic high lower bound", body: `{"thinking":{"type":"enabled","budget_tokens":8193}}`, want: "high"},
		{name: "claude classic high", body: `{"thinking":{"type":"enabled","budget_tokens":16000}}`, want: "high"},
		{name: "claude classic missing budget", body: `{"thinking":{"type":"enabled"}}`, want: "auto"},
		{name: "claude classic invalid negative budget", body: `{"thinking":{"type":"enabled","budget_tokens":-2}}`, want: ""},
		{name: "claude classic nonnumeric budget", body: `{"thinking":{"type":"enabled","budget_tokens":"8192"}}`, want: ""},
		{name: "claude disabled", body: `{"thinking":{"type":"disabled"}}`, want: "none"},
		{name: "gemini", body: `{"generationConfig":{"thinkingConfig":{"thinkingLevel":"HIGH"}}}`, want: "high"},
		{name: "gemini snake case", body: `{"generation_config":{"thinking_config":{"thinking_level":"low"}}}`, want: "low"},
		{name: "numeric effort rejected", body: `{"reasoning":{"effort":123}}`, want: ""},
		{name: "invalid value", body: `{"reasoning":{"effort":"high effort"}}`, want: ""},
		{name: "overlong value", body: `{"reasoning":{"effort":"abcdefghijklmnopqrstuvwxyz1234567"}}`, want: ""},
		{name: "unknown thinking type", body: `{"thinking":{"type":"manual"}}`, want: ""},
		{name: "invalid json", body: `{`, want: ""},
		{name: "empty body", body: ``, want: ""},
		{name: "missing", body: `{"model":"gpt-5"}`, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReasoningEffort([]byte(tt.body)); got != tt.want {
				t.Fatalf("ReasoningEffort() = %q, want %q", got, tt.want)
			}
		})
	}
}
