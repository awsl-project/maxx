package openrouter

import "testing"

func TestNormalizeModelSlug(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// Anthropic dash-versions are the case OpenRouter cannot auto-resolve.
		{"claude-sonnet-4-6", "anthropic/claude-sonnet-4.6"},
		{"claude-opus-4-1", "anthropic/claude-opus-4.1"},
		{"claude-haiku-4-5", "anthropic/claude-haiku-4.5"},
		{"claude-sonnet-4", "anthropic/claude-sonnet-4"},
		{"claude-opus-5", "anthropic/claude-opus-5"},
		{"claude-3-5-sonnet", "anthropic/claude-3.5-sonnet"},
		{"claude-sonnet-4-6-20250514", "anthropic/claude-sonnet-4.6"},
		// OpenAI / Google / xAI: prefix (dot versions already correct).
		{"gpt-4o-mini", "openai/gpt-4o-mini"},
		{"gpt-5.5", "openai/gpt-5.5"},
		{"gpt-5.6-luna", "openai/gpt-5.6-luna"},
		{"gpt-5.3-codex", "openai/gpt-5.3-codex"},
		{"gemini-3.5-flash", "google/gemini-3.5-flash"},
		{"gemini-3-pro-image", "google/gemini-3-pro-image"},
		{"grok-4.6", "x-ai/grok-4.6"},
		// Non-Anthropic digit-dash-digit runs are NOT version separators and must
		// survive prefixing unchanged (regression: the dash→dot rewrite is
		// Anthropic-scoped). These are all real OpenRouter ids.
		{"gpt-4-32k", "openai/gpt-4-32k"},
		{"gpt-4-0613", "openai/gpt-4-0613"},
		{"qwen3-8b", "qwen/qwen3-8b"},
		{"qwen3-235b-a22b-instruct", "qwen/qwen3-235b-a22b-instruct"},
		{"gemma-3-4b-it", "google/gemma-3-4b-it"},
		{"llama-3-70b", "meta-llama/llama-3-70b"},
		{"ministral-8b", "mistralai/ministral-8b"},
		// Explicit slugs and unknown vendors pass through untouched.
		{"anthropic/claude-sonnet-4.6", "anthropic/claude-sonnet-4.6"},
		{"openai/gpt-5.5", "openai/gpt-5.5"},
		{"some-unknown-model", "some-unknown-model"},
		{"", ""},
		{"  claude-sonnet-4-6  ", "anthropic/claude-sonnet-4.6"},
	}
	for _, tc := range cases {
		if got := normalizeModelSlug(tc.in); got != tc.want {
			t.Errorf("normalizeModelSlug(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Normalizing the adapter's own output must be a no-op.
func TestNormalizeModelSlugIdempotent(t *testing.T) {
	for _, in := range []string{"claude-sonnet-4-6", "gpt-5.5", "grok-4.6", "unknown"} {
		once := normalizeModelSlug(in)
		if twice := normalizeModelSlug(once); twice != once {
			t.Errorf("not idempotent for %q: %q -> %q", in, once, twice)
		}
	}
}
