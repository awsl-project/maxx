package converter

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOpenAIToGeminiStream(t *testing.T) {
	chunk := OpenAIStreamChunk{
		ID: "chat_1",
		Choices: []OpenAIChoice{{
			Delta: &OpenAIMessage{
				ReasoningContent: "think",
				Content:          "hi",
				ToolCalls: []OpenAIToolCall{{
					Index: 0,
					ID:    "call_1",
					Type:  "function",
					Function: OpenAIFunctionCall{
						Name:      "tool",
						Arguments: `{"x":1}`,
					},
				}},
			},
			FinishReason: "stop",
		}},
	}
	chunkBody, _ := json.Marshal(chunk)
	state := NewTransformState()
	respConv := &openaiToGeminiResponse{}
	stream := append(FormatSSE("", json.RawMessage(chunkBody)), FormatDone()...)
	out, err := respConv.TransformChunk(stream, state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if !strings.Contains(string(out), "thought") {
		t.Fatalf("missing thought part")
	}
	if !strings.Contains(string(out), "functionCall") {
		t.Fatalf("missing functionCall")
	}
}

func TestGeminiToOpenAIRequestStreamAndSplit(t *testing.T) {
	req := GeminiRequest{
		SystemInstruction: &GeminiContent{Parts: []GeminiPart{{Text: "sys"}}},
		GenerationConfig:  &GeminiGenerationConfig{StopSequences: []string{"x"}, ThinkingConfig: &GeminiThinkingConfig{ThinkingBudget: 0}},
		Contents: []GeminiContent{{
			Role: "model",
			Parts: []GeminiPart{{
				Thought: true,
				Text:    "think",
			}, {
				Text: "hi",
			}, {
				InlineData: &GeminiInlineData{MimeType: "image/png", Data: "Zm9v"},
			}, {
				FunctionResponse: &GeminiFunctionResponse{Name: "tool_call_1", Response: map[string]interface{}{"ok": true}},
			}},
		}},
		Tools: []GeminiTool{{FunctionDeclarations: []GeminiFunctionDecl{{Name: "tool"}}}},
	}
	body, _ := json.Marshal(req)
	conv := &geminiToOpenAIRequest{}
	out, err := conv.Transform(body, "gpt", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var openaiReq OpenAIRequest
	if err := json.Unmarshal(out, &openaiReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(openaiReq.Messages) == 0 {
		t.Fatalf("messages missing")
	}

	if name, callID := splitFunctionName("tool_call_1"); name != "tool" || callID != "call_1" {
		t.Fatalf("splitFunctionName mismatch: %s %s", name, callID)
	}

	streamChunk := GeminiStreamChunk{Candidates: []GeminiCandidate{{Content: GeminiContent{Role: "model", Parts: []GeminiPart{{Text: "hi"}, {Thought: true, Text: "t"}}}, FinishReason: "MAX_TOKENS", Index: 0}}}
	streamBody, _ := json.Marshal(streamChunk)
	state := NewTransformState()
	respConv := &geminiToOpenAIResponse{}
	streamOut, err := respConv.TransformChunk(FormatSSE("", json.RawMessage(streamBody)), state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if !strings.Contains(string(streamOut), "chat.completion.chunk") {
		t.Fatalf("missing openai chunk")
	}
}

func TestOpenAIToGeminiStreamFinishLength(t *testing.T) {
	chunk := OpenAIStreamChunk{
		ID: "chat_len",
		Choices: []OpenAIChoice{{
			Delta:        &OpenAIMessage{Content: "hi"},
			FinishReason: "length",
		}},
	}
	body, _ := json.Marshal(chunk)
	state := NewTransformState()
	conv := &openaiToGeminiResponse{}
	out, err := conv.TransformChunk(FormatSSE("", json.RawMessage(body)), state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if !strings.Contains(string(out), "MAX_TOKENS") {
		t.Fatalf("missing MAX_TOKENS finish reason")
	}
}

func TestOpenAIToCodexRequestAndStream(t *testing.T) {
	req := OpenAIRequest{
		Model:               "gpt",
		MaxCompletionTokens: 5,
		ReasoningEffort:     "auto",
		Messages: []OpenAIMessage{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "hi"},
			{Role: "assistant", ToolCalls: []OpenAIToolCall{{
				ID:   "call_1",
				Type: "function",
				Function: OpenAIFunctionCall{
					Name:      "tool",
					Arguments: `{"x":1}`,
				},
			}}},
			{Role: "tool", ToolCallID: "call_1", Content: "ok"},
		},
		Tools: []OpenAITool{{
			Type: "function",
			Function: OpenAIFunction{
				Name:        "tool",
				Description: "d",
				Parameters:  map[string]interface{}{"type": "object"},
			},
		}},
	}
	body, _ := json.Marshal(req)
	conv := &openaiToCodexRequest{}
	out, err := conv.Transform(body, "codex", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var codexReq CodexRequest
	if err := json.Unmarshal(out, &codexReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !codexInputHasRoleText(codexReq.Input, "developer", "sys") {
		t.Fatalf("expected system message")
	}
	if codexReq.Reasoning == nil || codexReq.Reasoning.Effort != "auto" {
		t.Fatalf("reasoning missing")
	}
	if codexReq.ParallelToolCalls == nil || !*codexReq.ParallelToolCalls {
		t.Fatalf("parallel tool calls missing")
	}

	chunk := OpenAIStreamChunk{ID: "chat_1", Model: "gpt", Choices: []OpenAIChoice{{
		Delta:        &OpenAIMessage{Content: "hi"},
		FinishReason: "stop",
	}}}
	chunkBody, _ := json.Marshal(chunk)
	state := NewTransformState()
	respConv := &openaiToCodexResponse{}
	stream := append(FormatSSE("", json.RawMessage(chunkBody)), FormatDone()...)
	streamOut, err := respConv.TransformChunk(stream, state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if !strings.Contains(string(streamOut), "response.created") {
		t.Fatalf("missing response.created")
	}
	if !strings.Contains(string(streamOut), "response.output_text.delta") {
		t.Fatalf("missing delta")
	}
	if !strings.Contains(string(streamOut), "response.completed") {
		t.Fatalf("missing completed")
	}
	if !strings.Contains(string(streamOut), "data: [DONE]") {
		t.Fatalf("missing terminal done sentinel")
	}
}

func TestOpenAIToCodexStreamForwardsDoneWhenSplitFromCompletion(t *testing.T) {
	state := NewTransformState()
	respConv := &openaiToCodexResponse{}
	chunk := OpenAIStreamChunk{ID: "chat_1", Model: "gpt", Choices: []OpenAIChoice{{
		Delta:        &OpenAIMessage{Content: "hi"},
		FinishReason: "stop",
	}}}
	chunkBody, _ := json.Marshal(chunk)
	firstOut, err := respConv.TransformChunk(FormatSSE("", json.RawMessage(chunkBody)), state)
	if err != nil {
		t.Fatalf("TransformChunk first: %v", err)
	}
	if !strings.Contains(string(firstOut), "response.completed") {
		t.Fatalf("missing completed before done: %s", string(firstOut))
	}
	doneOut, err := respConv.TransformChunk(FormatDone(), state)
	if err != nil {
		t.Fatalf("TransformChunk done: %v", err)
	}
	if string(doneOut) != string(FormatDone()) {
		t.Fatalf("expected terminal done sentinel, got: %q", string(doneOut))
	}
	duplicateOut, err := respConv.TransformChunk(FormatDone(), state)
	if err != nil {
		t.Fatalf("TransformChunk duplicate done: %v", err)
	}
	if len(duplicateOut) != 0 {
		t.Fatalf("expected duplicate done to be suppressed, got: %q", string(duplicateOut))
	}
}

func TestOpenAIToCodexStreamForwardsNativeResponsesEvents(t *testing.T) {
	state := NewTransformState()
	respConv := &openaiToCodexResponse{}

	created := map[string]interface{}{
		"type": "response.created",
		"response": map[string]interface{}{
			"id":         "resp_native",
			"object":     "response",
			"created_at": int64(1),
			"status":     "in_progress",
		},
	}
	delta := map[string]interface{}{
		"type":          "response.output_text.delta",
		"item_id":       "msg_resp_native_0",
		"output_index":  0,
		"content_index": 0,
		"delta":         "hi",
	}
	completed := map[string]interface{}{
		"type": "response.completed",
		"response": map[string]interface{}{
			"id":     "resp_native",
			"object": "response",
			"status": "completed",
		},
	}

	stream := append(FormatSSE("", created), FormatSSE("", delta)...)
	stream = append(stream, FormatSSE("", completed)...)
	stream = append(stream, FormatDone()...)

	out, err := respConv.TransformChunk(stream, state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	outStr := string(out)
	for _, want := range []string{"event: response.created", "event: response.output_text.delta", "event: response.completed", "data: [DONE]"} {
		if !strings.Contains(outStr, want) {
			t.Fatalf("missing %s in forwarded responses stream: %s", want, outStr)
		}
	}
	if strings.Count(outStr, "response.completed") != 2 {
		t.Fatalf("expected one completed event/data pair, got: %s", outStr)
	}
}

func TestClaudeToOpenAIStreamToolCalls(t *testing.T) {
	state := NewTransformState()
	start := ClaudeStreamEvent{Type: "message_start", Message: &ClaudeResponse{ID: "msg_1"}}
	startBody, _ := json.Marshal(start)
	blockStart := ClaudeStreamEvent{Type: "content_block_start", Index: 0, ContentBlock: &ClaudeContentBlock{Type: "tool_use", ID: "call_1", Name: "tool"}}
	blockBody, _ := json.Marshal(blockStart)
	delta := ClaudeStreamEvent{Type: "content_block_delta", Delta: &ClaudeStreamDelta{Type: "input_json_delta", PartialJSON: `{"x":1}`}}
	deltaBody, _ := json.Marshal(delta)
	msgDelta := ClaudeStreamEvent{Type: "message_delta", Delta: &ClaudeStreamDelta{StopReason: "tool_use"}, Usage: &ClaudeUsage{OutputTokens: 2}}
	msgDeltaBody, _ := json.Marshal(msgDelta)
	stop := ClaudeStreamEvent{Type: "message_stop"}
	stopBody, _ := json.Marshal(stop)

	stream := append(FormatSSE("", json.RawMessage(startBody)), FormatSSE("", json.RawMessage(blockBody))...)
	stream = append(stream, FormatSSE("", json.RawMessage(deltaBody))...)
	stream = append(stream, FormatSSE("", json.RawMessage(msgDeltaBody))...)
	stream = append(stream, FormatSSE("", json.RawMessage(stopBody))...)

	conv := &claudeToOpenAIResponse{}
	out, err := conv.TransformChunk(stream, state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if !strings.Contains(string(out), "tool_calls") {
		t.Fatalf("missing tool_calls finish")
	}
}

func TestOpenAIToClaudeStreamThinkingAndTool(t *testing.T) {
	chunk := OpenAIStreamChunk{ID: "chat_1", Model: "gpt", Choices: []OpenAIChoice{{
		Delta: &OpenAIMessage{
			ReasoningContent: "think",
			Content:          "hi",
			ToolCalls: []OpenAIToolCall{{
				Index: 0,
				ID:    "call_1",
				Type:  "function",
				Function: OpenAIFunctionCall{
					Name:      "tool",
					Arguments: `{"x":1}`,
				},
			}},
		},
		FinishReason: "length",
	}}}
	body, _ := json.Marshal(chunk)
	state := NewTransformState()
	conv := &openaiToClaudeResponse{}
	stream := append(FormatSSE("", json.RawMessage(body)), FormatDone()...)
	out, err := conv.TransformChunk(stream, state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if !strings.Contains(string(out), "thinking_delta") {
		t.Fatalf("missing thinking delta")
	}
	if !strings.Contains(string(out), "input_json_delta") {
		t.Fatalf("missing tool input delta")
	}
	if !strings.Contains(string(out), "message_stop") {
		t.Fatalf("missing message_stop")
	}
}

func TestGeminiToOpenAIStreamInlineAndFinish(t *testing.T) {
	state := NewTransformState()
	chunk := GeminiStreamChunk{Candidates: []GeminiCandidate{{
		Content:      GeminiContent{Role: "model", Parts: []GeminiPart{{InlineData: &GeminiInlineData{MimeType: "image/png", Data: "Zm9v"}}, {Text: "hi"}}},
		FinishReason: "MAX_TOKENS",
		Index:        0,
	}}}
	body, _ := json.Marshal(chunk)
	conv := &geminiToOpenAIResponse{}
	out, err := conv.TransformChunk(FormatSSE("", json.RawMessage(body)), state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if !strings.Contains(string(out), "image_url") {
		t.Fatalf("missing image_url")
	}
	if !strings.Contains(string(out), "finish_reason") {
		t.Fatalf("missing finish reason")
	}
}

func TestGeminiToOpenAIStreamFunctionCall(t *testing.T) {
	state := NewTransformState()
	chunk := GeminiStreamChunk{Candidates: []GeminiCandidate{{
		Content: GeminiContent{Role: "model", Parts: []GeminiPart{{FunctionCall: &GeminiFunctionCall{Name: "tool", Args: map[string]interface{}{"x": 1}}}}},
		Index:   0,
	}}}
	body, _ := json.Marshal(chunk)
	conv := &geminiToOpenAIResponse{}
	out, err := conv.TransformChunk(FormatSSE("", json.RawMessage(body)), state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if !strings.Contains(string(out), "tool_calls") {
		t.Fatalf("missing tool_calls")
	}
}

func TestClaudeToOpenAIStreamThinkingDelta(t *testing.T) {
	state := NewTransformState()
	start := ClaudeStreamEvent{Type: "message_start", Message: &ClaudeResponse{ID: "msg_2"}}
	startBody, _ := json.Marshal(start)
	blockStart := ClaudeStreamEvent{Type: "content_block_start", Index: 0, ContentBlock: &ClaudeContentBlock{Type: "thinking"}}
	blockBody, _ := json.Marshal(blockStart)
	delta := ClaudeStreamEvent{Type: "content_block_delta", Delta: &ClaudeStreamDelta{Type: "thinking_delta", Thinking: "t"}}
	deltaBody, _ := json.Marshal(delta)
	stop := ClaudeStreamEvent{Type: "message_stop"}
	stopBody, _ := json.Marshal(stop)

	stream := append(FormatSSE("", json.RawMessage(startBody)), FormatSSE("", json.RawMessage(blockBody))...)
	stream = append(stream, FormatSSE("", json.RawMessage(deltaBody))...)
	stream = append(stream, FormatSSE("", json.RawMessage(stopBody))...)

	conv := &claudeToOpenAIResponse{}
	out, err := conv.TransformChunk(stream, state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if !strings.Contains(string(out), "reasoning_content") {
		t.Fatalf("missing reasoning_content")
	}
}

func TestOpenAIToClaudeStreamToolUpdate(t *testing.T) {
	state := NewTransformState()
	chunk1 := OpenAIStreamChunk{ID: "chat_2", Model: "gpt", Choices: []OpenAIChoice{{
		Delta: &OpenAIMessage{ToolCalls: []OpenAIToolCall{{Index: 0, ID: "call_1", Type: "function", Function: OpenAIFunctionCall{Name: "tool"}}}},
	}}}
	chunk2 := OpenAIStreamChunk{ID: "chat_2", Choices: []OpenAIChoice{{
		Delta:        &OpenAIMessage{ToolCalls: []OpenAIToolCall{{Index: 0, Function: OpenAIFunctionCall{Arguments: `{"x":1}`}}}},
		FinishReason: "stop",
	}}}
	b1, _ := json.Marshal(chunk1)
	b2, _ := json.Marshal(chunk2)
	stream := append(FormatSSE("", json.RawMessage(b1)), FormatSSE("", json.RawMessage(b2))...)
	stream = append(stream, FormatDone()...)
	conv := &openaiToClaudeResponse{}
	out, err := conv.TransformChunk(stream, state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if !strings.Contains(string(out), "tool_use") {
		t.Fatalf("missing tool_use")
	}
}

func TestOpenAIToClaudeStreamTextOnly(t *testing.T) {
	state := NewTransformState()
	chunk := OpenAIStreamChunk{ID: "chat_3", Model: "gpt", Choices: []OpenAIChoice{{
		Delta: &OpenAIMessage{Content: "hi"},
	}}}
	body, _ := json.Marshal(chunk)
	conv := &openaiToClaudeResponse{}
	out, err := conv.TransformChunk(FormatSSE("", json.RawMessage(body)), state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if !strings.Contains(string(out), "text_delta") {
		t.Fatalf("expected text_delta")
	}
}

func TestOpenAIToClaudeStreamUsage(t *testing.T) {
	state := NewTransformState()
	chunk := OpenAIStreamChunk{ID: "chat_u", Model: "gpt", Usage: &OpenAIUsage{PromptTokens: 1, CompletionTokens: 2}, Choices: []OpenAIChoice{{
		Delta: &OpenAIMessage{Content: "hi"},
	}}}
	body, _ := json.Marshal(chunk)
	conv := &openaiToClaudeResponse{}
	out, err := conv.TransformChunk(FormatSSE("", json.RawMessage(body)), state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if state.Usage.OutputTokens != 2 {
		t.Fatalf("expected output tokens")
	}
	if !strings.Contains(string(out), "message_start") {
		t.Fatalf("expected message_start")
	}
}

func TestOpenAIToClaudeStreamReasoningThenText(t *testing.T) {
	state := NewTransformState()
	chunk := OpenAIStreamChunk{ID: "chat_rt", Model: "gpt", Choices: []OpenAIChoice{{
		Delta: &OpenAIMessage{ReasoningContent: "think", Content: "hi"},
	}}}
	body, _ := json.Marshal(chunk)
	conv := &openaiToClaudeResponse{}
	out, err := conv.TransformChunk(FormatSSE("", json.RawMessage(body)), state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if !strings.Contains(string(out), "thinking_delta") {
		t.Fatalf("expected thinking_delta")
	}
	if !strings.Contains(string(out), "text_delta") {
		t.Fatalf("expected text_delta")
	}
}

func TestClaudeToOpenAIStreamStopReason(t *testing.T) {
	state := NewTransformState()
	msgDelta := ClaudeStreamEvent{Type: "message_delta", Delta: &ClaudeStreamDelta{StopReason: "max_tokens"}, Usage: &ClaudeUsage{OutputTokens: 1}}
	msgDeltaBody, _ := json.Marshal(msgDelta)
	stop := ClaudeStreamEvent{Type: "message_stop"}
	stopBody, _ := json.Marshal(stop)
	stream := append(FormatSSE("", json.RawMessage(msgDeltaBody)), FormatSSE("", json.RawMessage(stopBody))...)
	conv := &claudeToOpenAIResponse{}
	out, err := conv.TransformChunk(stream, state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if !strings.Contains(string(out), "length") {
		t.Fatalf("expected length finish reason")
	}
}

func TestOpenAIToClaudeStreamToolFinishReason(t *testing.T) {
	state := NewTransformState()
	chunk := OpenAIStreamChunk{ID: "chat_tc", Model: "gpt", Choices: []OpenAIChoice{{
		Delta:        &OpenAIMessage{ToolCalls: []OpenAIToolCall{{Index: 0, ID: "call_1", Type: "function", Function: OpenAIFunctionCall{Name: "tool", Arguments: `{"x":1}`}}}},
		FinishReason: "tool_calls",
	}}}
	body, _ := json.Marshal(chunk)
	conv := &openaiToClaudeResponse{}
	out, err := conv.TransformChunk(append(FormatSSE("", json.RawMessage(body)), FormatDone()...), state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if !strings.Contains(string(out), "tool_use") {
		t.Fatalf("expected tool_use stop reason")
	}
}

func TestOpenAIToClaudeStreamThinkingThenText(t *testing.T) {
	state := NewTransformState()
	chunk := OpenAIStreamChunk{ID: "chat_tt", Model: "gpt", Choices: []OpenAIChoice{{
		Delta:        &OpenAIMessage{ReasoningContent: "think", Content: "hi"},
		FinishReason: "stop",
	}}}
	body, _ := json.Marshal(chunk)
	conv := &openaiToClaudeResponse{}
	out, err := conv.TransformChunk(append(FormatSSE("", json.RawMessage(body)), FormatDone()...), state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if !strings.Contains(string(out), "content_block_stop") {
		t.Fatalf("expected content_block_stop between blocks")
	}
}

func TestOpenAIToClaudeStreamReasoningOnly(t *testing.T) {
	state := NewTransformState()
	chunk := OpenAIStreamChunk{ID: "chat_r", Model: "gpt", Choices: []OpenAIChoice{{
		Delta: &OpenAIMessage{ReasoningContent: "think"},
	}}}
	body, _ := json.Marshal(chunk)
	conv := &openaiToClaudeResponse{}
	out, err := conv.TransformChunk(FormatSSE("", json.RawMessage(body)), state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if !strings.Contains(string(out), "thinking_delta") {
		t.Fatalf("expected thinking_delta")
	}
}

func TestOpenAIToClaudeStreamToolUpdateName(t *testing.T) {
	state := NewTransformState()
	chunk1 := OpenAIStreamChunk{ID: "chat_n", Model: "gpt", Choices: []OpenAIChoice{{
		Delta: &OpenAIMessage{ToolCalls: []OpenAIToolCall{{Index: 0, ID: "call_1", Type: "function", Function: OpenAIFunctionCall{Name: "tool"}}}},
	}}}
	chunk2 := OpenAIStreamChunk{ID: "chat_n", Choices: []OpenAIChoice{{
		Delta:        &OpenAIMessage{ToolCalls: []OpenAIToolCall{{Index: 0, Function: OpenAIFunctionCall{Name: "tool2", Arguments: `{"x":1}`}}}},
		FinishReason: "stop",
	}}}
	b1, _ := json.Marshal(chunk1)
	b2, _ := json.Marshal(chunk2)
	stream := append(FormatSSE("", json.RawMessage(b1)), FormatSSE("", json.RawMessage(b2))...)
	stream = append(stream, FormatDone()...)
	conv := &openaiToClaudeResponse{}
	out, err := conv.TransformChunk(stream, state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if !strings.Contains(string(out), "tool_use") {
		t.Fatalf("expected tool_use")
	}
}

func TestOpenAIToClaudeStreamFinishLengthNoTool(t *testing.T) {
	state := NewTransformState()
	chunk := OpenAIStreamChunk{ID: "chat_l", Model: "gpt", Choices: []OpenAIChoice{{
		Delta:        &OpenAIMessage{Content: "hi"},
		FinishReason: "length",
	}}}
	body, _ := json.Marshal(chunk)
	conv := &openaiToClaudeResponse{}
	out, err := conv.TransformChunk(append(FormatSSE("", json.RawMessage(body)), FormatDone()...), state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if !strings.Contains(string(out), "max_tokens") {
		t.Fatalf("expected max_tokens stop reason")
	}
}

func TestOpenAIToClaudeStreamDoneWithoutMessage(t *testing.T) {
	state := NewTransformState()
	conv := &openaiToClaudeResponse{}
	out, err := conv.TransformChunk(FormatDone(), state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected no output")
	}
}

func TestCodexToOpenAIStreamDoneFlow(t *testing.T) {
	state := NewTransformState()
	created := map[string]interface{}{"type": "response.created", "response": map[string]interface{}{"id": "resp_1"}}
	delta := map[string]interface{}{"type": "response.output_text.delta", "delta": "hi"}
	completed := map[string]interface{}{"type": "response.completed", "response": map[string]interface{}{"usage": map[string]interface{}{"input_tokens": 1}}}
	c1, _ := json.Marshal(created)
	c2, _ := json.Marshal(delta)
	c3, _ := json.Marshal(completed)
	stream := append(FormatSSE("", json.RawMessage(c1)), FormatSSE("", json.RawMessage(c2))...)
	stream = append(stream, FormatSSE("", json.RawMessage(c3))...)
	stream = append(stream, FormatDone()...)
	conv := &codexToOpenAIResponse{}
	out, err := conv.TransformChunk(stream, state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if !strings.Contains(string(out), "chat.completion.chunk") {
		t.Fatalf("expected openai chunk")
	}
}

func TestOpenAIToClaudeStreamInvalidJSON(t *testing.T) {
	state := NewTransformState()
	conv := &openaiToClaudeResponse{}
	out, err := conv.TransformChunk(FormatSSE("", "\"oops\""), state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected no output")
	}
	if state.MessageID != "" {
		t.Fatalf("unexpected message id")
	}
}

func TestOpenAIToClaudeStreamNoChoices(t *testing.T) {
	state := NewTransformState()
	chunk := OpenAIStreamChunk{ID: "msg_1", Model: "gpt", Choices: []OpenAIChoice{}}
	body, _ := json.Marshal(chunk)
	conv := &openaiToClaudeResponse{}
	out, err := conv.TransformChunk(FormatSSE("", json.RawMessage(body)), state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected no output")
	}
	if state.MessageID != "" {
		t.Fatalf("expected no message start")
	}
}

func TestOpenAIToClaudeStreamReasoningAfterText(t *testing.T) {
	state := NewTransformState()
	conv := &openaiToClaudeResponse{}
	first := OpenAIStreamChunk{ID: "msg_1", Model: "gpt", Choices: []OpenAIChoice{{
		Delta: &OpenAIMessage{Content: "hello"},
	}}}
	body1, _ := json.Marshal(first)
	if _, err := conv.TransformChunk(FormatSSE("", json.RawMessage(body1)), state); err != nil {
		t.Fatalf("TransformChunk first: %v", err)
	}
	second := OpenAIStreamChunk{ID: "msg_1", Model: "gpt", Choices: []OpenAIChoice{{
		Delta: &OpenAIMessage{ReasoningContent: "think"},
	}}}
	body2, _ := json.Marshal(second)
	out, err := conv.TransformChunk(FormatSSE("", json.RawMessage(body2)), state)
	if err != nil {
		t.Fatalf("TransformChunk second: %v", err)
	}
	if !strings.Contains(string(out), "content_block_stop") || !strings.Contains(string(out), "thinking_delta") {
		t.Fatalf("expected reasoning transition output")
	}
}

func TestOpenAIToClaudeStreamToolCallIDUpdate(t *testing.T) {
	state := NewTransformState()
	conv := &openaiToClaudeResponse{}
	first := OpenAIStreamChunk{ID: "msg_1", Model: "gpt", Choices: []OpenAIChoice{{
		Delta: &OpenAIMessage{ToolCalls: []OpenAIToolCall{{
			Index:    0,
			ID:       "",
			Type:     "function",
			Function: OpenAIFunctionCall{Name: "tool"},
		}}},
	}}}
	body1, _ := json.Marshal(first)
	if _, err := conv.TransformChunk(FormatSSE("", json.RawMessage(body1)), state); err != nil {
		t.Fatalf("TransformChunk first: %v", err)
	}
	second := OpenAIStreamChunk{ID: "msg_1", Model: "gpt", Choices: []OpenAIChoice{{
		Delta: &OpenAIMessage{ToolCalls: []OpenAIToolCall{{
			Index:    0,
			ID:       "call_1",
			Type:     "function",
			Function: OpenAIFunctionCall{},
		}}},
	}}}
	body2, _ := json.Marshal(second)
	if _, err := conv.TransformChunk(FormatSSE("", json.RawMessage(body2)), state); err != nil {
		t.Fatalf("TransformChunk second: %v", err)
	}
	if state.ToolCalls[0].ID != "call_1" {
		t.Fatalf("expected updated tool call id")
	}
}

func TestClaudeToOpenAIStreamDoneAndEndTurn(t *testing.T) {
	state := NewTransformState()
	conv := &claudeToOpenAIResponse{}

	start := ClaudeStreamEvent{
		Type:    "message_start",
		Message: &ClaudeResponse{ID: "msg_1"},
	}
	delta := ClaudeStreamEvent{
		Type:  "message_delta",
		Delta: &ClaudeStreamDelta{StopReason: "end_turn"},
	}
	stop := ClaudeStreamEvent{Type: "message_stop"}
	s1, _ := json.Marshal(start)
	s2, _ := json.Marshal(delta)
	s3, _ := json.Marshal(stop)
	stream := append(FormatSSE("", json.RawMessage(s1)), FormatSSE("", json.RawMessage(s2))...)
	stream = append(stream, FormatSSE("", json.RawMessage(s3))...)
	out, err := conv.TransformChunk(stream, state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if !strings.Contains(string(out), "\"finish_reason\":\"stop\"") {
		t.Fatalf("expected stop finish reason")
	}

	out, err = conv.TransformChunk(FormatDone(), state)
	if err != nil {
		t.Fatalf("TransformChunk done: %v", err)
	}
	if !strings.Contains(string(out), "[DONE]") {
		t.Fatalf("expected done marker")
	}
}

func TestClaudeToOpenAIStreamInvalidJSON(t *testing.T) {
	state := NewTransformState()
	conv := &claudeToOpenAIResponse{}
	out, err := conv.TransformChunk(FormatSSE("", "\"oops\""), state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected no output")
	}
}

func TestOpenAIToCodexStreamInvalidJSON(t *testing.T) {
	state := NewTransformState()
	conv := &openaiToCodexResponse{}
	out, err := conv.TransformChunk(FormatSSE("", "\"oops\""), state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected no output")
	}
}

func TestCodexToOpenAIStreamInvalidJSON(t *testing.T) {
	state := NewTransformState()
	conv := &codexToOpenAIResponse{}
	out, err := conv.TransformChunk(FormatSSE("", "\"oops\""), state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected no output")
	}
}

func TestOpenAIToGeminiStreamInvalidJSONAndToolInit(t *testing.T) {
	state := NewTransformState()
	state.ToolCalls = nil
	conv := &openaiToGeminiResponse{}
	chunk := OpenAIStreamChunk{Choices: []OpenAIChoice{{
		Delta: &OpenAIMessage{ToolCalls: []OpenAIToolCall{{
			Index:    0,
			ID:       "call_1",
			Type:     "function",
			Function: OpenAIFunctionCall{Name: "tool", Arguments: "{}"},
		}}},
	}}}
	body, _ := json.Marshal(chunk)
	if _, err := conv.TransformChunk(FormatSSE("", json.RawMessage(body)), state); err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	out, err := conv.TransformChunk(FormatSSE("", "\"oops\""), state)
	if err != nil {
		t.Fatalf("TransformChunk invalid: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected no output")
	}
}

func TestOpenAIToClaudeStreamToolInitNilMap(t *testing.T) {
	state := NewTransformState()
	state.ToolCalls = nil
	conv := &openaiToClaudeResponse{}
	chunk := OpenAIStreamChunk{ID: "msg_1", Model: "gpt", Choices: []OpenAIChoice{{
		Delta: &OpenAIMessage{ToolCalls: []OpenAIToolCall{{
			Index:    0,
			ID:       "call_1",
			Type:     "function",
			Function: OpenAIFunctionCall{Name: "tool", Arguments: "{}"},
		}}},
	}}}
	body, _ := json.Marshal(chunk)
	_, err := conv.TransformChunk(FormatSSE("", json.RawMessage(body)), state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if state.ToolCalls == nil {
		t.Fatalf("expected tool calls map initialized")
	}
}

func TestGeminiToOpenAIStreamDoneAndInvalidJSON(t *testing.T) {
	state := NewTransformState()
	conv := &geminiToOpenAIResponse{}
	out, err := conv.TransformChunk(FormatDone(), state)
	if err != nil {
		t.Fatalf("TransformChunk: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected no output on done")
	}
	out, err = conv.TransformChunk(FormatSSE("", "\"oops\""), state)
	if err != nil {
		t.Fatalf("TransformChunk invalid: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected no output")
	}
}
