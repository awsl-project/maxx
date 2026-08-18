package converter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

const applyPatchRaw = "*** Begin Patch\n*** Update File: notes.txt\n@@\n first line\n+hello\n*** End Patch\n"

// A Codex freeform tool (apply_patch, type "custom") must survive the OpenAI-Chat
// bridge as a function taking a single raw-text `input`, instead of being dropped.
func TestCodexToOpenAIRequest_WrapsFreeformCustomTool(t *testing.T) {
	req := CodexRequest{
		Model: "codex-test",
		Tools: []CodexTool{
			{Type: "function", Name: "shell", Description: "run", Parameters: map[string]interface{}{"type": "object"}},
			{Type: "custom", Name: "apply_patch", Description: "FREEFORM tool"},
			{Type: "web_search"}, // unrepresentable, no name -> dropped
		},
	}
	body, _ := json.Marshal(req)
	out, err := (&codexToOpenAIRequest{}).Transform(body, "gpt-test", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var got OpenAIRequest
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Tools) != 2 {
		t.Fatalf("tools len = %d, want 2 (shell + apply_patch): %#v", len(got.Tools), got.Tools)
	}
	// apply_patch must be a function tool with a required string `input` param.
	ap := gjson.GetBytes(out, `tools.#(function.name=="apply_patch")`)
	if !ap.Exists() {
		t.Fatalf("apply_patch not converted: %s", out)
	}
	if ap.Get("type").String() != "function" {
		t.Fatalf("apply_patch type = %q, want function", ap.Get("type").String())
	}
	if ap.Get("function.parameters.properties.input.type").String() != "string" {
		t.Fatalf("apply_patch missing string input param: %s", ap.Raw)
	}
	if req0 := ap.Get(`function.parameters.required.0`).String(); req0 != "input" {
		t.Fatalf("apply_patch required = %q, want input", req0)
	}
}

// A completed freeform tool round-trip (custom_tool_call + custom_tool_call_output)
// must map to an assistant tool_call whose args re-wrap the raw text and a tool
// message keyed on the same call_id.
func TestCodexToOpenAIRequest_FreeformToolCallRoundTrip(t *testing.T) {
	req := CodexRequest{
		Model: "codex-test",
		Input: []interface{}{
			map[string]interface{}{"type": "custom_tool_call", "id": "ctc_1", "call_id": "call_ap1", "name": "apply_patch", "input": applyPatchRaw},
			map[string]interface{}{"type": "custom_tool_call_output", "call_id": "call_ap1", "output": "Success"},
		},
	}
	body, _ := json.Marshal(req)
	out, err := (&codexToOpenAIRequest{}).Transform(body, "gpt-test", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var got OpenAIRequest
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("messages = %d, want 2: %#v", len(got.Messages), got.Messages)
	}
	asst := got.Messages[0]
	if asst.Role != "assistant" || len(asst.ToolCalls) != 1 {
		t.Fatalf("unexpected assistant message: %#v", asst)
	}
	tc := asst.ToolCalls[0]
	if tc.ID != "call_ap1" || tc.Function.Name != "apply_patch" {
		t.Fatalf("unexpected tool call id/name: %#v", tc)
	}
	if in := gjson.Get(tc.Function.Arguments, "input").String(); in != applyPatchRaw {
		t.Fatalf("args input = %q, want raw patch; args=%q", in, tc.Function.Arguments)
	}
	if tool := got.Messages[1]; tool.Role != "tool" || tool.ToolCallID != "call_ap1" || tool.Content != "Success" {
		t.Fatalf("unexpected tool output message: %#v", tool)
	}
}

