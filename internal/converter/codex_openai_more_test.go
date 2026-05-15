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
		Model: "codex-test",
		Input: []interface{}{
			map[string]interface{}{"type": "message", "role": "developer", "content": "follow instructions"},
			map[string]interface{}{"type": "message", "role": "user", "content": "use a tool"},
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
			Type: "web_search",
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
	if len(got.Messages) < 2 || got.Messages[0].Role != "system" || got.Messages[1].Role != "user" {
		t.Fatalf("unexpected converted roles: %#v", got.Messages)
	}
	if len(got.Tools) != 1 {
		t.Fatalf("tools len = %d, want only the function tool", len(got.Tools))
	}
	tool := got.Tools[0]
	if tool.Type != "function" || tool.Function.Name != "shell" {
		t.Fatalf("unexpected converted tool: %#v", tool)
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
