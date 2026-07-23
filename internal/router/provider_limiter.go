package router

import "sync"

// ProviderLimiter tracks upstream sessions for one gateway process.
type ProviderLimiter struct {
	mu       sync.Mutex
	inflight map[uint64]int
}

func NewProviderLimiter() *ProviderLimiter {
	return &ProviderLimiter{inflight: make(map[uint64]int)}
}

// TryAcquire reserves one upstream session. A non-positive limit is unlimited.
func (l *ProviderLimiter) TryAcquire(providerID uint64, limit int) (release func(), ok bool) {
	if l == nil {
		return func() {}, true
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if limit > 0 && l.inflight[providerID] >= limit {
		return nil, false
	}
	l.inflight[providerID]++

	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			if l.inflight[providerID] <= 1 {
				delete(l.inflight, providerID)
				return
			}
			l.inflight[providerID]--
		})
	}, true
}

func (l *ProviderLimiter) IsAtLimit(providerID uint64, limit int) bool {
	if l == nil || limit <= 0 {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.inflight[providerID] >= limit
}
