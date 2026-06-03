package executor

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestMatchWildcard(t *testing.T) {
	tests := []struct {
		pattern string
		input   string
		want    bool
	}{
		// Catch-all
		{"*", "anything", true},
		{"*", "", true},

		// Contains patterns (*xxx*)
		{"*sonnet*", "claude-sonnet-4-20250514", true},
		{"*sonnet*", "claude-3-5-sonnet-20241022", true},
		{"*opus*", "claude-opus-4-20250514", true},
		{"*haiku*", "claude-3-5-haiku-20241022", true},
		{"*claude*", "claude-sonnet-4-20250514", true},
		{"*o1*", "o1", true},
		{"*o1*", "o1-mini", true},
		{"*o1*", "o1-pro", true},
		{"*flash*", "gemini-2.5-flash", true},

		// Prefix patterns (xxx*)
		{"gpt-4*", "gpt-4-turbo", true},
		{"gpt-4*", "gpt-4o", true},
		{"gpt-4o-mini*", "gpt-4o-mini", true},
		{"gpt-4o-mini*", "gpt-4o-mini-2024", true},
		{"claude-*", "claude-sonnet-4", true},

		// Suffix patterns (*xxx)
		{"*-20241022", "claude-3-5-sonnet-20241022", true},
		{"*-turbo", "gpt-4-turbo", true},

		// Exact match (no wildcard)
		{"claude-sonnet-4", "claude-sonnet-4", true},
		{"gpt-4", "gpt-4", true},

		// Non-matches
		{"*sonnet*", "claude-opus-4", false},
		{"*opus*", "claude-sonnet-4", false},
		{"gpt-4*", "gpt-3.5-turbo", false},
		{"claude-sonnet-4", "claude-sonnet-4-20250514", false},
		{"*-20241022", "claude-3-5-sonnet-20250514", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.input, func(t *testing.T) {
			got := domain.MatchWildcard(tt.pattern, tt.input)
			if got != tt.want {
				t.Errorf("MatchWildcard(%q, %q) = %v, want %v", tt.pattern, tt.input, got, tt.want)
			}
		})
	}
}

func TestMatchModelMapping(t *testing.T) {
	mapping := map[string]string{
		"*sonnet*":     "gemini-2.5-pro",
		"*opus*":       "claude-opus-4-6-thinking",
		"*haiku*":      "gemini-2.5-flash-lite",
		"gpt-4o-mini*": "gemini-2.5-flash",
		"gpt-4*":       "gemini-2.5-pro",
		"exact-model":  "exact-target",
	}

	tests := []struct {
		requestModel string
		want         string
	}{
		// Wildcard matches
		{"claude-sonnet-4-20250514", "gemini-2.5-pro"},
		{"claude-3-5-sonnet-20241022", "gemini-2.5-pro"},
		{"claude-opus-4-6", "claude-opus-4-6-thinking"},
		{"claude-opus-4-6-thinking", "claude-opus-4-6-thinking"},
		{"claude-opus-4-20250514", "claude-opus-4-6-thinking"},
		{"claude-3-5-haiku-20241022", "gemini-2.5-flash-lite"},
		{"gpt-4-turbo", "gemini-2.5-pro"},
		{"gpt-4o", "gemini-2.5-pro"},

		// Exact match
		{"exact-model", "exact-target"},

		// No match
		{"unknown-model", ""},
		{"gemini-2.5-pro", ""},
	}

	// Helper function to match model mapping
	matchModelMapping := func(requestModel string, mapping map[string]string) string {
		for pattern, target := range mapping {
			if domain.MatchWildcard(pattern, requestModel) {
				return target
			}
		}
		return ""
	}

	for _, tt := range tests {
		t.Run(tt.requestModel, func(t *testing.T) {
			got := matchModelMapping(tt.requestModel, mapping)
			if got != tt.want {
				t.Errorf("matchModelMapping(%q) = %q, want %q", tt.requestModel, got, tt.want)
			}
		})
	}
}
