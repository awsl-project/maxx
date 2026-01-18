package gormdb

import (
	"errors"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"gorm.io/gorm"
)

type RetryConfigRepository struct {
	db *DB
}

func NewRetryConfigRepository(db *DB) *RetryConfigRepository {
	return &RetryConfigRepository{db: db}
}

func (r *RetryConfigRepository) Create(c *domain.RetryConfig) error {
	now := time.Now()
	c.CreatedAt = now
	c.UpdatedAt = now

	model := r.toModel(c)
	if err := r.db.gorm.Create(model).Error; err != nil {
		return err
	}
	c.ID = model.ID
	return nil
}

func (r *RetryConfigRepository) Update(c *domain.RetryConfig) error {
	c.UpdatedAt = time.Now()
	model := r.toModel(c)
	return r.db.gorm.Save(model).Error
}

func (r *RetryConfigRepository) Delete(id uint64) error {
	now := time.Now().UnixMilli()
	return r.db.gorm.Model(&RetryConfig{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"deleted_at": now,
			"updated_at": now,
		}).Error
}

func (r *RetryConfigRepository) GetByID(id uint64) (*domain.RetryConfig, error) {
	var model RetryConfig
	if err := r.db.gorm.First(&model, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *RetryConfigRepository) GetDefault() (*domain.RetryConfig, error) {
	var model RetryConfig
	if err := r.db.gorm.Where("is_default = 1 AND deleted_at = 0").First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *RetryConfigRepository) List() ([]*domain.RetryConfig, error) {
	var models []RetryConfig
	if err := r.db.gorm.Where("deleted_at = 0").Order("id").Find(&models).Error; err != nil {
		return nil, err
	}
	return r.toDomainList(models), nil
}

func (r *RetryConfigRepository) toModel(c *domain.RetryConfig) *RetryConfig {
	isDefault := 0
	if c.IsDefault {
		isDefault = 1
	}
	return &RetryConfig{
		SoftDeleteModel: SoftDeleteModel{
			BaseModel: BaseModel{
				ID:        c.ID,
				CreatedAt: toTimestamp(c.CreatedAt),
				UpdatedAt: toTimestamp(c.UpdatedAt),
			},
			DeletedAt: toTimestampPtr(c.DeletedAt),
		},
		Name:              c.Name,
		IsDefault:         isDefault,
		MaxRetries:        c.MaxRetries,
		InitialIntervalMs: int(c.InitialInterval.Milliseconds()),
		BackoffRate:       c.BackoffRate,
		MaxIntervalMs:     int(c.MaxInterval.Milliseconds()),
	}
}

func (r *RetryConfigRepository) toDomain(m *RetryConfig) *domain.RetryConfig {
	return &domain.RetryConfig{
		ID:              m.ID,
		CreatedAt:       fromTimestamp(m.CreatedAt),
		UpdatedAt:       fromTimestamp(m.UpdatedAt),
		DeletedAt:       fromTimestampPtr(m.DeletedAt),
		Name:            m.Name,
		IsDefault:       m.IsDefault == 1,
		MaxRetries:      m.MaxRetries,
		InitialInterval: time.Duration(m.InitialIntervalMs) * time.Millisecond,
		BackoffRate:     m.BackoffRate,
		MaxInterval:     time.Duration(m.MaxIntervalMs) * time.Millisecond,
	}
}

func (r *RetryConfigRepository) toDomainList(models []RetryConfig) []*domain.RetryConfig {
	configs := make([]*domain.RetryConfig, len(models))
	for i, m := range models {
		configs[i] = r.toDomain(&m)
	}
	return configs
}
