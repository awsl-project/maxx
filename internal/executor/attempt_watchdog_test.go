package executor

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestAttemptWatchdogStopIsConcurrentSafe(t *testing.T) {
	for i := 0; i < 200; i++ {
		watchdog := newAttemptWatchdog(time.Now(), time.Second, time.Second, func() {})
		if watchdog == nil {
			t.Fatal("newAttemptWatchdog returned nil")
		}

		start := make(chan struct{})
		var wg sync.WaitGroup
		for j := 0; j < 16; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				watchdog.Stop()
			}()
		}

		close(start)
		wg.Wait()
	}
}

func TestAttemptWatchdogStopNilIsSafe(t *testing.T) {
	var watchdog *attemptWatchdog
	watchdog.Stop()
}

func TestAttemptWatchdogCancelSetsTimeoutErrorOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	watchdog := newAttemptWatchdog(time.Now().Add(-20*time.Millisecond), 10*time.Millisecond, 0, cancel)
	if watchdog == nil {
		t.Fatal("newAttemptWatchdog returned nil")
	}
	defer watchdog.Stop()

	<-ctx.Done()

	if watchdog.TimeoutErr() == nil {
		t.Fatal("TimeoutErr = nil, want timeout error recorded")
	}
}
