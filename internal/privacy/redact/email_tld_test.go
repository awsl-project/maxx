package redact

import (
	"strings"
	"testing"
)

// What follows the @ of a decorator or a module's attribute (pytest.fixture)
// ends in a word no registry has, and an agent's own files (AGENTS.md)
// end in .md: neither was an address, and both were masked (ydzh on
// Discord). Addresses on real domains, a company's own (.local, .corp)
// and the .md of a domain that isn't such a file are still masked.
func TestEmailNeedsAMailDomain(t *testing.T) {
	o := Options{Secrets: true, Personal: true}
	for _, s := range []string{
		"xxx@Agents.md", "follow docs@AGENTS.md", "see repo@CLAUDE.md and x@README.md",
		"x@pytest.fixture", "def@pytest.mark.parametrize", "conftest@pytest.mark.asyncio",
		"cls@dataclasses.dataclass", "obj@functools.cache",
	} {
		in := "look at " + s + " now"
		if out, n := Mask(in, o); n != 0 || out != in {
			t.Errorf("%q: %d masked: %q", s, n, out)
		}
	}
	for _, s := range []string{
		"rina.k@fastmail.com", "dev@startup.io", "me@notes.md", "ops@corp.example.co.uk",
		"x@company.ai", "li.wei@qq.com", "admin@build.local", "ops@mail.corp", "j@Gmail.COM",
	} {
		in := "mail " + s + " today"
		if out, n := Mask(in, o); n != 1 || strings.Contains(out, s) {
			t.Errorf("%q: %d masked: %q", s, n, out)
		}
	}
	f := &fakeModel{values: map[string]string{"x@pytest.fixture": "private_email", "xxx@Agents.md": "private_email"}}
	for _, s := range []string{"x@pytest.fixture", "xxx@Agents.md"} {
		if out, n := Mask(s, Options{Personal: true, Model: f.find}); n != 0 {
			t.Errorf("model's %q: %d masked: %q", s, n, out)
		}
	}
}

// An address that ends a sentence is masked: the dot after it was a
// bound character, so "The user's email address is jo@quill.dev." (Claude
// Code's own line in every request) and "write to me at li.wei@qq.com."
// went to the vendor as written, the rules masking nothing. A dot that
// makes the address part of a longer name still keeps it as it is.
func TestEmailBeforeAFullStop(t *testing.T) {
	o := Options{Secrets: true, Personal: true}
	for _, in := range []string{
		"The user's email address is jo@quill.dev. Use it only to identify the user.",
		"The user's email address is jo@quill.dev.",
		"write to me at li.wei@qq.com.",
		"ask rina.k@fastmail.com... she knows",
		`{"text":"mail jo@quill.dev.\nthanks"}`,
	} {
		if out, n := Mask(in, o); n != 1 || strings.Contains(out, "@") {
			t.Errorf("%q: %d masked: %q", in, n, out)
		}
	}
	for _, in := range []string{
		"x@build.io.12 is a version",
		"a@b.co.x_y",
	} {
		if out, n := Mask(in, o); n != 0 {
			t.Errorf("%q: %d masked: %q", in, n, out)
		}
	}
}
