package executor

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

type attemptWatchdog struct {
	mu                sync.Mutex
	stopOnce          sync.Once
	now               func() time.Time
	start             time.Time
	firstByteTimeout  time.Duration
	streamIdleTimeout time.Duration
	cancel            context.CancelFunc
	stopCh            chan struct{}
	activitySeen      bool
	lastActivity      time.Time
	timeoutErr        error
	idleDisabled      bool
	firstByteDisabled bool
}

func newAttemptWatchdog(
	start time.Time,
	firstByteTimeout time.Duration,
	streamIdleTimeout time.Duration,
	cancel context.CancelFunc,
) *attemptWatchdog {
	if cancel == nil || (firstByteTimeout <= 0 && streamIdleTimeout <= 0) {
		return nil
	}

	w := &attemptWatchdog{
		now:               time.Now,
		start:             start,
		firstByteTimeout:  firstByteTimeout,
		streamIdleTimeout: streamIdleTimeout,
		cancel:            cancel,
		stopCh:            make(chan struct{}),
	}
	go w.run()
	return w
}

func (w *attemptWatchdog) Stop() {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() {
		close(w.stopCh)
	})
}

func (w *attemptWatchdog) TimeoutErr() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.timeoutErr
}

func (w *attemptWatchdog) NoteFirstByte() {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	now := w.now()
	w.activitySeen = true
	w.lastActivity = now
}

func (w *attemptWatchdog) NoteActivity() {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.activitySeen {
		return
	}
	w.lastActivity = w.now()
}

func (w *attemptWatchdog) CompleteResponseBody() {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.idleDisabled = true
}

func (w *attemptWatchdog) DisableFirstByteTimeout() {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.firstByteDisabled = true
}

func (w *attemptWatchdog) run() {
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			if timeoutErr := w.nextTimeoutErr(); timeoutErr != nil {
				w.mu.Lock()
				if w.timeoutErr == nil {
					w.timeoutErr = timeoutErr
				}
				w.mu.Unlock()
				w.cancel()
				return
			}
		}
	}
}

func (w *attemptWatchdog) nextTimeoutErr() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := w.now()
	if !w.activitySeen {
		if w.firstByteDisabled {
			return nil
		}
		if w.firstByteTimeout > 0 && now.Sub(w.start) >= w.firstByteTimeout {
			return domain.ErrFirstByteTimeout
		}
		return nil
	}
	if w.idleDisabled {
		return nil
	}

	if w.streamIdleTimeout > 0 && now.Sub(w.lastActivity) >= w.streamIdleTimeout {
		return domain.ErrStreamIdleTimeout
	}
	return nil
}

type attemptActivityWriter struct {
	http.ResponseWriter
	watchdog *attemptWatchdog
}

func newAttemptActivityWriter(w http.ResponseWriter, watchdog *attemptWatchdog) http.ResponseWriter {
	if watchdog == nil {
		return w
	}
	return &attemptActivityWriter{
		ResponseWriter: w,
		watchdog:       watchdog,
	}
}

func (w *attemptActivityWriter) WriteHeader(code int) {
	w.ResponseWriter.WriteHeader(code)
}

func (w *attemptActivityWriter) Write(b []byte) (int, error) {
	if len(b) > 0 {
		w.watchdog.NoteFirstByte()
	}
	return w.ResponseWriter.Write(b)
}

func (w *attemptActivityWriter) Flush() {
	w.watchdog.NoteActivity()
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}
