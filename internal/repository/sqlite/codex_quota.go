package sqlite

import (
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"gorm.io/gorm"
)

type CodexQuotaRepository struct {
	db *DB
}

func NewCodexQuotaRepository(d *DB) *CodexQuotaRepository {
	return &CodexQuotaRepository{db: d}
}

func (r *CodexQuotaRepository) Upsert(quota *domain.CodexQuota) error {
	now := time.Now()

	// Try to update first
	result := r.db.gorm.Model(&CodexQuota{}).
		Where("email = ? AND deleted_at = 0", quota.Email).
		Updates(map[string]any{
			"updated_at":          toTimestamp(now),
			"account_id":          quota.AccountID,
			"plan_type":           quota.PlanType,
			"is_forbidden":        quota.IsForbidden,
			"primary_window":      toJSON(quota.PrimaryWindow),
			"secondary_window":    toJSON(quota.SecondaryWindow),
			"code_review_window":  toJSON(quota.CodeReviewWindow),
		})

	if result.Error != nil {
		return result.Error
	}

	// If no rows updated, insert new record
	if result.RowsAffected == 0 {
		model := r.toModel(quota)
		model.CreatedAt = toTimestamp(now)
		model.UpdatedAt = toTimestamp(now)
		model.DeletedAt = 0

		if err := r.db.gorm.Create(model).Error; err != nil {
			return err
		}
		quota.ID = model.ID
		quota.CreatedAt = now
	}
	quota.UpdatedAt = now

	return nil
}

func (r *CodexQuotaRepository) GetByEmail(email string) (*domain.CodexQuota, error) {
	var model CodexQuota
	err := r.db.gorm.Where("email = ? AND deleted_at = 0", email).First(&model).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *CodexQuotaRepository) List() ([]*domain.CodexQuota, error) {
	var models []CodexQuota
	if err := r.db.gorm.Where("deleted_at = 0").Order("updated_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	return r.toDomainList(models), nil
}

func (r *CodexQuotaRepository) Delete(email string) error {
	now := time.Now().UnixMilli()
	return r.db.gorm.Model(&CodexQuota{}).
		Where("email = ?", email).
		Updates(map[string]any{
			"deleted_at": now,
			"updated_at": now,
		}).Error
}

func (r *CodexQuotaRepository) toModel(q *domain.CodexQuota) *CodexQuota {
	return &CodexQuota{
		SoftDeleteModel: SoftDeleteModel{
			BaseModel: BaseModel{
				ID:        q.ID,
				CreatedAt: toTimestamp(q.CreatedAt),
				UpdatedAt: toTimestamp(q.UpdatedAt),
			},
			DeletedAt: toTimestampPtr(q.DeletedAt),
		},
		Email:            q.Email,
		AccountID:        q.AccountID,
		PlanType:         q.PlanType,
		IsForbidden:      boolToInt(q.IsForbidden),
		PrimaryWindow:    LongText(toJSON(q.PrimaryWindow)),
		SecondaryWindow:  LongText(toJSON(q.SecondaryWindow)),
		CodeReviewWindow: LongText(toJSON(q.CodeReviewWindow)),
	}
}

func (r *CodexQuotaRepository) toDomain(m *CodexQuota) *domain.CodexQuota {
	return &domain.CodexQuota{
		ID:               m.ID,
		CreatedAt:        fromTimestamp(m.CreatedAt),
		UpdatedAt:        fromTimestamp(m.UpdatedAt),
		DeletedAt:        fromTimestampPtr(m.DeletedAt),
		Email:            m.Email,
		AccountID:        m.AccountID,
		PlanType:         m.PlanType,
		IsForbidden:      m.IsForbidden == 1,
		PrimaryWindow:    fromJSON[*domain.CodexQuotaWindow](string(m.PrimaryWindow)),
		SecondaryWindow:  fromJSON[*domain.CodexQuotaWindow](string(m.SecondaryWindow)),
		CodeReviewWindow: fromJSON[*domain.CodexQuotaWindow](string(m.CodeReviewWindow)),
	}
}

func (r *CodexQuotaRepository) toDomainList(models []CodexQuota) []*domain.CodexQuota {
	quotas := make([]*domain.CodexQuota, len(models))
	for i, m := range models {
		quotas[i] = r.toDomain(&m)
	}
	return quotas
}
