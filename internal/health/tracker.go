package health

import (
	"fmt"
	"math"
	"sync"
	"time"
)

type BreakerState string

const (
	BreakerClosed   BreakerState = "closed"
	BreakerOpen     BreakerState = "open"
	BreakerHalfOpen BreakerState = "half_open"
)

type AttemptResult struct {
	ProviderID  uint64
	ClientType  string
	Success     bool
	StartedAt   time.Time
	Duration    time.Duration
	TTFT        time.Duration
	StatusCode  int
	IsTimeout   bool
	IsNetwork   bool
	IsRateLimit bool
	IsServerErr bool
}

type ProviderTracker interface {
	BeginAttempt(providerID uint64, clientType string) func()
	Record(result AttemptResult)
	Score(providerID uint64, clientType string) float64
	IsCircuitOpen(providerID uint64, clientType string) bool
	AllowAttempt(providerID uint64, clientType string) bool
	ReleaseHalfOpenProbe(providerID uint64, clientType string, startedAt time.Time)
}

type Snapshot struct {
	State                        BreakerState
	EWMALatency                  time.Duration
	EWMATTFT                     time.Duration
	SuccessCount                 int
	FailureCount                 int
	ConsecutiveUnhealthyFailures int
	InFlight                     int
	OpenUntil                    time.Time
}

type Tracker struct {
	mu            sync.Mutex
	now           func() time.Time
	openDuration  time.Duration
	alpha         float64
	decayHalfLife time.Duration
	stats         map[string]*providerStats
}

var _ ProviderTracker = (*Tracker)(nil)

type providerStats struct {
	state                        BreakerState
	ewmaLatency                  time.Duration
	ewmaTTFT                     time.Duration
	successCount                 int
	failureCount                 int
	recentSuccess                float64
	recentFailure                float64
	recentTimeout                float64
	recentRateLimit              float64
	recentServerErr              float64
	consecutiveUnhealthyFailures int
	inFlight                     int
	openUntil                    time.Time
	halfOpenProbeInFlight        bool
	halfOpenProbeStartedAt       time.Time
	lastDecay                    time.Time
}

func NewTracker() *Tracker {
	return &Tracker{
		now:           time.Now,
		openDuration:  30 * time.Second,
		alpha:         0.35,
		decayHalfLife: 2 * time.Minute,
		stats:         make(map[string]*providerStats),
	}
}

func (t *Tracker) BeginAttempt(providerID uint64, clientType string) func() {
	t.mu.Lock()
	stats := t.getStatsLocked(providerID, clientType)
	t.refreshStateLocked(stats)
	t.decayLocked(stats)
	stats.inFlight++
	if stats.state == BreakerHalfOpen {
		stats.halfOpenProbeInFlight = true
	}
	t.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			t.mu.Lock()
			defer t.mu.Unlock()
			stats := t.getStatsLocked(providerID, clientType)
			if stats.inFlight > 0 {
				stats.inFlight--
			}
		})
	}
}

func (t *Tracker) Record(result AttemptResult) {
	t.mu.Lock()
	defer t.mu.Unlock()

	stats := t.getStatsLocked(result.ProviderID, result.ClientType)
	t.refreshStateLocked(stats)
	t.decayLocked(stats)

	if result.Duration > 0 {
		stats.ewmaLatency = ewmaDuration(stats.ewmaLatency, result.Duration, t.alpha)
	}
	if result.TTFT > 0 {
		stats.ewmaTTFT = ewmaDuration(stats.ewmaTTFT, result.TTFT, t.alpha)
	}

	if result.Success {
		stats.successCount++
		stats.recentSuccess++
		switch stats.state {
		case BreakerHalfOpen:
			if t.isHalfOpenProbeResultLocked(stats, result) {
				stats.consecutiveUnhealthyFailures = 0
				stats.state = BreakerClosed
				stats.openUntil = time.Time{}
				stats.halfOpenProbeInFlight = false
				stats.halfOpenProbeStartedAt = time.Time{}
			}
		case BreakerClosed:
			stats.consecutiveUnhealthyFailures = 0
		}
		return
	}

	stats.failureCount++
	stats.recentFailure++
	if result.IsTimeout || result.IsNetwork {
		stats.recentTimeout++
	}
	if result.IsRateLimit {
		stats.recentRateLimit++
	}
	if result.IsServerErr {
		stats.recentServerErr++
	}
	if isUnhealthyFailure(result) {
		stats.consecutiveUnhealthyFailures++
	} else {
		stats.consecutiveUnhealthyFailures = 0
	}

	if stats.state == BreakerHalfOpen {
		if t.isHalfOpenProbeResultLocked(stats, result) {
			if isHalfOpenUnhealthyFailure(result) {
				t.openBreakerLocked(stats)
			} else {
				stats.halfOpenProbeInFlight = false
				stats.halfOpenProbeStartedAt = time.Time{}
			}
		}
		return
	}

	if stats.consecutiveUnhealthyFailures >= 3 {
		t.openBreakerLocked(stats)
	}
}

