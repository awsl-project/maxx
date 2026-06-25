package executor

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
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
//
// 并发约定:与 http.ResponseWriter 本身一致,假定由单个请求处理 goroutine 串行
// 写入;StatusCode()/Body()/CapturedHeaders() 在响应写完后(dispatch 收尾)读取,
// 不与 Write 并发。故 body/total/truncated 不加锁——这也与本类型加上界改造前的
// 行为一致(原本就直接用非并发安全的 bytes.Buffer)。
type ResponseCapture struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
	headers    http.Header
	wrote      bool

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
	rc.wrote = true
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
		rc.wrote = true
		rc.captureBounded(b[:n])
	}
	return n, err
}

// WroteToClient reports whether any response header or body byte has already
// been forwarded to the downstream client. Once true, the executor must not
// transparently fail over to another provider: doing so could splice two
// upstream responses into one client-visible stream.
func (rc *ResponseCapture) WroteToClient() bool {
	return rc.wrote
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
// 字符:先丢掉末尾被切断的不完整 rune(而非让 ToValidUTF8 把它替换成 3 字节的 �
// 反而把快照撑过上限),再对内部残留的非法字节做清洗,保持前缀 ~maxBytes。
func (rc *ResponseCapture) Body() string {
	if !rc.truncated {
		return rc.body.String()
	}
	prefix := strings.ToValidUTF8(string(trimTrailingPartialRune(rc.body.Bytes())), "�")
	return fmt.Sprintf(
		"%s…<response body truncated, %d bytes total, snapshot cap %d>",
		prefix, rc.total, rc.maxBytes,
	)
}

// trimTrailingPartialRune 丢掉 b 末尾因按字节截断而残缺的最后一个 UTF-8 rune。
// 一个合法 rune 至多 utf8.UTFMax 字节,故从末尾回溯至多这么多字节找到最后一个
// rune 起始字节:若该 rune 完整则原样返回,残缺则连同它一起去掉。全是续字节
// (畸形数据)则不动,交由调用方的 ToValidUTF8 兜底。
func trimTrailingPartialRune(b []byte) []byte {
	for i := 0; i < utf8.UTFMax && i < len(b); i++ {
		start := len(b) - 1 - i
		if !utf8.RuneStart(b[start]) {
			continue
		}
		// DecodeRune 对非法/残缺编码返回 (RuneError, 1),而对一个**合法编码**的
		// U+FFFD 本身返回 (RuneError, 3)。故"残缺"的判据是 size==1 的 RuneError,
		// 不能只看 r==RuneError——否则末尾一个合法的 3 字节 U+FFFD 会被误当残缺丢掉。
		if r, size := utf8.DecodeRune(b[start:]); !(r == utf8.RuneError && size == 1) && start+size == len(b) {
			return b // 末尾 rune 完整(含合法编码的 U+FFFD)
		}
		return b[:start] // 末尾 rune 残缺,去掉
	}
	return b
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
