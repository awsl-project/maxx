package executor

import (
	"io"
	"net/http"
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

// TestResponseCaptureTruncationKeepsLegitReplacementChar 锁住边界:截断恰好落在
// 一个**合法编码**的 U+FFFD(3 字节 EF BF BD)末尾时,它是完整 rune,必须保留,
// 不能因为 DecodeRune 返回 RuneError 就被 trimTrailingPartialRune 误当残缺丢掉。
func TestResponseCaptureTruncationKeepsLegitReplacementChar(t *testing.T) {
	recorder := httptest.NewRecorder()
	rc := NewResponseCapture(recorder)
	rc.maxBytes = 3 // 恰好容下一个完整的 U+FFFD

	payload := "�xx" // 3 字节合法 U+FFFD + 2 字节,共 5 字节 > 3
	if _, err := rc.Write([]byte(payload)); err != nil {
		t.Fatalf("write: %v", err)
	}
	body := rc.Body()
	if !strings.HasPrefix(body, "�…") {
		t.Fatalf("legit trailing U+FFFD should be kept, got %q", body)
	}
	if !strings.Contains(body, "5 bytes total") {
		t.Fatalf("snapshot missing accurate total: %q", body)
	}
}

// shortWriter 模拟底层 ResponseWriter 短写(只接受前 accept 字节)并返回 err。
type shortWriter struct {
	http.ResponseWriter
	accept int
	err    error
}

func (s *shortWriter) Write(b []byte) (int, error) {
	n := len(b)
	if n > s.accept {
		n = s.accept
	}
	written, _ := s.ResponseWriter.Write(b[:n])
	return written, s.err
}

// TestResponseCaptureCapturesOnlyAcceptedBytes 锁住短写/出错契约:底层只接受
// b[:n] 时,快照与 total 都只计入 n 字节,且 n/err 原样透传给调用方——不能把
// 没真正发出去的字节记进审计快照。
func TestResponseCaptureCapturesOnlyAcceptedBytes(t *testing.T) {
	recorder := httptest.NewRecorder()
	sw := &shortWriter{ResponseWriter: recorder, accept: 3, err: io.ErrShortWrite}
	rc := NewResponseCapture(sw)
	rc.maxBytes = 1024

	n, err := rc.Write([]byte("abcdef"))
	if n != 3 || err != io.ErrShortWrite {
		t.Fatalf("passthrough wrong: n=%d err=%v, want n=3 err=ErrShortWrite", n, err)
	}
	if rc.total != 3 {
		t.Fatalf("total should count only accepted bytes, got %d", rc.total)
	}
	if got := rc.Body(); got != "abc" {
		t.Fatalf("snapshot should only contain accepted bytes, got %q", got)
	}
	if got := recorder.Body.String(); got != "abc" {
		t.Fatalf("client got %q, want only accepted bytes %q", got, "abc")
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
	// 被切断的第二个 "你" 必须被整体丢弃,而不是替换成 U+FFFD("�")——后者会把
	// 前缀撑过上限,正是 trimTrailingPartialRune 要避免的。只断言 ValidString 不够
	// (替换后仍是合法 UTF-8),这里把"丢弃而非替换"的契约钉死。
	if strings.ContainsRune(body, '�') {
		t.Fatalf("truncated partial rune should be dropped, not replaced with U+FFFD: %q", body)
	}
	if !strings.HasPrefix(body, "你…") {
		t.Fatalf("expected dropped-rune prefix %q, got %q", "你…", body)
	}
	if !strings.Contains(body, "9 bytes total") {
		t.Fatalf("snapshot missing accurate total: %q", body)
	}
}
