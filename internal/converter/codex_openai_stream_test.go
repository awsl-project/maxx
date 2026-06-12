package converter

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCodexToOpenAIStreamToolCalls(t *testing.T) {
	state := NewTransformState()
	conv := &codexToOpenAIResponse{}

	created := map[string]interface{}{
		"type": "response.created",
		"response": map[string]interface{}{
			"id": "resp_test_1",
		},
	}
	added := map[string]interface{}{
		"type":         "response.output_item.added",
		"output_index": 0,
		"item": map[string]interface{}{
			"id":      "fc_call1",
			"type":    "function_call",
			"call_id": "call1",
			"name":    "tool_alpha",
		},
	}
	doneItem := map[string]interface{}{
		"type": "response.output_item.done",
		"item": map[string]interface{}{
			"type":      "function_call",
			"call_id":   "call1",
			"name":      "tool_alpha",
			"arguments": `{"a":1}`,
		},
	}
	completed := map[string]interface{}{
		"type": "response.completed",
		"response": map[string]interface{}{
			"id": "resp_test_1",
		},
	}

	var out []byte
	for _, ev := range []interface{}{created, added, doneItem, completed} {
		chunk := FormatSSE("", ev)
		next, err := conv.TransformChunk(chunk, state)
		if err != nil {
			t.Fatalf("transform chunk error: %v", err)
		}
		out = append(out, next...)
	}

	events, _ := ParseSSE(string(out))
	if len(events) == 0 {
		t.Fatalf("no SSE events produced")
	}

	foundToolDelta := false
	foundFinishToolCalls := false

	for _, ev := range events {
		if ev.Event == "done" {
			continue
		}
		var chunk OpenAIStreamChunk
		if err := json.Unmarshal(ev.Data, &chunk); err != nil {
			t.Fatalf("invalid chunk JSON: %v", err)
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		if chunk.Choices[0].Delta != nil && len(chunk.Choices[0].Delta.ToolCalls) > 0 {
			tc := chunk.Choices[0].Delta.ToolCalls[0]
			if tc.Type == "function" && tc.Function.Arguments != "" {
				foundToolDelta = true
			}
		}
		if chunk.Choices[0].FinishReason == "tool_calls" {
			foundFinishToolCalls = true
		}
	}

	if !foundToolDelta {
		t.Fatalf("expected tool_calls delta in stream output")
	}
	if !foundFinishToolCalls {
		t.Fatalf("expected finish_reason=tool_calls in stream output")
	}
}

func TestCodexToClaudeStreamToolStopReason(t *testing.T) {
	state := NewTransformState()
	conv := &codexToClaudeResponse{}

	created := map[string]interface{}{
		"type": "response.created",
		"response": map[string]interface{}{
			"id": "resp_test_2",
		},
	}
	added := map[string]interface{}{
		"type":         "response.output_item.added",
		"output_index": 0,
		"item": map[string]interface{}{
			"id":      "fc_call2",
			"type":    "function_call",
			"call_id": "call2",
			"name":    "tool_beta",
		},
	}
	doneItem := map[string]interface{}{
		"type": "response.output_item.done",
		"item": map[string]interface{}{
			"type":      "function_call",
			"call_id":   "call2",
			"name":      "tool_beta",
			"arguments": `{"b":2}`,
		},
	}
	completed := map[string]interface{}{
		"type": "response.completed",
		"response": map[string]interface{}{
			"id": "resp_test_2",
		},
	}

	var out []byte
	for _, ev := range []interface{}{created, added, doneItem, completed} {
		chunk := FormatSSE("", ev)
		next, err := conv.TransformChunk(chunk, state)
		if err != nil {
			t.Fatalf("transform chunk error: %v", err)
		}
		out = append(out, next...)
	}

	events, _ := ParseSSE(string(out))
	if len(events) == 0 {
		t.Fatalf("no SSE events produced")
	}

	foundStopReason := false
	for _, ev := range events {
		if ev.Event != "message_delta" {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal(ev.Data, &payload); err != nil {
			t.Fatalf("invalid event JSON: %v", err)
		}
		if delta, ok := payload["delta"].(map[string]interface{}); ok {
			if sr, ok := delta["stop_reason"].(string); ok && sr == "tool_use" {
				foundStopReason = true
			}
		}
	}

	if !foundStopReason {
		t.Fatalf("expected stop_reason=tool_use in Claude stream output")
	}
}

func TestClaudeToCodexToolShortening(t *testing.T) {
	longName := "mcp__server__" + strings.Repeat("x", 80)
	claudeReq := map[string]interface{}{
		"model": "claude-3",
		"messages": []map[string]interface{}{
			{"role": "user", "content": "hi"},
		},
		"tools": []map[string]interface{}{
			{
				"name":         longName,
				"description":  "d",
				"input_schema": map[string]interface{}{"type": "object"},
			},
			{
				"type": "web_search_20250305",
			},
		},
	}

	raw, err := json.Marshal(claudeReq)
	if err != nil {
		t.Fatalf("marshal claude req: %v", err)
	}

	conv := &claudeToCodexRequest{}
	out, err := conv.Transform(raw, "gpt-5.2-codex", false)
	if err != nil {
		t.Fatalf("transform error: %v", err)
	}

	var codexReq CodexRequest
	if err := json.Unmarshal(out, &codexReq); err != nil {
		t.Fatalf("unmarshal codex req: %v", err)
	}

	if len(codexReq.Tools) != 2 {
		t.Fatalf("tools = %d, want 2", len(codexReq.Tools))
	}

	var fnTool *CodexTool
	var serverTool *CodexTool
	for i := range codexReq.Tools {
		switch codexReq.Tools[i].Type {
		case "function":
			fnTool = &codexReq.Tools[i]
		case "web_search_20250305":
			serverTool = &codexReq.Tools[i]
		}
	}

	if fnTool == nil || fnTool.Name == "" {
		t.Fatalf("missing function tool after transform")
	}
	if len(fnTool.Name) > maxToolNameLen {
		t.Fatalf("function tool name too long: %d", len(fnTool.Name))
	}
	if serverTool == nil {
		t.Fatalf("missing server tool type in codex tools")
	}
}

func TestCodexToOpenAIStreamToolCallArgumentDeltas(t *testing.T) {
	state := NewTransformState()
	conv := &codexToOpenAIResponse{}

	eventsIn := []interface{}{
		map[string]interface{}{"type": "response.created", "response": map[string]interface{}{"id": "resp_stream_delta"}},
		map[string]interface{}{
			"type":         "response.output_item.added",
			"output_index": 0,
			"item": map[string]interface{}{
				"id":      "fc_1",
				"type":    "function_call",
				"call_id": "call_1",
				"name":    "tool_alpha",
			},
		},
		map[string]interface{}{"type": "response.function_call_arguments.delta", "output_index": 0, "delta": `{"a"`},
		map[string]interface{}{"type": "response.function_call_arguments.delta", "output_index": 0, "delta": `:1}`},
		map[string]interface{}{
			"type":         "response.output_item.done",
			"output_index": 0,
			"item": map[string]interface{}{
				"type":      "function_call",
				"call_id":   "call_1",
				"name":      "tool_alpha",
				"arguments": `{"a":1}`,
			},
		},
		map[string]interface{}{"type": "response.completed", "response": map[string]interface{}{"id": "resp_stream_delta"}},
	}

	var out []byte
	for _, ev := range eventsIn {
		next, err := conv.TransformChunk(FormatSSE("", ev), state)
		if err != nil {
			t.Fatalf("transform chunk error: %v", err)
		}
		out = append(out, next...)
	}

	events, _ := ParseSSE(string(out))
	var nameChunk, firstArgDelta, secondArgDelta, duplicatedDoneArgs, finishToolCalls bool
	for _, ev := range events {
		if ev.Event == "done" {
			continue
		}
		var chunk OpenAIStreamChunk
		if err := json.Unmarshal(ev.Data, &chunk); err != nil {
			t.Fatalf("invalid chunk JSON: %v", err)
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		if choice.FinishReason == "tool_calls" {
			finishToolCalls = true
		}
		if choice.Delta == nil || len(choice.Delta.ToolCalls) == 0 {
			continue
		}
		call := choice.Delta.ToolCalls[0]
		if call.ID == "call_1" && call.Function.Name == "tool_alpha" {
			nameChunk = true
		}
		if call.Function.Arguments == `{"a"` {
			firstArgDelta = true
		}
		if call.Function.Arguments == `:1}` {
			secondArgDelta = true
		}
		if call.Function.Arguments == `{"a":1}` {
			duplicatedDoneArgs = true
		}
	}
	if !nameChunk || !firstArgDelta || !secondArgDelta || !finishToolCalls {
		t.Fatalf("missing expected stream tool call chunks; name=%v first=%v second=%v finish=%v output=%s", nameChunk, firstArgDelta, secondArgDelta, finishToolCalls, string(out))
	}
	if duplicatedDoneArgs {
		t.Fatalf("expected output_item.done not to duplicate completed arguments after deltas: %s", string(out))
	}
}
