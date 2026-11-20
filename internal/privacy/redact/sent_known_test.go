package redact

import (
	"strings"
	"testing"
)

// A text sent before goes as it went, though a value in it became known
// since: a vendor caches a conversation by its bytes, and masking the
// value now in a system prompt already sent with it rewrote the whole
// prompt to the cache on the next turn (audie on Discord: 400K tokens
// written, none read, ten seconds after the turn before). New text has
// the value masked as before.
func TestSentTextKeepsItsMasking(t *testing.T) {
	resetSent()
	t.Cleanup(resetSent)
	o := Options{Secrets: true, Personal: true}
	const secret = "Rv9sentPass27x"
	system := "You are dsh. Deploy notes: the staging pass is " + secret + " for now."
	first, n := Mask(system, o)
	if first != system || n != 0 {
		t.Fatalf("no rule finds it here, and nothing is known: %q", first)
	}
	body := `{"messages":[{"role":"system","content":"` + system + `"},{"role":"user","content":"go"}]}`
	firstBody, _ := MaskJSON([]byte(body), o)

	// a tool's output in the next turn has it where a rule finds it
	if out, _ := Mask("DB_PASSWORD="+secret, o); strings.Contains(out, secret) {
		t.Fatalf("the rule should mask it: %q", out)
	}

	// the system prompt and the body sent before go as they went
	if again, _ := Mask(system, o); again != first {
		t.Fatalf("a text sent before was masked again:\n%q\nwas\n%q", again, first)
	}
	if again, _ := MaskJSON([]byte(body), o); string(again) != string(firstBody) {
		t.Fatalf("a body sent before was masked again:\n%s\nwas\n%s", again, firstBody)
	}
	// with more after it, the earlier text is the same bytes
	longer := `{"messages":[{"role":"system","content":"` + system + `"},{"role":"user","content":"go"},{"role":"assistant","content":"the pass is ` + secret + `"}]}`
	got, _ := MaskJSON([]byte(longer), o)
	if !strings.HasPrefix(string(got), string(firstBody[:strings.Index(string(firstBody), `,{"role":"user"`)])) {
		t.Fatalf("the prefix changed: %s", got)
	}
	// the new text has the value as its placeholder
	if strings.Count(string(got), secret) != 1 {
		t.Fatalf("the new turn has the value as written: %s", got)
	}
	if out, _ := Mask("now the pass is "+secret+" again", o); strings.Contains(out, secret) {
		t.Fatalf("a known value went out in new text: %q", out)
	}

	// what is covered changing is another text: masked with what is known
	if out, _ := Mask(system, Options{Secrets: true}); strings.Contains(out, secret) {
		t.Fatalf("other settings, masked as known: %q", out)
	}
}
