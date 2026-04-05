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
