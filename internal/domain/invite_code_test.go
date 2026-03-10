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
	if got := InviteCodePrefix("ab cd"); got != "ABCD" {
		t.Fatalf("InviteCodePrefix(\"ab cd\") = %q, want %q", got, "ABCD")
	}
	if got := InviteCodePrefix("abcd-efgh-ijkl"); got != "ABCDEFGH" {
		t.Fatalf("InviteCodePrefix(\"abcd-efgh-ijkl\") = %q, want %q", got, "ABCDEFGH")
	}
}
