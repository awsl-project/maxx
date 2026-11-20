package event

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestPrivacyBroadcastUsesRequestPolicy(t *testing.T) {
	const errorText = "upstream password=Example1234"
	for _, enabled := range []bool{false, true} {
		req := &domain.ProxyRequest{PrivacyEnabled: enabled, Error: errorText}
		attempt := &domain.ProxyUpstreamAttempt{PrivacyEnabled: enabled, Error: errorText}
		requestOut := SanitizeProxyRequestForBroadcast(req)
		attemptOut := SanitizeProxyUpstreamAttemptForBroadcast(attempt)
		if enabled {
			if requestOut.Error == errorText || attemptOut.Error == errorText {
				t.Fatal("protected broadcast contains original error")
			}
		} else if requestOut != req || attemptOut != attempt || requestOut.Error != errorText || attemptOut.Error != errorText {
			t.Fatal("disabled policy changed existing broadcast fast path")
		}
		if req.Error != errorText || attempt.Error != errorText {
			t.Fatal("broadcast mutated execution errors")
		}
	}
}
