package cooldown

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/awsl-project/maxx/internal/coordinator"
)

// 事件 channel:RFC schema 中的 maxx:v1:event:cooldown
const eventChannel = "maxx:v1:event:cooldown"

// cooldownIOTimeout 是 Redis 调用(BumpGeneration / GetGeneration / Publish)的
// 最大等待时间。设小一些避免在 m.mu 写锁路径上挂太久。失败的代价仅是这次
// generation 同步缺失,下次 IsInCooldown 触发的节流 sync 会拉齐。
const cooldownIOTimeout = 200 * time.Millisecond

// providerEvent 是广播给其他实例的事件 payload。
// 故意只带 provider_id + generation:
//   - generation 是 provider 状态变化的版本号,变了就要重载该 provider 的本地条目
//   - 不带具体 op/key/until,因为本地重载从 store.ListByProvider 拿权威数据,
//     省去维护事件协议和 store 之间一致性的负担
type providerEvent struct {
	ProviderID uint64 `json:"provider_id"`
	Generation int64  `json:"generation"`
}

// SetCoordinator 注入 coordinator,构造对应的 CooldownStore 并启动事件订阅。
//   - 在 distributed 模式下 store 是 redis 实现,事件广播跨实例
//   - 在 standalone 模式下 store 是 memory 实现,事件广播只在本进程内(回环过滤)
//
// 公共方法签名保留:外部调用者(main.go, desktop launcher)不感知变化。
// ctx 取消时订阅 goroutine 自动结束。
func (m *Manager) SetCoordinator(ctx context.Context, c coordinator.Coordinator) {
	if c == nil {
		return
	}
	m.coord.Store(&c)
	store := StoreFor(c)
	m.store.Store(&store)

	ch, err := c.Subscribe(ctx, eventChannel)
	if err != nil {
		log.Printf("[Cooldown] subscribe %s failed: %v", eventChannel, err)
		return
	}
	selfID := c.InstanceID()
	go func() {
		for msg := range ch {
			if msg.Sender == selfID {
				continue
			}
			var ev providerEvent
			if err := json.Unmarshal(msg.Payload, &ev); err != nil {
				log.Printf("[Cooldown] discard malformed event: %v", err)
				continue
			}
			m.applyRemoteEvent(ev)
		}
	}()
}

// bumpAndPublishLocked 是 mutation 路径的通用收尾:递增 provider 的
// generation(对自己也算"看到了新版本",所以更新 providerGen + lastGenSync),
// 然后通过 coordinator 广播给其他实例。
//
// 必须在持有 m.mu 的路径内调用。内部不再获取锁。
//
// 三个分支:
//   - 有 store 且 BumpGeneration 成功:用 Redis 返回的新 generation
//   - 有 store 但 BumpGeneration 失败:**不**广播,**不**自增本地 gen。
//     原因:如果本地 gen 单独自增,会和 Redis 上的 gen 漂移。下次
//     syncProviderGeneration 看到 Redis gen < local gen,会触发 reload,
//     reload 又把 local gen 拉低到 Redis gen ——形成"先涨后跌"的回滚,
//     既不必要又给观测留下假信号。让 Redis 是 generation 的唯一真值。
//   - 无 store (standalone):本地自增,memory 模式下不存在多实例所以无差。
func (m *Manager) bumpAndPublishLocked(providerID uint64) {
	var newGen int64
	var canPublish bool

	if sp := m.store.Load(); sp != nil {
		bumpCtx, cancel := context.WithTimeout(context.Background(), cooldownIOTimeout)
		gen, err := (*sp).BumpGeneration(bumpCtx, providerID)
		cancel()
		if err != nil {
			log.Printf("[Cooldown] BumpGeneration failed for provider %d: %v (skipping publish; remote peers will resync on next generation change)", providerID, err)
			// 不更新本地 gen 也不 publish。
			// 后续成功的 mutation 会推进 Redis gen,然后本实例和其他实例
			// 都能通过 syncProviderGeneration / 事件感知。
			return
		}
		newGen = gen
		canPublish = true
	} else {
		// standalone:本地自增
		newGen = m.providerGen[providerID] + 1
	}

	m.providerGen[providerID] = newGen
	m.lastGenSync[providerID] = time.Now()

	if !canPublish {
		return
	}

	// 广播事件给其他实例
	if cp := m.coord.Load(); cp != nil {
		payload, err := json.Marshal(providerEvent{ProviderID: providerID, Generation: newGen})
		if err != nil {
			log.Printf("[Cooldown] marshal event: %v", err)
			return
		}
		pubCtx, cancel := context.WithTimeout(context.Background(), cooldownIOTimeout)
		err = (*cp).Publish(pubCtx, eventChannel, payload)
		cancel()
		if err != nil {
			log.Printf("[Cooldown] publish event: %v", err)
		}
	}
}

