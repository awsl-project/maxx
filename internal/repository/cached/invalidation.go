package cached

import (
	"context"
	"encoding/json"
	"log"
	"time"

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

// CacheInvalidateOp 标识发布事件时的操作类型。订阅者 v1 仍然全表 reload,
// 但事件协议保留这些字段,后续按 ID 失效的演进无需改 wire protocol。
type CacheInvalidateOp string

const (
	OpCreate CacheInvalidateOp = "create"
	OpUpdate CacheInvalidateOp = "update"
	OpDelete CacheInvalidateOp = "delete"
	OpReload CacheInvalidateOp = "reload" // 全量重置(SeedDefaults/DeleteAll 等)
)

const cacheChannelPrefix = "cache:invalidate:"

// invalidationEvent 是 publish 的载荷。订阅端 v1 只检查 Entity 是否匹配 channel,
// 其他字段为后续按 ID 失效的优化预留。
type invalidationEvent struct {
	Entity    string            `json:"entity"`
	Op        CacheInvalidateOp `json:"op"`
	ID        uint64            `json:"id,omitempty"`         // 0 表示批量/未指定
	UpdatedAt int64             `json:"updated_at,omitempty"` // unix milliseconds
}

// cacheBroadcast 是每个 cached repo 嵌入的小结构,
// 负责在写操作后向 coordinator 发布"本 entity 已变更"的事件。
//
// 设计取舍:广播 v1 仍由订阅端做"全量 reload",事件 payload 已经包含 op/id/
// updated_at 字段,后续若某个 entity 的 reload 成本显著到需要细粒度,可以在
// 订阅端切换到 targeted invalidation 而无需改 wire protocol。
type cacheBroadcast struct {
	coord coordinator.Coordinator
	name  string
}

// attach 绑定 coordinator 和 channel 名。可在 nil coord 下安全调用。
func (b *cacheBroadcast) attach(c coordinator.Coordinator, name string) {
	b.coord = c
	b.name = name
}

// publishTimeout 是单次 publish 的最大等待时间。
// Publish 走 coordinator → Redis,在 Redis 抖动时可能短暂阻塞;给一个
// 较小的上限避免写路径(管理 API、admin 操作)被外部存储拖死。
// 超时只意味着这次 invalidation 没被广播,数据本身已经写好。
const publishTimeout = 200 * time.Millisecond

// publish 通知其他实例:这个 entity 的某条记录发生了变更。
// op 指明操作类型,id 是变更对象的主键(批量操作传 0),订阅端 v1 全部按
// 全表 reload 处理,但 payload 字段为细粒度版本预留。
// coord 未绑定时是 no-op。失败/超时仅 log,不返回给调用方。
func (b *cacheBroadcast) publish(op CacheInvalidateOp, id uint64) {
	if b == nil || b.coord == nil {
		return
	}
	payload, err := json.Marshal(invalidationEvent{
		Entity:    b.name,
		Op:        op,
		ID:        id,
		UpdatedAt: time.Now().UnixMilli(),
	})
	if err != nil {
		log.Printf("[Cache] marshal invalidation event for %s: %v", b.name, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), publishTimeout)
	defer cancel()
	if err := b.coord.Publish(ctx, cacheChannelPrefix+b.name, payload); err != nil {
		log.Printf("[Cache] publish %s invalidation failed: %v", b.name, err)
	}
}

// AttachInvalidation 由 main 集中调用,启动一个订阅 goroutine。
// 收到非自身发出的失效事件时调用 onInvalidate(由调用方决定如何清缓存/重载)。
// ctx 取消后订阅自动结束。
//
// 当前 onInvalidate 是无参数的 callback,所有 entity 都走全表 reload。
// 后续如果需要按 ID 失效,可以扩展 onInvalidate 接受 *invalidationEvent
// 参数,这里也 unmarshal payload 传过去。
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
			// payload 当前不解析:v1 全表 reload 不需要事件内容。
			// 仅做 sanity check —— 旧版本发的空 payload 也容忍。
			_ = msg.Payload
			onInvalidate()
		}
	}()
}
