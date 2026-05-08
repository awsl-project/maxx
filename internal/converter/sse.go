package converter

import (
	"encoding/json"
	"strings"
)

// SSEEvent represents a parsed SSE event
type SSEEvent struct {
	Event string          `json:"event,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// ParseSSE parses SSE text into events, returning parsed events and remaining buffer
func ParseSSE(text string) ([]SSEEvent, string) {
	var events []SSEEvent
	lines := strings.Split(text, "\n")

	var currentEvent string
	var currentData []string
	var remaining strings.Builder

	for i, line := range lines {
		// Check if this is the last line and might be incomplete
		if i == len(lines)-1 && line != "" && !strings.HasSuffix(text, "\n") {
			remaining.WriteString(line)
			break
		}

		line = strings.TrimSpace(line)

		// Empty line = end of event
		if line == "" {
			if len(currentData) > 0 {
				dataStr := strings.Join(currentData, "\n")
				if dataStr == "[DONE]" {
					events = append(events, SSEEvent{Event: "done"})
				} else {
					var rawData json.RawMessage
					if json.Unmarshal([]byte(dataStr), &rawData) == nil {
						events = append(events, SSEEvent{
							Event: currentEvent,
							Data:  rawData,
						})
					}
				}
			}
			currentEvent = ""
			currentData = nil
			continue
		}

		if strings.HasPrefix(line, "event:") {
			currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			currentData = append(currentData, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}

	return events, remaining.String()
}

// IsSSE checks if text looks like SSE format
func IsSSE(text string) bool {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "event:") || strings.HasPrefix(line, "data:") {
			return true
		}
		// If we find a non-SSE line first, it's not SSE
		if line != "" {
			return false
		}
	}
	return false
}

// FormatSSE formats an event and data as SSE
func FormatSSE(event string, data interface{}) []byte {
	var sb strings.Builder
	if event != "" {
		sb.WriteString("event: ")
		sb.WriteString(event)
		sb.WriteString("\n")
	}

	var dataBytes []byte
	switch v := data.(type) {
	case []byte:
		dataBytes = v
	case string:
		dataBytes = []byte(v)
	default:
		dataBytes, _ = json.Marshal(v)
	}

	dataLines := strings.Split(string(dataBytes), "\n")
	if len(dataLines) == 0 {
		dataLines = []string{""}
	}
	for _, line := range dataLines {
		sb.WriteString("data: ")
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	return []byte(sb.String())
}

// FormatDone returns the SSE [DONE] marker
func FormatDone() []byte {
	return []byte("data: [DONE]\n\n")
}
