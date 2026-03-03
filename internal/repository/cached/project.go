package cached

import (
	"sync"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

type ProjectRepository struct {
	repo      repository.ProjectRepository
	cache     map[uint64]*domain.Project
	slugCache map[string]*domain.Project
	mu        sync.RWMutex
}

func NewProjectRepository(repo repository.ProjectRepository) *ProjectRepository {
	return &ProjectRepository{
		repo:      repo,
		cache:     make(map[uint64]*domain.Project),
		slugCache: make(map[string]*domain.Project),
	}
}

func (r *ProjectRepository) Load() error {
	list, err := r.repo.List(0)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range list {
		r.cache[p.ID] = p
		if p.Slug != "" {
			r.slugCache[p.Slug] = p
		}
	}
	return nil
}

func (r *ProjectRepository) Create(p *domain.Project) error {
	if err := r.repo.Create(p); err != nil {
		return err
	}
	r.mu.Lock()
	r.cache[p.ID] = p
	if p.Slug != "" {
		r.slugCache[p.Slug] = p
	}
	r.mu.Unlock()
	return nil
}

func (r *ProjectRepository) Update(p *domain.Project) error {
	// Get old project to remove old slug from cache
	r.mu.RLock()
	oldProject := r.cache[p.ID]
	var oldSlug string
	if oldProject != nil {
		oldSlug = oldProject.Slug
	}
	r.mu.RUnlock()

	if err := r.repo.Update(p); err != nil {
		return err
	}

	r.mu.Lock()
	// Remove old slug from cache if changed
	if oldSlug != "" && oldSlug != p.Slug {
		delete(r.slugCache, oldSlug)
	}
	r.cache[p.ID] = p
	if p.Slug != "" {
		r.slugCache[p.Slug] = p
	}
	r.mu.Unlock()
	return nil
}

func (r *ProjectRepository) Delete(tenantID uint64, id uint64) error {
	// Get project to remove slug from cache
	r.mu.RLock()
	p := r.cache[id]
	r.mu.RUnlock()

	if err := r.repo.Delete(tenantID, id); err != nil {
		return err
	}

	r.mu.Lock()
	delete(r.cache, id)
	if p != nil && p.Slug != "" {
		delete(r.slugCache, p.Slug)
	}
	r.mu.Unlock()
	return nil
}

func (r *ProjectRepository) GetByID(tenantID uint64, id uint64) (*domain.Project, error) {
	r.mu.RLock()
	if p, ok := r.cache[id]; ok {
		r.mu.RUnlock()
		return p, nil
	}
	r.mu.RUnlock()
	return r.repo.GetByID(tenantID, id)
}

func (r *ProjectRepository) GetBySlug(tenantID uint64, slug string) (*domain.Project, error) {
	r.mu.RLock()
	if p, ok := r.slugCache[slug]; ok {
		r.mu.RUnlock()
		return p, nil
	}
	r.mu.RUnlock()

	// Fallback to database
	p, err := r.repo.GetBySlug(tenantID, slug)
	if err != nil {
		return nil, err
	}

	// Update cache
	r.mu.Lock()
	r.cache[p.ID] = p
	r.slugCache[p.Slug] = p
	r.mu.Unlock()

	return p, nil
}

func (r *ProjectRepository) List(tenantID uint64) ([]*domain.Project, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]*domain.Project, 0, len(r.cache))
	for _, p := range r.cache {
		if tenantID == 0 || p.TenantID == tenantID {
			list = append(list, p)
		}
	}
	return list, nil
}
