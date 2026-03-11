package domain

import "testing"

func TestInviteCodePrefix_Empty(t *testing.T) {
	if got := InviteCodePrefix(""); got != InviteCodeInvalidPrefix {
		t.Fatalf("InviteCodePrefix(\"\") = %q, want %q", got, InviteCodeInvalidPrefix)
	}
}

func TestInviteCodePrefix_Whitespace(t *testing.T) {
	if got := InviteCodePrefix("   \t"); got != InviteCodeInvalidPrefix {
		t.Fatalf("InviteCodePrefix(\"whitespace\") = %q, want %q", got, InviteCodeInvalidPrefix)
	}
}

func TestInviteCodePrefix_Normalized(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "spaces normalized", input: "ab cd", want: "ABCD"},
		{name: "hyphens normalized", input: "abcd-efgh-ijkl", want: "ABCDEFGH"},
		{name: "edge-empty-placeholder", input: "", want: InviteCodeInvalidPrefix},
		{name: "edge-whitespace-placeholder", input: "   \t", want: InviteCodeInvalidPrefix},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InviteCodePrefix(tt.input); got != tt.want {
				t.Fatalf("InviteCodePrefix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeInviteCode_UnicodeSeparators(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "nbspace removed", input: "ab\u00A0cd", want: "ABCD"},
		{name: "fullwidth space removed", input: "ab\u3000cd", want: "ABCD"},
		{name: "tabs removed", input: "ab\tcd", want: "ABCD"},
		{name: "en dash removed", input: "ab\u2013cd", want: "ABCD"},
		{name: "em dash removed", input: "ab\u2014cd", want: "ABCD"},
		{name: "minus sign removed", input: "ab\u2212cd", want: "ABCD"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeInviteCode(tt.input); got != tt.want {
				t.Fatalf("NormalizeInviteCode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
