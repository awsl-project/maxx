package executor

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

type testNetError struct{}

func (testNetError) Error() string   { return "dial tcp: connection reset by peer" }
func (testNetError) Timeout() bool   { return false }
func (testNetError) Temporary() bool { return true }

var _ net.Error = testNetError{}

func TestNormalizeAttemptErrorOverridesTimeoutStatusCodeTo504(t *testing.T) {
	exec := &Executor{}
	attemptCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	proxyErr := domain.NewProxyErrorWithMessage(errors.New("upstream failed"), true, "upstream failed")
	proxyErr.HTTPStatusCode = http.StatusBadGateway

	err := exec.normalizeAttemptError(context.Background(), attemptCtx, domain.ErrFirstByteTimeout, proxyErr, false)

	got, ok := err.(*domain.ProxyError)
	if !ok {
		t.Fatalf("error type = %T, want *domain.ProxyError", err)
	}
	if got.HTTPStatusCode != http.StatusGatewayTimeout {
		t.Fatalf("HTTPStatusCode = %d, want %d", got.HTTPStatusCode, http.StatusGatewayTimeout)
	}
}

func TestNormalizeAttemptErrorWrapsRawNetworkError(t *testing.T) {
	exec := &Executor{}
	rawErr := testNetError{}

	err := exec.normalizeAttemptError(context.Background(), context.Background(), nil, rawErr, false)

	proxyErr, ok := err.(*domain.ProxyError)
	if !ok {
		t.Fatalf("error type = %T, want *domain.ProxyError", err)
	}
	if !proxyErr.IsNetworkError {
		t.Fatal("ProxyError.IsNetworkError = false, want true")
	}
	if !proxyErr.Retryable {
		t.Fatal("ProxyError.Retryable = false, want true for raw network error")
	}
	if !errors.Is(proxyErr.Err, rawErr) {
		t.Fatalf("ProxyError.Err = %v, want wrapped raw network error", proxyErr.Err)
	}
}

func TestRecordAttemptHealthMarksWrappedNetworkProxyError(t *testing.T) {
	tracker := newFakeHealthTracker()
	exec := &Executor{healthTracker: tracker}
	attempt := &domain.ProxyUpstreamAttempt{
		StartTime: time.Now(),
		Duration:  20 * time.Millisecond,
	}

	proxyErr := domain.NewProxyErrorWithMessage(testNetError{}, true, "stream read error")

	exec.recordAttemptHealth(7, domain.ClientTypeOpenAI, attempt, 0, proxyErr)

	if len(tracker.records) != 1 {
		t.Fatalf("record count = %d, want 1", len(tracker.records))
	}
	if !tracker.records[0].IsNetwork {
		t.Fatalf("tracker record = %#v, want IsNetwork=true", tracker.records[0])
	}
}

func TestNormalizeAttemptErrorPrefersAttemptDeadlineOverRawNetworkClassification(t *testing.T) {
	exec := &Executor{}
	attemptCtx, cancel := context.WithDeadline(context.Background(), time.Now())
	defer cancel()
	<-attemptCtx.Done()

	err := exec.normalizeAttemptError(context.Background(), attemptCtx, nil, context.DeadlineExceeded, false)

	proxyErr, ok := err.(*domain.ProxyError)
	if !ok {
		t.Fatalf("error type = %T, want *domain.ProxyError", err)
	}
	if proxyErr.HTTPStatusCode != http.StatusGatewayTimeout {
		t.Fatalf("HTTPStatusCode = %d, want %d", proxyErr.HTTPStatusCode, http.StatusGatewayTimeout)
	}
	if !errors.Is(proxyErr.Err, context.DeadlineExceeded) {
		t.Fatalf("ProxyError.Err = %v, want context.DeadlineExceeded", proxyErr.Err)
	}
}

func TestNormalizeResponseStartedErrorPrefersAttemptDeadlineOverRawNetworkClassification(t *testing.T) {
	attemptCtx, cancel := context.WithDeadline(context.Background(), time.Now())
	defer cancel()
	<-attemptCtx.Done()

	err := normalizeResponseStartedError(attemptCtx, nil, context.DeadlineExceeded)

	proxyErr, ok := err.(*domain.ProxyError)
	if !ok {
		t.Fatalf("error type = %T, want *domain.ProxyError", err)
	}
	if proxyErr.HTTPStatusCode != http.StatusGatewayTimeout {
		t.Fatalf("HTTPStatusCode = %d, want %d", proxyErr.HTTPStatusCode, http.StatusGatewayTimeout)
	}
	if !errors.Is(proxyErr.Err, context.DeadlineExceeded) {
		t.Fatalf("ProxyError.Err = %v, want context.DeadlineExceeded", proxyErr.Err)
	}
}
