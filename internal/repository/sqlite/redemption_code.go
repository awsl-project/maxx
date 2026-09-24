package sqlite

import (
	"errors"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"gorm.io/gorm"
)

type RedemptionCodeRepository struct {
	db      *DB
	nowFunc func() time.Time
}

func NewRedemptionCodeRepository(db *DB) *RedemptionCodeRepository {
	return &RedemptionCodeRepository{db: db, nowFunc: time.Now}
}

func (r *RedemptionCodeRepository) SetNowFunc(fn func() time.Time) {
	if fn == nil {
		r.nowFunc = time.Now
		return
	}
	r.nowFunc = fn
}

func (r *RedemptionCodeRepository) Create(code *domain.RedemptionCode) error {
	if r.nowFunc == nil {
		r.nowFunc = time.Now
	}
	now := r.nowFunc()
	code.CreatedAt = now
	code.UpdatedAt = now
	if code.Status == "" {
		code.Status = domain.RedemptionCodeStatusActive
	}
	model := r.toModel(code)
	if err := r.db.gorm.Create(model).Error; err != nil {
		return err
	}
	code.ID = model.ID
	return nil
}

func (r *RedemptionCodeRepository) CreateWithAPITokenDebit(tenantID uint64, apiTokenID uint64, amount uint64, codes []*domain.RedemptionCode) error {
	if r.nowFunc == nil {
		r.nowFunc = time.Now
	}
	if tenantID == 0 || apiTokenID == 0 || amount == 0 || len(codes) == 0 {
		return domain.ErrInvalidInput
	}
	now := r.nowFunc()
	return r.db.gorm.Transaction(func(tx *gorm.DB) error {
		debit := tenantScope(tx.Model(&APIToken{}), tenantID).
			Where("id = ? AND deleted_at = 0", apiTokenID).
			Where("quota_balance >= ?", amount).
			Update("quota_balance", gorm.Expr("quota_balance - ?", amount))
		if debit.Error != nil {
			return debit.Error
		}
		if debit.RowsAffected != 1 {
			return domain.ErrAPITokenQuotaExhausted
		}

		models := make([]*RedemptionCode, 0, len(codes))
		for _, code := range codes {
			if code == nil || code.Amount == 0 || code.CodeHash == "" || code.CodePrefix == "" {
				return domain.ErrInvalidInput
			}
			code.TenantID = tenantID
			code.Status = domain.RedemptionCodeStatusActive
			code.CreatedAt = now
			code.UpdatedAt = now
			models = append(models, r.toModel(code))
		}
		if err := tx.Create(&models).Error; err != nil {
			return err
		}
		for i, model := range models {
			codes[i].ID = model.ID
		}
		return nil
	})
}

func (r *RedemptionCodeRepository) Update(tenantID uint64, code *domain.RedemptionCode) error {
	if r.nowFunc == nil {
		r.nowFunc = time.Now
	}
	code.UpdatedAt = r.nowFunc()
	result := tenantScope(r.db.gorm.Model(&RedemptionCode{}), tenantID).
		Where("id = ? AND deleted_at = 0", code.ID).
		Where("used_at = 0").
		Updates(map[string]any{
			"updated_at": toTimestamp(code.UpdatedAt),
			"status":     string(code.Status),
			"amount":     code.Amount,
			"note":       LongText(code.Note),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var model RedemptionCode
		if err := tenantScope(r.db.gorm, tenantID).
			Where("id = ? AND deleted_at = 0", code.ID).
			First(&model).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}
		if model.UsedAt > 0 {
			return domain.ErrInvalidState
		}
	}
	return nil
}

