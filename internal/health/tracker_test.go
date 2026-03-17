package health

import (
	"math"
	"net/http"
	"testing"
	"time"
)

func TestTrackerPrefersLowerLatencyHigherSuccessProvider(t *testing.T) {
	tracker := NewTracker()

	for range 5 {
		tracker.Record(AttemptResult{
			ProviderID: 1,
			ClientType: "claude",
			Success:    true,
			Duration:   80 * time.Millisecond,
			TTFT:       30 * time.Millisecond,
		})
	}

	tracker.Record(AttemptResult{
		ProviderID: 2,
		ClientType: "claude",
		Success:    true,
		Duration:   400 * time.Millisecond,
		TTFT:       150 * time.Millisecond,
	})
	tracker.Record(AttemptResult{
		ProviderID:  2,
		ClientType:  "claude",
		Success:     false,
		Duration:    400 * time.Millisecond,
		IsServerErr: true,
	})

	if got1, got2 := tracker.Score(1, "claude"), tracker.Score(2, "claude"); got1 <= got2 {
		t.Fatalf("Score(provider1) = %f, Score(provider2) = %f, want provider1 > provider2", got1, got2)
	}
}

func TestTrackerInFlightPenaltyLowersScore(t *testing.T) {
	tracker := NewTracker()
	tracker.Record(AttemptResult{
		ProviderID: 1,
		ClientType: "codex",
		Success:    true,
		Duration:   100 * time.Millisecond,
		TTFT:       40 * time.Millisecond,
	})

	before := tracker.Score(1, "codex")
	done := tracker.BeginAttempt(1, "codex")
	during := tracker.Score(1, "codex")
	done()

	if during >= before {
		t.Fatalf("Score with in-flight penalty = %f, baseline = %f, want lower score during in-flight", during, before)
	}
}

func TestTrackerOpensBreakerAfterConsecutiveFailures(t *testing.T) {
	tracker := NewTracker()

	for range 3 {
		tracker.Record(AttemptResult{
			ProviderID: 7,
			ClientType: "kiro",
			Success:    false,
			Duration:   2 * time.Second,
			IsTimeout:  true,
		})
	}

	if !tracker.IsCircuitOpen(7, "kiro") {
		t.Fatal("IsCircuitOpen = false, want true after 3 consecutive unhealthy failures")
	}

	snapshot := tracker.Snapshot(7, "kiro")
	if snapshot.State != BreakerOpen {
		t.Fatalf("State = %s, want %s", snapshot.State, BreakerOpen)
	}
}

func TestTrackerHalfOpenAllowsSingleProbeAndClosesAfterSuccess(t *testing.T) {
	tracker := NewTracker()
	baseTime := time.Date(2026, 3, 17, 14, 0, 0, 0, time.UTC)
	tracker.now = func() time.Time { return baseTime }

	for range 3 {
		tracker.Record(AttemptResult{
			ProviderID: 9,
			ClientType: "openai",
			Success:    false,
			Duration:   time.Second,
			IsNetwork:  true,
		})
	}

	if tracker.AllowAttempt(9, "openai") {
		t.Fatal("AllowAttempt during open window = true, want false")
	}

	tracker.now = func() time.Time { return baseTime.Add(31 * time.Second) }

	if !tracker.AllowAttempt(9, "openai") {
		t.Fatal("AllowAttempt after open window = false, want true for half-open probe")
	}

	done := tracker.BeginAttempt(9, "openai")
	if tracker.AllowAttempt(9, "openai") {
		t.Fatal("AllowAttempt with half-open probe in flight = true, want false")
	}
	done()

	tracker.Record(AttemptResult{
		ProviderID: 9,
		ClientType: "openai",
		Success:    true,
		StartedAt:  baseTime.Add(31 * time.Second),
		Duration:   120 * time.Millisecond,
		TTFT:       50 * time.Millisecond,
	})

	if tracker.IsCircuitOpen(9, "openai") {
		t.Fatal("IsCircuitOpen after successful half-open probe = true, want false")
	}

	snapshot := tracker.Snapshot(9, "openai")
	if snapshot.State != BreakerClosed {
		t.Fatalf("State after successful half-open probe = %s, want %s", snapshot.State, BreakerClosed)
	}
}

