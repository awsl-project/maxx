package domain

import "testing"

func TestResponsesWebSocketAttemptError_SafeRetryInvariant(t *testing.T) {
	tests := []struct {
		name string
		err  *ResponsesWebSocketAttemptError
		want bool
	}{
		{name: "pre-send", err: &ResponsesWebSocketAttemptError{SafeToTryNextProvider: true}, want: true},
		{name: "may have sent", err: &ResponsesWebSocketAttemptError{SafeToTryNextProvider: true, RequestFrameMayHaveBeenSent: true}},
		{name: "event received", err: &ResponsesWebSocketAttemptError{SafeToTryNextProvider: true, FirstEventReceived: true}},
		{name: "client event sent", err: &ResponsesWebSocketAttemptError{SafeToTryNextProvider: true, ClientEventSent: true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.err.CanTryNextProvider(); got != test.want {
				t.Fatalf("CanTryNextProvider = %v, want %v", got, test.want)
			}
		})
	}
}
