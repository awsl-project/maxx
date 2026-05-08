package responsemodifier

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func claudeProvider(mapping map[string]string) *domain.Provider {
	return &domain.Provider{Type: "claude", Config: &domain.ProviderConfig{Claude: &domain.ProviderConfigClaude{ResponseModelMapping: mapping}}}
}

func TestMapModel(t *testing.T) {
	mapping := map[string]string{"*": "fallback", "claude-*": "claude-alias", "bad": "client-*", "empty": " "}
	cases := map[string]string{
		"claude-sonnet-4": "claude-alias",
		"gpt-5":           "fallback",
		"bad":             "fallback",
		"empty":           "fallback",
	}
	modifier := &claudeResponseModifier{mapping: mapping}
	for model, want := range cases {
		if got := modifier.mapModel(model); got != want {
			t.Fatalf("mapModel(%q) = %q, want %q", model, got, want)
		}
	}
}

func TestSortedMappingPatternsUsesDeterministicTieBreaker(t *testing.T) {
	mapping := map[string]string{
		"claude-3-*": "older",
		"claude-4-*": "newer",
		"claude-*":   "fallback",
		"*":          "global",
	}
	got := sortedMappingPatterns(mapping)
	want := []string{"claude-3-*", "claude-4-*", "claude-*", "*"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("sortedMappingPatterns() = %v, want %v", got, want)
	}
}

func TestResponseModifierWriterModifiesNonStreamingClaudeResponse(t *testing.T) {
	rr := httptest.NewRecorder()
	writer := NewResponseModifierWriter(rr, claudeProvider(map[string]string{"upstream": "alias", "nested": "nested-alias"}), domain.ClientTypeClaude, false)
	if writer == nil {
		t.Fatal("expected writer")
	}
	writer.Header().Set("Content-Length", "20")
	writer.WriteHeader(200)
	_, _ = writer.Write([]byte(`{"model":"upstream","message":{"model":"nested"},"text":"<b>hi</b>"}`))
	if err := writer.Finalize(); err != nil {
		t.Fatalf("finalize failed: %v", err)
	}
	got := rr.Body.String()
	for _, want := range []string{`"model":"alias"`, `"message":{"model":"nested-alias"}`, `<b>hi</b>`} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %s in %s", want, got)
		}
	}
	if rr.Header().Get("Content-Length") != strconv.Itoa(len(got)) {
		t.Fatalf("unexpected Content-Length: %s", rr.Header().Get("Content-Length"))
	}
}

func TestResponseModifierWriterModifiesStreamingClaudeResponse(t *testing.T) {
	rr := httptest.NewRecorder()
	writer := NewResponseModifierWriter(rr, claudeProvider(map[string]string{"upstream": "alias", "nested": "nested-alias"}), domain.ClientTypeClaude, true)
	if writer == nil {
		t.Fatal("expected writer")
	}
	_, _ = writer.Write([]byte("event: message\ndata: {\"model\":"))
	if got := rr.Body.String(); got != "" {
		t.Fatalf("expected incomplete SSE event to stay buffered, got %s", got)
	}
	_, _ = writer.Write([]byte("\"upstream\"}\n\n"))
	if got := rr.Body.String(); !strings.Contains(got, `data: {"model":"alias"}`) {
		t.Fatalf("expected first complete SSE event to flush immediately, got %s", got)
	}
	if !rr.Flushed {
		t.Fatal("expected complete SSE event to flush before finalize")
	}
	_, _ = writer.Write([]byte("data: {\"message\":{\"model\":\"nested\"}}\n\ndata: [DONE]\n"))
	if err := writer.Finalize(); err != nil {
		t.Fatalf("finalize failed: %v", err)
	}
	got := rr.Body.String()
	for _, want := range []string{`data: {"model":"alias"}`, `data: {"message":{"model":"nested-alias"}}`, "data: [DONE]"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %s in %s", want, got)
		}
	}
}

func TestResponseModifierWriterLeavesMalformedStreamEventUnchanged(t *testing.T) {
	rr := httptest.NewRecorder()
	writer := NewResponseModifierWriter(rr, claudeProvider(map[string]string{"upstream": "alias"}), domain.ClientTypeClaude, true)
	if writer == nil {
		t.Fatal("expected writer")
	}
	event := "data: {\"model\":\"upstream\"\n\n"
	_, _ = writer.Write([]byte(event))
	if err := writer.Finalize(); err != nil {
		t.Fatalf("finalize failed: %v", err)
	}
	if got := rr.Body.String(); got != event {
		t.Fatalf("expected malformed SSE event to pass through unchanged, got %s", got)
	}
}

func TestResponseModifierWriterDisabled(t *testing.T) {
	rr := httptest.NewRecorder()
	if NewResponseModifierWriter(rr, &domain.Provider{Type: "codex"}, domain.ClientTypeCodex, false) != nil {
		t.Fatal("expected non-claude provider to be disabled")
	}
	if NewResponseModifierWriter(rr, claudeProvider(nil), domain.ClientTypeClaude, false) != nil {
		t.Fatal("expected empty mapping to be disabled")
	}
	if NewResponseModifierWriter(rr, claudeProvider(map[string]string{"a": "b"}), domain.ClientTypeCodex, false) != nil {
		t.Fatal("expected non-claude client shape to be disabled")
	}
}
