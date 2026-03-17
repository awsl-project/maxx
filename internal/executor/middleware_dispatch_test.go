package executor

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/converter"
	"github.com/awsl-project/maxx/internal/cooldown"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/awsl-project/maxx/internal/health"
	"github.com/awsl-project/maxx/internal/repository"
	"github.com/awsl-project/maxx/internal/router"
)

func TestDispatchCancelsSlowAttemptWhenTotalTimeoutReached(t *testing.T) {
	tracker := newFakeHealthTracker()
	attemptRepo := &dispatchAttemptRepoSpy{}
	exec := newDispatchTestExecutor(t, AttemptBudget{
		TotalTimeout:      20 * time.Millisecond,
		FirstByteTimeout:  10 * time.Second,
		StreamIdleTimeout: 10 * time.Second,
		MaxRetryAfter:     20 * time.Millisecond,
		MaxRetryWait:      20 * time.Millisecond,
	}, tracker, attemptRepo)

	callCount := 0
	adapter := &dispatchStubProviderAdapter{
		execute: func(c *flow.Ctx, _ *domain.Provider) error {
			callCount++
			<-c.Request.Context().Done()
			return c.Request.Context().Err()
		},
	}

	ctx, state := newDispatchTestFlow(adapter, &domain.RetryConfig{MaxRetries: 0})
	start := time.Now()
	exec.dispatch(ctx)
	elapsed := time.Since(start)

	if callCount != 1 {
		t.Fatalf("adapter call count = %d, want 1", callCount)
	}
	if elapsed >= 200*time.Millisecond {
		t.Fatalf("dispatch elapsed = %v, want attempt timeout to cut off quickly", elapsed)
	}

	proxyErr, ok := ctx.Err.(*domain.ProxyError)
	if !ok {
		t.Fatalf("ctx.Err type = %T, want *domain.ProxyError", ctx.Err)
	}
	if !proxyErr.Retryable {
		t.Fatal("ProxyError.Retryable = false, want true for timeout fallback")
	}
	if !proxyErr.IsNetworkError {
		t.Fatal("ProxyError.IsNetworkError = false, want true for timeout classification")
	}
	if !errors.Is(proxyErr.Err, context.DeadlineExceeded) {
		t.Fatalf("ProxyError.Err = %v, want context deadline exceeded", proxyErr.Err)
	}
	if state.lastErr != ctx.Err {
		t.Fatal("state.lastErr was not updated to the dispatch error")
	}
	if len(tracker.records) != 1 || !tracker.records[0].IsTimeout {
		t.Fatalf("timeout record = %#v, want a single timeout record", tracker.records)
	}
	if attemptRepo.lastUpdated == nil || attemptRepo.lastUpdated.Status != "FAILED" {
		t.Fatalf("last attempt status = %#v, want FAILED attempt persisted", attemptRepo.lastUpdated)
	}
}

func TestDispatchCapsRetryAfterBeforeSleeping(t *testing.T) {
	tracker := newFakeHealthTracker()
	exec := newDispatchTestExecutor(t, AttemptBudget{
		TotalTimeout:      80 * time.Millisecond,
		FirstByteTimeout:  10 * time.Second,
		StreamIdleTimeout: 10 * time.Second,
		MaxRetryAfter:     5 * time.Millisecond,
		MaxRetryWait:      5 * time.Millisecond,
	}, tracker, &dispatchAttemptRepoSpy{})

	callCount := 0
	adapter := &dispatchStubProviderAdapter{
		execute: func(c *flow.Ctx, _ *domain.Provider) error {
			callCount++
			if callCount == 1 {
				return &domain.ProxyError{
					Err:            domain.ErrUpstreamError,
					Retryable:      true,
					RetryAfter:     50 * time.Millisecond,
					HTTPStatusCode: http.StatusTooManyRequests,
				}
			}
			c.Writer.WriteHeader(http.StatusOK)
			_, _ = c.Writer.Write([]byte(`{"ok":true}`))
			return nil
		},
	}

	ctx, _ := newDispatchTestFlow(adapter, &domain.RetryConfig{
		MaxRetries:      1,
		InitialInterval: 0,
		BackoffRate:     1,
		MaxInterval:     0,
	})

	start := time.Now()
	exec.dispatch(ctx)
	elapsed := time.Since(start)

	if ctx.Err != nil {
		t.Fatalf("ctx.Err = %v, want nil after fallback success", ctx.Err)
	}
	if callCount != 2 {
		t.Fatalf("adapter call count = %d, want 2", callCount)
	}
	if elapsed >= 40*time.Millisecond {
		t.Fatalf("dispatch elapsed = %v, want retry-after cap to keep wait small", elapsed)
	}
}

