package converter

import (
	"encoding/json"
	"testing"

	"github.com/tidwall/gjson"
)

// A Codex Responses turn that carries a prior tool call: the function_call item
// has BOTH an item id ("fc_call_X") and a call_id ("call_X"), and the matching
// function_call_output references the call_id. Downstream chat/messages APIs pair
// the assistant tool call to its result by id, so the converter must key the
// assistant side on call_id (not the item id) or the upstream rejects the turn
// with "No tool output found for function call fc_...".
const codexToolContinuationBody = `{
  "model": "gpt-5.5",
  "input": [
    {"type": "message", "role": "user", "content": "run pwd"},
    {"type": "function_call", "id": "fc_call_X", "call_id": "call_X", "name": "shell", "arguments": "{}"},
    {"type": "function_call_output", "call_id": "call_X", "output": "/work"}
  ]
}`

func TestCodexToOpenAIToolCallIDMatchesOutput(t *testing.T) {
	out, err := (&codexToOpenAIRequest{}).Transform([]byte(codexToolContinuationBody), "gpt-5.5", true)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}

	var assistantID, toolID string
	for _, m := range gjson.GetBytes(out, "messages").Array() {
		switch m.Get("role").String() {
		case "assistant":
			if tc := m.Get("tool_calls.0.id"); tc.Exists() {
				assistantID = tc.String()
			}
		case "tool":
			toolID = m.Get("tool_call_id").String()
		}
	}
	if assistantID == "" || toolID == "" {
		t.Fatalf("missing ids: assistant=%q tool=%q\n%s", assistantID, toolID, out)
	}
	if assistantID != toolID {
		t.Fatalf("assistant tool_call id %q != tool tool_call_id %q — upstream would 400", assistantID, toolID)
	}
	if assistantID != "call_X" {
		t.Errorf("tool call id = %q, want call_X (the call_id, not the fc_ item id)", assistantID)
	}
}

func TestCodexToClaudeToolUseIDMatchesResult(t *testing.T) {
	out, err := (&codexToClaudeRequest{}).Transform([]byte(codexToolContinuationBody), "claude-sonnet-4-6", true)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}

	var toolUseID, toolResultID string
	for _, m := range gjson.GetBytes(out, "messages").Array() {
		for _, block := range m.Get("content").Array() {
			switch block.Get("type").String() {
			case "tool_use":
				toolUseID = block.Get("id").String()
			case "tool_result":
				toolResultID = block.Get("tool_use_id").String()
			}
		}
	}
	if toolUseID == "" || toolResultID == "" {
		t.Fatalf("missing ids: tool_use=%q tool_result=%q\n%s", toolUseID, toolResultID, out)
	}
	if toolUseID != toolResultID {
		t.Fatalf("tool_use.id %q != tool_result.tool_use_id %q — upstream would reject", toolUseID, toolResultID)
	}
	if toolUseID != "call_X" {
		t.Errorf("tool id = %q, want call_X (the call_id, not the fc_ item id)", toolUseID)
	}
}

// Guard the fallback: when a function_call omits call_id, the item id is used so
// the turn is still internally consistent (the output would reference that id).
func TestCodexToOpenAIFallsBackToItemIDWhenNoCallID(t *testing.T) {
	body := `{"model":"gpt-5.5","input":[{"type":"function_call","id":"fc_only","name":"shell","arguments":"{}"}]}`
	out, err := (&codexToOpenAIRequest{}).Transform([]byte(body), "gpt-5.5", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	got := gjson.GetBytes(out, "messages.0.tool_calls.0.id").String()
	if got != "fc_only" {
		t.Errorf("tool call id = %q, want fc_only (fallback to item id)", got)
	}
	_ = json.Valid(out)
}