func (r *RedemptionCodeRepository) Delete(tenantID uint64, id uint64) error {
	if r.nowFunc == nil {
		r.nowFunc = time.Now
	}
	now := r.nowFunc().UnixMilli()
	result := tenantScope(r.db.gorm.Model(&RedemptionCode{}), tenantID).
		Where("id = ? AND deleted_at = 0", id).
		Updates(map[string]any{"deleted_at": now, "updated_at": now})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *RedemptionCodeRepository) GetByID(tenantID uint64, id uint64) (*domain.RedemptionCode, error) {
	var model RedemptionCode
	if err := tenantScope(r.db.gorm, tenantID).Where("deleted_at = 0").First(&model, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *RedemptionCodeRepository) List(tenantID uint64) ([]*domain.RedemptionCode, error) {
	var models []RedemptionCode
	if err := tenantScope(r.db.gorm, tenantID).
		Where("deleted_at = 0").
		Order("created_at DESC").
		Find(&models).Error; err != nil {
		return nil, err
	}
	codes := make([]*domain.RedemptionCode, len(models))
	for i := range models {
		codes[i] = r.toDomain(&models[i])
	}
	return codes, nil
}

func (r *RedemptionCodeRepository) Redeem(tenantID uint64, codeHash string, userID uint64, apiTokenID uint64, now time.Time) (*domain.RedemptionCode, error) {
	var result *domain.RedemptionCode
	err := r.db.gorm.Transaction(func(tx *gorm.DB) error {
		update := tenantScope(tx.Model(&RedemptionCode{}), tenantID).
			Where("code_hash = ? AND deleted_at = 0", codeHash).
			Where("status = ?", string(domain.RedemptionCodeStatusActive)).
			Where("used_at = 0").
			Updates(map[string]any{
				"used_by_user_id": userID,
				"used_at":         toTimestamp(now),
				"updated_at":      toTimestamp(now),
			})
		if update.Error != nil {
			return update.Error
		}

		var model RedemptionCode
		if err := tenantScope(tx, tenantID).
			Where("code_hash = ? AND deleted_at = 0", codeHash).
			First(&model).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrRedemptionCodeInvalid
			}
			return err
		}

		if update.RowsAffected == 0 {
			if model.Status != string(domain.RedemptionCodeStatusActive) {
				return domain.ErrRedemptionCodeDisabled
			}
			if model.UsedAt > 0 {
				return domain.ErrRedemptionCodeUsed
			}
			return domain.ErrRedemptionCodeInvalid
		}

		apiUpdate := tenantScope(tx.Model(&APIToken{}), tenantID).
			Where("id = ? AND deleted_at = 0", apiTokenID).
			Update("quota_balance", gorm.Expr("quota_balance + ?", model.Amount))
		if apiUpdate.Error != nil {
			return apiUpdate.Error
		}
		if apiUpdate.RowsAffected != 1 {
			return domain.ErrNotFound
		}
		result = r.toDomain(&model)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *RedemptionCodeRepository) toModel(code *domain.RedemptionCode) *RedemptionCode {
	if code == nil {
		return nil
	}
	return &RedemptionCode{
		SoftDeleteModel: SoftDeleteModel{
			BaseModel: BaseModel{ID: code.ID, CreatedAt: toTimestamp(code.CreatedAt), UpdatedAt: toTimestamp(code.UpdatedAt)},
			DeletedAt: toTimestampPtr(code.DeletedAt),
		},
		TenantID:        code.TenantID,
		CodeHash:        code.CodeHash,
		CodePrefix:      code.CodePrefix,
		Status:          string(code.Status),
		Amount:          code.Amount,
		UsedByUserID:    code.UsedByUserID,
		UsedAt:          toTimestampPtr(code.UsedAt),
		CreatedByUserID: code.CreatedByUserID,
		Note:            LongText(code.Note),
	}
}

func (r *RedemptionCodeRepository) toDomain(model *RedemptionCode) *domain.RedemptionCode {
	if model == nil {
		return nil
	}
	return &domain.RedemptionCode{
		ID:              model.ID,
		CreatedAt:       fromTimestamp(model.CreatedAt),
		UpdatedAt:       fromTimestamp(model.UpdatedAt),
		DeletedAt:       fromTimestampPtr(model.DeletedAt),
		TenantID:        model.TenantID,
		CodeHash:        model.CodeHash,
		CodePrefix:      model.CodePrefix,
		Status:          domain.RedemptionCodeStatus(model.Status),
		Amount:          model.Amount,
		UsedByUserID:    model.UsedByUserID,
		UsedAt:          fromTimestampPtr(model.UsedAt),
		CreatedByUserID: model.CreatedByUserID,
		Note:            string(model.Note),
	}
}
