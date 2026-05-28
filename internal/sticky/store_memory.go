package sticky

import (
	"context"
	"sync"
	"time"
)

// memoryStore is an in-process Store for standalone deployments. Different
// instances cannot share state but each request is still served correctly.
type memoryStore struct {
	mu      sync.RWMutex
	entries map[Key]memoryEntry
}

type memoryEntry struct {
	providerID uint64
	expiresAt  time.Time
}

// NewMemoryStore returns a fresh in-process sticky store.
func NewMemoryStore() Store {
	return &memoryStore{entries: make(map[Key]memoryEntry)}
}

func (s *memoryStore) Get(_ context.Context, key Key) (uint64, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entries[key]
	if !ok || time.Now().After(e.expiresAt) {
		return 0, false, nil
	}
	return e.providerID, true, nil
}

func (s *memoryStore) Set(_ context.Context, key Key, providerID uint64, ttl time.Duration) error {
	if ttl <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[key] = memoryEntry{providerID: providerID, expiresAt: time.Now().Add(ttl)}
	return nil
}

func (s *memoryStore) Delete(_ context.Context, key Key) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, key)
	return nil
}
