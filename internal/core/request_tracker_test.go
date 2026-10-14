package core

import (
	"errors"
	"sync/atomic"
	"testing"
)

func TestRequestTrackerCancelActiveInvokesRegisteredCancels(t *testing.T) {
	tracker := NewRequestTracker()
	cause := errors.New("admin stop")
	var called atomic.Int32
	var gotCause error

	unregister := tracker.RegisterActiveCancel(func(err error) {
		called.Add(1)
		gotCause = err
	})

	if cancelled := tracker.CancelActive(cause); cancelled != 1 {
		t.Fatalf("CancelActive cancelled = %d, want 1", cancelled)
	}
	if called.Load() != 1 {
		t.Fatalf("cancel called = %d, want 1", called.Load())
	}
	if !errors.Is(gotCause, cause) {
		t.Fatalf("cancel cause = %v, want %v", gotCause, cause)
	}

	unregister()
	if cancelled := tracker.CancelActive(cause); cancelled != 0 {
		t.Fatalf("CancelActive after unregister = %d, want 0", cancelled)
	}
}

func TestRequestTrackerCancelActiveDoesNotEnterShutdown(t *testing.T) {
	tracker := NewRequestTracker()
	tracker.RegisterActiveCancel(func(error) {})

	tracker.CancelActive(errors.New("admin stop"))

	if tracker.IsShuttingDown() {
		t.Fatal("CancelActive must not mark the server as shutting down")
	}
	if !tracker.Add() {
		t.Fatal("new requests should still be accepted after CancelActive")
	}
	tracker.Done()
}
