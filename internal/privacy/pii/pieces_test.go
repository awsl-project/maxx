package pii

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/awsl-project/maxx/internal/privacy/redact"
)

// alices are where each "Alice" in t is, as the stand-in finds them.
func alices(t string) []redact.Found {
	var out []redact.Found
	for i := 0; ; {
		j := strings.Index(t[i:], "Alice")
		if j < 0 {
			return out
		}
		out = append(out, person(i+j, i+j+5))
		i += j + 5
	}
}

// numbered is n lines, each with an Alice and each its own, so no two
// pieces of it are alike.
func numbered(n int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "line %d with Alice in it, and some words after\n", i)
	}
	return b.String()
}

// A long text's pieces: none empty, none over pieceSize, together the
// whole text, each overlapping the one before, cut after a line's end
// where there is one, and at a rune's start where there is no line's end
// or space at all.
func TestPiecesCut(t *testing.T) {
	if got := pieces(""); got != nil {
		t.Fatalf("empty text: %v", got)
	}
	if got := pieces(strings.Repeat("x", pieceSize)); !reflect.DeepEqual(got, []span{{0, pieceSize}}) {
		t.Fatalf("a text of pieceSize: %v", got)
	}
	lines := numbered(2000)
	cjk := strings.Repeat("张伟的密码是", 6000) // no line's end or space: 3-byte runes
	words := strings.Repeat("word ", 20000)
	for name, text := range map[string]string{"lines": lines, "cjk": cjk, "words": words} {
		ps := pieces(text)
		if len(ps) < 2 {
			t.Fatalf("%s: %d pieces of %d bytes", name, len(ps), len(text))
		}
		if ps[0].at != 0 || ps[len(ps)-1].end != len(text) {
			t.Fatalf("%s: pieces %v don't cover 0..%d", name, ps, len(text))
		}
		for i, p := range ps {
			piece := text[p.at:p.end]
			if piece == "" || len(piece) > pieceSize || !utf8.ValidString(piece) {
				t.Fatalf("%s: piece %d %v: %d bytes, valid %v", name, i, p, len(piece), utf8.ValidString(piece))
			}
			if i == 0 {
				continue
			}
			prev := ps[i-1]
			if p.at <= prev.at || p.at >= prev.end || prev.end-p.at > pieceOverlap {
				t.Fatalf("%s: piece %d %v after %v: want an overlap of at most %d", name, i, p, prev, pieceOverlap)
			}
			if name == "lines" && (text[prev.end-1] != '\n' || text[p.at-1] != '\n') {
				t.Fatalf("%s: piece %d %v after %v not cut at a line's end", name, i, p, prev)
			}
			if name == "words" && (text[prev.end-1] != ' ' || text[p.at-1] != ' ') {
				t.Fatalf("%s: piece %d %v after %v not cut at a space", name, i, p, prev)
			}
		}
	}
}

// Spans from overlapping pieces: one found twice is one, one cut by a
// piece's end and whole in the next is the whole, labels kept apart where
// spans only touch, and of one label joined, as sure as the surer.
func TestMergeAcrossPieces(t *testing.T) {
	email := func(s, e int) redact.Found { return redact.Found{Start: s, End: e, Label: "private_email"} }
	sure := email(209, 222)
	sure.Score = .99
	got := merge([]redact.Found{
		person(100, 105), person(10, 15), person(100, 105), // twice, from two pieces
		person(200, 205), person(200, 211), // cut by a piece's end, whole in the next
		sure,                              // runs into the name: one, the longer's label
		person(300, 305), email(305, 310), // touching, two labels
		person(400, 405), person(405, 410), // touching, one label
	})
	joined := email(200, 222)
	joined.Score = sure.Score
	want := []redact.Found{person(10, 15), person(100, 105), joined, person(300, 305), email(305, 310), person(400, 410)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merge =\n%v\nwant\n%v", got, want)
	}
}

