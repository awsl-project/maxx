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
	if len([]byte(longInput)) <= codexCompactStringInputTarget {
		t.Fatalf("long input should exceed compact target bytes")
	}
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
	if len([]byte(gotInput)) > codexCompactStringInputTarget {
		t.Fatalf("expected compacted input bytes <= %d, got %d", codexCompactStringInputTarget, len([]byte(gotInput)))
	}
}

func TestMaybeCompactCodexContext_CompactsLongMultibyteStringInputWithinByteBudget(t *testing.T) {
	longInput := "头部保留" + strings.Repeat("你好🙂", 50000) + "尾部保留"
	if len([]byte(longInput)) <= codexCompactStringInputTarget {
		t.Fatalf("multibyte input should exceed compact target bytes")
	}

	req := map[string]interface{}{
		"model": "gpt-5.3-codex",
		"input": longInput,
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
		t.Fatalf("expected multibyte string input to be compacted")
	}

	var compacted map[string]interface{}
	if err := json.Unmarshal(out, &compacted); err != nil {
		t.Fatalf("unmarshal compacted request: %v", err)
	}

	gotInput, _ := compacted["input"].(string)
	if !strings.Contains(gotInput, "[maxx] ... earlier context omitted ...") {
		t.Fatalf("expected omission marker in compacted string")
	}
	if !strings.Contains(gotInput, "头部保留") {
		t.Fatalf("expected multibyte string head to be preserved")
	}
	if !strings.Contains(gotInput, "尾部保留") {
		t.Fatalf("expected multibyte string tail to be preserved")
	}
	if len([]byte(gotInput)) > codexCompactStringInputTarget {
		t.Fatalf("expected compacted multibyte input bytes <= %d, got %d", codexCompactStringInputTarget, len([]byte(gotInput)))
	}
}

func TestMaybeCompactCodexContext_CompactsByTrimmingWithoutOmittingItems(t *testing.T) {
	req := map[string]interface{}{
		"model": "gpt-5.3-codex",
		"input": []interface{}{
			map[string]interface{}{
				"type":    "message",
				"role":    "system",
				"content": strings.Repeat("S", 260000),
			},
		},
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
		t.Fatalf("expected payload to be compacted by trimming")
	}
	if len(out) >= len(body) {
		t.Fatalf("expected compacted payload to be smaller: %d >= %d", len(out), len(body))
	}

	var compacted map[string]interface{}
	if err := json.Unmarshal(out, &compacted); err != nil {
		t.Fatalf("unmarshal compacted request: %v", err)
	}

	items, ok := compacted["input"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("expected one compacted item, got %#v", compacted["input"])
	}
	item, _ := items[0].(map[string]interface{})
	content, _ := item["content"].(string)
	if !strings.Contains(content, "[maxx] ... earlier context omitted ...") {
		t.Fatalf("expected trimmed marker in content")
	}
}

func TestTrimCodexItemText_CompactsArgumentsField(t *testing.T) {
	item := map[string]interface{}{
		"type": "function_call",
		"arguments": map[string]interface{}{
			"query": strings.Repeat("Q", 20000),
			"nested": []interface{}{
				map[string]interface{}{
					"text": strings.Repeat("N", 18000),
				},
			},
		},
	}

	trimmed := trimCodexItemText(item, codexCompactMaxTextChars)
	trimmedMap, ok := trimmed.(map[string]interface{})
	if !ok {
		t.Fatalf("trimmed item type = %T, want map", trimmed)
	}

	args, ok := trimmedMap["arguments"].(map[string]interface{})
	if !ok {
		t.Fatalf("arguments type = %T, want map", trimmedMap["arguments"])
	}
	query, _ := args["query"].(string)
	if !strings.Contains(query, "[maxx] ... earlier context omitted ...") {
		t.Fatalf("expected query argument to be compacted")
	}
	nested, _ := args["nested"].([]interface{})
	nestedMap, _ := nested[0].(map[string]interface{})
	nestedText, _ := nestedMap["text"].(string)
	if !strings.Contains(nestedText, "[maxx] ... earlier context omitted ...") {
		t.Fatalf("expected nested argument text to be compacted")
	}

	originalArgs, _ := item["arguments"].(map[string]interface{})
	originalQuery, _ := originalArgs["query"].(string)
	if strings.Contains(originalQuery, "[maxx] ... earlier context omitted ...") {
		t.Fatalf("expected original query argument to remain unchanged")
	}
	originalNested, _ := originalArgs["nested"].([]interface{})
	originalNestedMap, _ := originalNested[0].(map[string]interface{})
	originalNestedText, _ := originalNestedMap["text"].(string)
	if strings.Contains(originalNestedText, "[maxx] ... earlier context omitted ...") {
		t.Fatalf("expected original nested argument text to remain unchanged")
	}
}

func TestCompactNestedStrings_DoesNotMutateInput(t *testing.T) {
	value := map[string]interface{}{
		"query": strings.Repeat("Q", 20000),
		"nested": []interface{}{
			map[string]interface{}{
				"text": strings.Repeat("N", 18000),
			},
		},
	}

	compacted, changed := compactNestedStrings(value, codexCompactMaxTextChars)
	if !changed {
		t.Fatalf("expected nested strings to be compacted")
	}

	compactedMap, ok := compacted.(map[string]interface{})
	if !ok {
		t.Fatalf("compacted value type = %T, want map", compacted)
	}
	compactedQuery, _ := compactedMap["query"].(string)
	if !strings.Contains(compactedQuery, "[maxx] ... earlier context omitted ...") {
		t.Fatalf("expected compacted query to contain marker")
	}

	originalQuery, _ := value["query"].(string)
	if strings.Contains(originalQuery, "[maxx] ... earlier context omitted ...") {
		t.Fatalf("expected original query to remain unchanged")
	}
	originalNested, _ := value["nested"].([]interface{})
	originalNestedMap, _ := originalNested[0].(map[string]interface{})
	originalNestedText, _ := originalNestedMap["text"].(string)
	if strings.Contains(originalNestedText, "[maxx] ... earlier context omitted ...") {
		t.Fatalf("expected original nested text to remain unchanged")
	}
}
