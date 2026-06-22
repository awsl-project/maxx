package sqlite

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

func (r *RouteRepository) Delete(tenantID uint64, id uint64) error {
	now := time.Now().UnixMilli()
	return tenantScope(r.db.gorm.Model(&Route{}), tenantID).
		Where("id = ?", id).
		Updates(map[string]any{
			"deleted_at": now,
			"updated_at": now,
		}).Error
}

func (r *RouteRepository) BulkDelete(tenantID uint64, req domain.RouteBulkDeleteRequest) (*domain.RouteBulkDeleteResult, error) {
	result := &domain.RouteBulkDeleteResult{}
	if len(req.IDs) == 0 {
		return result, nil
	}

	seen := make(map[uint64]struct{}, len(req.IDs))
	ids := make([]uint64, 0, len(req.IDs))
	for _, id := range req.IDs {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return result, nil
	}

	err := r.db.gorm.Transaction(func(tx *gorm.DB) error {
		var models []Route
		if err := tenantScope(tx, tenantID).
			Where("id IN ? AND deleted_at = 0", ids).
			Find(&models).Error; err != nil {
			return err
		}

		found := make(map[uint64]struct{}, len(models))
		deleteIDs := make([]uint64, 0, len(models))
		for _, model := range models {
			found[model.ID] = struct{}{}
			if model.ClientType == string(req.ClientType) && model.ProjectID == req.ProjectID {
				deleteIDs = append(deleteIDs, model.ID)
			} else {
				result.SkippedIDs = append(result.SkippedIDs, model.ID)
			}
		}

		for _, id := range ids {
			if _, ok := found[id]; !ok {
				result.NotFoundIDs = append(result.NotFoundIDs, id)
			}
		}

		if len(deleteIDs) == 0 {
			return nil
		}

		now := time.Now().UnixMilli()
		if err := tenantScope(tx.Model(&Route{}), tenantID).
			Where("id IN ? AND deleted_at = 0", deleteIDs).
			Updates(map[string]any{
				"deleted_at": now,
				"updated_at": now,
			}).Error; err != nil {
			return err
		}

		result.DeletedIDs = deleteIDs
		result.DeletedCount = len(deleteIDs)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (r *RouteRepository) BatchUpdatePositions(tenantID uint64, updates []domain.RoutePositionUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	return r.db.gorm.Transaction(func(tx *gorm.DB) error {
		now := time.Now().UnixMilli()
		for _, update := range updates {
			if err := tenantScope(tx.Model(&Route{}), tenantID).
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

func (r *RouteRepository) GetByID(tenantID uint64, id uint64) (*domain.Route, error) {
	var model Route
	if err := tenantScope(r.db.gorm, tenantID).First(&model, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *RouteRepository) FindByKey(tenantID uint64, projectID, providerID uint64, clientType domain.ClientType) (*domain.Route, error) {
	var model Route
	if err := tenantScope(r.db.gorm, tenantID).Where("project_id = ? AND provider_id = ? AND client_type = ? AND deleted_at = 0", projectID, providerID, clientType).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *RouteRepository) List(tenantID uint64) ([]*domain.Route, error) {
	var models []Route
	if err := tenantScope(r.db.gorm, tenantID).Where("deleted_at = 0").Order("position").Find(&models).Error; err != nil {
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
		TenantID:      route.TenantID,
		IsEnabled:     isEnabled,
		IsNative:      isNative,
		ProjectID:     route.ProjectID,
		ClientType:    string(route.ClientType),
		ProviderID:    route.ProviderID,
		Position:      route.Position,
		Weight:        route.Weight,
		RetryConfigID: route.RetryConfigID,
	}
}

func (r *RouteRepository) toDomain(m *Route) *domain.Route {
	weight := m.Weight
	if weight <= 0 {
		weight = 1
	}
	return &domain.Route{
		ID:            m.ID,
		CreatedAt:     fromTimestamp(m.CreatedAt),
		UpdatedAt:     fromTimestamp(m.UpdatedAt),
		DeletedAt:     fromTimestampPtr(m.DeletedAt),
		TenantID:      m.TenantID,
		IsEnabled:     m.IsEnabled == 1,
		IsNative:      m.IsNative == 1,
		ProjectID:     m.ProjectID,
		ClientType:    domain.ClientType(m.ClientType),
		ProviderID:    m.ProviderID,
		Position:      m.Position,
		Weight:        weight,
		RetryConfigID: m.RetryConfigID,
	}
}
