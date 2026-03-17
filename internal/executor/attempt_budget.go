package executor

import "time"

type AttemptBudget struct {
	RequestTimeout    time.Duration
	TotalTimeout      time.Duration
	FirstByteTimeout  time.Duration
	StreamIdleTimeout time.Duration
	MaxRetryAfter     time.Duration
	MaxRetryWait      time.Duration
}

func DefaultAttemptBudget() AttemptBudget {
	return AttemptBudget{
		RequestTimeout:    45 * time.Second,
		TotalTimeout:      30 * time.Second,
		FirstByteTimeout:  10 * time.Second,
		StreamIdleTimeout: 15 * time.Second,
		MaxRetryAfter:     30 * time.Second,
		MaxRetryWait:      10 * time.Second,
	}
}

func (b AttemptBudget) ClampRetryWait(wait time.Duration) time.Duration {
	if wait <= 0 {
		return 0
	}
	if b.MaxRetryWait > 0 && wait > b.MaxRetryWait {
		return b.MaxRetryWait
	}
	return wait
}

func (b AttemptBudget) RemainingSince(start time.Time) time.Duration {
	if b.TotalTimeout <= 0 {
		return 0
	}

	remaining := b.TotalTimeout - time.Since(start)
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (b AttemptBudget) RequestRemainingSince(start time.Time) time.Duration {
	if b.RequestTimeout <= 0 {
		return 0
	}

	remaining := b.RequestTimeout - time.Since(start)
	if remaining < 0 {
		return 0
	}
	return remaining
}
