package handler

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	codexCompactMinBodyBytes      = 220 * 1024
	codexCompactTargetInputBytes  = 140 * 1024
	codexCompactKeepTailMaxItems  = 48
	codexCompactMaxTextChars      = 12000
	codexCompactStringInputTarget = 120000
)

func maybeCompactCodexContext(body []byte) ([]byte, bool) {
	if len(body) < codexCompactMinBodyBytes {
		return body, false
	}

	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return body, false
	}

	input, ok := req["input"]
	if !ok {
		return body, false
	}

	changed := false

	switch v := input.(type) {
	case string:
		if compacted, ok := compactLongString(v, codexCompactStringInputTarget); ok {
			req["input"] = compacted
			changed = true
		}
	case []interface{}:
		compactedItems, compacted := compactCodexInputItems(v)
		if compacted {
			req["input"] = compactedItems
			changed = true
		}
	default:
		return body, false
	}

	if !changed {
		return body, false
	}

	out, err := json.Marshal(req)
	if err != nil {
		return body, false
	}

	if len(out) >= len(body) {
		return body, false
	}
	return out, true
}

func compactCodexInputItems(items []interface{}) ([]interface{}, bool) {
	if len(items) == 0 {
		return items, false
	}

	totalSize := 0
	itemSizes := make([]int, len(items))
	pinned := make([]bool, len(items))
	keep := make([]bool, len(items))
	lastNonPinned := -1

	for i, item := range items {
		size := estimateJSONSize(item)
		itemSizes[i] = size
		totalSize += size

		role := codexItemRole(item)
		if role == "system" || role == "developer" {
			pinned[i] = true
			keep[i] = true
		} else {
			lastNonPinned = i
		}
	}

	if totalSize <= codexCompactTargetInputBytes && len(items) <= codexCompactKeepTailMaxItems {
		return items, false
	}

	tailBytes := 0
	tailCount := 0
	for i := len(items) - 1; i >= 0; i-- {
		if keep[i] {
			continue
		}
		size := itemSizes[i]
		if tailCount > 0 && (tailCount >= codexCompactKeepTailMaxItems || tailBytes+size > codexCompactTargetInputBytes) {
			continue
		}
		keep[i] = true
		tailBytes += size
		tailCount++
	}

	if tailCount == 0 && lastNonPinned >= 0 {
		keep[lastNonPinned] = true
	}

	omitted := 0
	for i := range items {
		if !keep[i] {
			omitted++
		}
	}

	trimmedAny := false
	pinnedItems := make([]interface{}, 0)
	recentItems := make([]interface{}, 0)
	for i, item := range items {
		if !keep[i] {
			continue
		}
		before := itemSizes[i]
		trimmed := trimCodexItemText(item, codexCompactMaxTextChars)
		if estimateJSONSize(trimmed) < before {
			trimmedAny = true
		}
		if pinned[i] {
			pinnedItems = append(pinnedItems, trimmed)
			continue
		}
		recentItems = append(recentItems, trimmed)
	}
	if omitted == 0 && !trimmedAny {
		return items, false
	}

	summary := map[string]interface{}{
		"type":    "message",
		"role":    "system",
		"content": fmt.Sprintf("[maxx] Earlier context was compacted automatically (%d items omitted) to reduce retry loops on long-context requests. Prioritize recent turns.", omitted),
	}

	compacted := make([]interface{}, 0, len(pinnedItems)+len(recentItems)+1)
	compacted = append(compacted, pinnedItems...)
	if omitted > 0 {
		compacted = append(compacted, summary)
	}
	compacted = append(compacted, recentItems...)

	return compacted, true
}

func trimCodexItemText(item interface{}, maxChars int) interface{} {
	m, ok := item.(map[string]interface{})
	if !ok {
		return item
	}

	if content, ok := m["content"].(string); ok {
		if compacted, changed := compactLongString(content, maxChars); changed {
			m["content"] = compacted
		}
	}

	if parts, ok := m["content"].([]interface{}); ok {
		for i, part := range parts {
			pm, ok := part.(map[string]interface{})
			if !ok {
				continue
			}
			text, ok := pm["text"].(string)
			if !ok {
				continue
			}
			if compacted, changed := compactLongString(text, maxChars); changed {
				pm["text"] = compacted
				parts[i] = pm
			}
		}
		m["content"] = parts
	}

	if output, ok := m["output"].(string); ok {
		if compacted, changed := compactLongString(output, maxChars); changed {
			m["output"] = compacted
		}
	}

	if arguments, ok := m["arguments"]; ok {
		if compacted, changed := compactNestedStrings(arguments, maxChars); changed {
			m["arguments"] = compacted
		}
	}

	return m
}

func compactNestedStrings(value interface{}, maxChars int) (interface{}, bool) {
	switch v := value.(type) {
	case string:
		if compacted, changed := compactLongString(v, maxChars); changed {
			return compacted, true
		}
		return v, false
	case map[string]interface{}:
		changed := false
		for key, raw := range v {
			compacted, partChanged := compactNestedStrings(raw, maxChars)
			if partChanged {
				changed = true
				v[key] = compacted
			}
		}
		return v, changed
	case []interface{}:
		changed := false
		for i := range v {
			compacted, partChanged := compactNestedStrings(v[i], maxChars)
			if partChanged {
				changed = true
				v[i] = compacted
			}
		}
		return v, changed
	default:
		return value, false
	}
}

func compactLongString(s string, target int) (string, bool) {
	runes := []rune(s)
	if len(runes) <= target || target <= 64 {
		return s, false
	}

	marker := "\n\n[maxx] ... earlier context omitted ...\n\n"
	markerRunes := []rune(marker)
	if len(markerRunes) >= target {
		return string(runes[len(runes)-target:]), true
	}

	headLen := target / 6
	if headLen < 48 {
		headLen = 48
	}
	tailLen := target - headLen - len(markerRunes)
	if tailLen < 48 {
		tailLen = 48
		headLen = target - tailLen - len(markerRunes)
		if headLen < 0 {
			headLen = 0
		}
	}

	if headLen+tailLen+len(markerRunes) > len(runes) {
		return s, false
	}

	head := string(runes[:headLen])
	tail := string(runes[len(runes)-tailLen:])
	return head + marker + tail, true
}

func codexItemRole(item interface{}) string {
	m, ok := item.(map[string]interface{})
	if !ok {
		return ""
	}
	role, _ := m["role"].(string)
	return strings.ToLower(strings.TrimSpace(role))
}

func estimateJSONSize(v interface{}) int {
	b, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return len(b)
}
