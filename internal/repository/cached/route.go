package cached

import (
	"sort"
	"sync"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

type RouteRepository struct {
	repo  repository.RouteRepository
	cache []*domain.Route
	mu    sync.RWMutex
}

func NewRouteRepository(repo repository.RouteRepository) *RouteRepository {
	return &RouteRepository{
		repo: repo,
	}
}

func (r *RouteRepository) Load() error {
	list, err := r.repo.List()
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.cache = list
	r.mu.Unlock()
	return nil
}

func (r *RouteRepository) Create(route *domain.Route) error {
	if err := r.repo.Create(route); err != nil {
		return err
	}
	r.mu.Lock()
	r.cache = append(r.cache, route)
	r.sortCacheLocked()
	r.mu.Unlock()
	return nil
}

func (r *RouteRepository) Update(route *domain.Route) error {
	if err := r.repo.Update(route); err != nil {
		return err
	}
	r.mu.Lock()
	for i, rt := range r.cache {
		if rt.ID == route.ID {
			r.cache[i] = route
			break
		}
	}
	r.sortCacheLocked()
	r.mu.Unlock()
	return nil
}

func (r *RouteRepository) Delete(id uint64) error {
	if err := r.repo.Delete(id); err != nil {
		return err
	}
	r.mu.Lock()
	for i, rt := range r.cache {
		if rt.ID == id {
			r.cache = append(r.cache[:i], r.cache[i+1:]...)
			break
		}
	}
	r.mu.Unlock()
	return nil
}

func (r *RouteRepository) BatchUpdatePositions(updates []domain.RoutePositionUpdate) error {
	if err := r.repo.BatchUpdatePositions(updates); err != nil {
		return err
	}
	// Reload cache to reflect position changes
	return r.Load()
}

func (r *RouteRepository) GetByID(id uint64) (*domain.Route, error) {
	r.mu.RLock()
	for _, rt := range r.cache {
		if rt.ID == id {
			r.mu.RUnlock()
			return rt, nil
		}
	}
	r.mu.RUnlock()
	return r.repo.GetByID(id)
}

func (r *RouteRepository) FindByKey(projectID, providerID uint64, clientType domain.ClientType) (*domain.Route, error) {
	r.mu.RLock()
	for _, rt := range r.cache {
		if rt.ProjectID == projectID && rt.ProviderID == providerID && rt.ClientType == clientType {
			r.mu.RUnlock()
			return rt, nil
		}
	}
	r.mu.RUnlock()
	return r.repo.FindByKey(projectID, providerID, clientType)
}

func (r *RouteRepository) List() ([]*domain.Route, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*domain.Route, len(r.cache))
	copy(result, r.cache)
	return result, nil
}

func (r *RouteRepository) GetAll() []*domain.Route {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*domain.Route, len(r.cache))
	copy(result, r.cache)
	return result
}

// sortCacheLocked sorts the cache by Position. Must be called with mu held.
func (r *RouteRepository) sortCacheLocked() {
	sort.Slice(r.cache, func(i, j int) bool {
		return r.cache[i].Position < r.cache[j].Position
	})
}