func (t *Tracker) Score(providerID uint64, clientType string) float64 {
	t.mu.Lock()
	defer t.mu.Unlock()

	stats := t.getStatsLocked(providerID, clientType)
	t.refreshStateLocked(stats)
	t.decayLocked(stats)

	total := stats.recentSuccess + stats.recentFailure
	successRate := 0.5
	if total > 0 {
		successRate = stats.recentSuccess / total
	}

	score := successRate * 1000
	score -= float64(stats.ewmaLatency.Milliseconds())
	score -= float64(stats.ewmaTTFT.Milliseconds()) * 0.5
	score -= float64(stats.inFlight) * 50
	score -= stats.recentTimeout * 150
	score -= stats.recentRateLimit * 120
	score -= stats.recentServerErr * 100
	score -= float64(stats.consecutiveUnhealthyFailures) * 100

	switch stats.state {
	case BreakerOpen:
		score -= 10000
	case BreakerHalfOpen:
		score -= 250
	}

	return score
}

func (t *Tracker) IsCircuitOpen(providerID uint64, clientType string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	stats := t.getStatsLocked(providerID, clientType)
	t.refreshStateLocked(stats)
	t.decayLocked(stats)
	return stats.state == BreakerOpen
}

func (t *Tracker) AllowAttempt(providerID uint64, clientType string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	stats := t.getStatsLocked(providerID, clientType)
	t.refreshStateLocked(stats)
	t.decayLocked(stats)
	if stats.state == BreakerOpen {
		return false
	}
	if stats.state == BreakerHalfOpen {
		if stats.halfOpenProbeInFlight {
			return false
		}
		stats.halfOpenProbeInFlight = true
		stats.halfOpenProbeStartedAt = t.now()
	}
	return true
}

func (t *Tracker) Snapshot(providerID uint64, clientType string) Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()

	stats := t.getStatsLocked(providerID, clientType)
	t.refreshStateLocked(stats)
	t.decayLocked(stats)
	return Snapshot{
		State:                        stats.state,
		EWMALatency:                  stats.ewmaLatency,
		EWMATTFT:                     stats.ewmaTTFT,
		SuccessCount:                 stats.successCount,
		FailureCount:                 stats.failureCount,
		ConsecutiveUnhealthyFailures: stats.consecutiveUnhealthyFailures,
		InFlight:                     stats.inFlight,
		OpenUntil:                    stats.openUntil,
	}
}

func (t *Tracker) ReleaseHalfOpenProbe(providerID uint64, clientType string, startedAt time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()

	stats := t.getStatsLocked(providerID, clientType)
	t.refreshStateLocked(stats)
	t.decayLocked(stats)
	if stats.state != BreakerHalfOpen || !stats.halfOpenProbeInFlight {
		return
	}
	if stats.halfOpenProbeStartedAt.IsZero() || startedAt.IsZero() {
		return
	}
	if startedAt.Before(stats.halfOpenProbeStartedAt) {
		return
	}
	stats.halfOpenProbeInFlight = false
	stats.halfOpenProbeStartedAt = time.Time{}
}

func (t *Tracker) getStatsLocked(providerID uint64, clientType string) *providerStats {
	key := fmt.Sprintf("%d:%s", providerID, clientType)
	stats, ok := t.stats[key]
	if !ok {
		stats = &providerStats{state: BreakerClosed}
		t.stats[key] = stats
	}
	return stats
}

func (t *Tracker) refreshStateLocked(stats *providerStats) {
	if stats.state == BreakerOpen && !stats.openUntil.IsZero() && !t.now().Before(stats.openUntil) {
		stats.state = BreakerHalfOpen
		stats.openUntil = time.Time{}
		stats.halfOpenProbeInFlight = false
	}
}

func (t *Tracker) openBreakerLocked(stats *providerStats) {
	stats.state = BreakerOpen
	stats.openUntil = t.now().Add(t.openDuration)
	stats.halfOpenProbeInFlight = false
	stats.halfOpenProbeStartedAt = time.Time{}
}

func (t *Tracker) decayLocked(stats *providerStats) {
	now := t.now()
	if stats.lastDecay.IsZero() {
		stats.lastDecay = now
		return
	}
	if t.decayHalfLife <= 0 {
		stats.lastDecay = now
		return
	}
	elapsed := now.Sub(stats.lastDecay)
	if elapsed <= 0 {
		return
	}

	factor := math.Exp(-math.Ln2 * float64(elapsed) / float64(t.decayHalfLife))
	stats.recentSuccess *= factor
	stats.recentFailure *= factor
	stats.recentTimeout *= factor
	stats.recentRateLimit *= factor
	stats.recentServerErr *= factor
	if stats.consecutiveUnhealthyFailures > 0 {
		decaySteps := int(elapsed / t.decayHalfLife)
		if decaySteps > 0 {
			stats.consecutiveUnhealthyFailures -= decaySteps
			if stats.consecutiveUnhealthyFailures < 0 {
				stats.consecutiveUnhealthyFailures = 0
			}
		}
	}
	stats.lastDecay = now
}

func ewmaDuration(previous, sample time.Duration, alpha float64) time.Duration {
	if previous <= 0 {
		return sample
	}
	blended := alpha*float64(sample) + (1-alpha)*float64(previous)
	return time.Duration(blended)
}

func isUnhealthyFailure(result AttemptResult) bool {
	return result.IsTimeout || result.IsNetwork || result.IsServerErr
}

func isHalfOpenUnhealthyFailure(result AttemptResult) bool {
	return isUnhealthyFailure(result) || result.IsRateLimit
}

func (t *Tracker) isHalfOpenProbeResultLocked(stats *providerStats, result AttemptResult) bool {
	if stats == nil {
		return false
	}
	if stats.halfOpenProbeStartedAt.IsZero() || result.StartedAt.IsZero() {
		return false
	}
	return !result.StartedAt.Before(stats.halfOpenProbeStartedAt)
}
