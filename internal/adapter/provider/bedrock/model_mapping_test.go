package bedrock

import "testing"

func TestResolveModelIDPriority(t *testing.T) {
	userMapping := map[string]string{
		"claude-opus-4-7": "anthropic.user-override-v1:0",
	}
	discovered := func(name string) (string, bool) {
		switch name {
		case "claude-opus-4-7":
			return "us.anthropic.claude-opus-4-7-20260115-v1:0", true
		case "claude-sonnet-4-9":
			return "us.anthropic.claude-sonnet-4-9-20260201-v1:0", true
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
			name:    "user mapping wins over discovery",
			model:   "claude-opus-4-7",
			mapping: userMapping,
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
			name:   "discovered ID with region prefix is not double-prefixed",
			model:  "claude-opus-4-7",
			lookup: discovered,
			prefix: "us",
			wantID: "us.anthropic.claude-opus-4-7-20260115-v1:0",
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
			model:  "claude-sonnet-4-6",
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
