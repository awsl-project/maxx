package redact

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// sameExceptDescriptions fails t where got differs from want other than in
// a "description" string.
func sameExceptDescriptions(t *testing.T, where string, want, got any) {
	t.Helper()
	var walk func(path, key string, a, b any)
	walk = func(path, key string, a, b any) {
		switch x := a.(type) {
		case map[string]any:
			y, _ := b.(map[string]any)
			for k, v := range x {
				walk(path+"."+k, k, v, y[k])
			}
		case []any:
			y, _ := b.([]any)
			if len(x) != len(y) {
				t.Errorf("%s%s: %d items became %d", where, path, len(x), len(y))
				return
			}
			for i := range x {
				walk(path, key, x[i], y[i])
			}
		case string:
			if s, _ := b.(string); s != x && key != "description" {
				t.Errorf("%s%s changed:\n%q\nbecame\n%q", where, path, x, s)
			}
		}
	}
	walk("", "", want, got)
}

// A word the user masks is masked in what the agent says, never inside
// what it defines for the vendor to check. Codex 0.162.1's own request
// (testdata, as Codex sent it) has "pd" inside the rule name update_hunk of
// apply_patch's Lark grammar; masked there, OpenAI turned the request away
// with 400 "Invalid lark grammar" (#1911, loveNangua). The tools go as
// they came, at the top or in Responses Lite's additional_tools item,
// save their descriptions; the developer message's "update" is masked.
func TestMaskJSONKeepsToolDefinitions(t *testing.T) {
	raw, err := os.ReadFile("testdata/codex-0.162.1-request.json")
	if err != nil {
		t.Fatal(err)
	}
	o := Options{Words: []string{"pd"}, Secrets: true, Personal: true}

	var req map[string]any
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatal(err)
	}
	// Responses Lite: Codex's tools as its first input item, its own in
	// the "functions" namespace
	lite := map[string]any{}
	for k, v := range req {
		lite[k] = v
	}
	delete(lite, "tools")
	lite["input"] = append([]any{map[string]any{"type": "additional_tools", "tools": []any{
		map[string]any{"type": "namespace", "name": "functions", "description": "", "tools": req["tools"]},
	}}}, req["input"].([]any)...)
	liteRaw, _ := json.Marshal(lite)

	for _, c := range []struct {
		name  string
		body  []byte
		tools func(map[string]any) any
	}{
		{"tools", raw, func(m map[string]any) any { return m["tools"] }},
		{"additional_tools", liteRaw, func(m map[string]any) any { return m["input"].([]any)[0] }},
	} {
		out, n := MaskJSON(c.body, o)
		var got, want map[string]any
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatalf("%s: %v\n%s", c.name, err, out)
		}
		_ = json.Unmarshal(c.body, &want)
		sameExceptDescriptions(t, c.name, c.tools(want), c.tools(got))
		for _, k := range []string{"include", "reasoning", "tool_choice"} {
			sameExceptDescriptions(t, c.name+"."+k, want[k], got[k])
		}
		b, _ := json.Marshal(c.tools(got))
		if !strings.Contains(string(b), `hunk: add_hunk | delete_hunk | update_hunk\n`) {
			t.Errorf("%s: apply_patch's grammar changed: %s", c.name, b)
		}
		if strings.Contains(unmarked(string(b)), "Update the existing goal") {
			t.Errorf("%s: a tool's description still has the word: %s", c.name, b)
		}
		if strings.Contains(unmarked(string(out)), "update it after") || n == 0 {
			t.Errorf("%s: the developer message's word went out (%d masked)", c.name, n)
		}
	}
}

// The same holds in the other protocols an agent speaks: a schema's enum,
// required list and pattern, and the output's schema, are structure. A
// "tools" or "text" key the model or the agent wrote, in a message or a
// tool call's arguments, is masked as before: only the protocol's own
// positions are kept.
func TestMaskJSONKeepsSchemas(t *testing.T) {
	o := Options{Words: []string{"pd"}, Secrets: true, Personal: true}
	for _, c := range []struct{ name, body, keep string }{
		{"anthropic tools", `{"messages":[{"role":"user","content":"hi"}],"tools":[{"name":"t","description":"x","input_schema":{"type":"object","properties":{"mode":{"type":"string","enum":["update","keep"]}},"required":["mode"]}}],"tool_choice":{"type":"tool","name":"t"}}`, `"enum":["update","keep"]`},
		{"chat response_format", `{"messages":[{"role":"user","content":"hi"}],"response_format":{"type":"json_schema","json_schema":{"name":"r","schema":{"type":"object","properties":{"updated":{"type":"string","pattern":"^upd"}},"required":["updated"]}}}}`, `"required":["updated"]`},
		{"chat pattern", `{"messages":[{"role":"user","content":"hi"}],"response_format":{"type":"json_schema","json_schema":{"name":"r","schema":{"type":"string","pattern":"^upd"}}}}`, `"pattern":"^upd"`},
		{"responses text.format", `{"input":"hi","text":{"format":{"type":"json_schema","name":"r","schema":{"enum":["update"]}}}}`, `"enum":["update"]`},
		{"gemini tools", `{"contents":[{"role":"user","parts":[{"text":"hi"}]}],"tools":[{"functionDeclarations":[{"name":"f","parameters":{"type":"OBJECT","properties":{"x":{"type":"STRING"}},"propertyOrdering":["x"],"required":["x"],"enum":["update"]}}]}]}`, `"enum":["update"]`},
		{"gemini responseSchema", `{"contents":[{"role":"user","parts":[{"text":"hi"}]}],"generationConfig":{"responseSchema":{"type":"STRING","enum":["update"]}}}`, `"enum":["update"]`},
	} {
		out, _ := MaskJSON([]byte(c.body), o)
		if !strings.Contains(string(out), c.keep) {
			t.Errorf("%s: %s changed: %s", c.name, c.keep, out)
		}
	}
	for _, c := range []struct{ name, body string }{
		{"tool_use input", `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"t","input":{"tools":"update","text":{"enum":"update"}}}]}]}`},
		{"message text", `{"messages":[{"role":"user","content":[{"type":"text","text":"update"}]}]}`},
		{"gemini part", `{"contents":[{"role":"user","parts":[{"text":"update","tools":"update"}]}]}`},
		{"completion text", `{"text":"update"}`},
		{"responses message", `{"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"update"}],"tools":"update"}]}`},
	} {
		out, _ := MaskJSON([]byte(c.body), o)
		if strings.Contains(unmarked(string(out)), "update") {
			t.Errorf("%s: the word went out: %s", c.name, out)
		}
	}
}
