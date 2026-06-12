package output

import (
	"testing"
	"unicode/utf8"
)

func TestTruncateNoTruncation(t *testing.T) {
	got := Truncate("abc", 10)
	if got != "abc" {
		t.Errorf("Truncate(\"abc\", 10) = %q", got)
	}
}

func TestTruncateASCII(t *testing.T) {
	got := Truncate("abcdef", 3)
	if got != "abc…" {
		t.Errorf("Truncate ASCII = %q, want %q", got, "abc…")
	}
}

// TestTruncateMultibyteUTF8 guards against the byte-index regression: cutting
// a 4-rune Chinese string at 3 runes must yield 3 well-formed runes, not a
// 3-byte truncation that splits the second rune.
func TestTruncateMultibyteUTF8(t *testing.T) {
	got := Truncate("你好世界", 3)
	if got != "你好世…" {
		t.Errorf("Truncate(\"你好世界\", 3) = %q, want %q", got, "你好世…")
	}
	if !utf8.ValidString(got) {
		t.Errorf("Truncate output is not valid UTF-8: %q", got)
	}
}

func TestTruncateZeroOrNegative(t *testing.T) {
	if got := Truncate("hello", 0); got != "hello" {
		t.Errorf("Truncate n=0 should pass through, got %q", got)
	}
	if got := Truncate("hello", -1); got != "hello" {
		t.Errorf("Truncate n<0 should pass through, got %q", got)
	}
}
