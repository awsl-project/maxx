package sqlite

import (
	"errors"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"gorm.io/gorm"
)

type InviteCodeRepository struct {
	db *DB
}

var nowFunc = time.Now

func NewInviteCodeRepository(db *DB) *InviteCodeRepository {
	return &InviteCodeRepository{db: db}
}

func (r *InviteCodeRepository) Create(code *domain.InviteCode) error {
	now := time.Now()
	code.CreatedAt = now
	code.UpdatedAt = now
	if code.Status == "" {
		code.Status = domain.InviteCodeStatusActive
	}

	model := r.toModel(code)
	if err := r.db.gorm.Create(model).Error; err != nil {
		return err
	}
	code.ID = model.ID
	return nil
}

func (r *InviteCodeRepository) Update(tenantID uint64, code *domain.InviteCode) error {
	code.UpdatedAt = nowFunc()
	result := tenantScope(r.db.gorm.Model(&InviteCode{}), tenantID).
		Where("id = ? AND deleted_at = 0", code.ID).
		Updates(map[string]any{
			"updated_at": toTimestamp(code.UpdatedAt),
			"status":     string(code.Status),
			"max_uses":   code.MaxUses,
			"expires_at": toTimestampPtr(code.ExpiresAt),
			"note":       LongText(code.Note),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var model InviteCode
		if err := tenantScope(r.db.gorm, tenantID).
			Where("id = ? AND deleted_at = 0", code.ID).
			First(&model).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrNotFound
			}
			return err
		}
		return nil
	}
	return nil
}

func (r *InviteCodeRepository) Delete(tenantID uint64, id uint64) error {
	now := time.Now().UnixMilli()
	return tenantScope(r.db.gorm.Model(&InviteCode{}), tenantID).
		Where("id = ?", id).
		Updates(map[string]any{
			"deleted_at": now,
			"updated_at": now,
		}).Error
}

