package codex

import (
	"testing"
	"time"
)

func TestTokenExpiresAtClampsMalformedExpiresIn(t *testing.T) {
	before := time.Now()
	expiresAt := TokenExpiresAt(0)
	after := time.Now()

	if expiresAt.Before(before.Add(60 * time.Second)) {
		t.Fatalf("expected malformed expires_in to be clamped to at least 60s, got %v before lower bound %v", expiresAt, before.Add(60*time.Second))
	}
	if expiresAt.After(after.Add(61 * time.Second)) {
		t.Fatalf("expected malformed expires_in clamp near 60s, got %v after upper bound %v", expiresAt, after.Add(61*time.Second))
	}
}

func TestTokenExpiresAtPreservesValidExpiresIn(t *testing.T) {
	before := time.Now()
	expiresAt := TokenExpiresAt(3600)
	after := time.Now()

	if expiresAt.Before(before.Add(3600 * time.Second)) {
		t.Fatalf("expected valid expires_in to be preserved, got %v before lower bound %v", expiresAt, before.Add(3600*time.Second))
	}
	if expiresAt.After(after.Add(3601 * time.Second)) {
		t.Fatalf("expected valid expires_in near 3600s, got %v after upper bound %v", expiresAt, after.Add(3601*time.Second))
	}
}
