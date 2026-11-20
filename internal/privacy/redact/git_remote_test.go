package redact

import (
	"strings"
	"testing"
)

// git@host is how every git host is reached over SSH, and was masked as
// an address, remote and all (jumy on Discord). It stays, whichever host;
// an address beside it, and one whose name only starts with git, is still
// masked, by the rules and the local model's EMAIL alike.
func TestGitRemoteIsNoEmail(t *testing.T) {
	o := Options{Secrets: true, Personal: true}
	for _, s := range []string{
		"git@github.com:yetone/magpie.git",
		"git clone git@gitlab.com:group/sub/repo.git",
		"origin\tgit@gitee.com:owner/repo.git (fetch)",
		"ssh -T git@github.com",
		"url = git@git.corp-internal.com:team/svc.git",
		"GIT@GitHub.com:o/r",
	} {
		if out, n := Mask(s, o); n != 0 || out != s {
			t.Errorf("%q: %d masked: %q", s, n, out)
		}
	}
	for _, s := range []string{"gitte@fastmail.com", "git.lee@acme-corp.com", "github@acme-corp.com"} {
		in := "mail " + s + " today"
		if out, n := Mask(in, o); n != 1 || strings.Contains(out, s) {
			t.Errorf("%q: %d masked: %q", s, n, out)
		}
	}
	body := `{"messages":[{"role":"user","content":"push to git@github.com:o/r.git and tell rina.k@fastmail.com"}]}`
	out, n := MaskJSON([]byte(body), o)
	if n != 1 || !strings.Contains(string(out), "git@github.com:o/r.git") || strings.Contains(string(out), "rina.k@fastmail.com") {
		t.Fatalf("request: %d masked: %s", n, out)
	}
	f := &fakeModel{values: map[string]string{"git@github.com": "private_email"}}
	if out, n := Mask("git@github.com:o/r.git", Options{Personal: true, Model: f.find}); n != 0 {
		t.Fatalf("model's: %d masked: %q", n, out)
	}
}
