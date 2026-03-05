package handler

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMaybeCompactCodexContext_SmallPayloadUnchanged(t *testing.T) {
	req := map[string]interface{}{
		"model": "gpt-5.3-codex",
		"input": []interface{}{
			map[string]interface{}{"type": "message", "role": "system", "content": "You are helpful."},
			map[string]interface{}{"type": "message", "role": "user", "content": "hello"},
		},
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	out, changed := maybeCompactCodexContext(body)
	if changed {
		t.Fatalf("expected unchanged payload")
	}
	if string(out) != string(body) {
		t.Fatalf("body changed unexpectedly")
	}
}

func TestMaybeCompactCodexContext_CompactsLongInputItems(t *testing.T) {
	items := make([]interface{}, 0, 80)
	items = append(items, map[string]interface{}{
		"type":    "message",
		"role":    "system",
		"content": "Always follow policy.",
	})

	for i := 0; i < 78; i++ {
		items = append(items, map[string]interface{}{
			"type":    "message",
			"role":    "user",
			"content": strings.Repeat("ctx-", 1800) + "-turn-" + string(rune('A'+(i%26))),
		})
	}
	items = append(items, map[string]interface{}{
		"type":    "message",
		"role":    "assistant",
		"content": "important-tail-keep-me",
	})

	req := map[string]interface{}{
		"model": "gpt-5.3-codex",
		"input": items,
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	if len(body) <= codexCompactMinBodyBytes {
		t.Fatalf("test payload too small: %d", len(body))
	}

	out, changed := maybeCompactCodexContext(body)
	if !changed {
		t.Fatalf("expected payload to be compacted")
	}
	if len(out) >= len(body) {
		t.Fatalf("expected compacted payload to be smaller: %d >= %d", len(out), len(body))
	}

	var compacted map[string]interface{}
	if err := json.Unmarshal(out, &compacted); err != nil {
		t.Fatalf("unmarshal compacted request: %v", err)
	}

	compactedItems, ok := compacted["input"].([]interface{})
	if !ok {
		t.Fatalf("compacted input not array")
	}
	if len(compactedItems) >= len(items) {
		t.Fatalf("expected omitted items, got %d >= %d", len(compactedItems), len(items))
	}

	first, _ := compactedItems[0].(map[string]interface{})
	if first["role"] != "system" {
		t.Fatalf("expected pinned system item to remain first, got %#v", first["role"])
	}

	foundSummary := false
	foundTail := false
	for _, item := range compactedItems {
		m, _ := item.(map[string]interface{})
		content, _ := m["content"].(string)
		if strings.Contains(content, "Earlier context was compacted automatically") {
			foundSummary = true
		}
		if strings.Contains(content, "important-tail-keep-me") {
			foundTail = true
		}
	}

	if !foundSummary {
		t.Fatalf("expected compaction summary item")
	}
	if !foundTail {
		t.Fatalf("expected newest tail item to be kept")
	}
}

func TestMaybeCompactCodexContext_CompactsLongStringInput(t *testing.T) {
	longInput := "HEAD-KEEP" + strings.Repeat("x", 260000) + "TAIL-KEEP"
	req := map[string]interface{}{
		"model": "gpt-5.3-codex",
		"input": longInput,
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	out, changed := maybeCompactCodexContext(body)
	if !changed {
		t.Fatalf("expected string input to be compacted")
	}

	var compacted map[string]interface{}
	if err := json.Unmarshal(out, &compacted); err != nil {
		t.Fatalf("unmarshal compacted request: %v", err)
	}

	gotInput, _ := compacted["input"].(string)
	if !strings.Contains(gotInput, "[maxx] ... earlier context omitted ...") {
		t.Fatalf("expected omission marker in compacted string")
	}
	if !strings.Contains(gotInput, "HEAD-KEEP") {
		t.Fatalf("expected string head to be preserved")
	}
	if !strings.Contains(gotInput, "TAIL-KEEP") {
		t.Fatalf("expected string tail to be preserved")
	}
}
