package cached

import (
    "sync"

    "github.com/awsl-project/maxx/internal/domain"
    "github.com/awsl-project/maxx/internal/repository"
)

type RetryConfigRepository struct {
    repo         repository.RetryConfigRepository
    cache        map[uint64]*domain.RetryConfig
    defaultCache *domain.RetryConfig
    mu           sync.RWMutex
}

func NewRetryConfigRepository(repo repository.RetryConfigRepository) *RetryConfigRepository {
    return &RetryConfigRepository{
        repo:  repo,
        cache: make(map[uint64]*domain.RetryConfig),
    }
}

func (r *RetryConfigRepository) Load() error {
    list, err := r.repo.List(domain.TenantIDAll)
    if err != nil {
        return err
    }
    r.mu.Lock()
    defer r.mu.Unlock()
    for _, c := range list {
        r.cache[c.ID] = c
        if c.IsDefault {
            r.defaultCache = c
        }
    }
    return nil
}

func (r *RetryConfigRepository) Create(c *domain.RetryConfig) error {
    if err := r.repo.Create(c); err != nil {
        return err
    }
    r.mu.Lock()
    r.cache[c.ID] = c
    if c.IsDefault {
        if r.defaultCache != nil && r.defaultCache.ID != c.ID {
            r.defaultCache.IsDefault = false
        }
        r.defaultCache = c
    }
    r.mu.Unlock()
    return nil
}

func (r *RetryConfigRepository) Update(c *domain.RetryConfig) error {
    if err := r.repo.Update(c); err != nil {
        return err
    }
    r.mu.Lock()
    r.cache[c.ID] = c
    if c.IsDefault {
        if r.defaultCache != nil && r.defaultCache.ID != c.ID {
            r.defaultCache.IsDefault = false
        }
        r.defaultCache = c
    } else if r.defaultCache != nil && r.defaultCache.ID == c.ID {
        r.defaultCache = nil
    }
    r.mu.Unlock()
    return nil
}

func (r *RetryConfigRepository) Delete(tenantID uint64, id uint64) error {
    if err := r.repo.Delete(tenantID, id); err != nil {
        return err
    }
    r.mu.Lock()
    if r.defaultCache != nil && r.defaultCache.ID == id {
        r.defaultCache = nil
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
    if r.defaultCache != nil && (tenantID == domain.TenantIDAll || r.defaultCache.TenantID == tenantID) {
        r.mu.RUnlock()
        return r.defaultCache, nil
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
