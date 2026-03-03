package cached

import (
	"sync"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

type RoutingStrategyRepository struct {
	repo  repository.RoutingStrategyRepository
	cache map[uint64]*domain.RoutingStrategy // projectID -> strategy
	mu    sync.RWMutex
}

func NewRoutingStrategyRepository(repo repository.RoutingStrategyRepository) *RoutingStrategyRepository {
	return &RoutingStrategyRepository{
		repo:  repo,
		cache: make(map[uint64]*domain.RoutingStrategy),
	}
}

func (r *RoutingStrategyRepository) Load() error {
	list, err := r.repo.List(domain.TenantIDAll)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range list {
		r.cache[s.ProjectID] = s
	}
	return nil
}

func (r *RoutingStrategyRepository) Create(s *domain.RoutingStrategy) error {
	if err := r.repo.Create(s); err != nil {
		return err
	}
	r.mu.Lock()
	r.cache[s.ProjectID] = s
	r.mu.Unlock()
	return nil
}

func (r *RoutingStrategyRepository) Update(s *domain.RoutingStrategy) error {
	// 先找到旧的 projectID（如果有的话）
	r.mu.RLock()
	var oldProjectID uint64
	var found bool
	for pid, cached := range r.cache {
		if cached.ID == s.ID {
			oldProjectID = pid
			found = true
			break
		}
	}
	r.mu.RUnlock()

	if err := r.repo.Update(s); err != nil {
		return err
	}

	r.mu.Lock()
	// 如果 projectID 改变了，删除旧的缓存条目
	if found && oldProjectID != s.ProjectID {
		delete(r.cache, oldProjectID)
	}
	r.cache[s.ProjectID] = s
	r.mu.Unlock()
	return nil
}

func (r *RoutingStrategyRepository) Delete(tenantID uint64, id uint64) error {
	r.mu.RLock()
	var projectID uint64
	var found bool
	for pid, s := range r.cache {
		if s.ID == id {
			projectID = pid
			found = true
			break
		}
	}
	r.mu.RUnlock()

	if err := r.repo.Delete(tenantID, id); err != nil {
		return err
	}

	if found {
		r.mu.Lock()
		delete(r.cache, projectID)
		r.mu.Unlock()
	}
	return nil
}

func (r *RoutingStrategyRepository) GetByProjectID(tenantID uint64, projectID uint64) (*domain.RoutingStrategy, error) {
	r.mu.RLock()
	if s, ok := r.cache[projectID]; ok && (tenantID == domain.TenantIDAll || s.TenantID == tenantID) {
		r.mu.RUnlock()
		return s, nil
	}
	r.mu.RUnlock()
	return r.repo.GetByProjectID(tenantID, projectID)
}

func (r *RoutingStrategyRepository) List(tenantID uint64) ([]*domain.RoutingStrategy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]*domain.RoutingStrategy, 0, len(r.cache))
	for _, s := range r.cache {
		if tenantID == domain.TenantIDAll || s.TenantID == tenantID {
			list = append(list, s)
		}
	}
	return list, nil
}