func TestDispatchStopsRetryingWhenBudgetIsExhausted(t *testing.T) {
	exec := newDispatchTestExecutor(t, AttemptBudget{
		RequestTimeout:    15 * time.Millisecond,
		TotalTimeout:      15 * time.Millisecond,
		FirstByteTimeout:  10 * time.Second,
		StreamIdleTimeout: 10 * time.Second,
		MaxRetryAfter:     15 * time.Millisecond,
		MaxRetryWait:      15 * time.Millisecond,
	}, newFakeHealthTracker(), &dispatchAttemptRepoSpy{})

	callCount := 0
	adapter := &dispatchStubProviderAdapter{
		execute: func(c *flow.Ctx, _ *domain.Provider) error {
			callCount++
			<-c.Request.Context().Done()
			return &domain.ProxyError{Err: c.Request.Context().Err(), Retryable: true}
		},
	}

	ctx, _ := newDispatchTestFlow(adapter, &domain.RetryConfig{
		MaxRetries:      3,
		InitialInterval: time.Millisecond,
		BackoffRate:     1,
		MaxInterval:     time.Millisecond,
	})
	exec.dispatch(ctx)

	if callCount != 1 {
		t.Fatalf("adapter call count = %d, want 1 after budget exhaustion", callCount)
	}
}

func TestDispatchDoesNotBlockOnFallbackWarmToken(t *testing.T) {
	exec := newDispatchTestExecutor(t, DefaultAttemptBudget(), newFakeHealthTracker(), &dispatchAttemptRepoSpy{})

	primaryCalls := 0
	primary := &dispatchStubProviderAdapter{
		execute: func(c *flow.Ctx, _ *domain.Provider) error {
			primaryCalls++
			c.Writer.WriteHeader(http.StatusOK)
			_, _ = c.Writer.Write([]byte(`{"ok":true}`))
			return nil
		},
	}
	fallback := &dispatchWarmableProviderAdapter{
		dispatchStubProviderAdapter: dispatchStubProviderAdapter{
			execute: func(c *flow.Ctx, _ *domain.Provider) error {
				t.Fatal("fallback execute should not be called when primary succeeds")
				return nil
			},
		},
		warmToken: func(context.Context) error {
			time.Sleep(40 * time.Millisecond)
			return nil
		},
	}

	ctx, _ := newDispatchTestFlowWithRoutes(context.Background(),
		newMatchedRouteWithProviderID(primary, 11),
		newMatchedRouteWithProviderID(fallback, 12),
	)

	start := time.Now()
	exec.dispatch(ctx)
	elapsed := time.Since(start)

	if ctx.Err != nil {
		t.Fatalf("ctx.Err = %v, want nil", ctx.Err)
	}
	if primaryCalls != 1 {
		t.Fatalf("primaryCalls = %d, want 1", primaryCalls)
	}
	if elapsed >= 25*time.Millisecond {
		t.Fatalf("dispatch elapsed = %v, want primary route to proceed without fallback WarmToken blocking", elapsed)
	}
}

func TestDispatchUsesSharedRequestBudgetAcrossRetries(t *testing.T) {
	exec := newDispatchTestExecutor(t, AttemptBudget{
		RequestTimeout:    25 * time.Millisecond,
		TotalTimeout:      20 * time.Millisecond,
		FirstByteTimeout:  time.Second,
		StreamIdleTimeout: time.Second,
		MaxRetryAfter:     10 * time.Millisecond,
		MaxRetryWait:      10 * time.Millisecond,
	}, newFakeHealthTracker(), &dispatchAttemptRepoSpy{})

	callCount := 0
	adapter := &dispatchStubProviderAdapter{
		execute: func(c *flow.Ctx, _ *domain.Provider) error {
			callCount++
			<-c.Request.Context().Done()
			return &domain.ProxyError{Err: c.Request.Context().Err(), Retryable: true}
		},
	}

	ctx, _ := newDispatchTestFlow(adapter, &domain.RetryConfig{
		MaxRetries:      3,
		InitialInterval: time.Millisecond,
		BackoffRate:     1,
		MaxInterval:     time.Millisecond,
	})

	start := time.Now()
	exec.dispatch(ctx)
	elapsed := time.Since(start)

	if callCount > 2 {
		t.Fatalf("adapter call count = %d, want shared request budget to prevent more than 2 attempts", callCount)
	}
	if elapsed >= 60*time.Millisecond {
		t.Fatalf("dispatch elapsed = %v, want request budget to cut serial retries short", elapsed)
	}
}

func TestDispatchCancelsSlowAttemptWhenFirstByteTimeoutReached(t *testing.T) {
	tracker := newFakeHealthTracker()
	exec := newDispatchTestExecutor(t, AttemptBudget{
		RequestTimeout:    200 * time.Millisecond,
		TotalTimeout:      120 * time.Millisecond,
		FirstByteTimeout:  15 * time.Millisecond,
		StreamIdleTimeout: 40 * time.Millisecond,
		MaxRetryAfter:     10 * time.Millisecond,
		MaxRetryWait:      10 * time.Millisecond,
	}, tracker, &dispatchAttemptRepoSpy{})

	adapter := &dispatchStubProviderAdapter{
		execute: func(c *flow.Ctx, _ *domain.Provider) error {
			<-c.Request.Context().Done()
			return c.Request.Context().Err()
		},
	}

	ctx, _ := newDispatchTestFlow(adapter, &domain.RetryConfig{MaxRetries: 0})
	exec.dispatch(ctx)

	proxyErr, ok := ctx.Err.(*domain.ProxyError)
	if !ok {
		t.Fatalf("ctx.Err type = %T, want *domain.ProxyError", ctx.Err)
	}
	if !errors.Is(proxyErr.Err, domain.ErrFirstByteTimeout) {
		t.Fatalf("proxyErr.Err = %v, want first byte timeout", proxyErr.Err)
	}
	if len(tracker.records) != 1 || !tracker.records[0].IsTimeout {
		t.Fatalf("tracker records = %#v, want a timeout record", tracker.records)
	}
}

