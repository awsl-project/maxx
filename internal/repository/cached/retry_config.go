package cached

import (
	"sync"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

type RetryConfigRepository struct {
	repo         repository.RetryConfigRepository
	cache        map[uint64]*domain.RetryConfig
	defaultCache map[uint64]*domain.RetryConfig // tenantID -> default config
	mu           sync.RWMutex
}

func NewRetryConfigRepository(repo repository.RetryConfigRepository) *RetryConfigRepository {
	return &RetryConfigRepository{
		repo:         repo,
		cache:        make(map[uint64]*domain.RetryConfig),
		defaultCache: make(map[uint64]*domain.RetryConfig),
	}
}

func (r *RetryConfigRepository) Load() error {
	list, err := r.repo.List(domain.TenantIDAll)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache = make(map[uint64]*domain.RetryConfig, len(list))
	r.defaultCache = make(map[uint64]*domain.RetryConfig)
	for _, c := range list {
		r.cache[c.ID] = c
		if c.IsDefault {
			r.defaultCache[c.TenantID] = c
		}
	}
	return nil
}

func (r *RetryConfigRepository) Create(c *domain.RetryConfig) error {
	if err := r.repo.Create(c); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache[c.ID] = c
	if c.IsDefault {
		if old, ok := r.defaultCache[c.TenantID]; ok && old.ID != c.ID {
			oldCopy := *old
			oldCopy.IsDefault = false
			if err := r.repo.Update(&oldCopy); err != nil {
				return err
			}
			old.IsDefault = false
		}
		r.defaultCache[c.TenantID] = c
	}
	return nil
}

func (r *RetryConfigRepository) Update(c *domain.RetryConfig) error {
	if err := r.repo.Update(c); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache[c.ID] = c
	if c.IsDefault {
		if old, ok := r.defaultCache[c.TenantID]; ok && old.ID != c.ID {
			oldCopy := *old
			oldCopy.IsDefault = false
			if err := r.repo.Update(&oldCopy); err != nil {
				return err
			}
			old.IsDefault = false
		}
		r.defaultCache[c.TenantID] = c
	} else if old, ok := r.defaultCache[c.TenantID]; ok && old.ID == c.ID {
		delete(r.defaultCache, c.TenantID)
	}
	return nil
}

func (r *RetryConfigRepository) Delete(tenantID uint64, id uint64) error {
	if err := r.repo.Delete(tenantID, id); err != nil {
		return err
	}
	r.mu.Lock()
	// Remove from default cache if this was the default for any tenant
	if c, ok := r.cache[id]; ok {
		if def, ok := r.defaultCache[c.TenantID]; ok && def.ID == id {
			delete(r.defaultCache, c.TenantID)
		}
	}
	delete(r.cache, id)
	r.mu.Unlock()
	return nil
}

func (r *RetryConfigRepository) GetByID(tenantID uint64, id uint64) (*domain.RetryConfig, error) {
	r.mu.RLock()
	if c, ok := r.cache[id]; ok && (tenantID == domain.TenantIDAll || c.TenantID == tenantID) {
		r.mu.RUnlock()
		return c, nil
	}
	r.mu.RUnlock()
	return r.repo.GetByID(tenantID, id)
}

func (r *RetryConfigRepository) GetDefault(tenantID uint64) (*domain.RetryConfig, error) {
	r.mu.RLock()
	if tenantID != domain.TenantIDAll {
		if c, ok := r.defaultCache[tenantID]; ok {
			r.mu.RUnlock()
			return c, nil
		}
	}
	r.mu.RUnlock()
	return r.repo.GetDefault(tenantID)
}

func (r *RetryConfigRepository) List(tenantID uint64) ([]*domain.RetryConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]*domain.RetryConfig, 0, len(r.cache))
	for _, c := range r.cache {
		if tenantID == domain.TenantIDAll || c.TenantID == tenantID {
			list = append(list, c)
		}
	}
	return list, nil
}
