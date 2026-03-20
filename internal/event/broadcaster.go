package event

import "github.com/awsl-project/maxx/internal/domain"

// Broadcaster 事件广播接口
// WebSocket 和 Wails 都实现此接口
type Broadcaster interface {
	BroadcastProxyRequest(req *domain.ProxyRequest)
	BroadcastProxyUpstreamAttempt(attempt *domain.ProxyUpstreamAttempt)
	BroadcastLog(message string)
	BroadcastMessage(messageType string, data interface{})
	BroadcastMessageToTenant(tenantID uint64, messageType string, data interface{})
}

// NopBroadcaster 空实现，用于测试或不需要广播的场景
type NopBroadcaster struct{}

func (n *NopBroadcaster) BroadcastProxyRequest(req *domain.ProxyRequest)                     {}
func (n *NopBroadcaster) BroadcastProxyUpstreamAttempt(attempt *domain.ProxyUpstreamAttempt) {}
func (n *NopBroadcaster) BroadcastLog(message string)                                        {}
func (n *NopBroadcaster) BroadcastMessage(messageType string, data interface{})              {}
func (n *NopBroadcaster) BroadcastMessageToTenant(tenantID uint64, messageType string, data interface{}) {}

// SanitizeProxyRequestForBroadcast 用于“实时广播”场景瘦身 payload：
// 去掉 request/response 大字段，避免 WebSocket 消息动辄几十/几百 KB，导致前端 JSON.parse / GC 卡死。
//
// 说明：
// - /requests 列表页只需要轻量字段（状态、耗时、tokens、成本等）。
// - 详情页需要的大字段应通过 /admin/requests/{id} 与 /admin/requests/{id}/attempts 拉取。
func SanitizeProxyRequestForBroadcast(req *domain.ProxyRequest) *domain.ProxyRequest {
	if req == nil {
		return nil
	}
	// 已经是瘦身后的对象，避免重复拷贝（高频场景会产生额外 GC 压力）
	if req.RequestInfo == nil && req.ResponseInfo == nil {
		return req
	}
	copied := *req
	copied.RequestInfo = nil
	copied.ResponseInfo = nil
	return &copied
}

// SanitizeProxyUpstreamAttemptForBroadcast 用于“实时广播”场景瘦身 payload。
func SanitizeProxyUpstreamAttemptForBroadcast(attempt *domain.ProxyUpstreamAttempt) *domain.ProxyUpstreamAttempt {
	if attempt == nil {
		return nil
	}
	// 已经是瘦身后的对象，避免重复拷贝（高频场景会产生额外 GC 压力）
	if attempt.RequestInfo == nil && attempt.ResponseInfo == nil {
		return attempt
	}
	copied := *attempt
	copied.RequestInfo = nil
	copied.ResponseInfo = nil
	return &copied
}
