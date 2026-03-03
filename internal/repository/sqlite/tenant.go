package sqlite

import (
	"errors"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"gorm.io/gorm"
)

type TenantRepository struct {
	db *DB
}

func NewTenantRepository(db *DB) *TenantRepository {
	return &TenantRepository{db: db}
}

func (r *TenantRepository) Create(t *domain.Tenant) error {
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

func (r *TenantRepository) Update(t *domain.Tenant) error {
	t.UpdatedAt = time.Now()
	model := r.toModel(t)
	return r.db.gorm.Save(model).Error
}

func (r *TenantRepository) Delete(id uint64) error {
	now := time.Now().UnixMilli()
	return r.db.gorm.Model(&Tenant{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"deleted_at": now,
			"updated_at": now,
		}).Error
}

func (r *TenantRepository) GetByID(id uint64) (*domain.Tenant, error) {
	var model Tenant
	if err := r.db.gorm.First(&model, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *TenantRepository) GetBySlug(slug string) (*domain.Tenant, error) {
	var model Tenant
	if err := r.db.gorm.Where("slug = ? AND deleted_at = 0", slug).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *TenantRepository) GetDefault() (*domain.Tenant, error) {
	var model Tenant
	if err := r.db.gorm.Where("is_default = 1 AND deleted_at = 0").First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *TenantRepository) List() ([]*domain.Tenant, error) {
	var models []Tenant
	if err := r.db.gorm.Where("deleted_at = 0").Order("id").Find(&models).Error; err != nil {
		return nil, err
	}
	tenants := make([]*domain.Tenant, len(models))
	for i, m := range models {
		tenants[i] = r.toDomain(&m)
	}
	return tenants, nil
}

func (r *TenantRepository) toModel(t *domain.Tenant) *Tenant {
	isDefault := 0
	if t.IsDefault {
		isDefault = 1
	}
	return &Tenant{
		SoftDeleteModel: SoftDeleteModel{
			BaseModel: BaseModel{
				ID:        t.ID,
				CreatedAt: toTimestamp(t.CreatedAt),
				UpdatedAt: toTimestamp(t.UpdatedAt),
			},
			DeletedAt: toTimestampPtr(t.DeletedAt),
		},
		Name:      t.Name,
		Slug:      t.Slug,
		IsDefault: isDefault,
	}
}

func (r *TenantRepository) toDomain(m *Tenant) *domain.Tenant {
	return &domain.Tenant{
		ID:        m.ID,
		CreatedAt: fromTimestamp(m.CreatedAt),
		UpdatedAt: fromTimestamp(m.UpdatedAt),
		DeletedAt: fromTimestampPtr(m.DeletedAt),
		Name:      m.Name,
		Slug:      m.Slug,
		IsDefault: m.IsDefault == 1,
	}
}
