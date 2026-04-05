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
	identityKey := domain.CodexQuotaIdentityKey(quota.Email, quota.AccountID)
	quota.IdentityKey = identityKey

	query := tenantScope(r.db.gorm.Model(&CodexQuota{}), quota.TenantID)
	if identityKey != "" {
		query = query.Where("identity_key = ? AND deleted_at = 0", identityKey)
	} else {
		query = query.Where("email = ? AND deleted_at = 0", quota.Email)
	}

	result := query.Updates(map[string]any{
		"updated_at":         toTimestamp(now),
		"identity_key":       identityKey,
		"email":              quota.Email,
		"account_id":         quota.AccountID,
		"plan_type":          quota.PlanType,
		"is_forbidden":       quota.IsForbidden,
		"primary_window":     toJSON(quota.PrimaryWindow),
		"secondary_window":   toJSON(quota.SecondaryWindow),
		"code_review_window": toJSON(quota.CodeReviewWindow),
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

func (r *CodexQuotaRepository) GetByIdentityKey(tenantID uint64, identityKey string) (*domain.CodexQuota, error) {
	if identityKey == "" {
		return nil, nil
	}
	var model CodexQuota
	err := tenantScope(r.db.gorm, tenantID).Where("identity_key = ? AND deleted_at = 0", identityKey).First(&model).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *CodexQuotaRepository) GetByEmail(tenantID uint64, email string) (*domain.CodexQuota, error) {
	var model CodexQuota
	err := tenantScope(r.db.gorm, tenantID).Where("email = ? AND deleted_at = 0", email).First(&model).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *CodexQuotaRepository) List(tenantID uint64) ([]*domain.CodexQuota, error) {
	var models []CodexQuota
	if err := tenantScope(r.db.gorm, tenantID).Where("deleted_at = 0").Order("updated_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	return r.toDomainList(models), nil
}

func (r *CodexQuotaRepository) Delete(tenantID uint64, email string) error {
	now := time.Now().UnixMilli()
	return tenantScope(r.db.gorm.Model(&CodexQuota{}), tenantID).
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
		TenantID:         q.TenantID,
		IdentityKey:      q.IdentityKey,
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
		TenantID:         m.TenantID,
		IdentityKey:      m.IdentityKey,
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
