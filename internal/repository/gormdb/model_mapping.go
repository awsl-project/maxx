package gormdb

import (
	"errors"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"gorm.io/gorm"
)

type ModelMappingRepository struct {
	db *DB
}

func NewModelMappingRepository(db *DB) *ModelMappingRepository {
	return &ModelMappingRepository{db: db}
}

func (r *ModelMappingRepository) Create(mapping *domain.ModelMapping) error {
	now := time.Now()
	mapping.CreatedAt = now
	mapping.UpdatedAt = now

	model := r.toModel(mapping)
	if err := r.db.gorm.Create(model).Error; err != nil {
		return err
	}
	mapping.ID = model.ID
	return nil
}

func (r *ModelMappingRepository) Update(mapping *domain.ModelMapping) error {
	mapping.UpdatedAt = time.Now()
	model := r.toModel(mapping)
	return r.db.gorm.Save(model).Error
}

func (r *ModelMappingRepository) Delete(id uint64) error {
	now := time.Now().UnixMilli()
	return r.db.gorm.Model(&ModelMapping{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"deleted_at": now,
			"updated_at": now,
		}).Error
}

func (r *ModelMappingRepository) GetByID(id uint64) (*domain.ModelMapping, error) {
	var model ModelMapping
	if err := r.db.gorm.Where("id = ? AND deleted_at = 0", id).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *ModelMappingRepository) List() ([]*domain.ModelMapping, error) {
	var models []ModelMapping
	if err := r.db.gorm.Where("deleted_at = 0").Order("CASE scope WHEN 'route' THEN 1 WHEN 'provider' THEN 2 ELSE 3 END, priority, id").Find(&models).Error; err != nil {
		return nil, err
	}
	return r.toDomainList(models), nil
}

func (r *ModelMappingRepository) ListEnabled() ([]*domain.ModelMapping, error) {
	var models []ModelMapping
	if err := r.db.gorm.Where("deleted_at = 0").Order("CASE scope WHEN 'route' THEN 1 WHEN 'provider' THEN 2 ELSE 3 END, priority, id").Find(&models).Error; err != nil {
		return nil, err
	}
	return r.toDomainList(models), nil
}

func (r *ModelMappingRepository) ListByQuery(query *domain.ModelMappingQuery) ([]*domain.ModelMapping, error) {
	var models []ModelMapping
	err := r.db.gorm.Where(
		`deleted_at = 0
		AND (client_type = '' OR client_type = ?)
		AND (provider_type = '' OR provider_type = ?)
		AND (provider_id = 0 OR provider_id = ?)
		AND (project_id = 0 OR project_id = ?)
		AND (route_id = 0 OR route_id = ?)
		AND (api_token_id = 0 OR api_token_id = ?)`,
		query.ClientType, query.ProviderType, query.ProviderID, query.ProjectID, query.RouteID, query.APITokenID,
	).Order("CASE scope WHEN 'route' THEN 1 WHEN 'provider' THEN 2 ELSE 3 END, priority, id").Find(&models).Error
	if err != nil {
		return nil, err
	}
	return r.toDomainList(models), nil
}

func (r *ModelMappingRepository) ListByClientType(clientType domain.ClientType) ([]*domain.ModelMapping, error) {
	var models []ModelMapping
	if err := r.db.gorm.Where("deleted_at = 0 AND (client_type = '' OR client_type = ?)", clientType).Order("CASE scope WHEN 'route' THEN 1 WHEN 'provider' THEN 2 ELSE 3 END, priority, id").Find(&models).Error; err != nil {
		return nil, err
	}
	return r.toDomainList(models), nil
}

func (r *ModelMappingRepository) Count() (int, error) {
	var count int64
	err := r.db.gorm.Model(&ModelMapping{}).Where("deleted_at = 0").Count(&count).Error
	return int(count), err
}

