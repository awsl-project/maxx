package gormdb

import (
	"errors"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"gorm.io/gorm"
)

type ProjectRepository struct {
	db *DB
}

func NewProjectRepository(db *DB) *ProjectRepository {
	return &ProjectRepository{db: db}
}

func (r *ProjectRepository) Create(p *domain.Project) error {
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now

	// Generate slug if not provided
	if p.Slug == "" {
		p.Slug = domain.GenerateSlug(p.Name)
	}

	// Ensure slug uniqueness (only among non-deleted projects)
	baseSlug := p.Slug
	counter := 1
	for {
		var count int64
		if err := r.db.gorm.Model(&Project{}).Where("slug = ? AND deleted_at = 0", p.Slug).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			break
		}
		counter++
		p.Slug = baseSlug + "-" + itoa(counter)
	}

	model := r.toModel(p)
	if err := r.db.gorm.Create(model).Error; err != nil {
		return err
	}
	p.ID = model.ID
	return nil
}

func (r *ProjectRepository) Update(p *domain.Project) error {
	p.UpdatedAt = time.Now()

	// Check slug uniqueness (excluding current project and deleted projects)
	if p.Slug != "" {
		var count int64
		if err := r.db.gorm.Model(&Project{}).Where("slug = ? AND id != ? AND deleted_at = 0", p.Slug, p.ID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return domain.ErrSlugExists
		}
	}

	model := r.toModel(p)
	return r.db.gorm.Save(model).Error
}

func (r *ProjectRepository) Delete(id uint64) error {
	now := time.Now().UnixMilli()
	return r.db.gorm.Model(&Project{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"deleted_at": now,
			"updated_at": now,
		}).Error
}

func (r *ProjectRepository) GetByID(id uint64) (*domain.Project, error) {
	var model Project
	if err := r.db.gorm.First(&model, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *ProjectRepository) GetBySlug(slug string) (*domain.Project, error) {
	var model Project
	if err := r.db.gorm.Where("slug = ? AND deleted_at = 0", slug).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *ProjectRepository) List() ([]*domain.Project, error) {
	var models []Project
	if err := r.db.gorm.Where("deleted_at = 0").Order("id").Find(&models).Error; err != nil {
		return nil, err
	}

	projects := make([]*domain.Project, len(models))
	for i, m := range models {
		projects[i] = r.toDomain(&m)
	}
	return projects, nil
}

func (r *ProjectRepository) toModel(p *domain.Project) *Project {
	return &Project{
		SoftDeleteModel: SoftDeleteModel{
			BaseModel: BaseModel{
				ID:        p.ID,
				CreatedAt: toTimestamp(p.CreatedAt),
				UpdatedAt: toTimestamp(p.UpdatedAt),
			},
			DeletedAt: toTimestampPtr(p.DeletedAt),
		},
		Name:                p.Name,
		Slug:                p.Slug,
		EnabledCustomRoutes: LongText(toJSON(p.EnabledCustomRoutes)),
	}
}

func (r *ProjectRepository) toDomain(m *Project) *domain.Project {
	return &domain.Project{
		ID:                  m.ID,
		CreatedAt:           fromTimestamp(m.CreatedAt),
		UpdatedAt:           fromTimestamp(m.UpdatedAt),
		DeletedAt:           fromTimestampPtr(m.DeletedAt),
		Name:                m.Name,
		Slug:                m.Slug,
		EnabledCustomRoutes: fromJSON[[]domain.ClientType](string(m.EnabledCustomRoutes)),
	}
}

// itoa converts int to string without importing strconv
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var result []byte
	for i > 0 {
		result = append([]byte{byte('0' + i%10)}, result...)
		i /= 10
	}
	return string(result)
}