func TestTrackerHalfOpenAllowsOnlySingleConcurrentProbe(t *testing.T) {
	tracker := NewTracker()
	baseTime := time.Date(2026, 3, 17, 14, 30, 0, 0, time.UTC)
	tracker.now = func() time.Time { return baseTime }

	for range 3 {
		tracker.Record(AttemptResult{
			ProviderID: 9,
			ClientType: "openai",
			Success:    false,
			Duration:   time.Second,
			IsTimeout:  true,
		})
	}

	tracker.now = func() time.Time { return baseTime.Add(31 * time.Second) }

	start := make(chan struct{})
	results := make(chan bool, 8)
	for range 8 {
		go func() {
			<-start
			results <- tracker.AllowAttempt(9, "openai")
		}()
	}
	close(start)

	allowed := 0
	for range 8 {
		if <-results {
			allowed++
		}
	}

	if allowed != 1 {
		t.Fatalf("allowed probes = %d, want exactly 1 concurrent half-open probe", allowed)
	}
}

func TestTrackerHalfOpenProbeStaysReservedUntilResultRecorded(t *testing.T) {
	tracker := NewTracker()
	baseTime := time.Date(2026, 3, 17, 14, 45, 0, 0, time.UTC)
	tracker.now = func() time.Time { return baseTime }

	for range 3 {
		tracker.Record(AttemptResult{
			ProviderID: 21,
			ClientType: "openai",
			Success:    false,
			Duration:   time.Second,
			IsTimeout:  true,
		})
	}

	tracker.now = func() time.Time { return baseTime.Add(31 * time.Second) }
	if !tracker.AllowAttempt(21, "openai") {
		t.Fatal("AllowAttempt after open window = false, want true for first half-open probe")
	}

	done := tracker.BeginAttempt(21, "openai")
	done()

	if tracker.AllowAttempt(21, "openai") {
		t.Fatal("second AllowAttempt after done but before Record = true, want false")
	}

	tracker.Record(AttemptResult{
		ProviderID: 21,
		ClientType: "openai",
		Success:    true,
		StartedAt:  baseTime.Add(31 * time.Second),
		Duration:   80 * time.Millisecond,
		TTFT:       20 * time.Millisecond,
	})

	if tracker.IsCircuitOpen(21, "openai") {
		t.Fatal("IsCircuitOpen after successful half-open probe = true, want false")
	}
	if !tracker.AllowAttempt(21, "openai") {
		t.Fatal("AllowAttempt after successful half-open probe = false, want closed breaker")
	}
}

func TestTrackerHalfOpenProbeFailureReopensBreakerAfterResultRecorded(t *testing.T) {
	tracker := NewTracker()
	baseTime := time.Date(2026, 3, 17, 14, 50, 0, 0, time.UTC)
	tracker.now = func() time.Time { return baseTime }

	for range 3 {
		tracker.Record(AttemptResult{
			ProviderID: 22,
			ClientType: "openai",
			Success:    false,
			Duration:   time.Second,
			IsTimeout:  true,
		})
	}

	tracker.now = func() time.Time { return baseTime.Add(31 * time.Second) }
	if !tracker.AllowAttempt(22, "openai") {
		t.Fatal("AllowAttempt after open window = false, want true for first half-open probe")
	}

	done := tracker.BeginAttempt(22, "openai")
	done()

	if tracker.AllowAttempt(22, "openai") {
		t.Fatal("second AllowAttempt after done but before failure Record = true, want false")
	}

	tracker.Record(AttemptResult{
		ProviderID: 22,
		ClientType: "openai",
		Success:    false,
		StartedAt:  baseTime.Add(31 * time.Second),
		Duration:   120 * time.Millisecond,
		IsTimeout:  true,
	})

	if !tracker.IsCircuitOpen(22, "openai") {
		t.Fatal("IsCircuitOpen after failed half-open probe = false, want true")
	}
	if tracker.AllowAttempt(22, "openai") {
		t.Fatal("AllowAttempt immediately after failed half-open probe = true, want false")
	}
}

