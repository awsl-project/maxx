package cached

import (
	"sort"
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

// Load 从数据库加载所有数据到内存（只在启动时调用一次）
func (r *ModelMappingRepository) Load() error {
	list, err := r.repo.List()
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache = list
	r.sortCache()
	return nil
}

// scopePriority 返回 scope 的优先级数值（数字越小优先级越高）
func scopePriority(scope domain.ModelMappingScope) int {
	switch scope {
	case domain.ModelMappingScopeRoute:
		return 1 // 最高优先级
	case domain.ModelMappingScopeProvider:
		return 2
	default: // global
		return 3 // 最低优先级
	}
}

// sortCache 对缓存进行排序（按 scope 优先级、priority、id）
// 调用前必须持有写锁
func (r *ModelMappingRepository) sortCache() {
	sort.Slice(r.cache, func(i, j int) bool {
		// 先按 scope 优先级排序
		sp1, sp2 := scopePriority(r.cache[i].Scope), scopePriority(r.cache[j].Scope)
		if sp1 != sp2 {
			return sp1 < sp2
		}
		// 同 scope 下按 priority 排序
		if r.cache[i].Priority != r.cache[j].Priority {
			return r.cache[i].Priority < r.cache[j].Priority
		}
		// priority 相同按 id 排序
		return r.cache[i].ID < r.cache[j].ID
	})
}

func (r *ModelMappingRepository) Create(mapping *domain.ModelMapping) error {
	if err := r.repo.Create(mapping); err != nil {
		return err
	}
	// 直接添加到缓存并重新排序
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache = append(r.cache, mapping)
	r.sortCache()
	return nil
}

func (r *ModelMappingRepository) Update(mapping *domain.ModelMapping) error {
	if err := r.repo.Update(mapping); err != nil {
		return err
	}
	// 直接更新缓存中的数据
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, m := range r.cache {
		if m.ID == mapping.ID {
			r.cache[i] = mapping
			break
		}
	}
	r.sortCache() // 可能 priority 或 scope 变了，需要重新排序
	return nil
}

func (r *ModelMappingRepository) Delete(id uint64) error {
	if err := r.repo.Delete(id); err != nil {
		return err
	}
	// 直接从缓存中删除
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, m := range r.cache {
		if m.ID == id {
			r.cache = append(r.cache[:i], r.cache[i+1:]...)
			break
		}
	}
	return nil
}

func (r *ModelMappingRepository) GetByID(id uint64) (*domain.ModelMapping, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, m := range r.cache {
		if m.ID == id {
			return m, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *ModelMappingRepository) List() ([]*domain.ModelMapping, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*domain.ModelMapping, len(r.cache))
	copy(result, r.cache)
	return result, nil
}

func (r *ModelMappingRepository) ListEnabled() ([]*domain.ModelMapping, error) {
	return r.List()
}

func (r *ModelMappingRepository) ListByClientType(clientType domain.ClientType) ([]*domain.ModelMapping, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*domain.ModelMapping, 0)
	for _, m := range r.cache {
		if m.ClientType == "" || m.ClientType == clientType {
			result = append(result, m)
		}
	}
	return result, nil
}

// ListByQuery returns all mappings matching the query conditions
func (r *ModelMappingRepository) ListByQuery(query *domain.ModelMappingQuery) ([]*domain.ModelMapping, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*domain.ModelMapping, 0)
	for _, m := range r.cache {
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
	// 直接清空缓存
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache = make([]*domain.ModelMapping, 0)
	return nil
}

func (r *ModelMappingRepository) ClearAll() error {
	if err := r.repo.ClearAll(); err != nil {
		return err
	}
	// 直接清空缓存
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache = make([]*domain.ModelMapping, 0)
	return nil
}

func (r *ModelMappingRepository) SeedDefaults() error {
	if err := r.repo.SeedDefaults(); err != nil {
		return err
	}
	// 重新加载（因为 seed 会创建多条记录）
	return r.Load()
}
