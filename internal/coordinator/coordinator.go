// Package coordinator 提供多实例协调能力:
//
//   - Pub/Sub:用于跨实例广播缓存失效事件
//   - KV (带 TTL):用于共享 cooldown / session 等短期状态
//   - 实例心跳:活实例集合,作为 MarkStaleAsFailed 的判定依据
//
// 提供两种实现:
//   - memory:进程内实现,单实例部署使用,行为等价于原有本地缓存
//   - redis:基于 Redis 的实现,用于多实例部署
package coordinator

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound 表示 KV 中未找到指定 key
var ErrNotFound = errors.New("coordinator: key not found")

// Message 是 Pub/Sub 传递的载荷
type Message struct {
	Sender  string // 发送方的 InstanceID,订阅者用以过滤自身事件
	Payload []byte
}

// Coordinator 协调器接口
type Coordinator interface {
	// InstanceID 返回当前实例 ID。Publish 时会自动填入 Message.Sender,
	// 订阅者据此过滤掉自己发布的事件,避免回环。
	InstanceID() string

	// Publish 向指定 channel 发布消息。Sender 由实现自动填入。
	Publish(ctx context.Context, channel string, payload []byte) error

	// Subscribe 订阅指定 channel。返回的 channel 在 ctx.Done() 后关闭。
	// 注意:订阅者收到自己发布的消息时仍会传递,过滤逻辑由调用方实现
	// (检查 Message.Sender 是否等于本实例 InstanceID),
	// 这样 coordinator 行为更显式、便于调试。
	Subscribe(ctx context.Context, channel string) (<-chan Message, error)

	// Get 读取一个 key,未命中返回 ErrNotFound
	Get(ctx context.Context, key string) ([]byte, error)

	// Set 设置 key,ttl<=0 表示不过期
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Del 删除 key,key 不存在时不报错
	Del(ctx context.Context, key string) error

	// RegisterInstance 注册本实例心跳,有效期 ttl。
	// 实现应当在过期前自动续期,但调用方也可主动调用 RefreshInstance。
	RegisterInstance(ctx context.Context, ttl time.Duration) error

	// RefreshInstance 续期本实例心跳
	RefreshInstance(ctx context.Context, ttl time.Duration) error

	// UnregisterInstance 立即下线(优雅退出时调用)
	UnregisterInstance(ctx context.Context) error

	// ListAliveInstances 列出当前所有活实例 ID
	ListAliveInstances(ctx context.Context) ([]string, error)

	// Close 释放资源
	Close() error
}
