package handler

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"
)

// 大上传准入控制(admission control)。
//
// OOM 的真实机制是"同时在内存里的大请求体数量 × 每个体积"没有上限:几十个
// 客户端并发上传几十 MB 的图片(/v1/images/edits, multipart/form-data),
// io.ReadAll 的字节会同时挤在堆上把进程撑爆。
//
// 这里不改 []byte 主链路、不落盘,而是给"同时在飞的大上传"设一个并发闸:超过
// 阈值的请求要先拿到名额才放行,名额满了就短暂排队、等不到则让客户端稍后重试。
// 这样内存峰值被钉死在 ≈ maxConcurrency × 体积,普通文本/对话请求(小于阈值)
// 完全不受影响。可选的硬上限用于挡住异常/恶意的超大上传。
//
// 名额在 ingress 里 acquire、defer release,覆盖整个请求生命周期(与 tracker
// 同语义)——因为请求体为支持重试会在内存里保留到请求结束,占的就是这段时间。
const (
	// envUploadThresholdBytes: body 超过此字节数才算"大上传"、纳入并发门控。
	// 默认 4MiB:正常对话 JSON 几乎不会超,图片/音视频上传轻松超过。
	envUploadThresholdBytes = "MAXX_LARGE_UPLOAD_THRESHOLD_BYTES"
	// envUploadMaxConcurrency: 大上传同时在飞的最大数量。<=0 关闭并发门控。默认 16。
	envUploadMaxConcurrency = "MAXX_LARGE_UPLOAD_MAX_CONCURRENCY"
	// envUploadMaxBytes: 单个请求体硬上限,超过返回 413。<=0 不限(默认),避免误伤合法大图。
	envUploadMaxBytes = "MAXX_MAX_UPLOAD_BYTES"
	// envUploadWaitTimeoutMs: 等待并发名额的最长毫秒数,超时让客户端稍后重试。默认 10000。
	envUploadWaitTimeoutMs = "MAXX_LARGE_UPLOAD_WAIT_TIMEOUT_MS"
)

const (
	defaultUploadThresholdBytes int64 = 4 << 20 // 4 MiB
	defaultUploadMaxConcurrency       = 16
	defaultUploadWaitTimeout          = 10 * time.Second
)

// uploadLimiter 对超过阈值的大上传做并发门控 + 可选硬上限。零值不可用,经
// newUploadLimiterFromEnv / newUploadLimiter 构造。
type uploadLimiter struct {
	thresholdBytes int64         // 达到此体积才门控
	maxBytes       int64         // 硬上限;<=0 表示不限
	waitTimeout    time.Duration // 等名额超时
	sem            chan struct{} // 并发名额;nil 表示不门控并发(仅硬上限生效)
}

func newUploadLimiterFromEnv() *uploadLimiter {
	threshold := envInt64(envUploadThresholdBytes, defaultUploadThresholdBytes)
	maxConc := int(envInt64(envUploadMaxConcurrency, int64(defaultUploadMaxConcurrency)))
	maxBytes := envInt64(envUploadMaxBytes, 0)
	waitTimeout := time.Duration(envInt64(envUploadWaitTimeoutMs, int64(defaultUploadWaitTimeout/time.Millisecond))) * time.Millisecond
	l := newUploadLimiter(threshold, maxConc, maxBytes, waitTimeout)
	if l.sem != nil {
		log.Printf("[Proxy] large-upload admission control: threshold=%dB maxConcurrency=%d maxBytes=%d waitTimeout=%s",
			l.thresholdBytes, cap(l.sem), l.maxBytes, l.waitTimeout)
	}
	return l
}

func newUploadLimiter(thresholdBytes int64, maxConcurrency int, maxBytes int64, waitTimeout time.Duration) *uploadLimiter {
	l := &uploadLimiter{
		thresholdBytes: thresholdBytes,
		maxBytes:       maxBytes,
		waitTimeout:    waitTimeout,
	}
	if l.thresholdBytes < 0 {
		l.thresholdBytes = 0
	}
	if maxConcurrency > 0 {
		l.sem = make(chan struct{}, maxConcurrency)
	}
	if l.waitTimeout < 0 {
		l.waitTimeout = 0
	}
	return l
}

// tooLarge 在读取 body 之前用 Content-Length 预判是否超硬上限。contentLength<=0
// (未知/chunked)时无法预判,返回 false,改由读取时的 LimitReader 兜底。
func (l *uploadLimiter) tooLarge(contentLength int64) bool {
	if l == nil || l.maxBytes <= 0 {
		return false
	}
	return contentLength > 0 && contentLength > l.maxBytes
}

// isLarge 判断该请求是否需要纳入大上传并发门控。
//
// Content-Length 未知(chunked, <0)时无法预判体积,**保守按大上传门控**——否则
// 客户端只要不带 Content-Length(分块传输)就能绕过并发闸,无论 body 多大都不
// 受限,这是 CodeRabbit & Codex 都指出的 OOM 缺口。空 body(==0)不门控。
// 已知长度则按阈值判断。
func (l *uploadLimiter) isLarge(contentLength int64) bool {
	if l == nil || l.sem == nil {
		return false
	}
	if contentLength < 0 {
		return true
	}
	if l.thresholdBytes <= 0 {
		return false
	}
	return contentLength >= l.thresholdBytes
}

// acquire 为大上传申请一个并发名额。
//   - 非大上传 / 门控关闭:立即放行,release 为 no-op。
//   - 名额可用:占用并返回 release(请求结束时调用)。
//   - 名额满且在 waitTimeout 内未释放,或 ctx 取消:返回 ok=false,调用方应让客户端稍后重试。
func (l *uploadLimiter) acquire(ctx context.Context, contentLength int64) (release func(), ok bool) {
	if !l.isLarge(contentLength) {
		return func() {}, true
	}
	// 先尝试无等待获取,常态走这条快路径。
	select {
	case l.sem <- struct{}{}:
		return func() { <-l.sem }, true
	default:
	}
	if l.waitTimeout <= 0 {
		return nil, false
	}
	timer := time.NewTimer(l.waitTimeout)
	defer timer.Stop()
	select {
	case l.sem <- struct{}{}:
		return func() { <-l.sem }, true
	case <-timer.C:
		return nil, false
	case <-ctx.Done():
		return nil, false
	}
}

func envInt64(key string, def int64) int64 {
	raw := os.Getenv(key)
	if raw == "" {
		return def
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		log.Printf("[Proxy] invalid %s=%q, using default %d: %v", key, raw, def, err)
		return def
	}
	return v
}
