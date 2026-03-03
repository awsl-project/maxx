package cached

import (
	"sync"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

type routingStrategyCacheKey struct {
	TenantID  uint64
	ProjectID uint64
}

type RoutingStrategyRepository struct {
	repo  repository.RoutingStrategyRepository
	cache map[routingStrategyCacheKey]*domain.RoutingStrategy
	mu    sync.RWMutex
}

func NewRoutingStrategyRepository(repo repository.RoutingStrategyRepository) *RoutingStrategyRepository {
	return &RoutingStrategyRepository{
		repo:  repo,
		cache: make(map[routingStrategyCacheKey]*domain.RoutingStrategy),
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
		r.cache[routingStrategyCacheKey{TenantID: s.TenantID, ProjectID: s.ProjectID}] = s
	}
	return nil
}

func (r *RoutingStrategyRepository) Create(s *domain.RoutingStrategy) error {
	if err := r.repo.Create(s); err != nil {
		return err
	}
	r.mu.Lock()
	r.cache[routingStrategyCacheKey{TenantID: s.TenantID, ProjectID: s.ProjectID}] = s
	r.mu.Unlock()
	return nil
}

func (r *RoutingStrategyRepository) Update(s *domain.RoutingStrategy) error {
	// 先找到旧的缓存键（如果有的话）
	r.mu.RLock()
	var oldKey routingStrategyCacheKey
	var found bool
	for key, cached := range r.cache {
		if cached.ID == s.ID {
			oldKey = key
			found = true
			break
		}
	}
	r.mu.RUnlock()

	if err := r.repo.Update(s); err != nil {
		return err
	}

	newKey := routingStrategyCacheKey{TenantID: s.TenantID, ProjectID: s.ProjectID}
	r.mu.Lock()
	// 如果缓存键改变了，删除旧的缓存条目
	if found && oldKey != newKey {
		delete(r.cache, oldKey)
	}
	r.cache[newKey] = s
	r.mu.Unlock()
	return nil
}

func (r *RoutingStrategyRepository) Delete(tenantID uint64, id uint64) error {
	r.mu.RLock()
	var cacheKey routingStrategyCacheKey
	var found bool
	for key, s := range r.cache {
		if s.ID == id {
			cacheKey = key
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
		delete(r.cache, cacheKey)
		r.mu.Unlock()
	}
	return nil
}

func (r *RoutingStrategyRepository) GetByProjectID(tenantID uint64, projectID uint64) (*domain.RoutingStrategy, error) {
	r.mu.RLock()
	if tenantID == domain.TenantIDAll {
		// TenantIDAll: scan all entries for matching projectID
		for key, s := range r.cache {
			if key.ProjectID == projectID {
				r.mu.RUnlock()
				return s, nil
			}
		}
	} else if s, ok := r.cache[routingStrategyCacheKey{TenantID: tenantID, ProjectID: projectID}]; ok {
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
