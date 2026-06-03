package executor

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// responseSnapshotMaxBytes 限制 ResponseCapture 缓冲(进而写入 ResponseInfo.Body)
// 的响应体快照最大字节数。这是请求侧 domain.RequestBodySnapshot 的对称兜底:
// 此前 ResponseCapture.Write 对每个写给客户端的 chunk 都无条件 body.Write,流式
// (SSE)与大 base64 图片响应也不例外——整个响应体被缓冲进内存,大小 ∝ 响应体 ×
// 并发,且不受上传准入控制约束,是典型的 OOM 来源。这里把缓冲上界 clamp 住:超过
// 上限的字节照常转发给客户端,但不再进缓冲,只在快照末尾留截断占位。
// 经 MAXX_RESPONSE_SNAPSHOT_MAX_BYTES 调整,默认 256 KiB:正常对话/补全响应足够
// 保留完整审计,异常超大响应(图片 base64、超长 SSE)才被截断。0 表示不限(不推荐)。
var responseSnapshotMaxBytes = func() int {
	if v := strings.TrimSpace(os.Getenv("MAXX_RESPONSE_SNAPSHOT_MAX_BYTES")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return 256 << 10
}()

// ResponseCapture wraps http.ResponseWriter to capture the response
// This allows us to record the actual response sent to the client
type ResponseCapture struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
	headers    http.Header

	// maxBytes 是 body 缓冲的上界(字节);超过后停止缓冲但继续转发。0 表示不限。
	maxBytes int
	// total 记录已成功写给客户端的总字节数,用于截断占位里的 "N bytes total"。
	total int64
	// truncated 标记响应体是否因超过 maxBytes 被截断(快照非完整)。
	truncated bool
}

// NewResponseCapture creates a new ResponseCapture wrapper
func NewResponseCapture(w http.ResponseWriter) *ResponseCapture {
	return &ResponseCapture{
		ResponseWriter: w,
		statusCode:     http.StatusOK, // Default status
		headers:        make(http.Header),
		maxBytes:       responseSnapshotMaxBytes,
	}
}

// WriteHeader captures the status code and forwards to underlying writer
func (rc *ResponseCapture) WriteHeader(code int) {
	rc.statusCode = code
	rc.ResponseWriter.WriteHeader(code)
}

// Write forwards every byte to the client and buffers up to maxBytes for the
// stored response snapshot. Bytes beyond the cap are still sent downstream but
// not retained, bounding memory to ~maxBytes per request regardless of response
// size or stream length. Only the bytes the underlying writer actually accepted
// (n) are captured, so a short write / error never records unsent bytes.
func (rc *ResponseCapture) Write(b []byte) (int, error) {
	n, err := rc.ResponseWriter.Write(b)
	if n > 0 {
		rc.captureBounded(b[:n])
	}
	return n, err
}

// captureBounded appends b to the snapshot buffer without exceeding maxBytes.
func (rc *ResponseCapture) captureBounded(b []byte) {
	rc.total += int64(len(b))
	if rc.maxBytes <= 0 { // unbounded (opt-out via env)
		rc.body.Write(b)
		return
	}
	remaining := rc.maxBytes - rc.body.Len()
	if remaining <= 0 {
		rc.truncated = true
		return
	}
	if len(b) > remaining {
		rc.body.Write(b[:remaining])
		rc.truncated = true
		return
	}
	rc.body.Write(b)
}

// Header returns the header map (for setting headers)
func (rc *ResponseCapture) Header() http.Header {
	return rc.ResponseWriter.Header()
}

// Flush implements http.Flusher for streaming support
func (rc *ResponseCapture) Flush() {
	if f, ok := rc.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// StatusCode returns the captured status code
func (rc *ResponseCapture) StatusCode() int {
	return rc.statusCode
}

// Body returns the captured response body. When the response exceeded the
// snapshot cap it returns the retained prefix followed by a truncation marker
// so the stored detail signals it is partial rather than silently incomplete.
// 注:截断时返回值是「上限以内前缀 + 固定占位尾巴」,因此略大于 maxBytes 一个常量;
// 目的是防 OOM / 防撑爆 DB TEXT 列,不是字节级硬上限。按字节截断可能切断多字节
// 字符,故前缀经 ToValidUTF8 清洗,避免快照里出现非法 UTF-8(与请求侧一致)。
func (rc *ResponseCapture) Body() string {
	if !rc.truncated {
		return rc.body.String()
	}
	prefix := strings.ToValidUTF8(rc.body.String(), "�")
	return fmt.Sprintf(
		"%s…<response body truncated, %d bytes total, snapshot cap %d>",
		prefix, rc.total, rc.maxBytes,
	)
}

// CapturedHeaders returns the headers that were set
func (rc *ResponseCapture) CapturedHeaders() map[string]string {
	result := make(map[string]string)
	for key, values := range rc.ResponseWriter.Header() {
		if len(values) > 0 {
			result[key] = values[0]
		}
	}
	return result
}
