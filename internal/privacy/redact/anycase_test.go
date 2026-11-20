package redact

import (
	"strings"
	"testing"
)

// A masked word matches in any case only when the user asked for it
// (ZoeCats on Discord): off, the default, it is matched as written, as
// before; on, NIGHTJAR and nightjar go as placeholders too, and each comes
// back as it was written.
func TestWordsAnyCase(t *testing.T) {
	in := "Nightjar, NIGHTJAR and nightjar; МОСКВА is Москва. A nightjars' night."
	for _, c := range []struct {
		name    string
		anyCase bool
		count   int
		gone    []string
		kept    []string
	}{
		{"as written", false, 1, []string{"Nightjar,"}, []string{"NIGHTJAR", "nightjar;", "МОСКВА", "nightjars'"}},
		{"any case", true, 6, []string{"Nightjar", "NIGHTJAR", "nightjar", "МОСКВА", "Москва"}, []string{"A ", "s' night."}},
	} {
		o := Options{Words: []string{"Nightjar", " москва "}, WordsAnyCase: c.anyCase}
		out, n := Mask(in, o)
		if n != c.count {
			t.Errorf("%s: %d masked, want %d: %q", c.name, n, c.count, out)
		}
		for _, g := range c.gone {
			if strings.Contains(unmarked(out), g) {
				t.Errorf("%s: %q still in %q", c.name, g, out)
			}
		}
		for _, k := range c.kept {
			if !strings.Contains(out, k) {
				t.Errorf("%s: %q gone from %q", c.name, k, out)
			}
		}
		if back := Restore(out, false); back != in {
			t.Errorf("%s: round trip %q → %q", c.name, out, back)
		}
		// and in a request body, as the gateway masks it
		body, _ := MaskJSON([]byte(`{"messages":[{"role":"user","content":"ship NIGHTJAR"}]}`), o)
		if hid := !strings.Contains(string(body), "NIGHTJAR"); hid != c.anyCase {
			t.Errorf("%s: NIGHTJAR in a request masked=%v: %s", c.name, hid, body)
		}
	}
}
