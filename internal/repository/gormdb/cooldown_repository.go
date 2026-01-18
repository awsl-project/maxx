package gormdb

import (
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CooldownRepository struct {
	db *DB
}

func NewCooldownRepository(db *DB) repository.CooldownRepository {
	return &CooldownRepository{db: db}
}

func (r *CooldownRepository) GetAll() ([]*domain.Cooldown, error) {
	now := time.Now().UnixMilli()
	var models []Cooldown
	if err := r.db.gorm.Where("until_time > ?", now).Find(&models).Error; err != nil {
		return nil, err
	}
	return r.toDomainList(models), nil
}

func (r *CooldownRepository) GetByProvider(providerID uint64) ([]*domain.Cooldown, error) {
	now := time.Now().UnixMilli()
	var models []Cooldown
	if err := r.db.gorm.Where("provider_id = ? AND until_time > ?", providerID, now).Find(&models).Error; err != nil {
		return nil, err
	}
	return r.toDomainList(models), nil
}

func (r *CooldownRepository) Get(providerID uint64, clientType string) (*domain.Cooldown, error) {
	now := time.Now().UnixMilli()
	var model Cooldown
	err := r.db.gorm.Where("provider_id = ? AND client_type = ? AND until_time > ?", providerID, clientType, now).First(&model).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *CooldownRepository) Upsert(cooldown *domain.Cooldown) error {
	now := time.Now()
	model := &Cooldown{
		BaseModel: BaseModel{
			CreatedAt: toTimestamp(now),
			UpdatedAt: toTimestamp(now),
		},
		ProviderID: cooldown.ProviderID,
		ClientType: cooldown.ClientType,
		UntilTime:  toTimestamp(cooldown.UntilTime),
		Reason:     string(cooldown.Reason),
	}

	err := r.db.gorm.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "provider_id"}, {Name: "client_type"}},
		DoUpdates: clause.Assignments(map[string]any{
			"until_time": model.UntilTime,
			"reason":     model.Reason,
			"updated_at": model.UpdatedAt,
		}),
	}).Create(model).Error

	if err != nil {
		return err
	}

	cooldown.CreatedAt = now
	cooldown.UpdatedAt = now
	return nil
}

func (r *CooldownRepository) Delete(providerID uint64, clientType string) error {
	return r.db.gorm.Where("provider_id = ? AND client_type = ?", providerID, clientType).Delete(&Cooldown{}).Error
}

func (r *CooldownRepository) DeleteAll(providerID uint64) error {
	return r.db.gorm.Where("provider_id = ?", providerID).Delete(&Cooldown{}).Error
}

func (r *CooldownRepository) DeleteExpired() error {
	now := time.Now().UnixMilli()
	return r.db.gorm.Where("until_time <= ?", now).Delete(&Cooldown{}).Error
}

func (r *CooldownRepository) toDomain(m *Cooldown) *domain.Cooldown {
	return &domain.Cooldown{
		ID:         m.ID,
		CreatedAt:  fromTimestamp(m.CreatedAt),
		UpdatedAt:  fromTimestamp(m.UpdatedAt),
		ProviderID: m.ProviderID,
		ClientType: m.ClientType,
		UntilTime:  fromTimestamp(m.UntilTime),
		Reason:     domain.CooldownReason(m.Reason),
	}
}

func (r *CooldownRepository) toDomainList(models []Cooldown) []*domain.Cooldown {
	cooldowns := make([]*domain.Cooldown, len(models))
	for i, m := range models {
		cooldowns[i] = r.toDomain(&m)
	}
	return cooldowns
}
