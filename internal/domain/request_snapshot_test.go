package domain

import (
	"strings"
	"testing"
)

func TestRequestBodySnapshot(t *testing.T) {
	imageBody := []byte(strings.Repeat("\x89PNG\r\n", 1000)) // 假装的大二进制

	t.Run("multipart upload is omitted", func(t *testing.T) {
		got := RequestBodySnapshot(imageBody, "multipart/form-data; boundary=----abc123", false)
		if strings.Contains(got, "PNG") {
			t.Fatalf("binary body leaked into snapshot: %q", got)
		}
		if !strings.Contains(got, "multipart/form-data") {
			t.Fatalf("snapshot missing content-type token: %q", got)
		}
		if strings.Contains(got, "boundary") {
			t.Fatalf("snapshot should drop boundary param: %q", got)
		}
	})

	t.Run("image content-type is omitted", func(t *testing.T) {
		if got := RequestBodySnapshot(imageBody, "image/png", false); strings.Contains(got, "PNG") {
			t.Fatalf("image body leaked: %q", got)
		}
	})

	t.Run("json conversation body is preserved", func(t *testing.T) {
		jsonBody := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`)
		if got := RequestBodySnapshot(jsonBody, "application/json", false); got != string(jsonBody) {
			t.Fatalf("json body must be preserved verbatim, got %q", got)
		}
	})

	t.Run("dev_mode keeps full body even for multipart", func(t *testing.T) {
		if got := RequestBodySnapshot(imageBody, "multipart/form-data; boundary=x", true); got != string(imageBody) {
			t.Fatalf("dev_mode must retain full body for debugging")
		}
	})

	t.Run("missing content-type preserves body", func(t *testing.T) {
		jsonBody := []byte(`{"a":1}`)
		if got := RequestBodySnapshot(jsonBody, "", false); got != string(jsonBody) {
			t.Fatalf("no content-type should default to preserve: %q", got)
		}
	})
}
