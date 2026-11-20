package redact

import (
	"encoding/json"
	"strings"
	"testing"
)

// The model took Codex's stream_options.reasoning_summary_delivery,
// "sequential_cutoff", for a name, and Codex turned the placeholder away:
// 400 "Invalid value" on every request (#3144, the reporter's body). Not
// asked about it as a setting, and not taking a snake_case word for a name
// wherever it is; a name beside it is still masked.
func TestMaskJSONKeepsReasoningSummaryDelivery(t *testing.T) {
	f := &fakeModel{values: map[string]string{"sequential_cutoff": "private_person", "Rina Kowalczyk": "private_person"}}
	body := `{"stream_options":{"reasoning_summary_delivery":"sequential_cutoff"},` +
		`"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"set sequential_cutoff for Rina Kowalczyk"}]}]}`
	out, n := MaskJSON([]byte(body), Options{Personal: true, Model: f.find})
	var v struct {
		StreamOptions map[string]string `json:"stream_options"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("body: %v: %s", err, out)
	}
	if v.StreamOptions["reasoning_summary_delivery"] != "sequential_cutoff" {
		t.Fatalf("setting masked: %s", out)
	}
	if n != 1 || !strings.Contains(string(out), "set sequential_cutoff for {{PERSON_") {
		t.Fatalf("%d masked: %s", n, out)
	}
}

// A user's word that is part of an MCP server's name went into a Responses
// function_call's namespace as a placeholder, and Codex checks it against
// ^[a-zA-Z0-9_-]+$: 400 on every turn that replayed the call (#3143, the
// reporter's body). The tool's server is named as it is in each protocol;
// the same word in the user's message is still masked.
func TestMaskJSONKeepsToolServerNames(t *testing.T) {
	o := Options{Secrets: true, Personal: true, Words: []string{"codex_app", "list_threads"}}
	for _, c := range []struct{ name, body, keep string }{
		{"responses namespace",
			`{"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"ask codex_app to list threads"}]},` +
				`{"type":"function_call","call_id":"c1","name":"list_threads","namespace":"mcp__codex_app","arguments":"{}"}]}`,
			`"namespace":"mcp__codex_app"`},
		{"responses mcp_call",
			`{"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"ask codex_app to list threads"}]},` +
				`{"type":"mcp_call","id":"mcp_1","server_label":"codex_app","name":"list_threads","arguments":"{}"}]}`,
			`"server_label":"codex_app"`},
		{"anthropic mcp_tool_use",
			`{"messages":[{"role":"user","content":"ask codex_app to list threads"},` +
				`{"role":"assistant","content":[{"type":"mcp_tool_use","id":"mcptoolu_1","name":"list_threads","server_name":"codex_app","input":{}}]}]}`,
			`"server_name":"codex_app"`},
	} {
		out, _ := MaskJSON([]byte(c.body), o)
		if !strings.Contains(string(out), c.keep) {
			t.Errorf("%s: server's name masked: %s", c.name, out)
		}
		if !strings.Contains(string(out), `"ask {{TERM_`) {
			t.Errorf("%s: the word in the message kept: %s", c.name, out)
		}
	}
}

// What the model takes for a name is never a snake_case word, and still a
// name with an underscore-free spelling.
func TestModelTakesNoSnakeWord(t *testing.T) {
	for _, v := range []string{"sequential_cutoff", "list_threads", "mcp__codex_app", "in_progress", "HTTP_PROXY"} {
		if modelTakes("PERSON", v) {
			t.Errorf("%q taken for a name", v)
		}
	}
	for _, v := range []string{"Rina Kowalczyk", "O'Brien", "张伟", "Jean-Luc"} {
		if !modelTakes("PERSON", v) {
			t.Errorf("%q not taken for a name", v)
		}
	}
}

// The model took a tool result's Unix time for a phone, "tool_call" before
// it was the cue, and the log went as "timestamp": {{PHONE_c2ukh7gk}}
// (#3577, the reporter's line). A time in seconds or milliseconds is
// kept, cued or not; a phone beside it is still masked.
func TestModelPhoneKeepsUnixTime(t *testing.T) {
	f := &fakeModel{values: map[string]string{"1792903687": "private_phone", "1792903687123": "private_phone", "+1 415 555 0132": "private_phone"}}
	body := `{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":` +
		`"{\"event\": \"tool_call\", \"timestamp\": 1792903687, \"ms\": 1792903687123, \"at\": \"2026-10-25T04:48:03.755Z\", \"desk\": \"+1 415 555 0132\"}"}]}]}`
	out, n := MaskJSON([]byte(body), Options{Personal: true, Model: f.find})
	for _, keep := range []string{`\"timestamp\": 1792903687,`, `\"ms\": 1792903687123,`} {
		if !strings.Contains(string(out), keep) {
			t.Errorf("time masked: %s", out)
		}
	}
	if n != 1 || strings.Contains(string(out), "415 555 0132") {
		t.Fatalf("%d masked: %s", n, out)
	}
	for _, v := range []string{"13800138000", "2125551234", "1792903687x", "179290368", "+11792903687"} {
		if epochNumber(v) {
			t.Errorf("%q taken for a time", v)
		}
	}
}
