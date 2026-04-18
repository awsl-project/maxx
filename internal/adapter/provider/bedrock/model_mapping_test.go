package bedrock

import "testing"

func TestResolveModelIDPriority(t *testing.T) {
	// Canonical dated+versioned anthropic value — gets the region prefix
	// attached to become a valid inference profile ID.
	userMapping := map[string]string{
		"claude-opus-4-7": "anthropic.claude-override-20260101-v1:0",
	}
	discovered := func(name string) (string, bool) {
		switch name {
		case "claude-opus-4-7":
			return "us.anthropic.claude-opus-4-7-20260115-v1:0", true
		case "claude-sonnet-4-9":
			return "us.anthropic.claude-sonnet-4-9-20260201-v1:0", true
		// Dated name indexed from a real Bedrock profile that uses v2:0.
		// resolveModelID must prefer this over the auto-derive fallback
		// that would otherwise synthesize an incorrect -v1:0 suffix.
		case "claude-3-5-sonnet-20241022":
			return "us.anthropic.claude-3-5-sonnet-20241022-v2:0", true
		// Foundation-model-only release (e.g. AWS's initial ship of 4.6).
		// resolveModelID must return the bare foundation model ID with
		// no region prefix added — prepending "us." produces an invalid
		// Bedrock target.
		case "claude-sonnet-4-6":
			return "anthropic.claude-sonnet-4-6", true
		case "claude-opus-4-6":
			return "anthropic.claude-opus-4-6-v1", true
		}
		return "", false
	}

	cases := []struct {
		name    string
		model   string
		mapping map[string]string
		lookup  discoveredLookup
		prefix  string
		wantID  string
		wantOK  bool
	}{
		{
			name:    "user mapping wins over discovery and gets prefixed when dated+versioned",
			model:   "claude-opus-4-7",
			mapping: userMapping,
			lookup:  discovered,
			prefix:  "us",
			wantID:  "us.anthropic.claude-override-20260101-v1:0",
			wantOK:  true,
		},
		{
			name:    "user mapping with bare foundation-model ID is not prefixed",
			model:   "claude-sonnet-4-6",
			mapping: map[string]string{"claude-sonnet-4-6": "anthropic.claude-sonnet-4-6"},
			lookup:  nil,
			prefix:  "us",
			wantID:  "anthropic.claude-sonnet-4-6",
			wantOK:  true,
		},
		{
			name:    "user mapping with foundation model + -v1 suffix is not prefixed",
			model:   "claude-opus-4-6",
			mapping: map[string]string{"claude-opus-4-6": "anthropic.claude-opus-4-6-v1"},
			lookup:  nil,
			prefix:  "us",
			wantID:  "anthropic.claude-opus-4-6-v1",
			wantOK:  true,
		},
		{
			name:    "user mapping already carrying region prefix is not double-prefixed",
			model:   "claude-opus-4-7",
			mapping: map[string]string{"claude-opus-4-7": "us.anthropic.user-override-v1:0"},
			lookup:  discovered,
			prefix:  "us",
			wantID:  "us.anthropic.user-override-v1:0",
			wantOK:  true,
		},
		{
			name:   "discovery resolves brand-new model",
			model:  "claude-sonnet-4-9",
			lookup: discovered,
			prefix: "us",
			wantID: "us.anthropic.claude-sonnet-4-9-20260201-v1:0",
			wantOK: true,
		},
		{
			name:   "discovered ID is returned verbatim (no re-prefixing)",
			model:  "claude-opus-4-7",
			lookup: discovered,
			prefix: "us",
			wantID: "us.anthropic.claude-opus-4-7-20260115-v1:0",
			wantOK: true,
		},
		{
			name:   "foundation-model discovery hit is not region-prefixed",
			model:  "claude-sonnet-4-6",
			lookup: discovered,
			prefix: "us",
			wantID: "anthropic.claude-sonnet-4-6",
			wantOK: true,
		},
		{
			name:   "foundation-model with -v1 suffix is preserved verbatim",
			model:  "claude-opus-4-6",
			lookup: discovered,
			prefix: "us",
			wantID: "anthropic.claude-opus-4-6-v1",
			wantOK: true,
		},
		{
			name:   "client-supplied dated name auto-derives",
			model:  "claude-haiku-4-5-20251001",
			lookup: discovered,
			prefix: "us",
			wantID: "us.anthropic.claude-haiku-4-5-20251001-v1:0",
			wantOK: true,
		},
		{
			name:   "dated name prefers discovery v2:0 over auto-derived v1:0",
			model:  "claude-3-5-sonnet-20241022",
			lookup: discovered,
			prefix: "us",
			wantID: "us.anthropic.claude-3-5-sonnet-20241022-v2:0",
			wantOK: true,
		},
		{
			// Non-anthropic IDs (e.g. Amazon Nova) are valid Bedrock targets
			// without a region prefix; our adapter only supports the Claude
			// client type, but if an operator uses a user mapping to reach
			// one we must not mangle it by prepending "us.".
			name:    "non-anthropic user mapping passes through unprefixed",
			model:   "custom-model",
			mapping: map[string]string{"custom-model": "amazon.nova-pro-v1:0"},
			lookup:  nil,
			prefix:  "us",
			wantID:  "amazon.nova-pro-v1:0",
			wantOK:  true,
		},
		{
			// A client that already knows the Bedrock foundation-model
			// shape can send it verbatim; applyPrefix must leave it alone.
			name:   "client-supplied bare foundation-model ID passes through unprefixed",
			model:  "anthropic.claude-sonnet-4-6",
			lookup: nil,
			prefix: "us",
			wantID: "anthropic.claude-sonnet-4-6",
			wantOK: true,
		},
		{
			name:   "client-supplied fully-qualified bedrock ID passes through",
			model:  "anthropic.claude-opus-4-5-20251101-v1:0",
			lookup: discovered,
			prefix: "us",
			wantID: "us.anthropic.claude-opus-4-5-20251101-v1:0",
			wantOK: true,
		},
		{
			name:   "region-prefixed bedrock ID is not double-prefixed",
			model:  "eu.anthropic.claude-sonnet-4-5-20250929-v1:0",
			lookup: discovered,
			prefix: "us",
			wantID: "eu.anthropic.claude-sonnet-4-5-20250929-v1:0",
			wantOK: true,
		},
		{
			name:   "bare short name with discovery miss is unresolvable",
			model:  "claude-unreleased-99",
			lookup: discovered,
			prefix: "us",
			wantOK: false,
		},
		{
			name:   "bare short name with no discoverer is unresolvable",
			model:  "claude-opus-4-6",
			lookup: nil,
			prefix: "us",
			wantOK: false,
		},
		{
			name:   "garbage model name is unresolvable",
			model:  "gpt-4",
			lookup: nil,
			prefix: "us",
			wantOK: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := resolveModelID(c.model, c.mapping, c.prefix, c.lookup)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v (got id=%q)", ok, c.wantOK, got)
			}
			if ok && got != c.wantID {
				t.Errorf("id = %q, want %q", got, c.wantID)
			}
		})
	}
}
