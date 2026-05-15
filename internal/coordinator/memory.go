package coordinator

import (
	"context"
	"sync"
	"time"
)

// memoryCoordinator 是进程内实现。
// 单实例部署下使用,所有跨实例语义降级为本地等价行为:
//   - Pub/Sub 在本进程内分发
//   - KV 是本地 map
//   - 活实例集合永远只包含自己
type memoryCoordinator struct {
	instanceID string

	mu     sync.RWMutex
	kv     map[string]memoryEntry
	subs   map[string]map[*memorySub]struct{}
	alive  map[string]time.Time // instanceID -> expiry
	closed bool
}

type memoryEntry struct {
	value  []byte
	expiry time.Time // 零值表示不过期
}

type memorySub struct {
	ch     chan Message
	once   sync.Once
	closed bool
}

func (s *memorySub) shut() {
	s.once.Do(func() {
		s.closed = true
		close(s.ch)
	})
}

// NewMemory 返回一个内存实现的 Coordinator
func NewMemory(instanceID string) Coordinator {
	return &memoryCoordinator{
		instanceID: instanceID,
		kv:         make(map[string]memoryEntry),
		subs:       make(map[string]map[*memorySub]struct{}),
		alive:      make(map[string]time.Time),
	}
}

func (c *memoryCoordinator) InstanceID() string { return c.instanceID }

func (c *memoryCoordinator) Publish(_ context.Context, channel string, payload []byte) error {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil
	}
	subs := make([]*memorySub, 0, len(c.subs[channel]))
	for s := range c.subs[channel] {
		subs = append(subs, s)
	}
	c.mu.RUnlock()

	msg := Message{Sender: c.instanceID, Payload: append([]byte(nil), payload...)}
	for _, s := range subs {
		// 非阻塞投递:订阅者跟不上就丢弃,避免拖垮整个广播
		select {
		case s.ch <- msg:
		default:
		}
	}
	return nil
}

func (c *memoryCoordinator) Subscribe(ctx context.Context, channel string) (<-chan Message, error) {
	sub := &memorySub{ch: make(chan Message, 64)}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		sub.shut()
		return sub.ch, nil
	}
	if c.subs[channel] == nil {
		c.subs[channel] = make(map[*memorySub]struct{})
	}
	c.subs[channel][sub] = struct{}{}
	c.mu.Unlock()

	go func() {
		<-ctx.Done()
		c.mu.Lock()
		if m := c.subs[channel]; m != nil {
			delete(m, sub)
		}
		c.mu.Unlock()
		sub.shut()
	}()

	return sub.ch, nil
}

func (c *memoryCoordinator) Get(_ context.Context, key string) ([]byte, error) {
	c.mu.RLock()
	e, ok := c.kv[key]
	c.mu.RUnlock()
	if !ok {
		return nil, ErrNotFound
	}
	if !e.expiry.IsZero() && time.Now().After(e.expiry) {
		// 过期条目主动清理,避免 map 在长期运行下无界增长
		// (生产环境用 Redis 实现走 TTL,memory 实现没有 background expirer)
		c.mu.Lock()
		if e2, still := c.kv[key]; still && e2.expiry == e.expiry {
			delete(c.kv, key)
		}
		c.mu.Unlock()
		return nil, ErrNotFound
	}
	return append([]byte(nil), e.value...), nil
}

func (c *memoryCoordinator) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := memoryEntry{value: append([]byte(nil), value...)}
	if ttl > 0 {
		e.expiry = time.Now().Add(ttl)
	}
	c.kv[key] = e
	return nil
}

func (c *memoryCoordinator) Del(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.kv, key)
	return nil
}

func (c *memoryCoordinator) RegisterInstance(_ context.Context, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.alive[c.instanceID] = time.Now().Add(ttl)
	return nil
}

func (c *memoryCoordinator) RefreshInstance(ctx context.Context, ttl time.Duration) error {
	return c.RegisterInstance(ctx, ttl)
}

func (c *memoryCoordinator) UnregisterInstance(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.alive, c.instanceID)
	return nil
}

func (c *memoryCoordinator) ListAliveInstances(_ context.Context) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	out := make([]string, 0, len(c.alive))
	for id, exp := range c.alive {
		if exp.Before(now) {
			delete(c.alive, id)
			continue
		}
		out = append(out, id)
	}
	return out, nil
}

func (c *memoryCoordinator) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	subs := c.subs
	c.subs = make(map[string]map[*memorySub]struct{})
	c.mu.Unlock()

	for _, m := range subs {
		for s := range m {
			s.shut()
		}
	}
	return nil
}
