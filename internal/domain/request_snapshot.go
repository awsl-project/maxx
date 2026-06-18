package domain

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// requestSnapshotMaxBytes 限制写入 RequestInfo.Body 的快照最大字节数。即便是
// 文本/JSON,超过此上限也只存占位——Content-Type 由客户端控制,攻击者可伪装成
// 非二进制类型携带超大 body,unbounded string(body) 会撑爆 DB TEXT 列和内存
// (CodeRabbit & Codex review 指出的兜底缺口)。0 表示不限。
// 经 MAXX_REQUEST_SNAPSHOT_MAX_BYTES 调整,默认 256 KiB:正常对话请求足够保留
// 完整审计,异常超大 body 才被截断成占位。
var requestSnapshotMaxBytes = func() int {
	if v := strings.TrimSpace(os.Getenv("MAXX_REQUEST_SNAPSHOT_MAX_BYTES")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return 256 << 10
}()

// RequestBodySnapshot 决定写入 RequestInfo.Body 的请求体快照内容。
//
// 对二进制/multipart 上传(典型:/v1/images/edits 的几十 MB 图片),不把原始
// 字节塞进快照——既避免一份大 string 拷贝随 ProxyRequest/Attempt 常驻内存,也
// 避免几十 MB 二进制灌进数据库的 TEXT 列。这些字节对 UI 审计几乎无价值,只存
// content-type + 大小占位即可。
//
// devMode 请求保留完整 body 方便调试,与 clearDetail 的 dev_mode 豁免一致。
// 普通文本/JSON(对话请求)在 requestSnapshotMaxBytes 以内原样保留以保审计价值;
// 超过上限的(含伪装成非二进制类型的超大 body)同样截断成占位。该阈值远大于正常
// 对话体积,日常请求不受影响。
//
// 放在 domain 包是因为 executor 与 handler 两条接入路径都要构造 RequestInfo,
// 这里集中策略避免两处实现漂移。
func RequestBodySnapshot(body []byte, contentType string, devMode bool) string {
	if devMode {
		return string(body)
	}
	if isBinaryUploadContentType(contentType) {
		return fmt.Sprintf("<%s, %d bytes, body omitted>", contentTypeToken(contentType), len(body))
	}
	// 文本/JSON 兜底:超过快照上限的只保留前缀 + 占位,挡住伪装成非二进制类型的
	// 超大 body 撑爆 DB,同时保留开头(model、首条 message 等)的部分审计价值。
	if requestSnapshotMaxBytes > 0 && len(body) > requestSnapshotMaxBytes {
		label := contentTypeToken(contentType)
		if label == "binary" { // 非二进制路径下空 content-type 标成 text 更贴切
			label = "text"
		}
		// 前缀长度不超过快照上限本身,避免运维把上限调到 <256 时占位反而超过上限。
		previewLen := snapshotPreviewBytes
		if requestSnapshotMaxBytes < previewLen {
			previewLen = requestSnapshotMaxBytes
		}
		preview := body
		if len(preview) > previewLen {
			preview = preview[:previewLen]
		}
		// 前缀按文本渲染:非二进制类型理论上仍可能含非法 UTF-8 字节,替换成 � 避免占位串里出现乱码。
		safePreview := strings.ToValidUTF8(string(preview), "�")
		return fmt.Sprintf("%s…<%s, %d bytes total, body truncated (over snapshot cap %d)>", safePreview, label, len(body), requestSnapshotMaxBytes)
	}
	return string(body)
}

// snapshotPreviewBytes 是非二进制超限 body 在占位前保留的前缀字节数,留作部分审计。
const snapshotPreviewBytes = 256

// isBinaryUploadContentType 判断 content-type 是否为不值得存进快照的二进制上传。
func isBinaryUploadContentType(contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	switch {
	case strings.HasPrefix(ct, "multipart/"),
		strings.HasPrefix(ct, "application/octet-stream"),
		strings.HasPrefix(ct, "image/"),
		strings.HasPrefix(ct, "audio/"),
		strings.HasPrefix(ct, "video/"):
		return true
	default:
		return false
	}
}

// contentTypeToken 取 content-type 的主类型部分(丢弃 ;boundary=... 等参数),
// 占位串里没必要带上冗长且每次都变的 multipart boundary。
func contentTypeToken(contentType string) string {
	ct := strings.TrimSpace(contentType)
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	if ct == "" {
		return "binary"
	}
	return ct
}
