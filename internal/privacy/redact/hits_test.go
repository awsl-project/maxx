package redact

import (
	"strings"
	"testing"
)

// Each value masked is told to Seen with the finder that found it: the
// user's word, the user's rule, magpie's rules, the model (with the
// Chinese names it brings), and, in a later request, a value masked
// before. The hit holds the placeholder, never the value.
func TestSeenSaysWhoFoundIt(t *testing.T) {
	const word, mine, key, name, zh = "Bluefin", "ACME-7731-QXZ9", "sk-proj-Hq3vX9tLm2Wc8RzK4nYp", "Rina Kowalczyk", "张伟"
	f := &fakeModel{values: map[string]string{name: "private_person"}}
	s := "Project " + word + " ref " + mine + " key " + key + " ask " + name + "，并联系" + zh + "老师。"
	var hits []Hit
	o := Options{Secrets: true, Personal: true, Words: []string{word},
		Rules: []Rule{{Kind: "ticket", Prefix: "ACME-"}}, Model: f.find,
		Seen: func(h Hit) { hits = append(hits, h) }}
	out, n := Mask(s, o)
	if n != len(hits) {
		t.Fatalf("masked %d, told of %d: %v", n, len(hits), hits)
	}
	by := map[string]string{}
	for _, h := range hits {
		v, ok := Value(h.Placeholder)
		if !ok || !strings.Contains(out, h.Placeholder) {
			t.Fatalf("hit %v: no value or not in %q", h, out)
		}
		if strings.Contains(h.Placeholder, v) {
			t.Fatalf("hit %v holds its value", h)
		}
		by[v] = h.By
	}
	want := map[string]string{word: ByWord, mine: ByYours, key: ByRule, name: ByModel, zh: ByModel}
	for v, b := range want {
		if by[v] != b {
			t.Errorf("%q found by %q, want %q (all: %v)", v, by[v], b, by)
		}
	}

	// a later request without the model: the name it found before is
	// masked as a value known before
	hits = nil
	if out, _ := Mask("Thanks, "+name+"!", Options{Personal: true, Seen: o.Seen}); strings.Contains(out, name) || len(hits) != 1 || hits[0].By != ByKnown {
		t.Errorf("a value known before: %q %v", out, hits)
	}
}

// What JSON bodies mask is told the same way, once per place it is.
func TestSeenInJSON(t *testing.T) {
	const key = "sk-proj-Hq3vX9tLm2Wc8RzK4nYp" // gitleaks:allow -- public upstream regression fixture or detector example
	var hits []Hit
	body := `{"messages":[{"role":"user","content":"key ` + key + `"},{"role":"user","content":"again ` + key + `"}]}`
	_, n := MaskJSON([]byte(body), Options{Secrets: true, Seen: func(h Hit) { hits = append(hits, h) }})
	if n != 2 || len(hits) != 2 || hits[0].Placeholder != hits[1].Placeholder || hits[0].By != ByRule {
		t.Fatalf("n=%d hits=%v", n, hits)
	}
}

// A hint keeps a key's prefix and a name's first character, and how long
// each is, nothing more.
func TestHint(t *testing.T) {
	for _, c := range []struct {
		kind, v, head string
		n             int
	}{
		{"API_KEY", "sk-proj-Hq3vX9tLm2Wc8RzK4nYp", "sk-", 28}, // gitleaks:allow -- public upstream regression fixture or detector example
		{"API_KEY", "ghp_abcdefghijklmnopqrstu", "ghp_", 25},
		{"PERSON", "张伟", "张", 2},
		{"PERSON", "Rina Kowalczyk", "R", 14},
		{"EMAIL", "rina@example.com", "r", 16},
		{"TERM", "Bluefin-codename-x", "B", 18},
		{"PHONE", "1", "", 1},
	} {
		if head, n := Hint(c.kind, c.v); head != c.head || n != c.n {
			t.Errorf("Hint(%s, %q) = %q, %d; want %q, %d", c.kind, c.v, head, n, c.head, c.n)
		}
	}
}
