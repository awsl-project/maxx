package sqlite

import (
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type FailureCountRepository struct {
	db *DB
}

func NewFailureCountRepository(db *DB) repository.FailureCountRepository {
	return &FailureCountRepository{db: db}
}

func (r *FailureCountRepository) Get(tenantID uint64, providerID uint64, clientType string, model string, reason string) (*domain.FailureCount, error) {
	var m FailureCount
	err := tenantScope(r.db.gorm, tenantID).Where("provider_id = ? AND client_type = ? AND model = ? AND reason = ?", providerID, clientType, model, reason).First(&m).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return r.toDomain(&m), nil
}

func (r *FailureCountRepository) GetAll(tenantID uint64) ([]*domain.FailureCount, error) {
	var models []FailureCount
	if err := tenantScope(r.db.gorm, tenantID).Find(&models).Error; err != nil {
		return nil, err
	}
	return r.toDomainList(models), nil
}

func (r *FailureCountRepository) Upsert(fc *domain.FailureCount) error {
	now := time.Now()
	m := &FailureCount{
		BaseModel: BaseModel{
			CreatedAt: toTimestamp(now),
			UpdatedAt: toTimestamp(now),
		},
		TenantID:      fc.TenantID,
		ProviderID:    fc.ProviderID,
		ClientType:    fc.ClientType,
		Model:         fc.Model,
		Reason:        fc.Reason,
		Count:         fc.Count,
		LastFailureAt: toTimestamp(fc.LastFailureAt),
	}

	err := r.db.gorm.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "tenant_id"}, {Name: "provider_id"}, {Name: "client_type"}, {Name: "model"}, {Name: "reason"}},
		DoUpdates: clause.Assignments(map[string]any{
			"count":           fc.Count,
			"last_failure_at": toTimestamp(fc.LastFailureAt),
			"updated_at":      toTimestamp(now),
		}),
	}).Create(m).Error

	if err != nil {
		return err
	}

	if fc.ID == 0 {
		fc.ID = m.ID
		fc.CreatedAt = now
	}
	fc.UpdatedAt = now
	return nil
}

func (r *FailureCountRepository) Delete(tenantID uint64, providerID uint64, clientType string, model string, reason string) error {
	return tenantScope(r.db.gorm, tenantID).Where("provider_id = ? AND client_type = ? AND model = ? AND reason = ?", providerID, clientType, model, reason).Delete(&FailureCount{}).Error
}

func (r *FailureCountRepository) DeleteAll(tenantID uint64, providerID uint64, clientType string, model string) error {
	// If clientType is empty, delete ALL failure counts for this provider
	if clientType == "" {
		return tenantScope(r.db.gorm, tenantID).Where("provider_id = ?", providerID).Delete(&FailureCount{}).Error
	}
	// If model is empty, delete all for this provider+clientType (all models)
	if model == "" {
		return tenantScope(r.db.gorm, tenantID).Where("provider_id = ? AND client_type = ?", providerID, clientType).Delete(&FailureCount{}).Error
	}
	// Otherwise, delete only for the specific provider+clientType+model
	return tenantScope(r.db.gorm, tenantID).Where("provider_id = ? AND client_type = ? AND model = ?", providerID, clientType, model).Delete(&FailureCount{}).Error
}

func (r *FailureCountRepository) DeleteExpired(olderThanSeconds int64) error {
	threshold := time.Now().Add(-time.Duration(olderThanSeconds) * time.Second).UnixMilli()
	return r.db.gorm.Where("last_failure_at < ?", threshold).Delete(&FailureCount{}).Error
}

func (r *FailureCountRepository) toDomain(m *FailureCount) *domain.FailureCount {
	return &domain.FailureCount{
		ID:            m.ID,
		CreatedAt:     fromTimestamp(m.CreatedAt),
		UpdatedAt:     fromTimestamp(m.UpdatedAt),
		TenantID:      m.TenantID,
		ProviderID:    m.ProviderID,
		ClientType:    m.ClientType,
		Model:         m.Model,
		Reason:        m.Reason,
		Count:         m.Count,
		LastFailureAt: fromTimestamp(m.LastFailureAt),
	}
}

func (r *FailureCountRepository) toDomainList(models []FailureCount) []*domain.FailureCount {
	counts := make([]*domain.FailureCount, len(models))
	for i, m := range models {
		counts[i] = r.toDomain(&m)
	}
	return counts
}
