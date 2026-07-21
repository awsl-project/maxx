package domain

import (
	"errors"
	"testing"
)

func TestResponsesWebSocketAttemptError_UnwrapAndMessage(t *testing.T) {
	inner := errors.New("dial failed")
	err := &ResponsesWebSocketAttemptError{
		Err:               inner,
		CapabilityFailure: true,
	}
	if err.Error() != "dial failed" {
		t.Fatalf("Error() = %q", err.Error())
	}
	if !errors.Is(err, inner) {
		t.Fatalf("Unwrap/Is failed for %#v", err)
	}
	if (&ResponsesWebSocketAttemptError{}).Error() != "responses websocket attempt failed" {
		t.Fatal("empty attempt error message mismatch")
	}
}
