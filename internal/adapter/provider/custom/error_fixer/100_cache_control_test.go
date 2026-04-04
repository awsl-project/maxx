package error_fixer

import (
	"net/http"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/tidwall/gjson"
)

func TestCacheControlFixer_MatchResponse(t *testing.T) {
	f := &cacheControlFixer{}

	// Should match: 400 + claude + cache_control in body
	if !f.MatchResponse(
		&http.Response{StatusCode: 400},
		[]byte(`{"error":{"message":"cache_control: Extra inputs"}}`),
		domain.ClientTypeClaude,
	) {
		t.Error("expected match for 400 with cache_control error")
	}

	// Should not match: wrong status
	if f.MatchResponse(
		&http.Response{StatusCode: 500},
		[]byte(`cache_control`),
		domain.ClientTypeClaude,
	) {
		t.Error("should not match non-400 status")
	}

	// Should not match: wrong client type
	if f.MatchResponse(
		&http.Response{StatusCode: 400},
		[]byte(`cache_control`),
		domain.ClientTypeGemini,
	) {
		t.Error("should not match non-Claude client")
	}

	// Should not match: no cache_control in body
	if f.MatchResponse(
		&http.Response{StatusCode: 400},
		[]byte(`{"error":"bad request"}`),
		domain.ClientTypeClaude,
	) {
		t.Error("should not match when body has no cache_control")
	}

	// Should not match: nil response (SSE error)
	if f.MatchResponse(nil, []byte(`cache_control`), domain.ClientTypeClaude) {
		t.Error("should not match nil response")
	}
}

func TestCacheControlFixer_FixRequest(t *testing.T) {
	f := &cacheControlFixer{}

	input := []byte(`{
		"system": [{"type":"text","text":"hello","cache_control":{"type":"ephemeral"}}],
		"tools": [{"name":"t1","cache_control":{"type":"ephemeral"}}],
		"messages": [{"role":"user","content":[{"type":"text","text":"hi","cache_control":{"type":"ephemeral"}}]}]
	}`)

	req, _ := http.NewRequest("POST", "https://example.com", nil)
	retReq, result := f.FixRequest(req, input)

	// Request should be returned as-is (no header/URL changes)
	if retReq.URL.String() != "https://example.com" {
		t.Error("URL was modified unexpectedly")
	}

	// Verify all cache_control removed
	if gjson.GetBytes(result, "system.0.cache_control").Exists() {
		t.Error("cache_control not stripped from system")
	}
	if gjson.GetBytes(result, "tools.0.cache_control").Exists() {
		t.Error("cache_control not stripped from tools")
	}
	if gjson.GetBytes(result, "messages.0.content.0.cache_control").Exists() {
		t.Error("cache_control not stripped from messages")
	}

	// Verify other fields preserved
	if gjson.GetBytes(result, "system.0.text").String() != "hello" {
		t.Error("system text was corrupted")
	}
	if gjson.GetBytes(result, "tools.0.name").String() != "t1" {
		t.Error("tool name was corrupted")
	}
	if gjson.GetBytes(result, "messages.0.content.0.text").String() != "hi" {
		t.Error("message text was corrupted")
	}
}

func TestFindFixers(t *testing.T) {
	fixers := FindFixers(
		&http.Response{StatusCode: 400},
		[]byte(`cache_control not permitted`),
		domain.ClientTypeClaude,
	)
	if len(fixers) == 0 {
		t.Fatal("expected to find fixers")
	}
	found := false
	for _, f := range fixers {
		if f.Name() == "cache_control" {
			found = true
		}
	}
	if !found {
		t.Error("expected cache_control fixer in results")
	}

	fixers = FindFixers(&http.Response{StatusCode: 200}, []byte(`ok`), domain.ClientTypeClaude)
	if len(fixers) != 0 {
		t.Error("expected no fixers for 200")
	}
}

func TestFindFixers_Multiple(t *testing.T) {
	// Error body mentions both cache_control and defer_loading — both fixers should match
	fixers := FindFixers(
		&http.Response{StatusCode: 400},
		[]byte(`cache_control: Extra inputs; tools.0.custom.defer_loading: Extra inputs`),
		domain.ClientTypeClaude,
	)
	names := make(map[string]bool)
	for _, f := range fixers {
		names[f.Name()] = true
	}
	if !names["cache_control"] {
		t.Error("expected cache_control fixer")
	}
	if !names["tool_custom_fields"] {
		t.Error("expected tool_custom_fields fixer")
	}
}
