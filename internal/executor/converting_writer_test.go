package executor

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestGetPreferredTargetType(t *testing.T) {
	// OpenRouter's canonical capability set. It natively speaks these three but
	// NOT the Gemini API, so a Gemini client must be converted to one of them.
	openrouter := []domain.ClientType{
		domain.ClientTypeClaude, domain.ClientTypeOpenAI, domain.ClientTypeCodex,
	}

	cases := []struct {
		name         string
		supported    []domain.ClientType
		original     domain.ClientType
		providerType string
		want         domain.ClientType
	}{
		{
			// The bug fix: a Gemini image request routed to OpenRouter must convert
			// via OpenAI (which carries generated images in message.images), not via
			// Claude (Anthropic Messages cannot represent a generated image, so it is
			// silently dropped).
			name:         "gemini source to openrouter prefers openai over claude",
			supported:    openrouter,
			original:     domain.ClientTypeGemini,
			providerType: "openrouter",
			want:         domain.ClientTypeOpenAI,
		},
		{
			// A provider that only speaks Claude still has to use Claude for a Gemini
			// source — the OpenAI preference must not break claude-only providers.
			name:         "gemini source to claude-only provider falls back to claude",
			supported:    []domain.ClientType{domain.ClientTypeClaude},
			original:     domain.ClientTypeGemini,
			providerType: "claude",
			want:         domain.ClientTypeClaude,
		},
		{
			// No conversion when the provider already speaks the client's protocol.
			name:         "gemini source to gemini-capable provider stays gemini",
			supported:    []domain.ClientType{domain.ClientTypeGemini, domain.ClientTypeClaude},
			original:     domain.ClientTypeGemini,
			providerType: "antigravity",
			want:         domain.ClientTypeGemini,
		},
		{
			// The OpenAI-for-Gemini preference must not affect other source clients:
			// a Claude client to OpenRouter still passes through as Claude.
			name:         "claude source to openrouter stays claude",
			supported:    openrouter,
			original:     domain.ClientTypeClaude,
			providerType: "openrouter",
			want:         domain.ClientTypeClaude,
		},
		{
			// A Codex provider still prefers Codex regardless of source.
			name:         "codex provider prefers codex",
			supported:    []domain.ClientType{domain.ClientTypeCodex, domain.ClientTypeOpenAI},
			original:     domain.ClientTypeOpenAI,
			providerType: "codex",
			want:         domain.ClientTypeOpenAI, // originalType supported → no conversion
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := GetPreferredTargetType(tc.supported, tc.original, tc.providerType)
			if got != tc.want {
				t.Fatalf("GetPreferredTargetType(%v, %q, %q) = %q, want %q",
					tc.supported, tc.original, tc.providerType, got, tc.want)
			}
		})
	}
}
