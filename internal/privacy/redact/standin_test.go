package redact

import (
	"strings"
	"testing"
)

// A user name in a home folder goes as a made-up name, so the path still
// reads as a home folder's (eerttreeeert on X, 10-28: C:\Users\{{…}} was
// taken for a temporary folder, and the agent set out to delete it). It
// is the same every turn, and comes back as the user's in a reply and in
// a tool call's path.
func TestUserNameGoesAsAMadeUpName(t *testing.T) {
	o := Options{Personal: true, Kinds: map[string]bool{"home": true}}
	for _, c := range []struct{ s, name, before, after string }{
		{`C:\Users\eerttreeeert\AppData\Local\Temp\x.txt`, "eerttreeeert", `C:\Users\`, `\AppData\Local\Temp\x.txt`},
		{`{"cwd":"C:\\Users\\eerttreeeert\\proj"}`, "eerttreeeert", `C:\\Users\\`, `\\proj`},
		{"cd /Users/rinak/workspace/app", "rinak", "/Users/", "/workspace/app"},
		{"/home/ops/.bashrc", "ops", "/home/", "/.bashrc"},
	} {
		out, n := Mask(c.s, o)
		f := standInOf(c.name)
		if n != 1 || strings.Contains(out, "{{") || out != strings.Replace(c.s, c.before+c.name+c.after, c.before+f+c.after, 1) {
			t.Errorf("%q: sent %q, want the name as %q", c.s, out, f)
		}
		if len(f) != standInLen || strings.Trim(f, "abcdefghijklmnopqrstuvwxyz") != "" {
			t.Errorf("%q: made-up name %q", c.name, f)
		}
		if again, _ := Mask(c.s, o); again != out {
			t.Errorf("%q: not the same name twice: %q, %q", c.s, out, again)
		}
		if back := Restore(out, false); back != c.s {
			t.Errorf("%q: put back as %q", c.s, back)
		}
		// the vendor's tool call with the path in it
		call := `{"command":"Remove-Item C:\\Users\\` + f + `\\AppData\\Local\\Temp\\old -Recurse"}`
		if back := Restore(call, true); !strings.Contains(back, c.name) || strings.Contains(back, f) {
			t.Errorf("%q: tool call put back as %q", c.s, back)
		}
		if p := Preview(c.s, o); p != out {
			t.Errorf("%q: preview %q, sent %q", c.s, p, out)
		}
	}
}
