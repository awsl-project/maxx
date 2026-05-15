package cached

import (
	"context"
	"log"

	"github.com/awsl-project/maxx/internal/coordinator"
)

// 缓存失效广播 channel 名常量。每个 cached repo 各占一个 channel。
const (
	InvalidateAPIToken        = "api_token"
	InvalidateProvider        = "provider"
	InvalidateRoute           = "route"
	InvalidateRetryConfig     = "retry_config"
	InvalidateRoutingStrategy = "routing_strategy"
	InvalidateProject         = "project"
	InvalidateModelMapping    = "model_mapping"
)

const cacheChannelPrefix = "cache:invalidate:"

// cacheBroadcast 是每个 cached repo 嵌入的小结构,
// 负责在写操作后向 coordinator 发布"本 entity 已变更"的事件。
//
// 设计取舍:广播只携带 entity 名,不带具体变更内容。订阅者收到事件后
// 全量重新加载该 entity 的缓存。这避免了序列化/反序列化每个 domain
// 对象,缓存大小都不大(几十到几千条),全清+重载完全可接受;同时也
// 规避了"只清单条但其他实例本地状态发散"这类难调的一致性问题。
type cacheBroadcast struct {
	coord coordinator.Coordinator
	name  string
}

// attach 绑定 coordinator 和 channel 名。可在 nil coord 下安全调用。
func (b *cacheBroadcast) attach(c coordinator.Coordinator, name string) {
	b.coord = c
	b.name = name
}

// publish 通知其他实例:这个 entity 的缓存需要刷新。
// coord 未绑定(单实例 memory 退化场景之前)时是 no-op。
func (b *cacheBroadcast) publish() {
	if b == nil || b.coord == nil {
		return
	}
	if err := b.coord.Publish(context.Background(), cacheChannelPrefix+b.name, nil); err != nil {
		log.Printf("[Cache] publish %s invalidation failed: %v", b.name, err)
	}
}

// AttachInvalidation 由 main 集中调用,启动一个订阅 goroutine。
// 收到非自身发出的失效事件时调用 onInvalidate(由调用方决定如何清缓存/重载)。
// ctx 取消后订阅自动结束。
func AttachInvalidation(
	ctx context.Context,
	c coordinator.Coordinator,
	name string,
	onInvalidate func(),
) {
	if c == nil {
		return
	}
	ch, err := c.Subscribe(ctx, cacheChannelPrefix+name)
	if err != nil {
		log.Printf("[Cache] subscribe %s failed: %v", name, err)
		return
	}
	selfID := c.InstanceID()
	go func() {
		for msg := range ch {
			if msg.Sender == selfID {
				// 自己发布的事件直接忽略,避免清掉刚写好的本地缓存
				continue
			}
			onInvalidate()
		}
	}()
}