func (r *ModelMappingRepository) DeleteAll() error {
	now := time.Now().UnixMilli()
	return r.db.gorm.Model(&ModelMapping{}).
		Where("deleted_at = 0").
		Updates(map[string]any{
			"deleted_at": now,
			"updated_at": now,
		}).Error
}

func (r *ModelMappingRepository) ClearAll() error {
	return r.db.gorm.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ModelMapping{}).Error
}

func (r *ModelMappingRepository) SeedDefaults() error {
	// Clear all existing mappings first
	if err := r.ClearAll(); err != nil {
		return err
	}

	defaultRules := []ModelMapping{
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "gpt-4o-mini*", Target: "gemini-2.5-flash", Priority: 0},
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "gpt-4o*", Target: "gemini-3-flash", Priority: 1},
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "gpt-4*", Target: "gemini-3-pro-high", Priority: 2},
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "gpt-3.5*", Target: "gemini-2.5-flash", Priority: 3},
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "o1-*", Target: "gemini-3-pro-high", Priority: 4},
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "o3-*", Target: "gemini-3-pro-high", Priority: 5},
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "claude-3-5-sonnet-*", Target: "claude-sonnet-4-5", Priority: 6},
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "claude-3-opus-*", Target: "claude-opus-4-5-thinking", Priority: 7},
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "claude-opus-4-*", Target: "claude-opus-4-5-thinking", Priority: 8},
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "claude-haiku-*", Target: "gemini-2.5-flash-lite", Priority: 9},
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "claude-3-haiku-*", Target: "gemini-2.5-flash-lite", Priority: 10},
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "*opus*", Target: "claude-opus-4-5-thinking", Priority: 11},
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "*sonnet*", Target: "claude-sonnet-4-5", Priority: 12},
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "*haiku*", Target: "gemini-2.5-flash-lite", Priority: 13},
	}

	return r.db.gorm.Create(&defaultRules).Error
}

func (r *ModelMappingRepository) toModel(mapping *domain.ModelMapping) *ModelMapping {
	scope := string(mapping.Scope)
	if scope == "" {
		scope = "global"
	}
	return &ModelMapping{
		SoftDeleteModel: SoftDeleteModel{
			BaseModel: BaseModel{
				ID:        mapping.ID,
				CreatedAt: toTimestamp(mapping.CreatedAt),
				UpdatedAt: toTimestamp(mapping.UpdatedAt),
			},
			DeletedAt: toTimestampPtr(mapping.DeletedAt),
		},
		Scope:        scope,
		ClientType:   string(mapping.ClientType),
		ProviderType: mapping.ProviderType,
		ProviderID:   mapping.ProviderID,
		ProjectID:    mapping.ProjectID,
		RouteID:      mapping.RouteID,
		APITokenID:   mapping.APITokenID,
		Pattern:      mapping.Pattern,
		Target:       mapping.Target,
		Priority:     mapping.Priority,
	}
}

func (r *ModelMappingRepository) toDomain(m *ModelMapping) *domain.ModelMapping {
	scope := domain.ModelMappingScope(m.Scope)
	if scope == "" {
		scope = domain.ModelMappingScopeGlobal
	}
	return &domain.ModelMapping{
		ID:           m.ID,
		CreatedAt:    fromTimestamp(m.CreatedAt),
		UpdatedAt:    fromTimestamp(m.UpdatedAt),
		DeletedAt:    fromTimestampPtr(m.DeletedAt),
		Scope:        scope,
		ClientType:   domain.ClientType(m.ClientType),
		ProviderType: m.ProviderType,
		ProviderID:   m.ProviderID,
		ProjectID:    m.ProjectID,
		RouteID:      m.RouteID,
		APITokenID:   m.APITokenID,
		Pattern:      m.Pattern,
		Target:       m.Target,
		Priority:     m.Priority,
	}
}

func (r *ModelMappingRepository) toDomainList(models []ModelMapping) []*domain.ModelMapping {
	mappings := make([]*domain.ModelMapping, len(models))
	for i, m := range models {
		mappings[i] = r.toDomain(&m)
	}
	return mappings
}
