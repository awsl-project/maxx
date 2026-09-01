package cooldown

import (
	"testing"
	"time"
)

// TestInsufficientBalancePolicyIsShort guards that an out-of-credit / account-
// locked state (fal "TOP_UP" / "Exhausted balance" 403) recovers quickly: it
// must map to a short fixed cooldown, NOT the heavy 1h auth-failure cooldown, so
// the provider comes back promptly after a top-up.
func TestInsufficientBalancePolicyIsShort(t *testing.T) {
	policies := DefaultPolicies()

	bal, ok := policies[ReasonInsufficientBalance]
	if !ok {
		t.Fatalf("DefaultPolicies missing ReasonInsufficientBalance")
	}
	got := bal.CalculateCooldown(1)
	if got != 2*time.Minute {
		t.Fatalf("insufficient_balance cooldown = %v, want 2m", got)
	}

	auth := policies[ReasonAuthFailure].CalculateCooldown(1)
	if got >= auth {
		t.Fatalf("insufficient_balance cooldown (%v) must be shorter than auth_failure (%v)", got, auth)
	}
}
