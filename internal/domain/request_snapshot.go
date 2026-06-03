package domain

import (
	"fmt"
	"strings"
)

// RequestBodySnapshot 决定写入 RequestInfo.Body 的请求体快照内容。
//
// 对二进制/multipart 上传(典型:/v1/images/edits 的几十 MB 图片),不把原始
// 字节塞进快照——既避免一份大 string 拷贝随 ProxyRequest/Attempt 常驻内存,也
// 避免几十 MB 二进制灌进数据库的 TEXT 列。这些字节对 UI 审计几乎无价值,只存
// content-type + 大小占位即可。
//
// devMode 请求保留完整 body 方便调试,与 clearDetail 的 dev_mode 豁免一致。
// 普通文本/JSON(对话请求)始终原样保留,不按大小截断,避免丢失审计价值。
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
	return string(body)
}

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
