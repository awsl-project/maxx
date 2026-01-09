package cached

import (
	"sync"

	"github.com/Bowl42/maxx-next/internal/domain"
	"github.com/Bowl42/maxx-next/internal/repository"
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
	list, err := r.repo.List()
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
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

func (r *ProviderRepository) Delete(id uint64) error {
	if err := r.repo.Delete(id); err != nil {
		return err
	}
	// 软删除：从缓存中移除（List 不会返回已删除的 provider）
	// GetByID 会从数据库回查已删除的 provider（用于历史记录显示）
	r.mu.Lock()
	delete(r.cache, id)
	r.mu.Unlock()
	return nil
}

func (r *ProviderRepository) GetByID(id uint64) (*domain.Provider, error) {
	r.mu.RLock()
	if p, ok := r.cache[id]; ok {
		r.mu.RUnlock()
		return p, nil
	}
	r.mu.RUnlock()
	return r.repo.GetByID(id)
}

func (r *ProviderRepository) List() ([]*domain.Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]*domain.Provider, 0, len(r.cache))
	for _, p := range r.cache {
		list = append(list, p)
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
