package sqlite

import (
	"errors"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SystemSettingRepository struct {
	db *DB
}

func NewSystemSettingRepository(db *DB) *SystemSettingRepository {
	return &SystemSettingRepository{db: db}
}

func (r *SystemSettingRepository) Get(key string) (string, error) {
	var model SystemSetting
	if err := r.db.gorm.Where("key = ?", key).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return model.Value, nil
}

func (r *SystemSettingRepository) Set(key, value string) error {
	now := time.Now().UnixMilli()
	model := &SystemSetting{
		Key:       key,
		Value:     value,
		CreatedAt: now,
		UpdatedAt: now,
	}
	return r.db.gorm.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.Assignments(map[string]any{"value": value, "updated_at": now}),
	}).Create(model).Error
}

func (r *SystemSettingRepository) GetAll() ([]*domain.SystemSetting, error) {
	var models []SystemSetting
	if err := r.db.gorm.Order("key").Find(&models).Error; err != nil {
		return nil, err
	}

	settings := make([]*domain.SystemSetting, len(models))
	for i, m := range models {
		settings[i] = &domain.SystemSetting{
			Key:       m.Key,
			Value:     m.Value,
			CreatedAt: fromTimestamp(m.CreatedAt),
			UpdatedAt: fromTimestamp(m.UpdatedAt),
		}
	}
	return settings, nil
}

func (r *SystemSettingRepository) Delete(key string) error {
	return r.db.gorm.Where("key = ?", key).Delete(&SystemSetting{}).Error
}