func TestDispatchCancelsStreamWhenIdleTimeoutReached(t *testing.T) {
	exec := newDispatchTestExecutor(t, AttemptBudget{
		RequestTimeout:    200 * time.Millisecond,
		TotalTimeout:      120 * time.Millisecond,
		FirstByteTimeout:  40 * time.Millisecond,
		StreamIdleTimeout: 15 * time.Millisecond,
		MaxRetryAfter:     10 * time.Millisecond,
		MaxRetryWait:      10 * time.Millisecond,
	}, newFakeHealthTracker(), &dispatchAttemptRepoSpy{})

	adapter := &dispatchStubProviderAdapter{
		execute: func(c *flow.Ctx, _ *domain.Provider) error {
			c.Writer.WriteHeader(http.StatusOK)
			_, _ = c.Writer.Write([]byte("chunk-1"))
			if flusher, ok := c.Writer.(http.Flusher); ok {
				flusher.Flush()
			}
			<-c.Request.Context().Done()
			return c.Request.Context().Err()
		},
	}

	ctx, _ := newDispatchTestFlow(adapter, &domain.RetryConfig{MaxRetries: 0})
	exec.dispatch(ctx)

	proxyErr, ok := ctx.Err.(*domain.ProxyError)
	if !ok {
		t.Fatalf("ctx.Err type = %T, want *domain.ProxyError", ctx.Err)
	}
	if !errors.Is(proxyErr.Err, domain.ErrStreamIdleTimeout) {
		t.Fatalf("proxyErr.Err = %v, want stream idle timeout", proxyErr.Err)
	}
}

func TestDispatchDoesNotRetryAfterResponseStartedAndIdleTimeout(t *testing.T) {
	exec := newDispatchTestExecutor(t, AttemptBudget{
		RequestTimeout:    200 * time.Millisecond,
		TotalTimeout:      120 * time.Millisecond,
		FirstByteTimeout:  40 * time.Millisecond,
		StreamIdleTimeout: 15 * time.Millisecond,
		MaxRetryAfter:     10 * time.Millisecond,
		MaxRetryWait:      10 * time.Millisecond,
	}, newFakeHealthTracker(), &dispatchAttemptRepoSpy{})

	fallbackCalls := 0
	primary := &dispatchStubProviderAdapter{
		execute: func(c *flow.Ctx, _ *domain.Provider) error {
			c.Writer.WriteHeader(http.StatusOK)
			_, _ = c.Writer.Write([]byte("chunk-1"))
			if flusher, ok := c.Writer.(http.Flusher); ok {
				flusher.Flush()
			}
			<-c.Request.Context().Done()
			return c.Request.Context().Err()
		},
	}
	fallback := &dispatchStubProviderAdapter{
		execute: func(c *flow.Ctx, _ *domain.Provider) error {
			fallbackCalls++
			c.Writer.WriteHeader(http.StatusOK)
			_, _ = c.Writer.Write([]byte("fallback"))
			return nil
		},
	}

	ctx, _, recorder := newDispatchTestFlowWithRecorder(context.Background(),
		newMatchedRouteWithProviderID(primary, 11),
		newMatchedRouteWithProviderID(fallback, 12),
	)
	exec.dispatch(ctx)

	if fallbackCalls != 0 {
		t.Fatalf("fallbackCalls = %d, want 0 after response already started", fallbackCalls)
	}
	proxyErr, ok := ctx.Err.(*domain.ProxyError)
	if !ok {
		t.Fatalf("ctx.Err type = %T, want *domain.ProxyError", ctx.Err)
	}
	if proxyErr.Retryable {
		t.Fatal("ProxyError.Retryable = true, want false once response has started")
	}
	if body := recorder.Body.String(); body != "chunk-1" {
		t.Fatalf("response body = %q, want only the primary chunk", body)
	}
}

