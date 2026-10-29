package redact

import (
	"strings"
	"testing"
)

// Masked words as whole words only, when the user asks for it (acmilan_ on
// Discord), with the reporter's own examples: ABC masks "ABC IS xxx" and
// name='ABC' but no part of XABCD or ABCDEF, and 123456 masks a 123456 of
// its own but no part of 91234567, 0.123456 or 123456.78. Off, the
// default, a word is found inside longer ones as before. Each masked one
// comes back as it was written.
func TestWordsWhole(t *testing.T) {
	for _, c := range []struct {
		in        string
		word      string
		anyCase   bool
		whole     int // how many masked with whole words on
		anywhere  int // and off
		keptWhole []string
	}{
		{"ABC IS xxx", "ABC", false, 1, 1, nil},
		{"name='ABC'", "ABC", false, 1, 1, nil},
		{"XABCD", "ABC", false, 0, 1, []string{"XABCD"}},
		{"ABCDEF", "ABC", false, 0, 1, []string{"ABCDEF"}},
		{"ABC_DEF and ABC1", "ABC", false, 0, 2, []string{"ABC_DEF", "ABC1"}},
		{"XABCD then ABC.", "ABC", false, 1, 2, []string{"XABCD then "}},
		{"id 123456 here", "123456", false, 1, 1, nil},
		{"91234567", "123456", false, 0, 1, []string{"91234567"}},
		{"0.123456", "123456", false, 0, 1, []string{"0.123456"}},
		{"123456.78", "123456", false, 0, 1, []string{"123456.78"}},
		// a full stop that ends a sentence is no decimal point
		{"the code is 123456.", "123456", false, 1, 1, nil},
		{"v.123456 and 123456,7", "123456", false, 2, 2, nil},
		// Chinese and Japanese put no spaces between words: a word of
		// theirs is found inside their text, and a Latin one beside it
		{"我的同事张三今天来了", "张三", false, 1, 1, nil},
		{"用ABC做的项目", "ABC", false, 1, 1, nil},
		{"これはABCです", "ABC", false, 1, 1, nil},
		{"用XABC做的", "ABC", false, 0, 1, []string{"XABC"}},
		// with any case too, and a word inside another is passed over to
		// the next whole one
		{"xabc then Abc!", "ABC", true, 1, 2, []string{"xabc then "}},
		{"Москва, москвабург", "москва", true, 1, 2, []string{"бург"}},
	} {
		for _, whole := range []bool{true, false} {
			o := Options{Words: []string{c.word}, WordsAnyCase: c.anyCase, WordsWhole: whole}
			out, n := Mask(c.in, o)
			want := c.anywhere
			if whole {
				want = c.whole
			}
			if n != want {
				t.Errorf("%q in %q, whole %v: %d masked, want %d: %q", c.word, c.in, whole, n, want, out)
			}
			if whole {
				for _, k := range c.keptWhole {
					if !strings.Contains(out, k) {
						t.Errorf("%q in %q: %q gone from %q", c.word, c.in, k, out)
					}
				}
			}
			if back := Restore(out, false); back != c.in {
				t.Errorf("%q in %q, whole %v: round trip %q → %q", c.word, c.in, whole, out, back)
			}
		}
	}
}

// In a request body, as the gateway masks it, a tool call's arguments
// included: the escapes around a word are read as the vendor reads them.
func TestWordsWholeJSON(t *testing.T) {
	o := Options{Words: []string{"ABC", "123456"}, WordsWhole: true}
	in := `{"messages":[{"role":"user","content":"name='ABC' XABCD 0.123456 pin 123456"},` +
		`{"role":"assistant","tool_calls":[{"type":"function","function":{"name":"f","arguments":"{\"q\":\"ABC\\nXABCD 123456.78\"}"}}]}]}`
	out, _ := MaskJSON([]byte(in), o)
	s := string(out)
	for _, k := range []string{"XABCD", "0.123456", "123456.78"} {
		if strings.Count(s, k) != map[string]int{"XABCD": 2, "0.123456": 1, "123456.78": 1}[k] {
			t.Errorf("%q not kept as written: %s", k, s)
		}
	}
	if strings.Contains(unmarked(s), "'ABC'") || strings.Contains(unmarked(s), "pin 123456") || strings.Contains(s, `\"ABC\\n`) {
		t.Errorf("a whole word went unmasked: %s", s)
	}
	if back := Restore(s, false); back == s || !strings.Contains(back, "name='ABC'") {
		t.Errorf("not put back: %s", back)
	}
}
