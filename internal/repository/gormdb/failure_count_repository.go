package gormdb

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

func (r *FailureCountRepository) Get(providerID uint64, clientType string, reason string) (*domain.FailureCount, error) {
	var model FailureCount
	err := r.db.gorm.Where("provider_id = ? AND client_type = ? AND reason = ?", providerID, clientType, reason).First(&model).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *FailureCountRepository) GetAll() ([]*domain.FailureCount, error) {
	var models []FailureCount
	if err := r.db.gorm.Find(&models).Error; err != nil {
		return nil, err
	}
	return r.toDomainList(models), nil
}

func (r *FailureCountRepository) Upsert(fc *domain.FailureCount) error {
	now := time.Now()
	model := &FailureCount{
		BaseModel: BaseModel{
			CreatedAt: toTimestamp(now),
			UpdatedAt: toTimestamp(now),
		},
		ProviderID:    fc.ProviderID,
		ClientType:    fc.ClientType,
		Reason:        fc.Reason,
		Count:         fc.Count,
		LastFailureAt: toTimestamp(fc.LastFailureAt),
	}

	err := r.db.gorm.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "provider_id"}, {Name: "client_type"}, {Name: "reason"}},
		DoUpdates: clause.Assignments(map[string]any{
			"count":           fc.Count,
			"last_failure_at": toTimestamp(fc.LastFailureAt),
			"updated_at":      toTimestamp(now),
		}),
	}).Create(model).Error

	if err != nil {
		return err
	}

	if fc.ID == 0 {
		fc.ID = model.ID
		fc.CreatedAt = now
	}
	fc.UpdatedAt = now
	return nil
}

func (r *FailureCountRepository) Delete(providerID uint64, clientType string, reason string) error {
	return r.db.gorm.Where("provider_id = ? AND client_type = ? AND reason = ?", providerID, clientType, reason).Delete(&FailureCount{}).Error
}

func (r *FailureCountRepository) DeleteAll(providerID uint64, clientType string) error {
	return r.db.gorm.Where("provider_id = ? AND client_type = ?", providerID, clientType).Delete(&FailureCount{}).Error
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
		ProviderID:    m.ProviderID,
		ClientType:    m.ClientType,
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
