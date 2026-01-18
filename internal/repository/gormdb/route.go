package gormdb

import (
	"errors"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"gorm.io/gorm"
)

type RouteRepository struct {
	db *DB
}

func NewRouteRepository(db *DB) *RouteRepository {
	return &RouteRepository{db: db}
}

func (r *RouteRepository) Create(route *domain.Route) error {
	now := time.Now()
	route.CreatedAt = now
	route.UpdatedAt = now

	model := r.toModel(route)
	if err := r.db.gorm.Create(model).Error; err != nil {
		return err
	}
	route.ID = model.ID
	return nil
}

func (r *RouteRepository) Update(route *domain.Route) error {
	route.UpdatedAt = time.Now()
	model := r.toModel(route)
	return r.db.gorm.Save(model).Error
}

func (r *RouteRepository) Delete(id uint64) error {
	now := time.Now().UnixMilli()
	return r.db.gorm.Model(&Route{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"deleted_at": now,
			"updated_at": now,
		}).Error
}

func (r *RouteRepository) BatchUpdatePositions(updates []domain.RoutePositionUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	return r.db.gorm.Transaction(func(tx *gorm.DB) error {
		now := time.Now().UnixMilli()
		for _, update := range updates {
			if err := tx.Model(&Route{}).
				Where("id = ?", update.ID).
				Updates(map[string]any{
					"position":   update.Position,
					"updated_at": now,
				}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *RouteRepository) GetByID(id uint64) (*domain.Route, error) {
	var model Route
	if err := r.db.gorm.First(&model, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *RouteRepository) FindByKey(projectID, providerID uint64, clientType domain.ClientType) (*domain.Route, error) {
	var model Route
	if err := r.db.gorm.Where("project_id = ? AND provider_id = ? AND client_type = ? AND deleted_at = 0", projectID, providerID, clientType).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *RouteRepository) List() ([]*domain.Route, error) {
	var models []Route
	if err := r.db.gorm.Where("deleted_at = 0").Order("position").Find(&models).Error; err != nil {
		return nil, err
	}

	routes := make([]*domain.Route, len(models))
	for i, m := range models {
		routes[i] = r.toDomain(&m)
	}
	return routes, nil
}

func (r *RouteRepository) toModel(route *domain.Route) *Route {
	isEnabled := 0
	if route.IsEnabled {
		isEnabled = 1
	}
	isNative := 0
	if route.IsNative {
		isNative = 1
	}
	return &Route{
		SoftDeleteModel: SoftDeleteModel{
			BaseModel: BaseModel{
				ID:        route.ID,
				CreatedAt: toTimestamp(route.CreatedAt),
				UpdatedAt: toTimestamp(route.UpdatedAt),
			},
			DeletedAt: toTimestampPtr(route.DeletedAt),
		},
		IsEnabled:     isEnabled,
		IsNative:      isNative,
		ProjectID:     route.ProjectID,
		ClientType:    string(route.ClientType),
		ProviderID:    route.ProviderID,
		Position:      route.Position,
		RetryConfigID: route.RetryConfigID,
	}
}

func (r *RouteRepository) toDomain(m *Route) *domain.Route {
	return &domain.Route{
		ID:            m.ID,
		CreatedAt:     fromTimestamp(m.CreatedAt),
		UpdatedAt:     fromTimestamp(m.UpdatedAt),
		DeletedAt:     fromTimestampPtr(m.DeletedAt),
		IsEnabled:     m.IsEnabled == 1,
		IsNative:      m.IsNative == 1,
		ProjectID:     m.ProjectID,
		ClientType:    domain.ClientType(m.ClientType),
		ProviderID:    m.ProviderID,
		Position:      m.Position,
		RetryConfigID: m.RetryConfigID,
	}
}
