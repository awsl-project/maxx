package cooldown

import "github.com/awsl-project/maxx/internal/coordinator"

// StoreFor 根据 coordinator 的实际实现选择合适的 CooldownStore:
//   - redis 实现 → redisCooldownStore(分布式真值)
//   - memory 实现 → memoryCooldownStore(进程内,行为等价于无 store)
//
// 集中在这一处选择,Manager 只看 CooldownStore 接口,不必感知 coordinator
// 的具体类型。
func StoreFor(c coordinator.Coordinator) CooldownStore {
	if rdb := coordinator.RedisClient(c); rdb != nil {
		return NewRedisCooldownStore(rdb)
	}
	return NewMemoryCooldownStore()
}