func TestDispatchDoesNotTreatBufferedUpstreamReadAsFirstByteTimeout(t *testing.T) {
	tracker := newFakeHealthTracker()
	exec := newDispatchTestExecutor(t, AttemptBudget{
		RequestTimeout:    200 * time.Millisecond,
		TotalTimeout:      120 * time.Millisecond,
		FirstByteTimeout:  15 * time.Millisecond,
		StreamIdleTimeout: 80 * time.Millisecond,
		MaxRetryAfter:     10 * time.Millisecond,
		MaxRetryWait:      10 * time.Millisecond,
	}, tracker, &dispatchAttemptRepoSpy{})

	adapter := &dispatchStubProviderAdapter{
		execute: func(c *flow.Ctx, _ *domain.Provider) error {
			bodyReader := flow.WrapResponseBody(c, io.NopCloser(bytes.NewBufferString(`{"ok":true}`)))
			body, err := io.ReadAll(bodyReader)
			if err != nil {
				return err
			}
			time.Sleep(30 * time.Millisecond)
			c.Writer.WriteHeader(http.StatusOK)
			_, _ = c.Writer.Write(body)
			return nil
		},
	}

	ctx, _ := newDispatchTestFlow(adapter, &domain.RetryConfig{MaxRetries: 0})
	exec.dispatch(ctx)

	if ctx.Err != nil {
		t.Fatalf("ctx.Err = %v, want buffered upstream read to succeed", ctx.Err)
	}
	if len(tracker.records) != 1 || !tracker.records[0].Success {
		t.Fatalf("tracker records = %#v, want one successful attempt", tracker.records)
	}
}

func TestDispatchAllowsBufferedProviderToDisableFirstByteTimeout(t *testing.T) {
	tracker := newFakeHealthTracker()
	exec := newDispatchTestExecutor(t, AttemptBudget{
		RequestTimeout:    200 * time.Millisecond,
		TotalTimeout:      120 * time.Millisecond,
		FirstByteTimeout:  15 * time.Millisecond,
		StreamIdleTimeout: 80 * time.Millisecond,
		MaxRetryAfter:     10 * time.Millisecond,
		MaxRetryWait:      10 * time.Millisecond,
	}, tracker, &dispatchAttemptRepoSpy{})

	adapter := &dispatchStubProviderAdapter{
		execute: func(c *flow.Ctx, _ *domain.Provider) error {
			flow.DisableFirstByteTimeout(c)
			time.Sleep(30 * time.Millisecond)
			c.Writer.WriteHeader(http.StatusOK)
			_, _ = c.Writer.Write([]byte(`{"ok":true}`))
			return nil
		},
	}

	ctx, _ := newDispatchTestFlow(adapter, &domain.RetryConfig{MaxRetries: 0})
	exec.dispatch(ctx)

	if ctx.Err != nil {
		t.Fatalf("ctx.Err = %v, want buffered provider to fall back to total timeout only", ctx.Err)
	}
	if len(tracker.records) != 1 || !tracker.records[0].Success {
		t.Fatalf("tracker records = %#v, want one successful attempt", tracker.records)
	}
}

func TestClearSuccessCooldownsClearsOriginalAndConvertedClientTypes(t *testing.T) {
	providerID := uint64(77)
	manager := cooldown.Default()
	until := time.Now().Add(time.Minute)

	manager.RecordFailure(providerID, string(domain.ClientTypeClaude), cooldown.ReasonNetworkError, &until)
	manager.RecordFailure(providerID, string(domain.ClientTypeCodex), cooldown.ReasonNetworkError, &until)
	t.Cleanup(func() {
		manager.ClearCooldown(providerID, string(domain.ClientTypeClaude))
		manager.ClearCooldown(providerID, string(domain.ClientTypeCodex))
	})

	if !manager.IsInCooldown(providerID, string(domain.ClientTypeClaude)) || !manager.IsInCooldown(providerID, string(domain.ClientTypeCodex)) {
		t.Fatal("expected both client types to start in cooldown")
	}

	exec := &Executor{}
	exec.clearSuccessCooldowns(providerID, domain.ClientTypeCodex, domain.ClientTypeClaude)

	if manager.IsInCooldown(providerID, string(domain.ClientTypeClaude)) {
		t.Fatal("original client type cooldown should be cleared after success")
	}
	if manager.IsInCooldown(providerID, string(domain.ClientTypeCodex)) {
		t.Fatal("converted client type cooldown should be cleared after success")
	}
}

func TestDispatchDoesNotTreatPostReadProcessingAsStreamIdleTimeout(t *testing.T) {
	tracker := newFakeHealthTracker()
	exec := newDispatchTestExecutor(t, AttemptBudget{
		RequestTimeout:    200 * time.Millisecond,
		TotalTimeout:      120 * time.Millisecond,
		FirstByteTimeout:  15 * time.Millisecond,
		StreamIdleTimeout: 15 * time.Millisecond,
		MaxRetryAfter:     10 * time.Millisecond,
		MaxRetryWait:      10 * time.Millisecond,
	}, tracker, &dispatchAttemptRepoSpy{})

	adapter := &dispatchStubProviderAdapter{
		execute: func(c *flow.Ctx, _ *domain.Provider) error {
			bodyReader := flow.WrapResponseBody(c, io.NopCloser(bytes.NewBufferString(`{"ok":true}`)))
			body, err := io.ReadAll(bodyReader)
			if err != nil {
				return err
			}
			time.Sleep(30 * time.Millisecond)
			c.Writer.WriteHeader(http.StatusOK)
			_, _ = c.Writer.Write(body)
			return nil
		},
	}

	ctx, _ := newDispatchTestFlow(adapter, &domain.RetryConfig{MaxRetries: 0})
	exec.dispatch(ctx)

	if ctx.Err != nil {
		t.Fatalf("ctx.Err = %v, want post-read local processing to succeed", ctx.Err)
	}
	if len(tracker.records) != 1 || !tracker.records[0].Success {
		t.Fatalf("tracker records = %#v, want one successful attempt", tracker.records)
	}
}

