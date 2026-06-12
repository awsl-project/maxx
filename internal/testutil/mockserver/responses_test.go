package mockserver

import (
	"encoding/json"
	"testing"
)

func TestDefaultSuccessResponse(t *testing.T) {
	protocols := []struct {
		protocol Protocol
		model    string
		checkKey string // key to verify exists in response
	}{
		{ProtocolOpenAI, "gpt-4o", "choices"},
		{ProtocolClaude, "claude-sonnet-4", "content"},
		{ProtocolGemini, "gemini-2.5-pro", "candidates"},
		{ProtocolCodex, "gpt-4o", "output"},
	}
	for _, tt := range protocols {
		t.Run(string(tt.protocol), func(t *testing.T) {
			body := DefaultSuccessResponse(tt.protocol, tt.model)
			if len(body) == 0 {
				t.Fatal("empty response")
			}
			var m map[string]any
			if err := json.Unmarshal(body, &m); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}
			if _, ok := m[tt.checkKey]; !ok {
				t.Errorf("missing key %q in response: %s", tt.checkKey, body)
			}
		})
	}
}

func TestDefaultErrorResponse(t *testing.T) {
	protocols := []Protocol{ProtocolOpenAI, ProtocolClaude, ProtocolGemini, ProtocolCodex}
	statusCodes := []int{401, 404, 429, 500, 503}

	for _, proto := range protocols {
		for _, code := range statusCodes {
			t.Run(string(proto)+"/"+string(rune('0'+code/100))+"xx", func(t *testing.T) {
				body := DefaultErrorResponse(proto, code, "test error")
				if len(body) == 0 {
					t.Fatal("empty response")
				}
				var m map[string]any
				if err := json.Unmarshal(body, &m); err != nil {
					t.Fatalf("invalid JSON: %v", err)
				}
				if _, ok := m["error"]; !ok {
					t.Errorf("missing 'error' key: %s", body)
				}
			})
		}
	}
}
