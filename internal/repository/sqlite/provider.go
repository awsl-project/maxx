package sqlite

import (
	"errors"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"gorm.io/gorm"
)

type ProviderRepository struct {
	db *DB
}

func NewProviderRepository(db *DB) *ProviderRepository {
	return &ProviderRepository{db: db}
}

func (r *ProviderRepository) Create(p *domain.Provider) error {
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now

	model := r.toModel(p)
	if err := r.db.gorm.Create(model).Error; err != nil {
		return err
	}
	p.ID = model.ID
	return nil
}

func (r *ProviderRepository) Update(p *domain.Provider) error {
	p.UpdatedAt = time.Now()
	model := r.toModel(p)
	return r.db.gorm.Save(model).Error
}

func (r *ProviderRepository) Delete(tenantID uint64, id uint64) error {
	now := time.Now().UnixMilli()
	return tenantScope(r.db.gorm.Model(&Provider{}), tenantID).
		Where("id = ?", id).
		Updates(map[string]any{
			"deleted_at": now,
			"updated_at": now,
		}).Error
}

func (r *ProviderRepository) BulkDeleteWithReferences(tenantID uint64, rawIDs []uint64) (*domain.ProviderBulkDeleteResult, error) {
	result := &domain.ProviderBulkDeleteResult{}
	ids := uniqueNonZeroIDs(rawIDs)
	if len(ids) == 0 {
		return result, nil
	}

	err := r.db.gorm.Transaction(func(tx *gorm.DB) error {
		var models []Provider
		if err := tenantScope(tx, tenantID).
			Where("id IN ? AND deleted_at = 0", ids).
			Find(&models).Error; err != nil {
			return err
		}

		found := make(map[uint64]struct{}, len(models))
		deleteSet := make(map[uint64]struct{}, len(models))
		for _, model := range models {
			found[model.ID] = struct{}{}
			deleteSet[model.ID] = struct{}{}
		}

		deleteIDs := make([]uint64, 0, len(models))
		for _, id := range ids {
			if _, ok := found[id]; ok {
				deleteIDs = append(deleteIDs, id)
				result.DeletedIDs = append(result.DeletedIDs, id)
			} else {
				result.NotFoundIDs = append(result.NotFoundIDs, id)
			}
		}
		if len(deleteIDs) == 0 {
			return nil
		}

		if count, err := bulkSoftDeleteProviderRoutesTx(tx, tenantID, deleteSet); err != nil {
			return err
		} else {
			result.RouteDeletedCount = count
		}

		if count, err := bulkSoftDeleteProviderModelMappingsTx(tx, tenantID, deleteIDs); err != nil {
			return err
		} else {
			result.ModelMappingDeletedCount = count
		}

		now := time.Now().UnixMilli()
		res := tenantScope(tx.Model(&Provider{}), tenantID).
			Where("id IN ? AND deleted_at = 0", deleteIDs).
			Updates(map[string]any{
				"deleted_at": now,
				"updated_at": now,
			})
		if res.Error != nil {
			return res.Error
		}
		result.DeletedCount = int(res.RowsAffected)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func bulkSoftDeleteProviderRoutesTx(tx *gorm.DB, tenantID uint64, providerIDs map[uint64]struct{}) (int, error) {
	if len(providerIDs) == 0 {
		return 0, nil
	}
	ids := make([]uint64, 0, len(providerIDs))
	for id := range providerIDs {
		ids = append(ids, id)
	}

	now := time.Now().UnixMilli()
	res := tenantScope(tx.Model(&Route{}), tenantID).
		Where("provider_id IN ? AND deleted_at = 0", ids).
		Updates(map[string]any{
			"deleted_at": now,
			"updated_at": now,
		})
	if res.Error != nil {
		return 0, res.Error
	}
	return int(res.RowsAffected), nil
}

func bulkSoftDeleteProviderModelMappingsTx(tx *gorm.DB, tenantID uint64, providerIDs []uint64) (int, error) {
	if len(providerIDs) == 0 {
		return 0, nil
	}
	now := time.Now().UnixMilli()
	res := tenantScope(tx.Model(&ModelMapping{}), tenantID).
		Where("scope = ? AND provider_id IN ? AND deleted_at = 0", string(domain.ModelMappingScopeProvider), providerIDs).
		Updates(map[string]any{
			"deleted_at": now,
			"updated_at": now,
		})
	if res.Error != nil {
		return 0, res.Error
	}
	return int(res.RowsAffected), nil
}

func uniqueNonZeroIDs(rawIDs []uint64) []uint64 {
	seen := make(map[uint64]struct{}, len(rawIDs))
	ids := make([]uint64, 0, len(rawIDs))
	for _, id := range rawIDs {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func (r *ProviderRepository) GetByID(tenantID uint64, id uint64) (*domain.Provider, error) {
	var model Provider
	if err := tenantScope(r.db.gorm, tenantID).First(&model, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *ProviderRepository) List(tenantID uint64) ([]*domain.Provider, error) {
	var models []Provider
	if err := tenantScope(r.db.gorm, tenantID).Where("deleted_at = 0").Order("id").Find(&models).Error; err != nil {
		return nil, err
	}

	providers := make([]*domain.Provider, len(models))
	for i, m := range models {
		providers[i] = r.toDomain(&m)
	}
	return providers, nil
}

// toModel converts domain.Provider to sqlite.Provider
func (r *ProviderRepository) toModel(p *domain.Provider) *Provider {
	return &Provider{
		SoftDeleteModel: SoftDeleteModel{
			BaseModel: BaseModel{
				ID:        p.ID,
				CreatedAt: toTimestamp(p.CreatedAt),
				UpdatedAt: toTimestamp(p.UpdatedAt),
			},
			DeletedAt: toTimestampPtr(p.DeletedAt),
		},
		TenantID:             p.TenantID,
		Type:                 p.Type,
		Name:                 p.Name,
		Logo:                 LongText(p.Logo),
		Config:               LongText(toJSON(p.Config)),
		SupportedClientTypes: LongText(toJSON(p.SupportedClientTypes)),
		SupportModels:        LongText(toJSON(p.SupportModels)),
		ExcludeFromExport:    boolToInt(p.ExcludeFromExport),
		BlackBox:             boolToInt(p.BlackBox),
	}
}

// toDomain converts sqlite.Provider to domain.Provider
func (r *ProviderRepository) toDomain(m *Provider) *domain.Provider {
	return &domain.Provider{
		ID:                   m.ID,
		CreatedAt:            fromTimestamp(m.CreatedAt),
		UpdatedAt:            fromTimestamp(m.UpdatedAt),
		DeletedAt:            fromTimestampPtr(m.DeletedAt),
		TenantID:             m.TenantID,
		Type:                 m.Type,
		Name:                 m.Name,
		Logo:                 string(m.Logo),
		Config:               fromJSON[*domain.ProviderConfig](string(m.Config)),
		SupportedClientTypes: fromJSON[[]domain.ClientType](string(m.SupportedClientTypes)),
		SupportModels:        fromJSON[[]string](string(m.SupportModels)),
		ExcludeFromExport:    m.ExcludeFromExport != 0,
		BlackBox:             m.BlackBox != 0,
	}
}
