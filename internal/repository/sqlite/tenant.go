package sqlite

import (
	"errors"
	"strconv"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"gorm.io/gorm"
)

// TenantRepository manages tenants with soft delete support.
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

	if t.Slug == "" {
		t.Slug = domain.GenerateSlug(t.Name)
	}

	// Ensure slug uniqueness among non-deleted tenants.
	baseSlug := t.Slug
	counter := 1
	for {
		var count int64
		if err := r.db.gorm.Model(&Tenant{}).Where("slug = ? AND deleted_at = 0", t.Slug).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			break
		}
		counter++
		t.Slug = baseSlug + "-" + strconv.Itoa(counter)
	}

	model := r.toModel(t)
	if err := r.db.gorm.Create(model).Error; err != nil {
		return err
	}
	t.ID = model.ID
	return nil
}

func (r *TenantRepository) Update(t *domain.Tenant) error {
	t.UpdatedAt = time.Now()

	// Check slug uniqueness (excluding current and deleted)
	if t.Slug != "" {
		var count int64
		if err := r.db.gorm.Model(&Tenant{}).Where("slug = ? AND id != ? AND deleted_at = 0", t.Slug, t.ID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return domain.ErrSlugExists
		}
	}

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
	return &Tenant{
		SoftDeleteModel: SoftDeleteModel{
			BaseModel: BaseModel{
				ID:        t.ID,
				CreatedAt: toTimestamp(t.CreatedAt),
				UpdatedAt: toTimestamp(t.UpdatedAt),
			},
			DeletedAt: toTimestampPtr(t.DeletedAt),
		},
		Name:   t.Name,
		Slug:   t.Slug,
		Status: t.Status,
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
		Status:    m.Status,
	}
}