// applyRemoteEvent 处理来自其他实例的事件。
//
// 跳过条件:ev.Generation <= 本地已知 generation。
// 原因:Redis pub/sub 偶尔会乱序送达(连接重连后回放队列),如果照单全收
// 会让本地 providerGen 倒退,然后 syncProviderGeneration 立刻又会跑一次
// reload 把它推回最新值——白做一次 ListByProvider。严格"只前进不后退"
// 既保证收敛,又避免乱序时的颠簸。
func (m *Manager) applyRemoteEvent(ev providerEvent) {
	if sp := m.store.Load(); sp == nil {
		return // 没有 store 就没有真值可重载
	}

	m.mu.RLock()
	local := m.providerGen[ev.ProviderID]
	m.mu.RUnlock()
	if ev.Generation <= local {
		return
	}

	m.reloadProvider(ev.ProviderID, ev.Generation)
}

// reloadProvider 从 store 全量拉取某 provider 的 cooldown 条目,
// 替换本地该 provider 的所有条目。
//
// targetGen > 0:reload 完成后把 providerGen 写成 targetGen(用于事件触发的 reload)。
// targetGen == 0:reload 完成后查 store 取当前 generation 写入(用于主动 sync)。
//
// 反复 reload 的并发安全:整个操作在 m.mu 内,其他读路径会等待。
func (m *Manager) reloadProvider(providerID uint64, targetGen int64) {
	sp := m.store.Load()
	if sp == nil {
		return
	}
	store := *sp

	ctx := context.Background()
	entries, err := store.ListByProvider(ctx, providerID)
	if err != nil {
		log.Printf("[Cooldown] reloadProvider %d: ListByProvider failed: %v", providerID, err)
		return
	}

	// 如果调用方没给 targetGen,自己查一次
	if targetGen == 0 {
		g, err := store.GetGeneration(ctx, providerID)
		if err != nil {
			log.Printf("[Cooldown] reloadProvider %d: GetGeneration failed: %v", providerID, err)
		}
		targetGen = g
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 清掉该 provider 的本地条目
	for k := range m.cooldowns {
		if k.ProviderID == providerID {
			delete(m.cooldowns, k)
			delete(m.reasons, k)
		}
	}
	// 装入 store 最新条目
	for _, e := range entries {
		m.cooldowns[e.Key] = e.Until
		// reason 在 store 中没有保留(为了减小 payload);用 Unknown 兜底,
		// 后续如果本地有写操作会自然把 reason 填回去。
		if _, ok := m.reasons[e.Key]; !ok {
			m.reasons[e.Key] = ReasonUnknown
		}
	}

	// providerGen 只能前进,不能后退。reloadProvider 可能被 applyRemoteEvent
	// 触发(targetGen 来自远程事件)、被 syncProviderGeneration 触发(targetGen
	// 来自 store GetGeneration),或被 store 错误时清空 lastGenSync 后的下一次
	// IsInCooldown 触发。在 ListByProvider I/O 期间,本地 bumpAndPublishLocked
	// 可能已经把 providerGen 推到比 targetGen 更高的值;此时不应该让 reload
	// 把它倒退。
	if current := m.providerGen[providerID]; targetGen > current {
		m.providerGen[providerID] = targetGen
	}
	m.lastGenSync[providerID] = time.Now()
}

// syncProviderGeneration 在 IsInCooldown 等读路径上被调用,
// 按节流窗口检查 store 的 generation 是否变了,变了就 reload。
//
// 不持锁调用;reloadProvider 内部会获取写锁。
func (m *Manager) syncProviderGeneration(providerID uint64) {
	sp := m.store.Load()
	if sp == nil {
		return
	}

	m.mu.RLock()
	last := m.lastGenSync[providerID]
	localGen := m.providerGen[providerID]
	every := m.genSyncEvery
	m.mu.RUnlock()

	if every <= 0 {
		every = 2 * time.Second
	}
	if time.Since(last) < every {
		return
	}

	syncCtx, cancel := context.WithTimeout(context.Background(), cooldownIOTimeout)
	remoteGen, err := (*sp).GetGeneration(syncCtx, providerID)
	cancel()
	if err != nil {
		log.Printf("[Cooldown] syncProviderGeneration %d: %v", providerID, err)
		// 即便查失败也记下尝试时间,避免热路径上反复打 Redis
		m.mu.Lock()
		m.lastGenSync[providerID] = time.Now()
		m.mu.Unlock()
		return
	}

	if remoteGen == localGen {
		m.mu.Lock()
		m.lastGenSync[providerID] = time.Now()
		m.mu.Unlock()
		return
	}

	// generation 变了,触发 reload
	m.reloadProvider(providerID, remoteGen)
}
