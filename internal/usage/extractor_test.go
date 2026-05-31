package usage

import (
	"fmt"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Functional tests for StreamCollector
// ---------------------------------------------------------------------------

func TestStreamCollector_CodexResponseCompleted(t *testing.T) {
	lines := []string{
		`event: response.created`,
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-4.1"}}`,
		``,
		`event: response.output_item.added`,
		`data: {"type":"response.output_item.added"}`,
		``,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-4.1","usage":{"input_tokens":100,"output_tokens":50}}}`,
		``,
		`data: [DONE]`,
	}

	var sc StreamCollector
	for _, line := range lines {
		sc.ProcessSSELine(line)
	}

	if sc.Metrics == nil {
		t.Fatal("expected metrics to be collected")
	}
	if sc.Metrics.InputTokens != 100 {
		t.Fatalf("expected InputTokens=100, got %d", sc.Metrics.InputTokens)
	}
	if sc.Metrics.OutputTokens != 50 {
		t.Fatalf("expected OutputTokens=50, got %d", sc.Metrics.OutputTokens)
	}
}

func TestStreamCollector_ClaudeMessageDelta(t *testing.T) {
	lines := []string{
		`data: {"type":"message_start","message":{"id":"msg_1","model":"claude-sonnet-4-20250514","usage":{"input_tokens":200,"output_tokens":0}}}`,
		`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Hello"}}`,
		`data: {"type":"message_delta","usage":{"output_tokens":75}}`,
		`data: [DONE]`,
	}

	var sc StreamCollector
	for _, line := range lines {
		sc.ProcessSSELine(line)
	}

	if sc.Metrics == nil {
		t.Fatal("expected metrics to be collected")
	}
	if sc.Metrics.OutputTokens != 75 {
		t.Fatalf("expected OutputTokens=75, got %d", sc.Metrics.OutputTokens)
	}
}

func TestStreamCollector_GeminiUsageMetadata(t *testing.T) {
	lines := []string{
		`data: {"candidates":[{"content":{"parts":[{"text":"Hi"}]}}],"usageMetadata":{"promptTokenCount":150,"candidatesTokenCount":30}}`,
	}

	var sc StreamCollector
	for _, line := range lines {
		sc.ProcessSSELine(line)
	}

	if sc.Metrics == nil {
		t.Fatal("expected metrics to be collected")
	}
	if sc.Metrics.InputTokens != 150 {
		t.Fatalf("expected InputTokens=150, got %d", sc.Metrics.InputTokens)
	}
	if sc.Metrics.OutputTokens != 30 {
		t.Fatalf("expected OutputTokens=30, got %d", sc.Metrics.OutputTokens)
	}
}

func TestStreamCollector_IgnoresNonDataLines(t *testing.T) {
	lines := []string{
		`event: message_start`,
		`: comment`,
		``,
		`retry: 5000`,
	}

	var sc StreamCollector
	for _, line := range lines {
		sc.ProcessSSELine(line)
	}

	if sc.Metrics != nil {
		t.Fatal("expected no metrics from non-data lines")
	}
}

func TestStreamCollector_IgnoresDONE(t *testing.T) {
	var sc StreamCollector
	sc.ProcessSSELine("data: [DONE]")

	if sc.Metrics != nil {
		t.Fatal("expected no metrics from [DONE]")
	}
}

func TestStreamCollector_IgnoresInvalidJSON(t *testing.T) {
	var sc StreamCollector
	sc.ProcessSSELine("data: not-json-at-all")

	if sc.Metrics != nil {
		t.Fatal("expected no metrics from invalid JSON")
	}
}

func TestStreamCollector_KeepsLastMetrics(t *testing.T) {
	// Simulates a stream where usage appears multiple times; last one wins.
	lines := []string{
		`data: {"usage":{"input_tokens":10,"output_tokens":5}}`,
		`data: {"usage":{"input_tokens":200,"output_tokens":100}}`,
	}

	var sc StreamCollector
	for _, line := range lines {
		sc.ProcessSSELine(line)
	}

	if sc.Metrics.InputTokens != 200 || sc.Metrics.OutputTokens != 100 {
		t.Fatalf("expected last metrics to win, got input=%d output=%d",
			sc.Metrics.InputTokens, sc.Metrics.OutputTokens)
	}
}

func TestStreamCollector_ClaudeCacheMetrics(t *testing.T) {
	line := `data: {"type":"message_start","message":{"usage":{"input_tokens":500,"output_tokens":0,"cache_read_input_tokens":80,"cache_creation_input_tokens":120,"cache_creation_5m_input_tokens":50,"cache_creation_1h_input_tokens":70}}}`

	var sc StreamCollector
	sc.ProcessSSELine(line)

	if sc.Metrics == nil {
		t.Fatal("expected metrics")
	}
	if sc.Metrics.CacheReadCount != 80 {
		t.Fatalf("expected CacheReadCount=80, got %d", sc.Metrics.CacheReadCount)
	}
	if sc.Metrics.CacheCreationCount != 120 {
		t.Fatalf("expected CacheCreationCount=120, got %d", sc.Metrics.CacheCreationCount)
	}
	if sc.Metrics.Cache5mCreationCount != 50 {
		t.Fatalf("expected Cache5mCreationCount=50, got %d", sc.Metrics.Cache5mCreationCount)
	}
	if sc.Metrics.Cache1hCreationCount != 70 {
		t.Fatalf("expected Cache1hCreationCount=70, got %d", sc.Metrics.Cache1hCreationCount)
	}
}

// ---------------------------------------------------------------------------
// Consistency test: StreamCollector vs ExtractFromStreamContent
// Ensures incremental approach produces identical results to full-buffer approach.
// ---------------------------------------------------------------------------

func TestStreamCollector_ConsistentWithFullBuffer(t *testing.T) {
	streams := []struct {
		name  string
		lines []string
	}{
		{
			name: "codex",
			lines: []string{
				"event: response.created\n",
				`data: {"type":"response.created","response":{"model":"gpt-4.1"}}` + "\n",
				"\n",
				`data: {"type":"response.output_text.delta","delta":"Hello"}` + "\n",
				"\n",
				`data: {"type":"response.completed","response":{"usage":{"input_tokens":300,"output_tokens":120}}}` + "\n",
				"\n",
				"data: [DONE]\n",
			},
		},
		{
			name: "claude",
			lines: []string{
				"event: message_start\n",
				`data: {"type":"message_start","message":{"id":"msg_1","model":"claude-sonnet-4-20250514","usage":{"input_tokens":400,"output_tokens":0}}}` + "\n",
				"\n",
				"event: content_block_delta\n",
				`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Hi"}}` + "\n",
				"\n",
				"event: message_delta\n",
				`data: {"type":"message_delta","usage":{"output_tokens":55}}` + "\n",
				"\n",
				"event: message_stop\n",
				`data: {"type":"message_stop"}` + "\n",
			},
		},
		{
			name: "gemini",
			lines: []string{
				`data: {"candidates":[{"content":{"parts":[{"text":"chunk1"}]}}]}` + "\n",
				"\n",
				`data: {"candidates":[{"content":{"parts":[{"text":"chunk2"}]}}],"usageMetadata":{"promptTokenCount":250,"candidatesTokenCount":60}}` + "\n",
			},
		},
	}

	for _, tc := range streams {
		t.Run(tc.name, func(t *testing.T) {
			// Old approach: full buffer
			fullContent := strings.Join(tc.lines, "")
			oldMetrics := ExtractFromStreamContent(fullContent)

			// New approach: incremental
			var sc StreamCollector
			for _, line := range tc.lines {
				sc.ProcessSSELine(line)
			}

			if oldMetrics == nil && sc.Metrics == nil {
				return // both nil, consistent
			}
			if (oldMetrics == nil) != (sc.Metrics == nil) {
				t.Fatalf("inconsistency: old=%v new=%v", oldMetrics, sc.Metrics)
			}
			if oldMetrics.InputTokens != sc.Metrics.InputTokens {
				t.Fatalf("InputTokens mismatch: old=%d new=%d", oldMetrics.InputTokens, sc.Metrics.InputTokens)
			}
			if oldMetrics.OutputTokens != sc.Metrics.OutputTokens {
				t.Fatalf("OutputTokens mismatch: old=%d new=%d", oldMetrics.OutputTokens, sc.Metrics.OutputTokens)
			}
			if oldMetrics.CacheReadCount != sc.Metrics.CacheReadCount {
				t.Fatalf("CacheReadCount mismatch: old=%d new=%d", oldMetrics.CacheReadCount, sc.Metrics.CacheReadCount)
			}
		})
	}
}

// TestExtractFromResponse_GptImageGenerations guards billing for the OpenAI
// Images API (gpt-image-2). The non-streaming response carries a top-level
// usage object that names tokens input_tokens/output_tokens (same as Claude),
// so the existing extractor must pick them up unchanged. If this breaks,
// gpt-image-2 requests would silently bill at zero cost.
func TestExtractFromResponse_GptImageGenerations(t *testing.T) {
	body := `{
		"created": 1700000000,
		"data": [{"b64_json": "iVBOR..."}],
		"usage": {
			"total_tokens": 150,
			"input_tokens": 50,
			"output_tokens": 100,
			"input_tokens_details": {"text_tokens": 50, "image_tokens": 0}
		}
	}`

	m := ExtractFromResponse(body)
	if m == nil {
		t.Fatal("expected metrics from images generations response, got nil")
	}
	if m.InputTokens != 50 {
		t.Errorf("InputTokens = %d, want 50", m.InputTokens)
	}
	if m.OutputTokens != 100 {
		t.Errorf("OutputTokens = %d, want 100", m.OutputTokens)
	}
}

// TestExtractFromResponse_GptImageEditsTokenSplit guards extraction of the
// image-token breakdown (input_tokens_details / output_tokens_details) from a
// real gpt-image-2 edits response, so pricing can bill image tokens at the image
// rate. These are subsets of input/output tokens.
func TestExtractFromResponse_GptImageEditsTokenSplit(t *testing.T) {
	body := `{
		"created": 1700000000,
		"data": [{"b64_json": "iVBOR..."}],
		"usage": {
			"total_tokens": 224,
			"input_tokens": 28,
			"output_tokens": 196,
			"input_tokens_details": {"text_tokens": 12, "image_tokens": 16},
			"output_tokens_details": {"text_tokens": 0, "image_tokens": 196}
		}
	}`

	m := ExtractFromResponse(body)
	if m == nil {
		t.Fatal("expected metrics, got nil")
	}
	if m.InputTokens != 28 || m.OutputTokens != 196 {
		t.Errorf("tokens = in %d / out %d, want 28 / 196", m.InputTokens, m.OutputTokens)
	}
	if m.InputImageTokens != 16 {
		t.Errorf("InputImageTokens = %d, want 16", m.InputImageTokens)
	}
	if m.OutputImageTokens != 196 {
		t.Errorf("OutputImageTokens = %d, want 196", m.OutputImageTokens)
	}
}

// TestExtractFromResponse_OpenAIChatCompletions pins the dispatcher contract
// for plain OpenAI chat-completions responses. Before the fix, the top-level
// {"usage": ...} branch unconditionally ran the Claude extractor, which only
// looks for input_tokens / output_tokens — so prompt_tokens / completion_tokens
// were silently dropped and the request billed at zero.
func TestExtractFromResponse_OpenAIChatCompletions(t *testing.T) {
	body := `{
		"id": "chatcmpl-xyz",
		"object": "chat.completion",
		"model": "gpt-4o-mini",
		"choices": [{"index": 0, "message": {"role": "assistant", "content": "hi"}, "finish_reason": "stop"}],
		"usage": {
			"prompt_tokens": 17,
			"completion_tokens": 5,
			"total_tokens": 22
		}
	}`

	m := ExtractFromResponse(body)
	if m == nil {
		t.Fatal("expected metrics for chat-completions response, got nil")
	}
	if m.InputTokens != 17 || m.OutputTokens != 5 {
		t.Errorf("tokens = in %d / out %d, want 17 / 5", m.InputTokens, m.OutputTokens)
	}
}

// TestExtractFromResponse_OpenAIChatCompletionsZeroPadded covers the real-world
// shape returned by OpenAI-compatible aggregators (linkapi, oneapi, etc.) that
// dual-emit Claude-style zero-padded input_tokens / output_tokens alongside the
// real OpenAI numbers. The dispatcher must pick OpenAI by detecting
// prompt_tokens / completion_tokens, not silently zero out billing.
func TestExtractFromResponse_OpenAIChatCompletionsZeroPadded(t *testing.T) {
	body := `{
		"object": "chat.completion",
		"choices": [{"index": 0, "message": {"role": "assistant", "content": "hi"}, "finish_reason": "stop"}],
		"usage": {
			"prompt_tokens": 12,
			"completion_tokens": 34,
			"total_tokens": 46,
			"input_tokens": 0,
			"output_tokens": 0,
			"input_tokens_details": null
		}
	}`

	m := ExtractFromResponse(body)
	if m == nil {
		t.Fatal("expected metrics, got nil")
	}
	if m.InputTokens != 12 || m.OutputTokens != 34 {
		t.Errorf("zero-padded relay should not zero billing: tokens = in %d / out %d, want 12 / 34", m.InputTokens, m.OutputTokens)
	}
}

// TestExtractFromResponse_GeminiFlashImageViaChatCompletions covers the
// "nano-banana" (gemini-2.5-flash-image) shape relayed through OpenAI chat
// completions: image tokens land under completion_tokens_details.image_tokens,
// not output_tokens_details.image_tokens like the OpenAI Images API. Without
// this branch the image-rate price wouldn't be applied (image_tokens stays 0).
func TestExtractFromResponse_GeminiFlashImageViaChatCompletions(t *testing.T) {
	body := `{
		"id": "chatcmpl-banana",
		"object": "chat.completion",
		"model": "gemini-2.5-flash-image",
		"choices": [{"index": 0, "message": {"role": "assistant", "content": "![image](data:...)"}, "finish_reason": "stop"}],
		"usage": {
			"prompt_tokens": 7,
			"completion_tokens": 1290,
			"total_tokens": 1297,
			"prompt_tokens_details": {"text_tokens": 7, "image_tokens": 0},
			"completion_tokens_details": {"text_tokens": 0, "image_tokens": 1290}
		}
	}`

	m := ExtractFromResponse(body)
	if m == nil {
		t.Fatal("expected metrics for banana response, got nil")
	}
	if m.InputTokens != 7 || m.OutputTokens != 1290 {
		t.Errorf("tokens = in %d / out %d, want 7 / 1290", m.InputTokens, m.OutputTokens)
	}
	if m.OutputImageTokens != 1290 {
		t.Errorf("OutputImageTokens = %d, want 1290 (banana image output should bill at image rate)", m.OutputImageTokens)
	}
}

// ---------------------------------------------------------------------------
// Benchmarks: Old (full-buffer) vs New (incremental)
// ---------------------------------------------------------------------------

// generateCodexStream builds a realistic Codex SSE stream with N content chunks.
func generateCodexStream(numChunks int) []string {
	lines := make([]string, 0, numChunks*3+6)
	lines = append(lines, `data: {"type":"response.created","response":{"model":"gpt-4.1"}}`)
	lines = append(lines, "")

	for i := 0; i < numChunks; i++ {
		lines = append(lines, fmt.Sprintf(`data: {"type":"response.output_text.delta","delta":"chunk-%d with some content text here."}`, i))
		lines = append(lines, "")
	}

	lines = append(lines, `data: {"type":"response.completed","response":{"model":"gpt-4.1","usage":{"input_tokens":5000,"output_tokens":2000}}}`)
	lines = append(lines, "")
	lines = append(lines, "data: [DONE]")
	return lines
}

// generateClaudeStream builds a realistic Claude SSE stream with N content chunks.
func generateClaudeStream(numChunks int) []string {
	lines := make([]string, 0, numChunks*3+10)
	lines = append(lines, `event: message_start`)
	lines = append(lines, `data: {"type":"message_start","message":{"id":"msg_1","model":"claude-sonnet-4-20250514","usage":{"input_tokens":3000,"output_tokens":0,"cache_read_input_tokens":500}}}`)
	lines = append(lines, "")
	lines = append(lines, `event: content_block_start`)
	lines = append(lines, `data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
	lines = append(lines, "")

	for i := 0; i < numChunks; i++ {
		lines = append(lines, `event: content_block_delta`)
		lines = append(lines, fmt.Sprintf(`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"chunk-%d with some response text content."}}`, i))
		lines = append(lines, "")
	}

	lines = append(lines, `event: message_delta`)
	lines = append(lines, `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1500}}`)
	lines = append(lines, "")
	lines = append(lines, `event: message_stop`)
	lines = append(lines, `data: {"type":"message_stop"}`)
	return lines
}

func BenchmarkOldFullBuffer_Codex(b *testing.B) {
	for _, size := range []int{100, 1000, 10000} {
		stream := generateCodexStream(size)
		fullContent := strings.Join(stream, "\n")
		b.Run(fmt.Sprintf("chunks=%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				ExtractFromStreamContent(fullContent)
			}
		})
	}
}

func BenchmarkNewIncremental_Codex(b *testing.B) {
	for _, size := range []int{100, 1000, 10000} {
		stream := generateCodexStream(size)
		b.Run(fmt.Sprintf("chunks=%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				var sc StreamCollector
				for _, line := range stream {
					sc.ProcessSSELine(line)
				}
			}
		})
	}
}

func BenchmarkOldFullBuffer_Claude(b *testing.B) {
	for _, size := range []int{100, 1000, 10000} {
		stream := generateClaudeStream(size)
		fullContent := strings.Join(stream, "\n")
		b.Run(fmt.Sprintf("chunks=%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				ExtractFromStreamContent(fullContent)
			}
		})
	}
}

func BenchmarkNewIncremental_Claude(b *testing.B) {
	for _, size := range []int{100, 1000, 10000} {
		stream := generateClaudeStream(size)
		b.Run(fmt.Sprintf("chunks=%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				var sc StreamCollector
				for _, line := range stream {
					sc.ProcessSSELine(line)
				}
			}
		})
	}
}

// BenchmarkOldFullBuffer_Memory measures the memory cost of full-buffer approach.
// The old approach allocates a strings.Builder that grows to hold the entire stream,
// then creates a full copy via .String(), and splits it again into lines.
func BenchmarkOldFullBuffer_Memory(b *testing.B) {
	for _, size := range []int{100, 1000, 10000} {
		stream := generateCodexStream(size)
		b.Run(fmt.Sprintf("chunks=%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				// Simulate what the old handleStreamResponse did:
				// 1. Accumulate all lines into a strings.Builder
				var buf strings.Builder
				for _, line := range stream {
					buf.WriteString(line)
					buf.WriteByte('\n')
				}
				// 2. Extract metrics from the full buffer
				ExtractFromStreamContent(buf.String())
			}
		})
	}
}

func BenchmarkNewIncremental_Memory(b *testing.B) {
	for _, size := range []int{100, 1000, 10000} {
		stream := generateCodexStream(size)
		b.Run(fmt.Sprintf("chunks=%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				var sc StreamCollector
				for _, line := range stream {
					sc.ProcessSSELine(line)
				}
			}
		})
	}
}
