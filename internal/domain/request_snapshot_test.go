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

	// 以下两个用例固定快照上限,不依赖 env 派生的包级变量,保证 hermetic
	//(否则设了 MAXX_REQUEST_SNAPSHOT_MAX_BYTES=0 时会误失败)。
	const testCap = 1024
	t.Run("oversized non-binary body is truncated to placeholder", func(t *testing.T) {
		defer withSnapshotCap(testCap)()
		// 伪装成 JSON 的超大 body:超过快照上限也必须只存占位,不得 string(body) 入库。
		huge := make([]byte, testCap*4)
		for i := range huge {
			huge[i] = 'a'
		}
		got := RequestBodySnapshot(huge, "application/json", false)
		if len(got) >= len(huge) {
			t.Fatalf("oversized body must not be stored verbatim (got %d bytes)", len(got))
		}
		if !strings.Contains(got, "body truncated") {
			t.Fatalf("expected truncation placeholder, got len=%d", len(got))
		}
	})

	t.Run("dev_mode keeps oversized non-binary body", func(t *testing.T) {
		defer withSnapshotCap(testCap)()
		huge := make([]byte, testCap*4)
		if got := RequestBodySnapshot(huge, "application/json", true); len(got) != len(huge) {
			t.Fatalf("dev_mode must retain full body even when oversized")
		}
	})
}

// withSnapshotCap 临时覆盖快照上限,返回恢复函数(defer 调用)。
func withSnapshotCap(n int) func() {
	old := requestSnapshotMaxBytes
	requestSnapshotMaxBytes = n
	return func() { requestSnapshotMaxBytes = old }
}
