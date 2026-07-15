package requestmeta

import "testing"

func TestReasoningEffort(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "codex responses", body: `{"reasoning":{"effort":"high"}}`, want: "high"},
		{name: "openai chat", body: `{"reasoning_effort":"xhigh"}`, want: "xhigh"},
		{name: "claude adaptive", body: `{"thinking":{"type":"adaptive"},"output_config":{"effort":"medium"}}`, want: "medium"},
		{name: "claude adaptive default", body: `{"thinking":{"type":"adaptive"}}`, want: "auto"},
		{name: "claude classic low", body: `{"thinking":{"type":"enabled","budget_tokens":1024}}`, want: "low"},
		{name: "claude classic medium", body: `{"thinking":{"type":"enabled","budget_tokens":8192}}`, want: "medium"},
		{name: "claude classic high", body: `{"thinking":{"type":"enabled","budget_tokens":16000}}`, want: "high"},
		{name: "claude disabled", body: `{"thinking":{"type":"disabled"}}`, want: "none"},
		{name: "gemini", body: `{"generationConfig":{"thinkingConfig":{"thinkingLevel":"HIGH"}}}`, want: "high"},
		{name: "invalid value", body: `{"reasoning":{"effort":"high effort"}}`, want: ""},
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
