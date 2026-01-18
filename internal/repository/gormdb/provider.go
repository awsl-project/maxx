package gormdb

import (
	"errors"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"gorm.io/gorm"
)

type ProviderRepository struct {
	db *DB
}

func NewProviderRepository(db *DB) *ProviderRepository {
	return &ProviderRepository{db: db}
}

func (r *ProviderRepository) Create(p *domain.Provider) error {
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now

	model := r.toModel(p)
	if err := r.db.gorm.Create(model).Error; err != nil {
		return err
	}
	p.ID = model.ID
	return nil
}

func (r *ProviderRepository) Update(p *domain.Provider) error {
	p.UpdatedAt = time.Now()
	model := r.toModel(p)
	return r.db.gorm.Save(model).Error
}

func (r *ProviderRepository) Delete(id uint64) error {
	now := time.Now().UnixMilli()
	return r.db.gorm.Model(&Provider{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"deleted_at": now,
			"updated_at": now,
		}).Error
}

func (r *ProviderRepository) GetByID(id uint64) (*domain.Provider, error) {
	var model Provider
	if err := r.db.gorm.First(&model, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *ProviderRepository) List() ([]*domain.Provider, error) {
	var models []Provider
	if err := r.db.gorm.Where("deleted_at = 0").Order("id").Find(&models).Error; err != nil {
		return nil, err
	}

	providers := make([]*domain.Provider, len(models))
	for i, m := range models {
		providers[i] = r.toDomain(&m)
	}
	return providers, nil
}

// toModel converts domain.Provider to sqlite.Provider
func (r *ProviderRepository) toModel(p *domain.Provider) *Provider {
	return &Provider{
		SoftDeleteModel: SoftDeleteModel{
			BaseModel: BaseModel{
				ID:        p.ID,
				CreatedAt: toTimestamp(p.CreatedAt),
				UpdatedAt: toTimestamp(p.UpdatedAt),
			},
			DeletedAt: toTimestampPtr(p.DeletedAt),
		},
		Type:                 p.Type,
		Name:                 p.Name,
		Config:               LongText(toJSON(p.Config)),
		SupportedClientTypes: LongText(toJSON(p.SupportedClientTypes)),
		SupportModels:        LongText(toJSON(p.SupportModels)),
	}
}

// toDomain converts sqlite.Provider to domain.Provider
func (r *ProviderRepository) toDomain(m *Provider) *domain.Provider {
	return &domain.Provider{
		ID:                   m.ID,
		CreatedAt:            fromTimestamp(m.CreatedAt),
		UpdatedAt:            fromTimestamp(m.UpdatedAt),
		DeletedAt:            fromTimestampPtr(m.DeletedAt),
		Type:                 m.Type,
		Name:                 m.Name,
		Config:               fromJSON[*domain.ProviderConfig](string(m.Config)),
		SupportedClientTypes: fromJSON[[]domain.ClientType](string(m.SupportedClientTypes)),
		SupportModels:        fromJSON[[]string](string(m.SupportModels)),
	}
}
