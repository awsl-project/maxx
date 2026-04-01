package mockserver

import (
	"bufio"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteSSEStream_Default(t *testing.T) {
	protocols := []Protocol{ProtocolOpenAI, ProtocolClaude, ProtocolGemini, ProtocolCodex}
	for _, proto := range protocols {
		t.Run(string(proto), func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteSSEStream(rec, proto, "test-model", nil)

			resp := rec.Result()
			if resp.Header.Get("Content-Type") != "text/event-stream" {
				t.Errorf("expected text/event-stream, got %s", resp.Header.Get("Content-Type"))
			}

			// Count data lines
			scanner := bufio.NewScanner(resp.Body)
			dataCount := 0
			for scanner.Scan() {
				line := scanner.Text()
				if strings.HasPrefix(line, "data: ") {
					dataCount++
				}
			}
			if dataCount < 3 {
				t.Errorf("expected at least 3 data lines, got %d", dataCount)
			}
		})
	}
}

func TestWriteSSEStream_CustomChunks(t *testing.T) {
	rec := httptest.NewRecorder()
	stream := &MockStreamDirective{
		Chunks: []MockStreamChunk{
			{Data: json.RawMessage(`{"text":"chunk1"}`)},
			{Data: json.RawMessage(`{"text":"chunk2"}`)},
		},
	}
	WriteSSEStream(rec, ProtocolOpenAI, "gpt-4o", stream)

	body := rec.Body.String()
	if !strings.Contains(body, "chunk1") || !strings.Contains(body, "chunk2") {
		t.Errorf("expected custom chunks in body: %s", body)
	}
	if !strings.Contains(body, "[DONE]") {
		t.Error("expected [DONE] marker for OpenAI")
	}
}

func TestWriteSSEStream_ErrorChunk(t *testing.T) {
	rec := httptest.NewRecorder()
	stream := &MockStreamDirective{
		Chunks: []MockStreamChunk{
			{Data: json.RawMessage(`{"text":"before error"}`)},
			{Error: &MockStreamError{Status: 503, Body: json.RawMessage(`{"error":"overloaded"}`)}},
			{Data: json.RawMessage(`{"text":"should not appear"}`)},
		},
	}
	WriteSSEStream(rec, ProtocolOpenAI, "gpt-4o", stream)

	body := rec.Body.String()
	if !strings.Contains(body, "before error") {
		t.Error("expected first chunk")
	}
	if !strings.Contains(body, "overloaded") {
		t.Error("expected error chunk")
	}
	if strings.Contains(body, "should not appear") {
		t.Error("stream should have stopped after error")
	}
	// Should NOT have [DONE] since error terminated the stream
	if strings.Contains(body, "[DONE]") {
		t.Error("should not have [DONE] after error termination")
	}
}

func TestDefaultStreamChunk_ValidJSON(t *testing.T) {
	protocols := []Protocol{ProtocolOpenAI, ProtocolClaude, ProtocolGemini, ProtocolCodex}
	for _, proto := range protocols {
		t.Run(string(proto), func(t *testing.T) {
			data := defaultStreamChunk(proto, "test-model", "hello")
			var m map[string]any
			if err := json.Unmarshal(data, &m); err != nil {
				t.Fatalf("invalid JSON: %v — data: %s", err, data)
			}
		})
	}
}
