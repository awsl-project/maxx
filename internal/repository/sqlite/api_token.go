package sqlite

import (
	"errors"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"gorm.io/gorm"
)

type APITokenRepository struct {
	db *DB
}

func NewAPITokenRepository(db *DB) *APITokenRepository {
	return &APITokenRepository{db: db}
}

func (r *APITokenRepository) Create(t *domain.APIToken) error {
	now := time.Now()
	t.CreatedAt = now
	t.UpdatedAt = now

	model := r.toModel(t)
	if err := r.db.gorm.Create(model).Error; err != nil {
		return err
	}
	t.ID = model.ID
	return nil
}

func (r *APITokenRepository) Update(t *domain.APIToken) error {
	t.UpdatedAt = time.Now()
	return r.db.gorm.Model(&APIToken{}).
		Where("id = ?", t.ID).
		Updates(map[string]any{
			"updated_at":  toTimestamp(t.UpdatedAt),
			"name":        t.Name,
			"description": t.Description,
			"project_id":  t.ProjectID,
			"is_enabled":  boolToInt(t.IsEnabled),
			"expires_at":  toTimestampPtr(t.ExpiresAt),
		}).Error
}

func (r *APITokenRepository) Delete(id uint64) error {
	now := time.Now().UnixMilli()
	return r.db.gorm.Model(&APIToken{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"deleted_at": now,
			"updated_at": now,
		}).Error
}

func (r *APITokenRepository) GetByID(id uint64) (*domain.APIToken, error) {
	var model APIToken
	if err := r.db.gorm.First(&model, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *APITokenRepository) GetByToken(token string) (*domain.APIToken, error) {
	var model APIToken
	if err := r.db.gorm.Where("token = ? AND deleted_at = 0", token).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *APITokenRepository) List() ([]*domain.APIToken, error) {
	var models []APIToken
	if err := r.db.gorm.Where("deleted_at = 0").Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}

	tokens := make([]*domain.APIToken, len(models))
	for i, m := range models {
		tokens[i] = r.toDomain(&m)
	}
	return tokens, nil
}

func (r *APITokenRepository) IncrementUseCount(id uint64) error {
	now := time.Now().UnixMilli()
	return r.db.gorm.Model(&APIToken{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"use_count":    gorm.Expr("use_count + 1"),
			"last_used_at": now,
			"updated_at":   now,
		}).Error
}

func (r *APITokenRepository) toModel(t *domain.APIToken) *APIToken {
	return &APIToken{
		SoftDeleteModel: SoftDeleteModel{
			BaseModel: BaseModel{
				ID:        t.ID,
				CreatedAt: toTimestamp(t.CreatedAt),
				UpdatedAt: toTimestamp(t.UpdatedAt),
			},
			DeletedAt: toTimestampPtr(t.DeletedAt),
		},
		Token:       t.Token,
		TokenPrefix: t.TokenPrefix,
		Name:        t.Name,
		Description: t.Description,
		ProjectID:   t.ProjectID,
		IsEnabled:   boolToInt(t.IsEnabled),
		ExpiresAt:   toTimestampPtr(t.ExpiresAt),
		LastUsedAt:  toTimestampPtr(t.LastUsedAt),
		UseCount:    t.UseCount,
	}
}

func (r *APITokenRepository) toDomain(m *APIToken) *domain.APIToken {
	return &domain.APIToken{
		ID:          m.ID,
		CreatedAt:   fromTimestamp(m.CreatedAt),
		UpdatedAt:   fromTimestamp(m.UpdatedAt),
		DeletedAt:   fromTimestampPtr(m.DeletedAt),
		Token:       m.Token,
		TokenPrefix: m.TokenPrefix,
		Name:        m.Name,
		Description: m.Description,
		ProjectID:   m.ProjectID,
		IsEnabled:   m.IsEnabled == 1,
		ExpiresAt:   fromTimestampPtr(m.ExpiresAt),
		LastUsedAt:  fromTimestampPtr(m.LastUsedAt),
		UseCount:    m.UseCount,
	}
}

// boolToInt converts bool to int
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