func (r *InviteCodeRepository) GetByID(tenantID uint64, id uint64) (*domain.InviteCode, error) {
	var model InviteCode
	if err := tenantScope(r.db.gorm, tenantID).Where("deleted_at = 0").First(&model, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *InviteCodeRepository) GetByCodeHash(tenantID uint64, codeHash string) (*domain.InviteCode, error) {
	var model InviteCode
	if err := tenantScope(r.db.gorm, tenantID).
		Where("code_hash = ? AND deleted_at = 0", codeHash).
		First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *InviteCodeRepository) List(tenantID uint64) ([]*domain.InviteCode, error) {
	var models []InviteCode
	if err := tenantScope(r.db.gorm, tenantID).
		Where("deleted_at = 0").
		Order("created_at DESC").
		Find(&models).Error; err != nil {
		return nil, err
	}
	codes := make([]*domain.InviteCode, len(models))
	for i := range models {
		codes[i] = r.toDomain(&models[i])
	}
	return codes, nil
}

func (r *InviteCodeRepository) Consume(tenantID uint64, codeHash string, now time.Time) (*domain.InviteCode, error) {
	var result *domain.InviteCode
	err := r.db.gorm.Transaction(func(tx *gorm.DB) error {
		var model InviteCode
		if err := tenantScope(tx, tenantID).
			Where("code_hash = ? AND deleted_at = 0", codeHash).
			First(&model).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrInviteCodeInvalid
			}
			return err
		}

		if model.Status != string(domain.InviteCodeStatusActive) {
			return domain.ErrInviteCodeDisabled
		}
		if model.ExpiresAt > 0 && model.ExpiresAt <= toTimestamp(now) {
			return domain.ErrInviteCodeExpired
		}
		if model.MaxUses > 0 && model.UsedCount >= model.MaxUses {
			return domain.ErrInviteCodeExhausted
		}

		update := tenantScope(tx.Model(&InviteCode{}), tenantID).
			Where("id = ? AND deleted_at = 0 AND status = ?", model.ID, string(domain.InviteCodeStatusActive)).
			Where("(max_uses = 0 OR used_count < max_uses)").
			Where("(expires_at = 0 OR expires_at > ?)", toTimestamp(now)).
			Updates(map[string]any{
				"used_count": gorm.Expr("used_count + 1"),
				"updated_at": toTimestamp(now),
			})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected == 0 {
			// Re-check latest state for accurate error.
			var current InviteCode
			if err := tenantScope(tx, tenantID).
				Where("id = ? AND deleted_at = 0", model.ID).
				First(&current).Error; err == nil {
				if current.Status != string(domain.InviteCodeStatusActive) {
					return domain.ErrInviteCodeDisabled
				}
				if current.ExpiresAt > 0 && current.ExpiresAt <= toTimestamp(now) {
					return domain.ErrInviteCodeExpired
				}
				if current.MaxUses > 0 && current.UsedCount >= current.MaxUses {
					return domain.ErrInviteCodeExhausted
				}
			}
			return domain.ErrInviteCodeInvalid
		}

		model.UsedCount += 1
		model.UpdatedAt = toTimestamp(now)
		result = r.toDomain(&model)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *InviteCodeRepository) RollbackConsume(tenantID uint64, id uint64) error {
	now := time.Now().UnixMilli()
	return tenantScope(r.db.gorm.Model(&InviteCode{}), tenantID).
		Where("id = ? AND deleted_at = 0", id).
		Updates(map[string]any{
			"used_count": gorm.Expr("CASE WHEN used_count > 0 THEN used_count - 1 ELSE 0 END"),
			"updated_at": now,
		}).Error
}

func (r *InviteCodeRepository) toModel(code *domain.InviteCode) *InviteCode {
	status := string(code.Status)
	if status == "" {
		status = string(domain.InviteCodeStatusActive)
	}
	return &InviteCode{
		SoftDeleteModel: SoftDeleteModel{
			BaseModel: BaseModel{
				ID:        code.ID,
				CreatedAt: toTimestamp(code.CreatedAt),
				UpdatedAt: toTimestamp(code.UpdatedAt),
			},
			DeletedAt: toTimestampPtr(code.DeletedAt),
		},
		TenantID:        code.TenantID,
		CodeHash:        code.CodeHash,
		CodePrefix:      code.CodePrefix,
		Status:          status,
		MaxUses:         code.MaxUses,
		UsedCount:       code.UsedCount,
		ExpiresAt:       toTimestampPtr(code.ExpiresAt),
		CreatedByUserID: code.CreatedByUserID,
		Note:            LongText(code.Note),
	}
}

func (r *InviteCodeRepository) toDomain(model *InviteCode) *domain.InviteCode {
	status := domain.InviteCodeStatus(model.Status)
	if status != domain.InviteCodeStatusActive && status != domain.InviteCodeStatusDisabled {
		status = domain.InviteCodeStatusActive
	}
	return &domain.InviteCode{
		ID:              model.ID,
		CreatedAt:       fromTimestamp(model.CreatedAt),
		UpdatedAt:       fromTimestamp(model.UpdatedAt),
		DeletedAt:       fromTimestampPtr(model.DeletedAt),
		TenantID:        model.TenantID,
		CodeHash:        model.CodeHash,
		CodePrefix:      model.CodePrefix,
		Status:          status,
		MaxUses:         model.MaxUses,
		UsedCount:       model.UsedCount,
		ExpiresAt:       fromTimestampPtr(model.ExpiresAt),
		CreatedByUserID: model.CreatedByUserID,
		Note:            string(model.Note),
	}
}

type InviteCodeUsageRepository struct {
	db *DB
}

func NewInviteCodeUsageRepository(db *DB) *InviteCodeUsageRepository {
	return &InviteCodeUsageRepository{db: db}
}

func (r *InviteCodeUsageRepository) Create(usage *domain.InviteCodeUsage) error {
	now := time.Now()
	usage.CreatedAt = now
	if usage.UsedAt.IsZero() {
		usage.UsedAt = now
	}

	model := r.toUsageModel(usage)
	if err := r.db.gorm.Create(model).Error; err != nil {
		return err
	}
	usage.ID = model.ID
	return nil
}

func (r *InviteCodeUsageRepository) ListByCodeID(tenantID uint64, codeID uint64) ([]*domain.InviteCodeUsage, error) {
	var models []InviteCodeUsage
	if err := tenantScope(r.db.gorm, tenantID).
		Where("invite_code_id = ?", codeID).
		Order("used_at DESC").
		Find(&models).Error; err != nil {
		return nil, err
	}
	usages := make([]*domain.InviteCodeUsage, len(models))
	for i := range models {
		usages[i] = r.toUsageDomain(&models[i])
	}
	return usages, nil
}

func (r *InviteCodeUsageRepository) ListByUserID(tenantID uint64, userID uint64) ([]*domain.InviteCodeUsage, error) {
	var models []InviteCodeUsage
	if err := tenantScope(r.db.gorm, tenantID).
		Where("user_id = ?", userID).
		Order("used_at DESC").
		Find(&models).Error; err != nil {
		return nil, err
	}
	usages := make([]*domain.InviteCodeUsage, len(models))
	for i := range models {
		usages[i] = r.toUsageDomain(&models[i])
	}
	return usages, nil
}

func (r *InviteCodeUsageRepository) toUsageModel(usage *domain.InviteCodeUsage) *InviteCodeUsage {
	return &InviteCodeUsage{
		BaseModel: BaseModel{
			ID:        usage.ID,
			CreatedAt: toTimestamp(usage.CreatedAt),
			UpdatedAt: toTimestamp(usage.CreatedAt),
		},
		TenantID:     usage.TenantID,
		InviteCodeID: usage.InviteCodeID,
		UserID:       usage.UserID,
		Username:     usage.Username,
		UsedAt:       toTimestamp(usage.UsedAt),
		IP:           usage.IP,
		UserAgent:    usage.UserAgent,
		Result:       usage.Result,
		Reason:       usage.Reason,
	}
}

func (r *InviteCodeUsageRepository) toUsageDomain(model *InviteCodeUsage) *domain.InviteCodeUsage {
	return &domain.InviteCodeUsage{
		ID:          model.ID,
		CreatedAt:   fromTimestamp(model.CreatedAt),
		TenantID:    model.TenantID,
		InviteCodeID: model.InviteCodeID,
		UserID:      model.UserID,
		Username:    model.Username,
		UsedAt:      fromTimestamp(model.UsedAt),
		IP:          model.IP,
		UserAgent:   model.UserAgent,
		Result:      model.Result,
		Reason:      model.Reason,
	}
}