func TestDispatchRecordsSuccessfulAttemptIntoHealthTracker(t *testing.T) {
	tracker := newFakeHealthTracker()
	exec := newDispatchTestExecutor(t, DefaultAttemptBudget(), tracker, &dispatchAttemptRepoSpy{})
	adapter := &dispatchStubProviderAdapter{
		execute: func(c *flow.Ctx, _ *domain.Provider) error {
			rawChan, ok := c.Get(flow.KeyEventChan)
			if !ok {
				t.Fatal("event channel missing from flow context")
			}
			eventChan, ok := rawChan.(domain.AdapterEventChan)
			if !ok {
				t.Fatalf("event channel type = %T, want domain.AdapterEventChan", rawChan)
			}
			time.Sleep(2 * time.Millisecond)
			eventChan.SendFirstToken(time.Now().UnixMilli())
			c.Writer.WriteHeader(http.StatusOK)
			_, _ = c.Writer.Write([]byte(`{"ok":true}`))
			return nil
		},
	}

	ctx, _ := newDispatchTestFlow(adapter, &domain.RetryConfig{MaxRetries: 0})
	exec.dispatch(ctx)

	if len(tracker.records) != 1 {
		t.Fatalf("record count = %d, want 1", len(tracker.records))
	}
	record := tracker.records[0]
	if !record.Success {
		t.Fatalf("record.Success = false, want true: %#v", record)
	}
	if record.TTFT <= 0 {
		t.Fatalf("record.TTFT = %v, want positive TTFT from adapter events", record.TTFT)
	}
}

func TestDispatchRecordsTimeoutAndOpensBreakerAfterThreshold(t *testing.T) {
	tracker := health.NewTracker()
	exec := newDispatchTestExecutor(t, AttemptBudget{
		TotalTimeout:      10 * time.Millisecond,
		FirstByteTimeout:  time.Second,
		StreamIdleTimeout: time.Second,
		MaxRetryAfter:     10 * time.Millisecond,
		MaxRetryWait:      10 * time.Millisecond,
	}, tracker, &dispatchAttemptRepoSpy{})
	adapter := &dispatchStubProviderAdapter{
		execute: func(c *flow.Ctx, _ *domain.Provider) error {
			<-c.Request.Context().Done()
			return c.Request.Context().Err()
		},
	}

	for range 3 {
		ctx, _ := newDispatchTestFlow(adapter, &domain.RetryConfig{MaxRetries: 0})
		exec.dispatch(ctx)
	}

	if !tracker.IsCircuitOpen(11, string(domain.ClientTypeClaude)) {
		t.Fatal("tracker circuit is still closed, want open after 3 timeout attempts")
	}
}

func TestDispatchDecrementsInFlightOnEveryExitPath(t *testing.T) {
	tracker := newFakeHealthTracker()
	exec := newDispatchTestExecutor(t, DefaultAttemptBudget(), tracker, &dispatchAttemptRepoSpy{})
	adapter := &dispatchStubProviderAdapter{
		execute: func(c *flow.Ctx, _ *domain.Provider) error {
			c.Writer.WriteHeader(http.StatusBadGateway)
			return &domain.ProxyError{
				Err:            domain.ErrUpstreamError,
				Retryable:      false,
				IsServerError:  true,
				HTTPStatusCode: http.StatusBadGateway,
			}
		},
	}

	ctx, _ := newDispatchTestFlow(adapter, &domain.RetryConfig{MaxRetries: 0})
	exec.dispatch(ctx)

	if got := tracker.inFlight("11:claude"); got != 0 {
		t.Fatalf("in-flight count = %d, want 0 after dispatch exits", got)
	}
	if tracker.beginCount("11:claude") != tracker.doneCount("11:claude") {
		t.Fatalf("begin/done mismatch: %d vs %d", tracker.beginCount("11:claude"), tracker.doneCount("11:claude"))
	}
}

