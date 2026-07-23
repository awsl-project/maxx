package router

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestProviderLimiterHardLimitAndRelease(t *testing.T) {
	limiter := NewProviderLimiter()
	release, ok := limiter.TryAcquire(42, 1)
	if !ok {
		t.Fatal("first acquire should succeed")
	}
	if _, ok := limiter.TryAcquire(42, 1); ok {
		t.Fatal("second acquire must not exceed the hard limit")
	}
	if !limiter.IsAtLimit(42, 1) {
		t.Fatal("provider should report at limit")
	}

	release()
	release()
	if _, ok := limiter.TryAcquire(42, 1); !ok {
		t.Fatal("slot should be reusable after release")
	}
}

func TestProviderLimiterTracksProvidersSeparately(t *testing.T) {
	limiter := NewProviderLimiter()
	if _, ok := limiter.TryAcquire(1, 1); !ok {
		t.Fatal("provider 1 acquire failed")
	}
	if _, ok := limiter.TryAcquire(2, 1); !ok {
		t.Fatal("provider 2 should have an independent slot")
	}
}

func TestProviderLimiterConcurrentAcquireHonorsLimit(t *testing.T) {
	limiter := NewProviderLimiter()
	start := make(chan struct{})
	var acquired atomic.Int32
	var wg sync.WaitGroup

	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, ok := limiter.TryAcquire(7, 1); ok {
				acquired.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := acquired.Load(); got != 1 {
		t.Fatalf("successful acquires = %d, want 1", got)
	}
}

func TestProviderLimiterZeroMeansUnlimited(t *testing.T) {
	limiter := NewProviderLimiter()
	releases := make([]func(), 0, 100)
	for range 100 {
		release, ok := limiter.TryAcquire(9, 0)
		if !ok {
			t.Fatal("zero limit should be unlimited")
		}
		releases = append(releases, release)
	}
	if _, ok := limiter.TryAcquire(9, 100); ok {
		t.Fatal("existing unlimited sessions must count after changing to a positive limit")
	}
	releases[0]()
	if release, ok := limiter.TryAcquire(9, 100); !ok {
		t.Fatal("a slot should become available after an existing session ends")
	} else {
		release()
	}
}
