package error_fixer

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestFindFixers_BasicMatch(t *testing.T) {
	fixers := FindFixers(
		&http.Response{StatusCode: 400},
		[]byte(`cache_control not permitted`),
		domain.ClientTypeClaude,
	)
	if len(fixers) == 0 {
		t.Fatal("expected to find fixers")
	}
	if fixers[0].Name() != "cache_control" {
		t.Errorf("expected cache_control fixer, got %s", fixers[0].Name())
	}
}

func TestFindFixers_NoMatch(t *testing.T) {
	fixers := FindFixers(&http.Response{StatusCode: 200}, []byte(`ok`), domain.ClientTypeClaude)
	if len(fixers) != 0 {
		t.Error("expected no fixers for 200")
	}
}

func TestFindFixers_PriorityExclusive(t *testing.T) {
	// Bedrock error contains "InvokeModel" AND "cache_control"
	// Bedrock fixer (P0) should match; cache_control fixer (P100) should be excluded
	fixers := FindFixers(
		&http.Response{StatusCode: 400},
		[]byte(`InvokeModel: Bedrock Runtime error, cache_control: Extra inputs are not permitted`),
		domain.ClientTypeClaude,
	)
	if len(fixers) == 0 {
		t.Fatal("expected at least one fixer")
	}
	for _, f := range fixers {
		if f.Priority() > 0 {
			t.Errorf("priority-100 fixer %q should be excluded when bedrock (P0) matches", f.Name())
		}
	}
	if fixers[0].Name() != "bedrock" {
		t.Errorf("expected bedrock fixer first, got %s", fixers[0].Name())
	}
}

func TestFindFixers_BedrockDefersToThinkingEnvelope(t *testing.T) {
	// Real-world Bedrock SDK errors wrap the Anthropic validator's
	// envelope rejection in their own error chain. bedrockFixer (P0)
	// sees the "Bedrock Runtime"/"InvokeModel" markers, but must
	// decline so the priority-100 thinking_envelope fixer can run —
	// its narrow strip is what unblocks the request. Without the
	// opt-out, priority-exclusive matching would let bedrockFixer
	// strip unrelated fields, leave the offending thinking blocks
	// in place, and the next round-trip would fail the same way.
	cases := []struct {
		name string
		body string
	}{
		{
			name: "redacted_thinking data variant",
			body: "{\"error\":{\"message\":\"operation error Bedrock Runtime: InvokeModel, ValidationException: messages.0.content.0: Invalid `data` in `redacted_thinking` block\"}}",
		},
		{
			name: "thinking signature variant",
			body: "{\"error\":{\"message\":\"operation error Bedrock Runtime: InvokeModelWithResponseStream, ValidationException: Invalid `signature` in `thinking` block\"}}",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fixers := FindFixers(&http.Response{StatusCode: 400}, []byte(c.body), domain.ClientTypeClaude)
			if len(fixers) == 0 {
				t.Fatal("expected at least one fixer to match")
			}
			for _, f := range fixers {
				if f.Name() == "bedrock" {
					t.Errorf("bedrock fixer should defer to thinking_envelope; got %v", fixerNames(fixers))
				}
			}
			found := false
			for _, f := range fixers {
				if f.Name() == "thinking_envelope" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected thinking_envelope fixer to handle envelope rejection; got %v", fixerNames(fixers))
			}
		})
	}
}

func fixerNames(fixers []ErrorFixer) []string {
	names := make([]string, 0, len(fixers))
	for _, f := range fixers {
		names = append(names, f.Name())
	}
	return names
}

func TestFindFixers_SamePriorityMultiple(t *testing.T) {
	// Non-Bedrock error with both cache_control and beta header issues
	// Both are P100, both should match
	fixers := FindFixers(
		&http.Response{StatusCode: 400},
		[]byte(`cache_control rejected; anthropic-beta header invalid`),
		domain.ClientTypeClaude,
	)
	names := make(map[string]bool)
	for _, f := range fixers {
		names[f.Name()] = true
	}
	if !names["cache_control"] {
		t.Error("expected cache_control fixer")
	}
	if !names["beta_header"] {
		t.Error("expected beta_header fixer")
	}
}

func TestFindFixers_PriorityOrdering(t *testing.T) {
	// Verify returned fixers are sorted by priority
	fixers := FindFixers(
		&http.Response{StatusCode: 400},
		[]byte(`cache_control rejected`),
		domain.ClientTypeClaude,
	)
	for i := 1; i < len(fixers); i++ {
		if fixers[i].Priority() < fixers[i-1].Priority() {
			t.Errorf("fixers not sorted: [%d]=%s(P%d) before [%d]=%s(P%d)",
				i-1, fixers[i-1].Name(), fixers[i-1].Priority(),
				i, fixers[i].Name(), fixers[i].Priority())
		}
	}
}

func TestBedrockFixer_FixRequest_NoInputMutation(t *testing.T) {
	f := &bedrockFixer{}
	input := []byte(`{"system":[{"type":"text","text":"hi","cache_control":{"type":"ephemeral"}}],"output_config":{"effort":"high"}}`)
	original := make([]byte, len(input))
	copy(original, input)

	req, _ := http.NewRequest("POST", "https://example.com", nil)
	f.FixRequest(req, input)

	if !bytes.Equal(original, input) {
		t.Error("FixRequest mutated the input slice")
	}
}

func TestBedrockFixer_FixRequest_EmptyBody(t *testing.T) {
	f := &bedrockFixer{}
	req, _ := http.NewRequest("POST", "https://example.com", nil)

	_, result := f.FixRequest(req, []byte(`{}`))
	if string(result) != "{}" {
		t.Errorf("expected empty object, got %s", string(result))
	}
}

func TestBedrockFixer_FixRequest_MalformedJSON(t *testing.T) {
	f := &bedrockFixer{}
	req, _ := http.NewRequest("POST", "https://example.com", nil)
	malformed := []byte(`not json at all`)

	// Should not panic
	_, result := f.FixRequest(req, malformed)
	if string(result) != string(malformed) {
		t.Errorf("expected passthrough of malformed input, got %s", string(result))
	}
}
