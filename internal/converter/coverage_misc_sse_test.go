package converter

import (
	"strings"
	"testing"
)

func TestIsSSEAdditional(t *testing.T) {
	if !IsSSE("data: {}\n\n") {
		t.Fatalf("expected SSE")
	}
	if IsSSE("hello") {
		t.Fatalf("expected non-SSE")
	}
}

func TestIsSSEEmptyLines(t *testing.T) {
	if IsSSE("\n\n") {
		t.Fatalf("expected false for empty lines")
	}
}

func TestIsSSEEventLine(t *testing.T) {
	if !IsSSE("event: message\n\n") {
		t.Fatalf("expected SSE event")
	}
}

func TestSSE_ParseIncompleteLine(t *testing.T) {
	events, remaining := ParseSSE("data: {\"a\":1}")
	if len(events) != 0 {
		t.Fatalf("expected no events")
	}
	if remaining == "" {
		t.Fatalf("expected remaining buffer")
	}
}

func TestSSE_FormatStringData(t *testing.T) {
	out := FormatSSE("", "hello")
	if !strings.Contains(string(out), "data: hello") {
		t.Fatalf("expected string data")
	}
}

func TestSSE_FormatMultilineDataPrefixesEveryLine(t *testing.T) {
	out := FormatSSE("response.completed", []byte("{\n  \"type\": \"response.completed\"\n}"))
	outStr := string(out)
	for _, want := range []string{
		"event: response.completed",
		"data: {",
		"data:   \"type\": \"response.completed\"",
		"data: }",
	} {
		if !strings.Contains(outStr, want) {
			t.Fatalf("expected %q in %q", want, outStr)
		}
	}

	events, remaining := ParseSSE(outStr)
	if remaining != "" || len(events) != 1 {
		t.Fatalf("expected one complete parsed event, remaining=%q events=%d", remaining, len(events))
	}
	if events[0].Event != "response.completed" || !strings.Contains(string(events[0].Data), "response.completed") {
		t.Fatalf("unexpected parsed event: %+v", events[0])
	}
}
