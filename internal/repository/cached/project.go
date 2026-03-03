package cached

import (
	"sync"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

type projectSlugKey struct {
	TenantID uint64
	Slug     string
}

type ProjectRepository struct {
	repo      repository.ProjectRepository
	cache     map[uint64]*domain.Project
	slugCache map[projectSlugKey]*domain.Project
	mu        sync.RWMutex
}

func NewProjectRepository(repo repository.ProjectRepository) *ProjectRepository {
	return &ProjectRepository{
		repo:      repo,
		cache:     make(map[uint64]*domain.Project),
		slugCache: make(map[projectSlugKey]*domain.Project),
	}
}

func (r *ProjectRepository) Load() error {
	list, err := r.repo.List(domain.TenantIDAll)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache = make(map[uint64]*domain.Project, len(list))
	r.slugCache = make(map[projectSlugKey]*domain.Project)
	for _, p := range list {
		r.cache[p.ID] = p
		if p.Slug != "" {
			r.slugCache[projectSlugKey{TenantID: p.TenantID, Slug: p.Slug}] = p
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
		r.slugCache[projectSlugKey{TenantID: p.TenantID, Slug: p.Slug}] = p
	}
	r.mu.Unlock()
	return nil
}

func (r *ProjectRepository) Update(p *domain.Project) error {
	// Get old project to remove old slug from cache
	r.mu.RLock()
	oldProject := r.cache[p.ID]
	r.mu.RUnlock()

	if err := r.repo.Update(p); err != nil {
		return err
	}

	r.mu.Lock()
	// Remove old slug from cache if changed
	if oldProject != nil && oldProject.Slug != "" && (oldProject.Slug != p.Slug || oldProject.TenantID != p.TenantID) {
		delete(r.slugCache, projectSlugKey{TenantID: oldProject.TenantID, Slug: oldProject.Slug})
	}
	r.cache[p.ID] = p
	if p.Slug != "" {
		r.slugCache[projectSlugKey{TenantID: p.TenantID, Slug: p.Slug}] = p
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
		delete(r.slugCache, projectSlugKey{TenantID: p.TenantID, Slug: p.Slug})
	}
	r.mu.Unlock()
	return nil
}

func (r *ProjectRepository) GetByID(tenantID uint64, id uint64) (*domain.Project, error) {
	r.mu.RLock()
	if p, ok := r.cache[id]; ok && (tenantID == domain.TenantIDAll || p.TenantID == tenantID) {
		r.mu.RUnlock()
		return p, nil
	}
	r.mu.RUnlock()
	return r.repo.GetByID(tenantID, id)
}

func (r *ProjectRepository) GetBySlug(tenantID uint64, slug string) (*domain.Project, error) {
	r.mu.RLock()
	if tenantID == domain.TenantIDAll {
		// Scan for any tenant with this slug
		for key, p := range r.slugCache {
			if key.Slug == slug {
				r.mu.RUnlock()
				return p, nil
			}
		}
	} else if p, ok := r.slugCache[projectSlugKey{TenantID: tenantID, Slug: slug}]; ok {
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
	r.slugCache[projectSlugKey{TenantID: p.TenantID, Slug: p.Slug}] = p
	r.mu.Unlock()

	return p, nil
}

func (r *ProjectRepository) List(tenantID uint64) ([]*domain.Project, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]*domain.Project, 0, len(r.cache))
	for _, p := range r.cache {
		if tenantID == domain.TenantIDAll || p.TenantID == tenantID {
			list = append(list, p)
		}
	}
	return list, nil
}
