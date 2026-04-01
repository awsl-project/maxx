package mockserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// WriteSSEStream writes a streaming response based on the MockStreamDirective.
func WriteSSEStream(w http.ResponseWriter, protocol Protocol, model string, stream *MockStreamDirective) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	if stream != nil && len(stream.Chunks) > 0 {
		// Custom chunks from directive
		for _, chunk := range stream.Chunks {
			if chunk.Delay != "" {
				if d, err := time.ParseDuration(chunk.Delay); err == nil {
					time.Sleep(d)
				}
			}
			if chunk.Error != nil {
				errBody := chunk.Error.Body
				if errBody == nil {
					errBody = DefaultErrorResponse(protocol, chunk.Error.Status, "stream error")
				}
				fmt.Fprintf(w, "data: %s\n\n", errBody)
				flusher.Flush()
				return
			}
			if chunk.Data != nil {
				fmt.Fprintf(w, "data: %s\n\n", chunk.Data)
				flusher.Flush()
			}
		}
	} else {
		// Default streaming: 3 text chunks
		texts := []string{"Hello", " from", " mock!"}
		for _, text := range texts {
			data := defaultStreamChunk(protocol, model, text)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}

	// Send done marker
	switch protocol {
	case ProtocolOpenAI, ProtocolCodex:
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}
}

// defaultStreamChunk generates a single SSE data payload for the given protocol.
func defaultStreamChunk(protocol Protocol, model string, text string) []byte {
	var chunk any
	switch protocol {
	case ProtocolOpenAI:
		chunk = map[string]any{
			"id": "chatcmpl-mock", "object": "chat.completion.chunk", "model": model,
			"choices": []map[string]any{{
				"index": 0,
				"delta": map[string]any{"content": text},
			}},
		}
	case ProtocolClaude:
		chunk = map[string]any{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]any{"type": "text_delta", "text": text},
		}
	case ProtocolGemini:
		chunk = map[string]any{
			"candidates": []map[string]any{{
				"content": map[string]any{
					"role":  "model",
					"parts": []map[string]any{{"text": text}},
				},
			}},
		}
	case ProtocolCodex:
		chunk = map[string]any{
			"id": "rsp_mock", "object": "response.output_text.delta",
			"delta": text,
		}
	}
	b, _ := json.Marshal(chunk)
	return b
}
