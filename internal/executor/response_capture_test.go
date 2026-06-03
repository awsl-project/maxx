package executor

import (
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestResponseCaptureBoundsSnapshotButForwardsFull 锁住核心契约:超过快照上限的
// 响应体仍整份转发给客户端,但缓冲只保留上限以内的前缀 + 截断占位,避免整个响应体
// 常驻内存(OOM 来源)同时把超大 body 灌进 DB TEXT 列。
func TestResponseCaptureBoundsSnapshotButForwardsFull(t *testing.T) {
	recorder := httptest.NewRecorder()
	rc := NewResponseCapture(recorder)
	rc.maxBytes = 16 // 测试用小上限

	chunk1 := strings.Repeat("a", 10)
	chunk2 := strings.Repeat("b", 20) // 累计 30 > 16

	if _, err := rc.Write([]byte(chunk1)); err != nil {
		t.Fatalf("write chunk1: %v", err)
	}
	if _, err := rc.Write([]byte(chunk2)); err != nil {
		t.Fatalf("write chunk2: %v", err)
	}

	// 客户端必须收到完整 30 字节。
	if got := recorder.Body.String(); got != chunk1+chunk2 {
		t.Fatalf("client body = %q, want full %q", got, chunk1+chunk2)
	}

	body := rc.Body()
	if !strings.HasPrefix(body, "aaaaaaaaaabbbbbb") { // 10 a + 6 b = 16 字节前缀
		t.Fatalf("snapshot prefix not clamped to cap: %q", body)
	}
	if !strings.Contains(body, "truncated") {
		t.Fatalf("snapshot missing truncation marker: %q", body)
	}
	if !strings.Contains(body, "30 bytes total") {
		t.Fatalf("snapshot missing accurate total: %q", body)
	}
}

// TestResponseCaptureWithinCapIsExact 上限以内的响应体应原样保留,不加占位。
func TestResponseCaptureWithinCapIsExact(t *testing.T) {
	recorder := httptest.NewRecorder()
	rc := NewResponseCapture(recorder)
	rc.maxBytes = 1024

	payload := `{"content":[{"type":"text","text":"ok"}]}`
	if _, err := rc.Write([]byte(payload)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := rc.Body(); got != payload {
		t.Fatalf("snapshot = %q, want exact %q", got, payload)
	}
}

// TestResponseCaptureUnboundedOptOut maxBytes<=0 时保留旧的不限行为(env 显式 opt-out)。
func TestResponseCaptureUnboundedOptOut(t *testing.T) {
	recorder := httptest.NewRecorder()
	rc := NewResponseCapture(recorder)
	rc.maxBytes = 0

	payload := strings.Repeat("x", 4096)
	if _, err := rc.Write([]byte(payload)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := rc.Body(); got != payload {
		t.Fatalf("unbounded snapshot truncated unexpectedly: len=%d", len(got))
	}
}

// TestResponseCaptureTruncationKeepsValidUTF8 按字节截断可能切断多字节字符,
// 快照前缀必须经 ToValidUTF8 清洗,不得在末尾留下半个 rune 的非法字节。
func TestResponseCaptureTruncationKeepsValidUTF8(t *testing.T) {
	recorder := httptest.NewRecorder()
	rc := NewResponseCapture(recorder)
	// "你" 占 3 字节;上限 4 会把第二个 "你" 从中间切断。
	rc.maxBytes = 4

	payload := "你你你" // 9 字节
	if _, err := rc.Write([]byte(payload)); err != nil {
		t.Fatalf("write: %v", err)
	}
	body := rc.Body()
	if !utf8.ValidString(body) {
		t.Fatalf("snapshot is not valid UTF-8: %q", body)
	}
	if !strings.Contains(body, "9 bytes total") {
		t.Fatalf("snapshot missing accurate total: %q", body)
	}
}
