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
			// Client sends "anthropic.<short>" for a model that Bedrock
			// refuses on-demand (no foundation SKU — must invoke via an
			// inference profile). Discovery knows the profile under the
			// short name, so stripping "anthropic." and consulting
			// discovery lets us resolve to an invoke-ready profile ID
			// instead of letting the bare foundation ID hit Bedrock and
			// fail with "on-demand throughput isn't supported".
			name:   "client-supplied bare anthropic.X resolves via discovery",
			model:  "anthropic.claude-sonnet-4-9",
			lookup: discovered,
			prefix: "us",
			wantID: "us.anthropic.claude-sonnet-4-9-20260201-v1:0",
			wantOK: true,
		},
		{
			// Discovery-miss on the stripped short name must fall back to
			// the original passthrough, not synthesize a profile ID.
			name:   "client-supplied bare anthropic.X with discovery miss falls back to passthrough",
			model:  "anthropic.claude-unreleased-99",
			lookup: discovered,
			prefix: "us",
			wantID: "anthropic.claude-unreleased-99",
			wantOK: true,
		},
		{
			// Foundation-only release: client sends "anthropic.<short>"
			// and discovery returns a bare foundation ID (no region
			// prefix). The invoke-ready value from discovery must be
			// returned verbatim — applyPrefix must not be re-applied on
			// top of it, since region-prefixing a foundation model ID
			// makes it invalid.
			name:   "client-supplied bare anthropic.X with foundation-only discovery hit returns verbatim",
			model:  "anthropic.claude-sonnet-4-6",
			lookup: discovered,
			prefix: "us",
			wantID: "anthropic.claude-sonnet-4-6",
			wantOK: true,
		},
		{
			// Degenerate input "anthropic." (empty short after strip)
			// must not query discovery — an empty-string lookup key is
			// meaningless and a future discoverer change could surprise.
			// Falls through to the existing passthrough contract.
			name:   "degenerate anthropic. passes through without querying discovery",
			model:  "anthropic.",
			lookup: func(name string) (string, bool) {
				if name == "" {
					t.Fatalf("discovery must not be called with empty key")
				}
				return "", false
			},
			prefix: "us",
			wantID: "anthropic.",
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

func TestDegradeCandidates(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"claude-sonnet-4-6", []string{"claude-sonnet-4-5", "claude-sonnet-4-4", "claude-sonnet-4-3", "claude-sonnet-4-2", "claude-sonnet-4-1", "claude-sonnet-4-0", "claude-sonnet-4"}},
		{"claude-opus-4-1", []string{"claude-opus-4-0", "claude-opus-4"}},
		{"claude-opus-4-0", []string{"claude-opus-4"}},
		// No minor: nothing to degrade to; we never downshift across majors.
		{"claude-sonnet-4", nil},
		// Old-style "claude-3-5-sonnet": version before family, no degrade.
		{"claude-3-5-sonnet", nil},
		// Dated name: authoritative, no degrade.
		{"claude-sonnet-4-5-20250929", nil},
		{"gpt-4-1", nil},
	}
	for _, c := range cases {
		got := degradeCandidates(c.in)
		if len(got) != len(c.want) {
			t.Errorf("degradeCandidates(%q) = %v; want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("degradeCandidates(%q)[%d] = %q; want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

func TestResolveModelIDDegradesOnDiscoveryMiss(t *testing.T) {
	// Only claude-sonnet-4-5 and claude-opus-4 are discoverable; requests
	// for newer minor versions must degrade to the nearest available.
	lookup := func(name string) (string, bool) {
		switch name {
		case "claude-sonnet-4-5":
			return "us.anthropic.claude-sonnet-4-5-20250929-v1:0", true
		case "claude-opus-4":
			return "us.anthropic.claude-opus-4-20250514-v1:0", true
		}
		return "", false
	}

	cases := []struct {
		name   string
		model  string
		wantID string
	}{
		{"sonnet-4-6 degrades to sonnet-4-5", "claude-sonnet-4-6", "us.anthropic.claude-sonnet-4-5-20250929-v1:0"},
		{"sonnet-4-7 degrades past missing 4-6 to 4-5", "claude-sonnet-4-7", "us.anthropic.claude-sonnet-4-5-20250929-v1:0"},
		{"opus-4-6 degrades all the way to bare opus-4", "claude-opus-4-6", "us.anthropic.claude-opus-4-20250514-v1:0"},
		{"bare anthropic.<short> also degrades", "anthropic.claude-sonnet-4-6", "us.anthropic.claude-sonnet-4-5-20250929-v1:0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := resolveModelID(c.model, nil, "us", lookup)
			if !ok || got != c.wantID {
				t.Errorf("got (%q,%v); want (%q,true)", got, ok, c.wantID)
			}
		})
	}
}