func TestDispatchDoesNotRecordHealthWhenParentDeadlineExceeded(t *testing.T) {
	parentCtx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()

	tracker := newFakeHealthTracker()
	exec := newDispatchTestExecutor(t, AttemptBudget{
		RequestTimeout:    200 * time.Millisecond,
		TotalTimeout:      150 * time.Millisecond,
		FirstByteTimeout:  100 * time.Millisecond,
		StreamIdleTimeout: 100 * time.Millisecond,
		MaxRetryAfter:     10 * time.Millisecond,
		MaxRetryWait:      10 * time.Millisecond,
	}, tracker, &dispatchAttemptRepoSpy{})

	adapter := &dispatchStubProviderAdapter{
		execute: func(c *flow.Ctx, _ *domain.Provider) error {
			<-c.Request.Context().Done()
			return c.Request.Context().Err()
		},
	}

	ctx, _ := newDispatchTestFlowWithRoutes(parentCtx, newMatchedRoute(adapter, &domain.RetryConfig{MaxRetries: 0}))
	exec.dispatch(ctx)

	if len(tracker.records) != 0 {
		t.Fatalf("tracker records = %#v, want none when parent context timed out", tracker.records)
	}
	if tracker.releaseCount("11:claude") != 1 {
		t.Fatalf("release count = %d, want 1 when parent context timed out", tracker.releaseCount("11:claude"))
	}
}

func TestDispatchRecordsHealthBeforeEndingAttempt(t *testing.T) {
	tracker := newFakeHealthTracker()
	exec := newDispatchTestExecutor(t, DefaultAttemptBudget(), tracker, &dispatchAttemptRepoSpy{})
	adapter := &dispatchStubProviderAdapter{
		execute: func(c *flow.Ctx, _ *domain.Provider) error {
			c.Writer.WriteHeader(http.StatusOK)
			_, _ = c.Writer.Write([]byte(`{"ok":true}`))
			return nil
		},
	}

	ctx, _ := newDispatchTestFlow(adapter, &domain.RetryConfig{MaxRetries: 0})
	exec.dispatch(ctx)

	snapshots := tracker.recordInFlights("11:claude")
	if len(snapshots) != 1 {
		t.Fatalf("record in-flight snapshots = %#v, want exactly one", snapshots)
	}
	if snapshots[0] != 1 {
		t.Fatalf("in-flight count during Record = %d, want 1 before attemptDone runs", snapshots[0])
	}
}

func newDispatchTestExecutor(t *testing.T, budget AttemptBudget, tracker health.ProviderTracker, attemptRepo repository.ProxyUpstreamAttemptRepository) *Executor {
	t.Helper()

	return &Executor{
		proxyRequestRepo: &dispatchProxyRequestRepoSpy{},
		attemptRepo:      attemptRepo,
		modelMappingRepo: &dispatchModelMappingRepoStub{},
		converter:        converter.GetGlobalRegistry(),
		cooldownSem:      make(chan struct{}, 1),
		attemptBudget:    budget,
		healthTracker:    tracker,
	}
}

func newDispatchTestFlow(adapter provider.ProviderAdapter, retryConfig *domain.RetryConfig) (*flow.Ctx, *execState) {
	return newDispatchTestFlowWithRoutes(context.Background(), newMatchedRoute(adapter, retryConfig))
}

func newDispatchTestFlowWithRoutes(parentCtx context.Context, routes ...*router.MatchedRoute) (*flow.Ctx, *execState) {
	ctx, state, _ := newDispatchTestFlowWithRecorder(parentCtx, routes...)
	return ctx, state
}

func newDispatchTestFlowWithRecorder(parentCtx context.Context, routes ...*router.MatchedRoute) (*flow.Ctx, *execState, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	recorder := httptest.NewRecorder()
	ctx := flow.NewCtx(recorder, req)
	state := &execState{
		ctx:         parentCtx,
		proxyReq:    &domain.ProxyRequest{ID: 101, TenantID: 1, StartTime: time.Now(), Status: "IN_PROGRESS"},
		routes:      routes,
		tenantID:    1,
		clientType:  domain.ClientTypeClaude,
		requestBody: []byte(`{"model":"claude-sonnet-4"}`),
		requestURI:  "/v1/messages",
	}
	ctx.Set(flow.KeyExecutorState, state)
	return ctx, state, recorder
}

func newMatchedRoute(adapter provider.ProviderAdapter, retryConfig *domain.RetryConfig) *router.MatchedRoute {
	return newMatchedRouteWithProviderID(adapter, 11, retryConfig)
}

func newMatchedRouteWithProviderID(adapter provider.ProviderAdapter, providerID uint64, retryConfig ...*domain.RetryConfig) *router.MatchedRoute {
	cfg := &domain.RetryConfig{MaxRetries: 0}
	if len(retryConfig) > 0 {
		cfg = retryConfig[0]
	}
	return &router.MatchedRoute{
		Route: &domain.Route{
			ID:         21,
			TenantID:   1,
			IsEnabled:  true,
			ClientType: domain.ClientTypeClaude,
			ProviderID: providerID,
			Position:   1,
		},
		Provider: &domain.Provider{
			ID:       providerID,
			TenantID: 1,
			Type:     "custom",
			Name:     "provider",
			Config:   &domain.ProviderConfig{DisableErrorCooldown: true},
		},
		ProviderAdapter: adapter,
		RetryConfig:     cfg,
	}
}

type dispatchStubProviderAdapter struct {
	execute func(c *flow.Ctx, provider *domain.Provider) error
}