func TestTrackerHalfOpenProbeRateLimitReopensBreakerAfterResultRecorded(t *testing.T) {
	tracker := NewTracker()
	baseTime := time.Date(2026, 3, 17, 14, 55, 0, 0, time.UTC)
	tracker.now = func() time.Time { return baseTime }

	for range 3 {
		tracker.Record(AttemptResult{
			ProviderID: 23,
			ClientType: "openai",
			Success:    false,
			Duration:   time.Second,
			IsTimeout:  true,
		})
	}

	tracker.now = func() time.Time { return baseTime.Add(31 * time.Second) }
	if !tracker.AllowAttempt(23, "openai") {
		t.Fatal("AllowAttempt after open window = false, want true for first half-open probe")
	}

	done := tracker.BeginAttempt(23, "openai")
	done()

	if tracker.AllowAttempt(23, "openai") {
		t.Fatal("second AllowAttempt after done but before rate-limit Record = true, want false")
	}

	tracker.Record(AttemptResult{
		ProviderID:  23,
		ClientType:  "openai",
		Success:     false,
		StartedAt:   baseTime.Add(31 * time.Second),
		Duration:    200 * time.Millisecond,
		StatusCode:  http.StatusTooManyRequests,
		IsRateLimit: true,
	})

	if !tracker.IsCircuitOpen(23, "openai") {
		t.Fatal("IsCircuitOpen after rate-limited half-open probe = false, want true")
	}
	if tracker.AllowAttempt(23, "openai") {
		t.Fatal("AllowAttempt during reopened breaker window = true, want false")
	}
}

func TestTrackerHalfOpenProbeClientErrorKeepsHalfOpen(t *testing.T) {
	tracker := NewTracker()
	baseTime := time.Date(2026, 3, 17, 14, 57, 0, 0, time.UTC)
	tracker.now = func() time.Time { return baseTime }

	for range 3 {
		tracker.Record(AttemptResult{
			ProviderID: 25,
			ClientType: "openai",
			Success:    false,
			Duration:   time.Second,
			IsTimeout:  true,
		})
	}

	tracker.now = func() time.Time { return baseTime.Add(31 * time.Second) }
	if !tracker.AllowAttempt(25, "openai") {
		t.Fatal("AllowAttempt after open window = false, want true for first half-open probe")
	}

	done := tracker.BeginAttempt(25, "openai")
	done()

	tracker.Record(AttemptResult{
		ProviderID: 25,
		ClientType: "openai",
		Success:    false,
		StartedAt:  baseTime.Add(31 * time.Second),
		Duration:   80 * time.Millisecond,
		StatusCode: http.StatusBadRequest,
	})

	snapshot := tracker.Snapshot(25, "openai")
	if snapshot.State != BreakerHalfOpen {
		t.Fatalf("State after half-open client error = %s, want %s", snapshot.State, BreakerHalfOpen)
	}
	if !tracker.AllowAttempt(25, "openai") {
		t.Fatal("AllowAttempt after half-open client error = false, want probe slot released")
	}
}

