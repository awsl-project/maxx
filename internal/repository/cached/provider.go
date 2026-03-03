package cached

import (
	"sync"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

type ProviderRepository struct {
	repo  repository.ProviderRepository
	cache map[uint64]*domain.Provider
	mu    sync.RWMutex
}

func NewProviderRepository(repo repository.ProviderRepository) *ProviderRepository {
	return &ProviderRepository{
		repo:  repo,
		cache: make(map[uint64]*domain.Provider),
	}
}

func (r *ProviderRepository) Load() error {
	list, err := r.repo.List(domain.TenantIDAll)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache = make(map[uint64]*domain.Provider, len(list))
	for _, p := range list {
		r.cache[p.ID] = p
	}
	return nil
}

func (r *ProviderRepository) Create(p *domain.Provider) error {
	if err := r.repo.Create(p); err != nil {
		return err
	}
	r.mu.Lock()
	r.cache[p.ID] = p
	r.mu.Unlock()
	return nil
}

func (r *ProviderRepository) Update(p *domain.Provider) error {
	if err := r.repo.Update(p); err != nil {
		return err
	}
	r.mu.Lock()
	r.cache[p.ID] = p
	r.mu.Unlock()
	return nil
}

func (r *ProviderRepository) Delete(tenantID uint64, id uint64) error {
	if err := r.repo.Delete(tenantID, id); err != nil {
		return err
	}
	// 软删除：从缓存中移除（List 不会返回已删除的 provider）
	// GetByID 会从数据库回查已删除的 provider（用于历史记录显示）
	r.mu.Lock()
	delete(r.cache, id)
	r.mu.Unlock()
	return nil
}

func (r *ProviderRepository) GetByID(tenantID uint64, id uint64) (*domain.Provider, error) {
	r.mu.RLock()
	if p, ok := r.cache[id]; ok && (tenantID == domain.TenantIDAll || p.TenantID == tenantID) {
		r.mu.RUnlock()
		return p, nil
	}
	r.mu.RUnlock()
	return r.repo.GetByID(tenantID, id)
}

func (r *ProviderRepository) List(tenantID uint64) ([]*domain.Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]*domain.Provider, 0, len(r.cache))
	for _, p := range r.cache {
		if tenantID == domain.TenantIDAll || p.TenantID == tenantID {
			list = append(list, p)
		}
	}
	return list, nil
}

func (r *ProviderRepository) GetAll() map[uint64]*domain.Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[uint64]*domain.Provider, len(r.cache))
	for k, v := range r.cache {
		result[k] = v
	}
	return result
}
