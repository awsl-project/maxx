package cached

import (
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

// APITokenRepository caches API token records around a backing repository.
type APITokenRepository struct {
	repo       repository.APITokenRepository
	cache      map[uint64]*domain.APIToken // by ID
	tokenCache map[string]*domain.APIToken // by token (plaintext)
	mu         sync.RWMutex
}

func NewAPITokenRepository(repo repository.APITokenRepository) *APITokenRepository {
	return &APITokenRepository{
		repo:       repo,
		cache:      make(map[uint64]*domain.APIToken),
		tokenCache: make(map[string]*domain.APIToken),
	}
}

func (r *APITokenRepository) Create(t *domain.APIToken) error {
	if err := r.repo.Create(t); err != nil {
		return err
	}
	r.mu.Lock()
	r.cache[t.ID] = t
	r.tokenCache[t.Token] = t
	r.mu.Unlock()
	return nil
}

func (r *APITokenRepository) Update(t *domain.APIToken) error {
	// Get old token to remove from tokenCache if token changed
	r.mu.RLock()
	old, exists := r.cache[t.ID]
	r.mu.RUnlock()

	if err := r.repo.Update(t); err != nil {
		return err
	}
	r.mu.Lock()
	if exists && old != nil && old.Token != t.Token {
		delete(r.tokenCache, old.Token)
	}
	r.cache[t.ID] = t
	r.tokenCache[t.Token] = t
	r.mu.Unlock()
	return nil
}

func (r *APITokenRepository) Delete(id uint64) error {
	// Get token first to remove from token cache
	r.mu.RLock()
	t, exists := r.cache[id]
	r.mu.RUnlock()

	if err := r.repo.Delete(id); err != nil {
		return err
	}

	r.mu.Lock()
	delete(r.cache, id)
	if exists && t != nil {
		delete(r.tokenCache, t.Token)
	}
	r.mu.Unlock()
	return nil
}

func (r *APITokenRepository) GetByID(id uint64) (*domain.APIToken, error) {
	r.mu.RLock()
	if t, ok := r.cache[id]; ok {
		r.mu.RUnlock()
		return t, nil
	}
	r.mu.RUnlock()

	t, err := r.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.cache[t.ID] = t
	r.tokenCache[t.Token] = t
	r.mu.Unlock()
	return t, nil
}

func (r *APITokenRepository) GetByToken(token string) (*domain.APIToken, error) {
	r.mu.RLock()
	if t, ok := r.tokenCache[token]; ok {
		r.mu.RUnlock()
		return t, nil
	}
	r.mu.RUnlock()

	t, err := r.repo.GetByToken(token)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.cache[t.ID] = t
	r.tokenCache[t.Token] = t
	r.mu.Unlock()
	return t, nil
}

func (r *APITokenRepository) List() ([]*domain.APIToken, error) {
	return r.repo.List()
}

func (r *APITokenRepository) IncrementUseCount(id uint64) error {
	if err := r.repo.IncrementUseCount(id); err != nil {
		return err
	}

	// Update cache if exists
	r.mu.Lock()
	if t, ok := r.cache[id]; ok {
		t.UseCount++
		now := time.Now()
		t.LastUsedAt = &now
	}
	r.mu.Unlock()
	return nil
}

// InvalidateCache clears all cached tokens
func (r *APITokenRepository) InvalidateCache() {
	r.mu.Lock()
	r.cache = make(map[uint64]*domain.APIToken)
	r.tokenCache = make(map[string]*domain.APIToken)
	r.mu.Unlock()
}

// Load preloads all tokens into cache
func (r *APITokenRepository) Load() error {
	tokens, err := r.repo.List()
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range tokens {
		r.cache[t.ID] = t
		r.tokenCache[t.Token] = t
	}
	return nil
}
