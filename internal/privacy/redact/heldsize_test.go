package redact

import (
	"fmt"
	"strings"
	"testing"
)

// A masked body is held as the request's body while it streams (#3383),
// so it is held at about its size: a buffer grown by doubling was up to
// twice that, with few values masked or with many placeholders longer
// than what they stand for.
func TestMaskedBodyHeldAtItsSize(t *testing.T) {
	for _, every := range []int{1, 400} {
		var sb strings.Builder
		sb.WriteString(`{"messages":[`)
		for i := 0; i < 4000; i++ {
			if i > 0 {
				sb.WriteString(",")
			}
			text := strings.Repeat("some words about the build, nothing to mask in them. ", 20)
			if i%every == 0 {
				text += fmt.Sprintf("mail me at user%d@example.com or call 1380013%04d", i, i)
			}
			fmt.Fprintf(&sb, `{"role":"user","content":%q}`, text)
		}
		sb.WriteString(`]}`)
		body := []byte(sb.String())
		out, n := MaskJSON(body, Options{Personal: true})
		if n == 0 {
			t.Fatalf("every %d: nothing masked", every)
		}
		if slack := cap(out) - len(out); slack > len(out)/8+len(body)/16+64 {
			t.Errorf("every %d: the masked body of %d bytes is held in %d", every, len(out), cap(out))
		}
	}
}