// #3682: a long text goes to pf-serve a piece at a time, none over
// pieceSize, and what it finds comes back once each, at the text's own
// offsets: a name in two pieces' overlap once, and one a piece's end cut
// in two (a text with no line's end or space to cut at) whole.
func TestLongTextFoundAcrossPieces(t *testing.T) {
	log := fakeInstall(t, "ok", false)
	lines := numbered(900)
	// "Alice" from byte 12286: the first piece is cut at a rune's start at
	// 12288, between "Al" and "ice"; the next begins 1 KB before
	straddle := strings.Repeat("é", 6143) + "Alice" + strings.Repeat("é", 8000)
	got, err := Find([]string{lines, straddle, "hi Alice"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got[0], alices(lines)) {
		t.Fatalf("lines: %d spans, want %d: %v", len(got[0]), len(alices(lines)), got[0])
	}
	if !reflect.DeepEqual(got[1], []redact.Found{person(12286, 12291)}) {
		t.Fatalf("straddling: %v", got[1])
	}
	if !reflect.DeepEqual(got[2], []redact.Found{person(3, 8)}) {
		t.Fatalf("short: %v", got[2])
	}
	reqs := fakeLines(t, log, "request")
	// a piece is a chunk, joined only with what is left beside it
	if want := len(pieces(lines)) + len(pieces(straddle)); len(reqs) < want-2 {
		t.Fatalf("%d requests for %d pieces", len(reqs), want)
	}
	for _, r := range reqs {
		// the line quotes the texts, at most a chunk's bytes each
		if len(r) > 3*chunkSize {
			t.Fatalf("a request of %d bytes", len(r))
		}
	}
	// kept whole, and by its pieces: asked again, nothing goes to pf-serve
	n := len(reqs)
	again, err := Find([]string{lines, straddle})
	if err != nil || !reflect.DeepEqual(again[0], got[0]) || !reflect.DeepEqual(again[1], got[1]) {
		t.Fatalf("again: %v", err)
	}
	if reqs := fakeLines(t, log, "request"); len(reqs) != n {
		t.Fatalf("asked again: %d requests more", len(reqs)-n)
	}
}

// #3682: a long request whose client went away keeps the pieces pf-serve
// finished, so sent again it asks only about the rest; none is asked
// twice.
func TestLongTextKeepsPiecesWhenStopped(t *testing.T) {
	log := fakeInstall(t, "slow", false)
	text := numbered(1500)
	n := len(pieces(text))
	if n < 4 {
		t.Fatalf("%d pieces", n)
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := findAsync(ctx, text)
	waitFor(t, "a first piece asked", func() bool { return len(fakeLines(t, log, "request")) >= 1 })
	cancel()
	if r := <-c; r.err == nil {
		t.Fatal("a stopped request answered")
	}
	got, err := Find([]string{text})
	if err != nil || !reflect.DeepEqual(got[0], alices(text)) {
		t.Fatalf("sent again: %d spans, %v", len(got[0]), err)
	}
	if reqs := fakeLines(t, log, "request"); len(reqs) != n {
		t.Fatalf("%d requests for %d pieces: the stopped one's were asked again", len(reqs), n)
	}
}

// #3682: a short request waiting for the model goes between a long one's
// pieces, not after all of them.
func TestShortRequestGoesBetweenPieces(t *testing.T) {
	log := fakeInstall(t, "slow", false)
	text := numbered(1500)
	n := len(pieces(text))
	long := findAsync(context.Background(), text)
	waitFor(t, "a first piece asked", func() bool { return len(fakeLines(t, log, "request")) >= 1 })
	short := findAsync(context.Background(), "hi Alice")
	var r found
	select {
	case r = <-short:
	case r = <-long:
		t.Fatal("the long request finished first")
	}
	if r.err != nil || !reflect.DeepEqual(r.fs, [][]redact.Found{{person(3, 8)}}) {
		t.Fatalf("short: %v %v", r.fs, r.err)
	}
	if l := <-long; l.err != nil || !reflect.DeepEqual(l.fs[0], alices(text)) {
		t.Fatalf("long: %v", l.err)
	}
	reqs := fakeLines(t, log, "request")
	at := -1
	for i, q := range reqs {
		if strings.Contains(q, `"hi Alice"`) {
			at = i
		}
	}
	if len(reqs) != n+1 || at < 1 || at >= n {
		t.Fatalf("the short request was request %d of %d (%d pieces)", at, len(reqs), n)
	}
}