func TestTrackerReleaseHalfOpenProbeAllowsNextProbeWithoutRecording(t *testing.T) {
	tracker := NewTracker()
	baseTime := time.Date(2026, 3, 17, 15, 0, 0, 0, time.UTC)
	tracker.now = func() time.Time { return baseTime }

	for range 3 {
		tracker.Record(AttemptResult{
			ProviderID: 24,
			ClientType: "openai",
			Success:    false,
			Duration:   time.Second,
			IsTimeout:  true,
		})
	}

	tracker.now = func() time.Time { return baseTime.Add(31 * time.Second) }
	if !tracker.AllowAttempt(24, "openai") {
		t.Fatal("AllowAttempt after open window = false, want true for first half-open probe")
	}

	done := tracker.BeginAttempt(24, "openai")
	done()

	tracker.ReleaseHalfOpenProbe(24, "openai", baseTime.Add(31*time.Second))

	if !tracker.AllowAttempt(24, "openai") {
		t.Fatal("AllowAttempt after releasing half-open probe = false, want true for next probe")
	}

	snapshot := tracker.Snapshot(24, "openai")
	if snapshot.State != BreakerHalfOpen {
		t.Fatalf("State after releasing half-open probe = %s, want %s", snapshot.State, BreakerHalfOpen)
	}
}

func TestTrackerOpenBreakerIgnoresStaleSuccessUntilHalfOpenProbeSucceeds(t *testing.T) {
	tracker := NewTracker()
	baseTime := time.Date(2026, 3, 17, 15, 30, 0, 0, time.UTC)
	tracker.now = func() time.Time { return baseTime }

	for range 3 {
		tracker.Record(AttemptResult{
			ProviderID: 11,
			ClientType: "claude",
			Success:    false,
			Duration:   500 * time.Millisecond,
			IsTimeout:  true,
		})
	}

	if !tracker.IsCircuitOpen(11, "claude") {
		t.Fatal("breaker should be open after 3 unhealthy failures")
	}

	// Simulate an older in-flight request succeeding after the breaker was opened.
	tracker.Record(AttemptResult{
		ProviderID: 11,
		ClientType: "claude",
		Success:    true,
		StartedAt:  baseTime.Add(-time.Second),
		Duration:   50 * time.Millisecond,
		TTFT:       20 * time.Millisecond,
	})

	if !tracker.IsCircuitOpen(11, "claude") {
		t.Fatal("stale success should not close an already-open breaker")
	}

	tracker.now = func() time.Time { return baseTime.Add(31 * time.Second) }
	if !tracker.AllowAttempt(11, "claude") {
		t.Fatal("half-open probe should be allowed after open window elapses")
	}
	tracker.Record(AttemptResult{
		ProviderID: 11,
		ClientType: "claude",
		Success:    true,
		StartedAt:  baseTime.Add(31 * time.Second),
		Duration:   40 * time.Millisecond,
		TTFT:       10 * time.Millisecond,
	})

	if tracker.IsCircuitOpen(11, "claude") {
		t.Fatal("successful half-open probe should close the breaker")
	}
}

func TestTrackerDoesNotOpenBreakerForFailuresSpreadAcrossDecayWindow(t *testing.T) {
	tracker := NewTracker()
	baseTime := time.Date(2026, 3, 17, 16, 0, 0, 0, time.UTC)
	current := baseTime
	tracker.now = func() time.Time { return current }

	for range 3 {
		tracker.Record(AttemptResult{
			ProviderID: 12,
			ClientType: "openai",
			Success:    false,
			Duration:   400 * time.Millisecond,
			IsTimeout:  true,
		})
		current = current.Add(3 * time.Minute)
	}

	if tracker.IsCircuitOpen(12, "openai") {
		t.Fatal("breaker should stay closed when unhealthy failures are spread across the decay window")
	}
}