// Non-stream: an upstream function tool_call whose name was declared freeform in
// the original Codex request must be re-emitted as a custom_tool_call carrying the
// unwrapped raw input; a normal function tool_call stays a function_call.
func TestOpenAIToCodexResponse_EmitsCustomToolCallForFreeform(t *testing.T) {
	origReq := `{"tools":[{"type":"custom","name":"apply_patch"},{"type":"function","name":"shell"}]}`
	args := wrapFreeformInputAsArgs(applyPatchRaw)
	resp := `{"id":"chatcmpl-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","tool_calls":[` +
		`{"id":"call_ap1","type":"function","function":{"name":"apply_patch","arguments":` + mustJSON(args) + `}},` +
		`{"id":"call_sh1","type":"function","function":{"name":"shell","arguments":"{\"cmd\":\"ls\"}"}}` +
		`]},"finish_reason":"tool_calls"}]}`

	state := NewTransformState()
	state.OriginalRequestBody = []byte(origReq)
	out, err := (&openaiToCodexResponse{}).TransformWithState([]byte(resp), state)
	if err != nil {
		t.Fatalf("TransformWithState: %v", err)
	}
	ap := gjson.GetBytes(out, `output.#(name=="apply_patch")`)
	if ap.Get("type").String() != "custom_tool_call" {
		t.Fatalf("apply_patch output type = %q, want custom_tool_call: %s", ap.Get("type").String(), out)
	}
	if ap.Get("input").String() != applyPatchRaw {
		t.Fatalf("apply_patch input = %q, want raw patch", ap.Get("input").String())
	}
	if ap.Get("call_id").String() != "call_ap1" {
		t.Fatalf("apply_patch call_id = %q, want call_ap1", ap.Get("call_id").String())
	}
	sh := gjson.GetBytes(out, `output.#(name=="shell")`)
	if sh.Get("type").String() != "function_call" {
		t.Fatalf("shell output type = %q, want function_call", sh.Get("type").String())
	}
}

// Stream: the same freeform re-emission must hold for streamed tool calls — a
// custom_tool_call item with the unwrapped input, and no function_call_arguments
// events for it.
func TestOpenAIToCodexResponse_StreamEmitsCustomToolCall(t *testing.T) {
	origReq := `{"tools":[{"type":"custom","name":"apply_patch"}]}`
	state := NewTransformState()
	state.OriginalRequestBody = []byte(origReq)
	conv := &openaiToCodexResponse{}

	half1 := `{"input":"` + strings.ReplaceAll(applyPatchRaw[:20], "\n", "\\n")
	half2 := strings.ReplaceAll(applyPatchRaw[20:], "\n", "\\n") + `"}`
	chunks := []map[string]interface{}{
		{"object": "chat.completion.chunk", "id": "cc1", "choices": []map[string]interface{}{{"index": 0, "delta": map[string]interface{}{"tool_calls": []map[string]interface{}{{"index": 0, "id": "call_ap1", "function": map[string]interface{}{"name": "apply_patch"}}}}}}},
		{"object": "chat.completion.chunk", "id": "cc1", "choices": []map[string]interface{}{{"index": 0, "delta": map[string]interface{}{"tool_calls": []map[string]interface{}{{"index": 0, "function": map[string]interface{}{"arguments": half1}}}}}}},
		{"object": "chat.completion.chunk", "id": "cc1", "choices": []map[string]interface{}{{"index": 0, "delta": map[string]interface{}{"tool_calls": []map[string]interface{}{{"index": 0, "function": map[string]interface{}{"arguments": half2}}}}}}},
		{"object": "chat.completion.chunk", "id": "cc1", "choices": []map[string]interface{}{{"index": 0, "delta": map[string]interface{}{}, "finish_reason": "tool_calls"}}},
	}
	var all strings.Builder
	for _, ch := range chunks {
		raw, _ := json.Marshal(ch)
		got, err := conv.TransformChunk(FormatSSE("", raw), state)
		if err != nil {
			t.Fatalf("TransformChunk: %v", err)
		}
		all.Write(got)
	}
	s := all.String()
	if !strings.Contains(s, `"type":"custom_tool_call"`) {
		t.Fatalf("stream missing custom_tool_call item:\n%s", s)
	}
	if strings.Contains(s, "response.function_call_arguments.delta") || strings.Contains(s, "response.function_call_arguments.done") {
		t.Fatalf("stream leaked function_call_arguments events for a freeform tool:\n%s", s)
	}
	// The final output_item.done must carry the unwrapped raw patch as input.
	var doneInput string
	for _, ev := range strings.Split(s, "\n\n") {
		line := sseDataLine(ev)
		if line == "" {
			continue
		}
		r := gjson.Parse(line)
		if r.Get("type").String() == "response.output_item.done" && r.Get("item.type").String() == "custom_tool_call" {
			doneInput = r.Get("item.input").String()
		}
	}
	if doneInput != applyPatchRaw {
		t.Fatalf("custom_tool_call done input = %q, want raw patch", doneInput)
	}
}

func mustJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func sseDataLine(block string) string {
	for _, ln := range strings.Split(block, "\n") {
		if strings.HasPrefix(ln, "data: ") {
			return strings.TrimPrefix(ln, "data: ")
		}
	}
	return ""
}
