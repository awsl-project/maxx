package custom

import (
	"net/http"
	"testing"
)

func TestCopyHeadersFilteredDropsSensitiveHeaders(t *testing.T) {
	src := make(http.Header)
	src.Set("Host", "example.com")
	src.Set("X-Forwarded-For", "1.2.3.4")
	src.Set("Content-Length", "123")
	src.Set("X-Custom", "ok")

	dst := make(http.Header)
	copyHeadersFiltered(dst, src)

	if dst.Get("Host") != "" {
		t.Fatalf("expected Host to be filtered")
	}
	if dst.Get("X-Forwarded-For") != "" {
		t.Fatalf("expected X-Forwarded-For to be filtered")
	}
	if dst.Get("Content-Length") != "" {
		t.Fatalf("expected Content-Length to be filtered")
	}
	if dst.Get("X-Custom") != "ok" {
		t.Fatalf("expected X-Custom to be preserved")
	}
}

func TestSanitizeHeadersForEventRedactsProviderCredentials(t *testing.T) {
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer sk-secret")
	headers.Set("X-Api-Key", "sk-api")
	headers.Set("x-goog-api-key", "gemini-secret")
	headers.Set("X-Amz-Security-Token", "aws-session-token")
	headers.Set("Cookie", "session=secret")
	headers.Set("X-Custom", "ok")

	sanitized := sanitizeHeadersForEvent(headers)

	for _, key := range []string{"Authorization", "X-Api-Key", "X-Goog-Api-Key", "X-Amz-Security-Token", "Cookie"} {
		if got := sanitized[key]; got != "[REDACTED]" {
			t.Fatalf("%s = %q, want [REDACTED]; headers=%+v", key, got, sanitized)
		}
	}
	if sanitized["X-Custom"] != "ok" {
		t.Fatalf("X-Custom = %q, want preserved", sanitized["X-Custom"])
	}
}
