package executor

import (
	"testing"
	"time"
)

func TestDefaultAttemptBudgetProvidesPositiveCaps(t *testing.T) {
	budget := DefaultAttemptBudget()

	if budget.RequestTimeout <= 0 {
		t.Fatalf("RequestTimeout = %v, want > 0", budget.RequestTimeout)
	}
	if budget.TotalTimeout <= 0 {
		t.Fatalf("TotalTimeout = %v, want > 0", budget.TotalTimeout)
	}
	if budget.FirstByteTimeout <= 0 {
		t.Fatalf("FirstByteTimeout = %v, want > 0", budget.FirstByteTimeout)
	}
	if budget.StreamIdleTimeout <= 0 {
		t.Fatalf("StreamIdleTimeout = %v, want > 0", budget.StreamIdleTimeout)
	}
	if budget.MaxRetryAfter <= 0 {
		t.Fatalf("MaxRetryAfter = %v, want > 0", budget.MaxRetryAfter)
	}
	if budget.MaxRetryWait <= 0 {
		t.Fatalf("MaxRetryWait = %v, want > 0", budget.MaxRetryWait)
	}
}

func TestAttemptBudgetCapsRetryWait(t *testing.T) {
	budget := AttemptBudget{
		MaxRetryWait: 5 * time.Second,
	}

	got := budget.ClampRetryWait(30 * time.Second)

	if got != 5*time.Second {
		t.Fatalf("ClampRetryWait(30s) = %v, want 5s", got)
	}
}

func TestAttemptBudgetKeepsShorterRetryWait(t *testing.T) {
	budget := AttemptBudget{
		MaxRetryWait: 5 * time.Second,
	}

	got := budget.ClampRetryWait(2 * time.Second)

	if got != 2*time.Second {
		t.Fatalf("ClampRetryWait(2s) = %v, want 2s", got)
	}
}

func TestAttemptBudgetReturnsZeroWhenRemainingBudgetExhausted(t *testing.T) {
	budget := AttemptBudget{
		TotalTimeout: 2 * time.Second,
	}

	start := time.Now().Add(-3 * time.Second)
	got := budget.RemainingSince(start)

	if got != 0 {
		t.Fatalf("RemainingSince(exhausted) = %v, want 0", got)
	}
}

func TestAttemptBudgetReturnsZeroWhenRequestBudgetExhausted(t *testing.T) {
	budget := AttemptBudget{
		RequestTimeout: 2 * time.Second,
	}

	start := time.Now().Add(-3 * time.Second)
	got := budget.RequestRemainingSince(start)

	if got != 0 {
		t.Fatalf("RequestRemainingSince(exhausted) = %v, want 0", got)
	}
}