func (a *dispatchStubProviderAdapter) SupportedClientTypes() []domain.ClientType {
	return []domain.ClientType{domain.ClientTypeClaude}
}

func (a *dispatchStubProviderAdapter) Execute(c *flow.Ctx, provider *domain.Provider) error {
	return a.execute(c, provider)
}

type dispatchWarmableProviderAdapter struct {
	dispatchStubProviderAdapter
	warmToken func(ctx context.Context) error
}

func (a *dispatchWarmableProviderAdapter) WarmToken(ctx context.Context) error {
	if a.warmToken == nil {
		return nil
	}
	return a.warmToken(ctx)
}

type dispatchProxyRequestRepoSpy struct{}

func (r *dispatchProxyRequestRepoSpy) Create(req *domain.ProxyRequest) error { return nil }
func (r *dispatchProxyRequestRepoSpy) Update(req *domain.ProxyRequest) error { return nil }
func (r *dispatchProxyRequestRepoSpy) GetByID(tenantID uint64, id uint64) (*domain.ProxyRequest, error) {
	return nil, domain.ErrNotFound
}
func (r *dispatchProxyRequestRepoSpy) List(tenantID uint64, limit, offset int) ([]*domain.ProxyRequest, error) {
	return nil, nil
}
func (r *dispatchProxyRequestRepoSpy) ListCursor(tenantID uint64, limit int, before, after uint64, filter *repository.ProxyRequestFilter) ([]*domain.ProxyRequest, error) {
	return nil, nil
}
func (r *dispatchProxyRequestRepoSpy) ListActive(tenantID uint64) ([]*domain.ProxyRequest, error) {
	return nil, nil
}
func (r *dispatchProxyRequestRepoSpy) Count(tenantID uint64) (int64, error) { return 0, nil }
func (r *dispatchProxyRequestRepoSpy) CountWithFilter(tenantID uint64, filter *repository.ProxyRequestFilter) (int64, error) {
	return 0, nil
}
func (r *dispatchProxyRequestRepoSpy) UpdateProjectIDBySessionID(tenantID uint64, sessionID string, projectID uint64) (int64, error) {
	return 0, nil
}
func (r *dispatchProxyRequestRepoSpy) MarkStaleAsFailed(currentInstanceID string) (int64, error) {
	return 0, nil
}
func (r *dispatchProxyRequestRepoSpy) FixFailedRequestsWithoutEndTime() (int64, error) {
	return 0, nil
}
func (r *dispatchProxyRequestRepoSpy) DeleteOlderThan(before time.Time) (int64, error) { return 0, nil }
func (r *dispatchProxyRequestRepoSpy) HasRecentRequests(since time.Time) (bool, error) {
	return false, nil
}
func (r *dispatchProxyRequestRepoSpy) UpdateCost(id uint64, cost uint64) error          { return nil }
func (r *dispatchProxyRequestRepoSpy) AddCost(id uint64, delta int64) error             { return nil }
func (r *dispatchProxyRequestRepoSpy) BatchUpdateCosts(updates map[uint64]uint64) error { return nil }
func (r *dispatchProxyRequestRepoSpy) RecalculateCostsFromAttempts() (int64, error)     { return 0, nil }
func (r *dispatchProxyRequestRepoSpy) RecalculateCostsFromAttemptsWithProgress(progress chan<- domain.Progress) (int64, error) {
	return 0, nil
}
func (r *dispatchProxyRequestRepoSpy) ClearDetailOlderThan(before time.Time) (int64, error) {
	return 0, nil
}

type dispatchAttemptRepoSpy struct {
	mu          sync.Mutex
	nextID      uint64
	lastCreated *domain.ProxyUpstreamAttempt
	lastUpdated *domain.ProxyUpstreamAttempt
}

func (r *dispatchAttemptRepoSpy) Create(attempt *domain.ProxyUpstreamAttempt) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	attempt.ID = r.nextID
	r.lastCreated = cloneAttempt(attempt)
	return nil
}

func (r *dispatchAttemptRepoSpy) Update(attempt *domain.ProxyUpstreamAttempt) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastUpdated = cloneAttempt(attempt)
	return nil
}

func (r *dispatchAttemptRepoSpy) ListByProxyRequestID(proxyRequestID uint64) ([]*domain.ProxyUpstreamAttempt, error) {
	return nil, nil
}
func (r *dispatchAttemptRepoSpy) ListAll() ([]*domain.ProxyUpstreamAttempt, error) { return nil, nil }
func (r *dispatchAttemptRepoSpy) CountAll() (int64, error)                         { return 0, nil }
func (r *dispatchAttemptRepoSpy) StreamForCostCalc(batchSize int, callback func(batch []*domain.AttemptCostData) error) error {
	return nil
}
func (r *dispatchAttemptRepoSpy) UpdateCost(id uint64, cost uint64) error          { return nil }
func (r *dispatchAttemptRepoSpy) BatchUpdateCosts(updates map[uint64]uint64) error { return nil }
func (r *dispatchAttemptRepoSpy) MarkStaleAttemptsFailed() (int64, error)          { return 0, nil }
func (r *dispatchAttemptRepoSpy) FixFailedAttemptsWithoutEndTime() (int64, error)  { return 0, nil }
func (r *dispatchAttemptRepoSpy) ClearDetailOlderThan(before time.Time) (int64, error) {
	return 0, nil
}

