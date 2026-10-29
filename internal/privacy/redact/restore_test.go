package redact

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// phonePlaceholder masks a phone number as a request has it and gives the
// placeholder the vendor saw.
func phonePlaceholder(t *testing.T, phone string) string {
	t.Helper()
	out, n := Mask("请回拨 "+phone+" 谢谢", Options{Personal: true})
	if n != 1 {
		t.Fatalf("not masked: %q", out)
	}
	return strings.TrimSuffix(strings.TrimPrefix(out, "请回拨 "), " 谢谢")
}

// A model writes a placeholder back in the ways models do: as a template's
// variable with spaces, a brace dropped, without braces, in another case.
// Each is the value again; what only looks like one stays as it is.
func TestRestoreRewrittenPlaceholder(t *testing.T) {
	const phone = "13912345678"
	p := phonePlaceholder(t, phone)
	body := p[2 : len(p)-2] // PHONE_xxxxxxxx
	lower, upper := strings.ToLower(body), body[:6]+strings.ToUpper(body[6:])
	for _, c := range []struct{ in, want string }{
		{"拨打 " + p + " 即可", "拨打 " + phone + " 即可"},
		{"拨打 {{ " + body + " }} 即可", "拨打 " + phone + " 即可"},
		{"拨打 {{" + body + "} 即可", "拨打 " + phone + " 即可"},
		{"拨打 {" + body + "}} 即可", "拨打 " + phone + " 即可"},
		{"拨打 {" + body + "} 即可", "拨打 " + phone + " 即可"},
		{"拨打 {{" + body + " 即可", "拨打 " + phone + " 即可"},
		{"拨打" + body + "即可", "拨打" + phone + "即可"},
		{"phone: `" + body + "`", "phone: `" + phone + "`"},
		{"拨打 {{" + lower + "}} 即可", "拨打 " + phone + " 即可"},
		{"拨打 {{" + upper + "}} 即可", "拨打 " + phone + " 即可"},
		{`{"to":"` + body + `"}`, `{"to":"` + phone + `"}`},
		{"a {" + body + " b", "a {" + phone + " b"}, // a lone { is the text's
		// not one of magpie's: another hash, inside a longer word
		{"{{PHONE_zzzzzzzz}} x" + body + " " + body + "x", "{{PHONE_zzzzzzzz}} x" + body + " " + body + "x"},
	} {
		if got := Restore(c.in, false); got != c.want {
			t.Errorf("Restore(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// a tool call's arguments, a secret's lower-cased placeholder in them
	masked, _ := Mask(`DB_PASSWORD=hunter2abc1`, Options{Secrets: true})
	_, ph, _ := strings.Cut(masked, "=")
	got := Restore(`{"env":"{{ `+strings.ToLower(ph[2:len(ph)-2])+` }}"}`, true)
	var args map[string]string
	if err := json.Unmarshal([]byte(got), &args); err != nil || args["env"] != "hunter2abc1" {
		t.Fatalf("args %s: %v", got, err)
	}
}

// A stream the model writes a rewritten placeholder into, token by token
// as a vendor sends it, reads as the value with nothing of the
// placeholder left: no brace whose partner came an event later, the same
// events as came.
func TestWriterStreamRewrittenPlaceholder(t *testing.T) {
	const phone = "13987654321"
	p := phonePlaceholder(t, phone)
	body := p[2 : len(p)-2]
	chat := func(s string) string {
		b, _ := json.Marshal(s)
		return `data: {"id":"chatcmpl-9a1","object":"chat.completion.chunk","created":1760000000,"model":"deepseek-chat","system_fingerprint":"fp_1","choices":[{"index":0,"delta":{"content":` + string(b) + `},"logprobs":null,"finish_reason":null}]}` + "\n\n"
	}
	// as a tokenizer splits them: the braces, the kind, the hash
	pieces := []string{
		"已记录：", "{{", " ", "PH", "ONE", "_" + body[6:9], body[9:], " ", "}}", "。\n",
		"备用 ", "PH", "ONE_", body[6:10], body[10:], "；",
		"再次 {{", body[:8], body[8:], "}", "}", "。",
		"最后 ", body, "",
	}
	var events []string
	for _, s := range pieces {
		events = append(events, chat(s))
	}
	events = append(events, `data: {"id":"chatcmpl-9a1","object":"chat.completion.chunk","created":1760000000,"model":"deepseek-chat","choices":[{"index":0,"delta":{},"logprobs":null,"finish_reason":"stop"}]}`+"\n\n", "data: [DONE]\n\n")

	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "text/event-stream")
	w := NewWriter(rec)
	w.WriteHeader(200)
	for _, e := range events {
		w.Write([]byte(e))
	}
	w.Finish()

	var text strings.Builder
	for _, ev := range strings.Split(rec.Body.String(), "\n\n") {
		d, ok := strings.CutPrefix(ev, "data: ")
		if !ok || d == "[DONE]" {
			continue
		}
		var e struct {
			Choices []struct{ Delta struct{ Content string } }
		}
		if err := json.Unmarshal([]byte(d), &e); err != nil {
			t.Fatalf("event %q: %v", d, err)
		}
		text.WriteString(e.Choices[0].Delta.Content)
	}
	want := "已记录：" + phone + "。\n备用 " + phone + "；再次 " + phone + "。最后 " + phone
	if text.String() != want {
		t.Fatalf("text:\n got %q\nwant %q", text.String(), want)
	}
	if strings.Count(rec.Body.String(), "\n\n") != len(events) {
		t.Fatalf("events changed:\n%s", rec.Body.String())
	}
}

// Anthropic's tool call written as it streams, the placeholder in another
// case and split in its partial_json: the arguments the agent puts
// together are JSON with the value in them.
func TestWriterStreamToolArgsRewritten(t *testing.T) {
	const phone = "13911112222"
	p := phonePlaceholder(t, phone)
	lower := strings.ToLower(p)
	args := `{"file_path":"/tmp/contact.md","content":"电话：` + lower + `\n"}`
	ev := func(s string) string {
		b, _ := json.Marshal(map[string]any{"type": "content_block_delta", "index": 1, "delta": map[string]any{"type": "input_json_delta", "partial_json": s}})
		return "event: content_block_delta\ndata: " + string(b) + "\n\n"
	}
	at := strings.Index(args, lower) + 9
	events := []string{ev(args[:at]), ev(args[at:]), "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":1}\n\n"}
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "text/event-stream")
	w := NewWriter(rec)
	w.WriteHeader(200)
	for _, e := range events {
		w.Write([]byte(e))
	}
	w.Finish()
	var js strings.Builder
	for _, e := range strings.Split(rec.Body.String(), "\n\n") {
		_, d, ok := strings.Cut(e, "data: ")
		if !ok {
			continue
		}
		var x struct {
			Delta struct {
				PartialJSON string `json:"partial_json"`
			}
		}
		json.Unmarshal([]byte(d), &x)
		js.WriteString(x.Delta.PartialJSON)
	}
	var in map[string]string
	if err := json.Unmarshal([]byte(js.String()), &in); err != nil || in["content"] != "电话："+phone+"\n" {
		t.Fatalf("args %q: %v", js.String(), err)
	}
}

// A keepalive comment goes to the agent as it is written, not with the
// event after it, even while an event is held for the rest of a
// placeholder: held, a proxy in front of magpie heard nothing while the
// vendor reasoned silently and hung up (#2402). The placeholder split
// around it still comes back whole.
func TestWriterStreamKeepaliveNotHeld(t *testing.T) {
	const phone = "13987654321"
	p := phonePlaceholder(t, phone)
	chat := func(s string) string {
		b, _ := json.Marshal(s)
		return `data: {"choices":[{"index":0,"delta":{"content":` + string(b) + `}}]}` + "\n\n"
	}
	rec := httptest.NewRecorder()
	rec.Header().Set("Content-Type", "text/event-stream")
	w := NewWriter(rec)
	w.WriteHeader(200)
	for i, s := range []string{": keepalive\n", chat("call " + p[:5]), ": keepalive\r\n", ": keep"} {
		w.Write([]byte(s))
		if want := []int{1, 1, 2, 2}[i]; strings.Count(rec.Body.String(), ": keepalive") != want {
			t.Fatalf("after %q: %q", s, rec.Body.String())
		}
	}
	w.Write([]byte("alive\n"))
	w.Write([]byte(chat(p[5:] + " now")))
	w.Finish()
	var text strings.Builder
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		if d, ok := strings.CutPrefix(line, "data: "); ok {
			var e struct {
				Choices []struct{ Delta struct{ Content string } }
			}
			if err := json.Unmarshal([]byte(d), &e); err != nil {
				t.Fatalf("event %q: %v", d, err)
			}
			text.WriteString(e.Choices[0].Delta.Content)
		}
	}
	if text.String() != "call "+phone+" now" || strings.Count(rec.Body.String(), ": keepalive") != 3 {
		t.Fatalf("text %q in %q", text.String(), rec.Body.String())
	}
}
