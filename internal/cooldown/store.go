package cooldown

import (
	"context"
	"time"
)

// CooldownStore 是 cooldown 分布式真值层的子接口。Manager 通过它读写 Redis
// (或在 standalone 模式下走 memory 实现)。设计上对 Manager 隐藏底层细节,
// 这样 PR 评审者不必同时看 Redis 命令和 cooldown 业务逻辑。
//
// 关键语义:
//   - Set 是"无条件覆盖",用于显式管理操作(SetCooldownUntil 等)。
//   - SetIfLater 仅当新 until 比已有更晚时才写入。RecordFailure 走这条路径
//     以避免"早到但比当前 cooldown 更近的失败"覆盖"已经设了更长 cooldown"
//     的状态。返回 (true, nil) 表示写入生效,(false, nil) 表示已有更晚的值。
//   - ListByProvider 用于 reload:某 provider 的 generation 变了之后,本地
//     需要丢弃该 provider 的所有本地缓存条目,再从 store 全量回填。
//   - GetGeneration / BumpGeneration 必须是原子的;BumpGeneration 返回新值。
//     当前 ClearAll 走 BumpGeneration + 一次性删除该 provider 所有 key 的
//     模式;为了让多实例看到一致的"clear all 时间点",BumpGeneration 应该
//     先于实际删除。
type CooldownStore interface {
	Get(ctx context.Context, key CooldownKey) (until time.Time, found bool, err error)
	ListByProvider(ctx context.Context, providerID uint64) ([]CooldownStoreEntry, error)

	Set(ctx context.Context, key CooldownKey, until time.Time) error
	SetIfLater(ctx context.Context, key CooldownKey, until time.Time) (bool, error)
	Delete(ctx context.Context, key CooldownKey) error
	DeleteByProvider(ctx context.Context, providerID uint64) error

	GetGeneration(ctx context.Context, providerID uint64) (int64, error)
	BumpGeneration(ctx context.Context, providerID uint64) (int64, error)
}

// CooldownStoreEntry 是 ListByProvider 返回的单条记录。
type CooldownStoreEntry struct {
	Key   CooldownKey
	Until time.Time
}
