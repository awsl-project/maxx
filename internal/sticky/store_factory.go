package sticky

import "github.com/awsl-project/maxx/internal/coordinator"

// StoreFor selects a Store based on the coordinator's actual implementation:
//   - redis  → redisStore (distributed truth)
//   - memory → memoryStore (single-process)
func StoreFor(c coordinator.Coordinator) Store {
	if rdb := coordinator.RedisClient(c); rdb != nil {
		return NewRedisStore(rdb)
	}
	return NewMemoryStore()
}