func cloneAttempt(attempt *domain.ProxyUpstreamAttempt) *domain.ProxyUpstreamAttempt {
	if attempt == nil {
		return nil
	}
	cloned := *attempt
	return &cloned
}

type dispatchModelMappingRepoStub struct{}

func (r *dispatchModelMappingRepoStub) Create(mapping *domain.ModelMapping) error { return nil }
func (r *dispatchModelMappingRepoStub) Update(mapping *domain.ModelMapping) error { return nil }
func (r *dispatchModelMappingRepoStub) Delete(tenantID uint64, id uint64) error   { return nil }
func (r *dispatchModelMappingRepoStub) GetByID(tenantID uint64, id uint64) (*domain.ModelMapping, error) {
	return nil, domain.ErrNotFound
}
func (r *dispatchModelMappingRepoStub) List(tenantID uint64) ([]*domain.ModelMapping, error) {
	return nil, nil
}
func (r *dispatchModelMappingRepoStub) ListEnabled(tenantID uint64) ([]*domain.ModelMapping, error) {
	return nil, nil
}
func (r *dispatchModelMappingRepoStub) ListByClientType(tenantID uint64, clientType domain.ClientType) ([]*domain.ModelMapping, error) {
	return nil, nil
}
func (r *dispatchModelMappingRepoStub) ListByQuery(tenantID uint64, query *domain.ModelMappingQuery) ([]*domain.ModelMapping, error) {
	return nil, nil
}
func (r *dispatchModelMappingRepoStub) Count(tenantID uint64) (int, error) { return 0, nil }
func (r *dispatchModelMappingRepoStub) DeleteAll(tenantID uint64) error    { return nil }
func (r *dispatchModelMappingRepoStub) ClearAll(tenantID uint64) error     { return nil }
func (r *dispatchModelMappingRepoStub) SeedDefaults(tenantID uint64) error { return nil }

type fakeHealthTracker struct {
	mu         sync.Mutex
	beginCalls map[string]int
	doneCalls  map[string]int
	inFlights  map[string]int
	records    []health.AttemptResult
	recordSeen map[string][]int
	releases   map[string][]time.Time
	allowMap   map[string]bool
	openMap    map[string]bool
	scoreMap   map[string]float64
}

func newFakeHealthTracker() *fakeHealthTracker {
	return &fakeHealthTracker{
		beginCalls: make(map[string]int),
		doneCalls:  make(map[string]int),
		inFlights:  make(map[string]int),
		recordSeen: make(map[string][]int),
		releases:   make(map[string][]time.Time),
		allowMap:   make(map[string]bool),
		openMap:    make(map[string]bool),
		scoreMap:   make(map[string]float64),
	}
}

func (t *fakeHealthTracker) BeginAttempt(providerID uint64, clientType string) func() {
	key := trackerKey(providerID, clientType)
	t.mu.Lock()
	t.beginCalls[key]++
	t.inFlights[key]++
	t.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.doneCalls[key]++
			if t.inFlights[key] > 0 {
				t.inFlights[key]--
			}
		})
	}
}

func (t *fakeHealthTracker) Record(result health.AttemptResult) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.records = append(t.records, result)
	key := trackerKey(result.ProviderID, result.ClientType)
	t.recordSeen[key] = append(t.recordSeen[key], t.inFlights[key])
}

func (t *fakeHealthTracker) ReleaseHalfOpenProbe(providerID uint64, clientType string, startedAt time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	key := trackerKey(providerID, clientType)
	t.releases[key] = append(t.releases[key], startedAt)
}

func (t *fakeHealthTracker) Score(providerID uint64, clientType string) float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.scoreMap[trackerKey(providerID, clientType)]
}

func (t *fakeHealthTracker) IsCircuitOpen(providerID uint64, clientType string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.openMap[trackerKey(providerID, clientType)]
}

func (t *fakeHealthTracker) AllowAttempt(providerID uint64, clientType string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	key := trackerKey(providerID, clientType)
	allowed, ok := t.allowMap[key]
	if !ok {
		return true
	}
	return allowed
}

func (t *fakeHealthTracker) inFlight(key string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.inFlights[key]
}

func (t *fakeHealthTracker) beginCount(key string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.beginCalls[key]
}

func (t *fakeHealthTracker) doneCount(key string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.doneCalls[key]
}

func (t *fakeHealthTracker) recordInFlights(key string) []int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]int(nil), t.recordSeen[key]...)
}

func (t *fakeHealthTracker) releaseCount(key string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.releases[key])
}

func trackerKey(providerID uint64, clientType string) string {
	return strconv.FormatUint(providerID, 10) + ":" + clientType
}
