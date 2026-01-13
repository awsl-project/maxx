package event

import "github.com/Bowl42/maxx-next/internal/domain"

// Broadcaster 事件广播接口
// WebSocket 和 Wails 都实现此接口
type Broadcaster interface {
	BroadcastProxyRequest(req *domain.ProxyRequest)
	BroadcastProxyUpstreamAttempt(attempt *domain.ProxyUpstreamAttempt)
	BroadcastLog(message string)
	BroadcastStats(stats interface{})
	BroadcastMessage(messageType string, data interface{})
}

// NopBroadcaster 空实现，用于测试或不需要广播的场景
type NopBroadcaster struct{}

func (n *NopBroadcaster) BroadcastProxyRequest(req *domain.ProxyRequest)                {}
func (n *NopBroadcaster) BroadcastProxyUpstreamAttempt(attempt *domain.ProxyUpstreamAttempt) {}
func (n *NopBroadcaster) BroadcastLog(message string)                                   {}
func (n *NopBroadcaster) BroadcastStats(stats interface{})                              {}
func (n *NopBroadcaster) BroadcastMessage(messageType string, data interface{})         {}
