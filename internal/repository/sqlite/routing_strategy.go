package sqlite

import (
	"errors"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"gorm.io/gorm"
)

type RoutingStrategyRepository struct {
	db *DB
}

func NewRoutingStrategyRepository(db *DB) *RoutingStrategyRepository {
	return &RoutingStrategyRepository{db: db}
}

func (r *RoutingStrategyRepository) Create(s *domain.RoutingStrategy) error {
	now := time.Now()
	s.CreatedAt = now
	s.UpdatedAt = now

	model := r.toModel(s)
	if err := r.db.gorm.Create(model).Error; err != nil {
		return err
	}
	s.ID = model.ID
	return nil
}

func (r *RoutingStrategyRepository) Update(s *domain.RoutingStrategy) error {
	s.UpdatedAt = time.Now()
	model := r.toModel(s)
	return r.db.gorm.Save(model).Error
}

func (r *RoutingStrategyRepository) Delete(id uint64) error {
	now := time.Now().UnixMilli()
	return r.db.gorm.Model(&RoutingStrategy{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"deleted_at": now,
			"updated_at": now,
		}).Error
}

func (r *RoutingStrategyRepository) GetByProjectID(projectID uint64) (*domain.RoutingStrategy, error) {
	var model RoutingStrategy
	if err := r.db.gorm.Where("project_id = ? AND deleted_at = 0", projectID).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *RoutingStrategyRepository) List() ([]*domain.RoutingStrategy, error) {
	var models []RoutingStrategy
	if err := r.db.gorm.Where("deleted_at = 0").Order("id").Find(&models).Error; err != nil {
		return nil, err
	}
	return r.toDomainList(models), nil
}

func (r *RoutingStrategyRepository) toModel(s *domain.RoutingStrategy) *RoutingStrategy {
	return &RoutingStrategy{
		SoftDeleteModel: SoftDeleteModel{
			BaseModel: BaseModel{
				ID:        s.ID,
				CreatedAt: toTimestamp(s.CreatedAt),
				UpdatedAt: toTimestamp(s.UpdatedAt),
			},
			DeletedAt: toTimestampPtr(s.DeletedAt),
		},
		ProjectID: s.ProjectID,
		Type:      string(s.Type),
		Config:    toJSON(s.Config),
	}
}

func (r *RoutingStrategyRepository) toDomain(m *RoutingStrategy) *domain.RoutingStrategy {
	return &domain.RoutingStrategy{
		ID:        m.ID,
		CreatedAt: fromTimestamp(m.CreatedAt),
		UpdatedAt: fromTimestamp(m.UpdatedAt),
		DeletedAt: fromTimestampPtr(m.DeletedAt),
		ProjectID: m.ProjectID,
		Type:      domain.RoutingStrategyType(m.Type),
		Config:    fromJSON[*domain.RoutingStrategyConfig](m.Config),
	}
}

func (r *RoutingStrategyRepository) toDomainList(models []RoutingStrategy) []*domain.RoutingStrategy {
	strategies := make([]*domain.RoutingStrategy, len(models))
	for i, m := range models {
		strategies[i] = r.toDomain(&m)
	}
	return strategies
}
