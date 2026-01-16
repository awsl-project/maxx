package cached

import (
	"sync"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

type ModelMappingRepository struct {
	repo  repository.ModelMappingRepository
	cache []*domain.ModelMapping
	mu    sync.RWMutex
}

func NewModelMappingRepository(repo repository.ModelMappingRepository) *ModelMappingRepository {
	return &ModelMappingRepository{
		repo:  repo,
		cache: make([]*domain.ModelMapping, 0),
	}
}

func (r *ModelMappingRepository) Load() error {
	list, err := r.repo.List()
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache = list
	return nil
}

func (r *ModelMappingRepository) Create(mapping *domain.ModelMapping) error {
	if err := r.repo.Create(mapping); err != nil {
		return err
	}
	return r.Load() // Reload cache to maintain order
}

func (r *ModelMappingRepository) Update(mapping *domain.ModelMapping) error {
	if err := r.repo.Update(mapping); err != nil {
		return err
	}
	return r.Load() // Reload cache to maintain order
}

func (r *ModelMappingRepository) Delete(id uint64) error {
	if err := r.repo.Delete(id); err != nil {
		return err
	}
	return r.Load() // Reload cache
}

func (r *ModelMappingRepository) GetByID(id uint64) (*domain.ModelMapping, error) {
	r.mu.RLock()
	for _, m := range r.cache {
		if m.ID == id {
			r.mu.RUnlock()
			return m, nil
		}
	}
	r.mu.RUnlock()
	return r.repo.GetByID(id)
}

func (r *ModelMappingRepository) List() ([]*domain.ModelMapping, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*domain.ModelMapping, len(r.cache))
	copy(result, r.cache)
	return result, nil
}

func (r *ModelMappingRepository) ListEnabled() ([]*domain.ModelMapping, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*domain.ModelMapping, 0)
	for _, m := range r.cache {
		if m.IsEnabled {
			result = append(result, m)
		}
	}
	return result, nil
}

func (r *ModelMappingRepository) ListByClientType(clientType domain.ClientType) ([]*domain.ModelMapping, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*domain.ModelMapping, 0)
	for _, m := range r.cache {
		if m.IsEnabled && (m.ClientType == "" || m.ClientType == clientType) {
			result = append(result, m)
		}
	}
	return result, nil
}

// ListByQuery returns all enabled mappings matching the query conditions
func (r *ModelMappingRepository) ListByQuery(query *domain.ModelMappingQuery) ([]*domain.ModelMapping, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*domain.ModelMapping, 0)
	for _, m := range r.cache {
		if !m.IsEnabled {
			continue
		}
		// Match conditions: field is 0/empty OR field matches query
		if m.ClientType != "" && m.ClientType != query.ClientType {
			continue
		}
		if m.ProviderType != "" && m.ProviderType != query.ProviderType {
			continue
		}
		if m.ProviderID != 0 && m.ProviderID != query.ProviderID {
			continue
		}
		if m.ProjectID != 0 && m.ProjectID != query.ProjectID {
			continue
		}
		if m.RouteID != 0 && m.RouteID != query.RouteID {
			continue
		}
		if m.APITokenID != 0 && m.APITokenID != query.APITokenID {
			continue
		}
		result = append(result, m)
	}
	return result, nil
}

func (r *ModelMappingRepository) Count() (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.cache), nil
}

func (r *ModelMappingRepository) DeleteAll() error {
	if err := r.repo.DeleteAll(); err != nil {
		return err
	}
	return r.Load()
}

func (r *ModelMappingRepository) DeleteBuiltin() error {
	if err := r.repo.DeleteBuiltin(); err != nil {
		return err
	}
	return r.Load()
}

func (r *ModelMappingRepository) ClearAll() error {
	if err := r.repo.ClearAll(); err != nil {
		return err
	}
	return r.Load()
}

func (r *ModelMappingRepository) SeedDefaults() error {
	if err := r.repo.SeedDefaults(); err != nil {
		return err
	}
	return r.Load()
}
