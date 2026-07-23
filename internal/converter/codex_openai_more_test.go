package converter

import (
	"encoding/json"
	"testing"
)

func TestCodexToOpenAIRequest_ResponseInputString(t *testing.T) {
	req := CodexRequest{
		Model: "codex-test",
		Input: "hi",
	}
	body, _ := json.Marshal(req)
	conv := &codexToOpenAIRequest{}
	out, err := conv.Transform(body, "gpt-test", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var got OpenAIRequest
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Messages) != 1 || got.Messages[0].Role != "user" {
		t.Fatalf("unexpected messages")
	}
}

func TestCodexToOpenAIRequest_ConvertsResponseToolsToFunctionTools(t *testing.T) {
	req := CodexRequest{
		Model:        "codex-test",
		Instructions: "prefer safe edits",
		Input: []interface{}{
			map[string]interface{}{"type": "message", "role": "developer", "content": "follow instructions"},
			map[string]interface{}{"type": "message", "role": "user", "content": "use a tool"},
			map[string]interface{}{"type": "message", "role": "", "content": "missing role defaults to user"},
		},
		Tools: []CodexTool{{
			Type:        "function",
			Name:        "shell",
			Description: "run shell command",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"cmd": map[string]interface{}{"type": "string"},
				},
			},
		}, {
			Type:        "FUNCTION",
			Name:        "read_file",
			Description: "read a file",
		}, {
			Type: "web_search",
		}, {
			Type: "image_generation",
		}, {
			Type: "function",
		}},
	}
	body, _ := json.Marshal(req)
	conv := &codexToOpenAIRequest{}
	out, err := conv.Transform(body, "deepseek-test", true)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var got OpenAIRequest
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Model != "deepseek-test" || !got.Stream {
		t.Fatalf("unexpected model/stream: %#v", got)
	}
	if len(got.Messages) != 4 {
		t.Fatalf("messages len = %d, want 4: %#v", len(got.Messages), got.Messages)
	}
	wantRoles := []string{"system", "system", "user", "user"}
	for i, want := range wantRoles {
		if got.Messages[i].Role != want {
			t.Fatalf("message[%d].Role = %q, want %q; messages=%#v", i, got.Messages[i].Role, want, got.Messages)
		}
	}
	if len(got.Tools) != 2 {
		t.Fatalf("tools len = %d, want only the two named function tools: %#v", len(got.Tools), got.Tools)
	}
	if got.Tools[0].Type != "function" || got.Tools[0].Function.Name != "shell" {
		t.Fatalf("unexpected first converted tool: %#v", got.Tools[0])
	}
	if got.Tools[1].Type != "function" || got.Tools[1].Function.Name != "read_file" {
		t.Fatalf("unexpected second converted tool: %#v", got.Tools[1])
	}
}

func TestCodexToOpenAIRequest_PreservesToolCallRoundTripShape(t *testing.T) {
	req := CodexRequest{
		Model: "codex-test",
		Input: []interface{}{
			map[string]interface{}{
				"type":      "function_call",
				"call_id":   "call_shell_1",
				"name":      "shell",
				"arguments": `{"cmd":"pwd"}`,
			},
			map[string]interface{}{
				"type":    "function_call_output",
				"call_id": "call_shell_1",
				"output":  "workspace",
			},
		},
	}
	body, _ := json.Marshal(req)
	conv := &codexToOpenAIRequest{}
	out, err := conv.Transform(body, "deepseek-test", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var got OpenAIRequest
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("messages len = %d, want 2: %#v", len(got.Messages), got.Messages)
	}
	assistant := got.Messages[0]
	if assistant.Role != "assistant" || len(assistant.ToolCalls) != 1 {
		t.Fatalf("unexpected assistant tool call message: %#v", assistant)
	}
	if assistant.ToolCalls[0].ID != "call_shell_1" || assistant.ToolCalls[0].Function.Name != "shell" || assistant.ToolCalls[0].Function.Arguments != `{"cmd":"pwd"}` {
		t.Fatalf("unexpected tool call: %#v", assistant.ToolCalls[0])
	}
	tool := got.Messages[1]
	if tool.Role != "tool" || tool.ToolCallID != "call_shell_1" || tool.Content != "workspace" {
		t.Fatalf("unexpected tool output message: %#v", tool)
	}
}

func TestCodexToOpenAIResponse_StreamMore(t *testing.T) {
	conv := &codexToOpenAIResponse{}
	state := NewTransformState()

	created := map[string]interface{}{
		"type": "response.created",
		"response": map[string]interface{}{
			"id": "resp_1",
		},
	}
	if _, err := conv.TransformChunk(FormatSSE("", created), state); err != nil {
		t.Fatalf("TransformChunk created: %v", err)
	}
	delta := map[string]interface{}{
		"type": "response.output_item.delta",
		"delta": map[string]interface{}{
			"text": "hi",
		},
	}
	if _, err := conv.TransformChunk(FormatSSE("", delta), state); err != nil {
		t.Fatalf("TransformChunk delta: %v", err)
	}
	done := map[string]interface{}{
		"type": "response.done",
	}
	if _, err := conv.TransformChunk(FormatSSE("", done), state); err != nil {
		t.Fatalf("TransformChunk done: %v", err)
	}
}