func TestTrackerRecentFailureOutweighsOldSuccesses(t *testing.T) {
	tracker := NewTracker()
	baseTime := time.Date(2026, 3, 17, 15, 0, 0, 0, time.UTC)
	tracker.now = func() time.Time { return baseTime }

	for range 100 {
		tracker.Record(AttemptResult{
			ProviderID: 1,
			ClientType: "claude",
			Success:    true,
			Duration:   10 * time.Millisecond,
			TTFT:       5 * time.Millisecond,
		})
	}

	tracker.now = func() time.Time { return baseTime.Add(15 * time.Minute) }
	tracker.Record(AttemptResult{
		ProviderID: 1,
		ClientType: "claude",
		Success:    false,
		Duration:   150 * time.Millisecond,
		IsTimeout:  true,
	})

	tracker.Record(AttemptResult{
		ProviderID: 2,
		ClientType: "claude",
		Success:    true,
		Duration:   120 * time.Millisecond,
		TTFT:       40 * time.Millisecond,
	})

	if got1, got2 := tracker.Score(1, "claude"), tracker.Score(2, "claude"); got1 >= got2 {
		t.Fatalf("Score(provider1) = %f, Score(provider2) = %f, want old success history to decay below recent healthy provider", got1, got2)
	}
}

func TestTrackerRateLimitPenaltyAffectsScore(t *testing.T) {
	tracker := NewTracker()

	tracker.Record(AttemptResult{
		ProviderID: 1,
		ClientType: "codex",
		Success:    true,
		Duration:   90 * time.Millisecond,
		TTFT:       30 * time.Millisecond,
	})
	tracker.Record(AttemptResult{
		ProviderID: 2,
		ClientType: "codex",
		Success:    true,
		Duration:   90 * time.Millisecond,
		TTFT:       30 * time.Millisecond,
	})
	tracker.Record(AttemptResult{
		ProviderID:  1,
		ClientType:  "codex",
		Success:     false,
		Duration:    100 * time.Millisecond,
		IsRateLimit: true,
		StatusCode:  http.StatusTooManyRequests,
	})
	tracker.Record(AttemptResult{
		ProviderID: 2,
		ClientType: "codex",
		Success:    false,
		Duration:   100 * time.Millisecond,
		StatusCode: http.StatusBadRequest,
	})

	if got1, got2 := tracker.Score(1, "codex"), tracker.Score(2, "codex"); got1 >= got2 {
		t.Fatalf("Score(rate-limited provider) = %f, Score(generic failure provider) = %f, want lower score for recent rate limit", got1, got2)
	}
}

func TestTrackerDecayUsesConfiguredHalfLife(t *testing.T) {
	tracker := NewTracker()
	baseTime := time.Date(2026, 3, 17, 16, 30, 0, 0, time.UTC)
	tracker.now = func() time.Time { return baseTime }
	tracker.decayHalfLife = 2 * time.Minute

	tracker.Record(AttemptResult{
		ProviderID: 1,
		ClientType: "claude",
		Success:    true,
	})

	tracker.now = func() time.Time { return baseTime.Add(tracker.decayHalfLife) }
	_ = tracker.Score(1, "claude")

	tracker.mu.Lock()
	got := tracker.stats["1:claude"].recentSuccess
	tracker.mu.Unlock()

	if math.Abs(got-0.5) > 0.01 {
		t.Fatalf("recentSuccess after one half-life = %f, want about 0.5", got)
	}
}

func TestTrackerDecayHalfLifeHalvesRecentSignal(t *testing.T) {
	tracker := NewTracker()
	baseTime := time.Date(2026, 3, 17, 16, 0, 0, 0, time.UTC)
	tracker.now = func() time.Time { return baseTime }

	tracker.Record(AttemptResult{
		ProviderID: 31,
		ClientType: "openai",
		Success:    true,
		Duration:   50 * time.Millisecond,
	})

	tracker.now = func() time.Time { return baseTime.Add(tracker.decayHalfLife) }
	_ = tracker.Score(31, "openai")

	tracker.mu.Lock()
	recentSuccess := tracker.getStatsLocked(31, "openai").recentSuccess
	tracker.mu.Unlock()

	if diff := recentSuccess - 0.5; diff < -0.05 || diff > 0.05 {
		t.Fatalf("recentSuccess after one half-life = %.4f, want about 0.5", recentSuccess)
	}
}
