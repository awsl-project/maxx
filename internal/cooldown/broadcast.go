package cooldown

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/awsl-project/maxx/internal/coordinator"
)

// 广播 channel 名。所有实例都订阅此 channel。
const broadcastChannel = "cooldown:event"

// cooldownOp 是广播事件的操作类型
type cooldownOp string

const (
	opSet      cooldownOp = "set"      // 设置/更新一条 cooldown
	opClear    cooldownOp = "clear"    // 清除一条 cooldown(具体到 key)
	opClearAll cooldownOp = "clear_all" // 清除某 provider 的所有 cooldown
)

// cooldownEvent 是发布在广播 channel 上的事件载荷
type cooldownEvent struct {
	Op         cooldownOp `json:"op"`
	ProviderID uint64     `json:"provider_id"`
	ClientType string     `json:"client_type,omitempty"`
	Model      string     `json:"model,omitempty"`
	UntilUnix  int64      `json:"until_unix,omitempty"` // op=set 时有效,秒级
	Reason     string     `json:"reason,omitempty"`     // op=set 时有效
}

// SetCoordinator 注入 coordinator,启用跨实例 cooldown 同步。
//
// 设计要点:
//   - Manager 的本地内存仍是 IsInCooldown 的快路径。
//   - 所有写操作(setCooldownLocked / RecordSuccess / ClearCooldown 等)在
//     完成本地内存 + DB 写入后调用 m.broadcast() 通知其他实例。
//   - 订阅 goroutine 收到非自身事件时,仅更新本地内存,不再写 DB
//     (因为发起方已经持久化过了),避免回环和重复写。
//
// 必须在 Manager 构造完成且持有 Repository 之后再调用。ctx 取消时订阅 goroutine 结束。
func (m *Manager) SetCoordinator(ctx context.Context, c coordinator.Coordinator) {
	if c == nil {
		return
	}
	m.coord.Store(&c)

	ch, err := c.Subscribe(ctx, broadcastChannel)
	if err != nil {
		log.Printf("[Cooldown] subscribe broadcast failed: %v", err)
		return
	}
	selfID := c.InstanceID()
	go func() {
		for msg := range ch {
			if msg.Sender == selfID {
				continue
			}
			var ev cooldownEvent
			if err := json.Unmarshal(msg.Payload, &ev); err != nil {
				log.Printf("[Cooldown] discard malformed broadcast: %v", err)
				continue
			}
			m.applyRemoteEvent(ev)
		}
	}()
}

// broadcast 由 Manager 内部写操作调用,发出一条事件。
// 不获取 Manager 锁,可在持锁路径内调用而不引入嵌套锁。
// coord 未注入时 no-op。
func (m *Manager) broadcast(ev cooldownEvent) {
	p := m.coord.Load()
	if p == nil {
		return
	}
	c := *p
	data, err := json.Marshal(ev)
	if err != nil {
		log.Printf("[Cooldown] marshal broadcast event: %v", err)
		return
	}
	if err := c.Publish(context.Background(), broadcastChannel, data); err != nil {
		log.Printf("[Cooldown] publish broadcast: %v", err)
	}
}

// applyRemoteEvent 把来自其他实例的事件应用到本地内存。
// 不写 DB(对方写过了),不广播(避免回环)。
func (m *Manager) applyRemoteEvent(ev cooldownEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch ev.Op {
	case opSet:
		key := CooldownKey{ProviderID: ev.ProviderID, ClientType: ev.ClientType, Model: ev.Model}
		m.cooldowns[key] = time.Unix(ev.UntilUnix, 0)
		m.reasons[key] = CooldownReason(ev.Reason)
	case opClear:
		key := CooldownKey{ProviderID: ev.ProviderID, ClientType: ev.ClientType, Model: ev.Model}
		delete(m.cooldowns, key)
		delete(m.reasons, key)
	case opClearAll:
		for k := range m.cooldowns {
			if k.ProviderID == ev.ProviderID {
				delete(m.cooldowns, k)
				delete(m.reasons, k)
			}
		}
	}
}
