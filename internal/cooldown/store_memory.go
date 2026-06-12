package cooldown

import (
	"context"
	"sync"
	"time"
)

// memoryCooldownStore 是 CooldownStore 的纯内存实现。
// standalone 模式下 cooldown.Manager 用它,行为退化为"单进程内一致",和
// 引入 store 接口之前的代码等价。
//
// 不做 TTL 自动过期:Manager 已经在内存层维护 until > now 判断;过期清理
// 由 CleanupExpired 周期任务处理。
type memoryCooldownStore struct {
	mu          sync.RWMutex
	entries     map[CooldownKey]time.Time
	generation  map[uint64]int64
}

// NewMemoryCooldownStore 返回一个进程内 CooldownStore。
func NewMemoryCooldownStore() CooldownStore {
	return &memoryCooldownStore{
		entries:    make(map[CooldownKey]time.Time),
		generation: make(map[uint64]int64),
	}
}

func (s *memoryCooldownStore) Get(_ context.Context, key CooldownKey) (time.Time, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.entries[key]
	if !ok || time.Now().After(t) {
		return time.Time{}, false, nil
	}
	return t, true, nil
}

func (s *memoryCooldownStore) ListByProvider(_ context.Context, providerID uint64) ([]CooldownStoreEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	out := make([]CooldownStoreEntry, 0)
	for k, until := range s.entries {
		if k.ProviderID == providerID && now.Before(until) {
			out = append(out, CooldownStoreEntry{Key: k, Until: until})
		}
	}
	return out, nil
}

func (s *memoryCooldownStore) Set(_ context.Context, key CooldownKey, until time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[key] = until
	return nil
}

func (s *memoryCooldownStore) SetIfLater(_ context.Context, key CooldownKey, until time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 严格大于:同 until 仍接受,保证"mutation 必产生事件"的不变量
	// (详见 store_redis.go::setIfLaterScript 注释)
	if existing, ok := s.entries[key]; ok && existing.After(until) {
		return false, nil
	}
	s.entries[key] = until
	return true, nil
}

func (s *memoryCooldownStore) Delete(_ context.Context, key CooldownKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, key)
	return nil
}

func (s *memoryCooldownStore) DeleteByProvider(_ context.Context, providerID uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.entries {
		if k.ProviderID == providerID {
			delete(s.entries, k)
		}
	}
	return nil
}

func (s *memoryCooldownStore) GetGeneration(_ context.Context, providerID uint64) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.generation[providerID], nil
}

func (s *memoryCooldownStore) BumpGeneration(_ context.Context, providerID uint64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.generation[providerID]++
	return s.generation[providerID], nil
}
