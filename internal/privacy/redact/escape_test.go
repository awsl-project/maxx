package redact

import (
	"encoding/json"
	"strings"
	"testing"
)

// A tool call's arguments ride in the request as a string that holds JSON
// of its own: the newline in the command in it is the two characters \
// and n, and what follows reads with the n for the start of a word. The
// EMAIL rule took n@mcp.tool for an address, and the placeholder went in
// where the n had been, the backslash left before it — \{{EMAIL_…}}, and
// the arguments no longer parsed. The vendor turned the whole request away
// with 400 "invalid tool call arguments", the history replayed, and the
// conversation stayed broken (#3022).
func TestMaskJSONKeepsArgumentsJSON(t *testing.T) {
	command := "print('before')\n@mcp.tool()\ndef demo():\n    return 1"
	inner, _ := json.Marshal(map[string]string{"command": command})
	args, _ := json.Marshal(string(inner)) // the string that carries the arguments
	body := `{"messages":[` +
		`{"role":"user","content":"write mara@quill.dev about the demo"},` +
		`{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function",` +
		`"function":{"name":"pwsh","arguments":` + string(args) + `}}]},` +
		`{"role":"tool","tool_call_id":"call_1","content":"done"}]}`
	out, n := MaskJSON([]byte(body), Options{Personal: true})
	if !json.Valid(out) {
		t.Fatalf("body no longer JSON: %s", out)
	}
	if n != 1 { // the real email in the user's words, not the decorator
		t.Fatalf("%d masked, want 1: %s", n, out)
	}
	var v struct {
		Messages []struct {
			ToolCalls []struct {
				Function struct {
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("body: %v", err)
	}
	arguments := v.Messages[1].ToolCalls[0].Function.Arguments
	if !json.Valid([]byte(arguments)) {
		t.Fatalf("arguments no longer JSON: %s", arguments)
	}
	var got struct {
		Command string
	}
	if err := json.Unmarshal([]byte(arguments), &got); err != nil {
		t.Fatalf("arguments: %v", err)
	}
	if got.Command != command {
		t.Fatalf("command changed: %q", got.Command)
	}
	// and the email the same request really has still goes
	if !strings.Contains(string(out), "{{EMAIL_") || strings.Contains(unmarked(string(out)), "mara@quill.dev") {
		t.Fatalf("the address went through: %s", out)
	}

	// a string that is no JSON of its own is read after one unescaping, so
	// a Windows path in it goes as before
	body2 := `{"cwd":"C:\\Users\\rkato\\proj"}`
	out2, n2 := MaskJSON([]byte(body2), Options{Personal: true, Kinds: map[string]bool{"home": true}})
	if n2 != 1 || !strings.Contains(string(out2), `C:\\Users\\`+standInOf("rkato")+`\\proj`) {
		t.Fatalf("%d: %s", n2, out2)
	}
}

// argsOf is the arguments of each tool call in a Chat Completions body.
func argsOf(t *testing.T, body []byte) []string {
	t.Helper()
	var v struct {
		Messages []struct {
			ToolCalls []struct {
				Function struct {
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"messages"`
		Choices []struct {
			Message struct {
				ToolCalls []struct {
					Function struct {
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("body no longer JSON: %v: %s", err, body)
	}
	var out []string
	for _, m := range v.Messages {
		for _, c := range m.ToolCalls {
			out = append(out, c.Function.Arguments)
		}
	}
	for _, c := range v.Choices {
		for _, tc := range c.Message.ToolCalls {
			out = append(out, tc.Function.Arguments)
		}
	}
	return out
}

// chatWithCall is a Chat Completions history whose assistant turn called
// pwsh with arguments.
func chatWithCall(arguments string) []byte {
	a, _ := json.Marshal(arguments)
	return []byte(`{"messages":[{"role":"user","content":"Reply OK."},` +
		`{"role":"assistant","content":null,"tool_calls":[{"id":"call_repro","type":"function",` +
		`"function":{"name":"pwsh","arguments":` + string(a) + `}}]},` +
		`{"role":"tool","tool_call_id":"call_repro","content":"Completed."}]}`)
}

// A tool call's arguments in the history hold what a session really has
// in them: decorators after newlines (xiaobaifly7's \n@mcp.tool(),
// Shane-Meng's \n\n\n@pytest.mark…), and real addresses and keys right
// after an escape — a newline, a tab, a quote, a backslash, a \u escape.
// Each of the arguments still parses, each real value is masked, and
// every decorator reads as it was (#3022).
func TestMaskJSONArgumentsEscapes(t *testing.T) {
	const npmKey = "npm_abcdEFGH1234ijklMNOP5678qrstUVWX9012" // gitleaks:allow -- public upstream regression fixture or detector example
	commands := []struct {
		command string
		gone    []string
	}{
		{command: "print('before')\n@mcp.tool()\ndef demo():\n    return 1"},
		{command: "looked into a possible new issue\n\n\n@pytest.mark.parametrize('x', [1])\ndef test_x(x):\n    pass"},
		{command: "@app.route('/')\n@login.required\r\n@cache.cached(timeout=60)\ndef index(): pass"},
		{command: "mail\njo@quill.dev about it", gone: []string{"jo@quill.dev"}},
		{command: "to:\tmara@quill.dev", gone: []string{"mara@quill.dev"}},
		{command: `git config user.email "rkato@quill.dev"`, gone: []string{"rkato@quill.dev"}},
		{command: `type C:\Users\rkato\notes.txt` + "\nzoe@quill.dev", gone: []string{"zoe@quill.dev"}},
		{command: "caf\u00e9 \u2014 lin@quill.dev", gone: []string{"lin@quill.dev"}},
		{command: "export NPM_TOKEN=\n" + npmKey, gone: []string{npmKey}},
	}
	for _, c := range commands {
		inner, _ := json.Marshal(map[string]any{"command": c.command, "timeout": 30})
		// \u escapes too, as Python's json.dumps writes them
		ascii := strings.NewReplacer("é", `\u00e9`, "—", `\u2014`).Replace(string(inner))
		for _, args := range []string{string(inner), ascii} {
			out, _ := MaskJSON(chatWithCall(args), Options{Personal: true, Secrets: true})
			got := argsOf(t, out)[0]
			var v struct {
				Command string
				Timeout int
			}
			if err := json.Unmarshal([]byte(got), &v); err != nil {
				t.Fatalf("arguments no longer JSON (%v): %s", err, got)
			}
			if v.Timeout != 30 {
				t.Fatalf("timeout changed: %s", got)
			}
			if len(c.gone) == 0 && v.Command != c.command {
				t.Fatalf("command changed:\n%q\n%q", v.Command, c.command)
			}
			for _, g := range c.gone {
				if strings.Contains(v.Command, g) || strings.Contains(string(out), g) {
					t.Fatalf("%s went through: %s", g, got)
				}
			}
			if len(c.gone) > 0 && unmarked(v.Command) == v.Command {
				t.Fatalf("nothing masked: %q", v.Command)
			}
		}
	}

	// a secret known by its field's name, which only the arguments' text
	// says: masked as it was
	out, n := MaskJSON(chatWithCall(`{"password":"hunter2Secret99","note":"line\nzoe@quill.dev"}`), Options{Personal: true, Secrets: true})
	if got := argsOf(t, out)[0]; n != 2 || !json.Valid([]byte(got)) || strings.Contains(got, "hunter2Secret99") || strings.Contains(got, "zoe@") {
		t.Fatalf("%d: %s", n, got)
	}

	// a word of the user's own, after an escape: "\tango" is a tab and ango
	out, _ = MaskJSON(chatWithCall(`{"command":"a\tango and tango"}`), Options{Words: []string{"tango"}})
	got := argsOf(t, out)[0]
	var v struct{ Command string }
	if err := json.Unmarshal([]byte(got), &v); err != nil {
		t.Fatalf("arguments no longer JSON (%v): %s", err, got)
	}
	if !strings.HasPrefix(v.Command, "a\tango and {{TERM_") {
		t.Fatalf("command: %q", v.Command)
	}
}

// What the vendor writes back in a tool call of its own, the placeholder
// in its arguments, is put back as the value the agent wrote: a private
// key's newlines are newlines again, not the two characters \ and n, and
// the arguments still parse.
func TestRestoreArgumentsFromArguments(t *testing.T) {
	key := "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA3Tz2mr\n-----END RSA PRIVATE KEY-----"
	command := "cat > id_rsa <<'EOF'\n" + key + "\nEOF\ngit config user.email \"rkato@quill.dev\""
	inner, _ := json.Marshal(map[string]string{"command": command})
	out, n := MaskJSON(chatWithCall(string(inner)), Options{Personal: true, Secrets: true})
	if n != 2 {
		t.Fatalf("%d masked, want 2: %s", n, out)
	}
	sent := argsOf(t, out)[0]
	if strings.Contains(sent, "MIIEow") || strings.Contains(sent, "rkato@quill.dev") {
		t.Fatalf("went through: %s", sent)
	}
	// the vendor calls the tool again with what it read
	a, _ := json.Marshal(sent)
	reply := []byte(`{"choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_2","type":"function","function":{"name":"pwsh","arguments":` + string(a) + `}}]},"finish_reason":"tool_calls"}]}`)
	back := argsOf(t, RestoreJSON(reply))[0]
	var v struct{ Command string }
	if err := json.Unmarshal([]byte(back), &v); err != nil {
		t.Fatalf("restored arguments no longer JSON (%v): %s", err, back)
	}
	if v.Command != command {
		t.Fatalf("restored:\n%q\nwant\n%q", v.Command, command)
	}
}

// Arguments cut short by the model are no JSON as a whole, and are read
// the same way: the decorator goes as it is, and the address after a
// quote is still masked.
func TestMaskJSONArgumentsCutShort(t *testing.T) {
	args := `{"command":"print('before')\n@mcp.tool()\ngit config user.email \"rkato@quill.dev\"\ndef demo(`
	out, n := MaskJSON(chatWithCall(args), Options{Personal: true})
	got := argsOf(t, out)[0]
	if n != 1 || strings.Contains(got, `\{{`) || !strings.Contains(got, `\n@mcp.tool()`) || strings.Contains(got, "rkato@quill.dev") {
		t.Fatalf("%d: %s", n, got)
	}

	// a value is cut back to the escapes' edges: a \u escape's digits are
	// inside it too
	s := `x\u0040y\nz`
	found := outsideEscapes(s, []span{
		{3, 5, "A", ByRule, 0},  // 00 of \u0040: nothing left
		{1, 7, "B", ByRule, 0},  // the whole \u0040: as it is
		{0, 4, "C", ByRule, 0},  // ends in it: x
		{7, 9, "D", ByRule, 0},  // y and the backslash of \n: y
		{9, 11, "E", ByRule, 0}, // the n of \n and z: z
		{8, 11, "F", ByRule, 0}, // \nz: as it is
	})
	var kept []string
	for _, f := range found {
		kept = append(kept, f.kind+":"+s[f.start:f.end])
	}
	if got := strings.Join(kept, " "); got != `B:\u0040 C:x D:y E:z F:\nz` {
		t.Fatalf("kept %s", got)
	}

	// a user's rule that reads up to the backslash of \": the JSON holds,
	// and what the rule found before it is masked
	out, n = MaskJSON(chatWithCall(`{"cmd":"x REL-ab\"c d"}`), Options{Secrets: true, Rules: []Rule{{Kind: "rel", Regex: `REL-[a-z\\]+`}}})
	if got := argsOf(t, out)[0]; n != 1 || !json.Valid([]byte(got)) || strings.Contains(got, "REL-ab") || !strings.Contains(got, `\"c d`) {
		t.Fatalf("%d: %s", n, got)
	}
}
