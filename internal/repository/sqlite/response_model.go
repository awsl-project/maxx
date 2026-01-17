package sqlite

import (
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"gorm.io/gorm/clause"
)

type ResponseModelRepository struct {
	db *DB
}

func NewResponseModelRepository(db *DB) *ResponseModelRepository {
	return &ResponseModelRepository{db: db}
}

// Upsert 更新或插入 response model（基于 name）
func (r *ResponseModelRepository) Upsert(name string) error {
	if name == "" {
		return nil
	}

	now := time.Now().UnixMilli()
	model := &ResponseModel{
		CreatedAt:  now,
		Name:       name,
		LastSeenAt: now,
		UseCount:   1,
	}

	return r.db.gorm.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "name"}},
		DoUpdates: clause.Assignments(map[string]any{
			"last_seen_at": now,
			"use_count":    clause.Expr{SQL: "use_count + 1"},
		}),
	}).Create(model).Error
}

// BatchUpsert 批量更新或插入 response models
func (r *ResponseModelRepository) BatchUpsert(names []string) error {
	if len(names) == 0 {
		return nil
	}

	// 去重
	seen := make(map[string]bool)
	unique := make([]string, 0, len(names))
	for _, name := range names {
		if name != "" && !seen[name] {
			seen[name] = true
			unique = append(unique, name)
		}
	}

	for _, name := range unique {
		if err := r.Upsert(name); err != nil {
			return err
		}
	}
	return nil
}

// List 获取所有 response models
func (r *ResponseModelRepository) List() ([]*domain.ResponseModel, error) {
	var models []ResponseModel
	if err := r.db.gorm.Order("use_count DESC, last_seen_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}

	results := make([]*domain.ResponseModel, len(models))
	for i, m := range models {
		results[i] = r.toDomain(&m)
	}
	return results, nil
}

// ListNames 获取所有 response model 名称
func (r *ResponseModelRepository) ListNames() ([]string, error) {
	var names []string
	if err := r.db.gorm.Model(&ResponseModel{}).Order("use_count DESC, last_seen_at DESC").Pluck("name", &names).Error; err != nil {
		return nil, err
	}
	return names, nil
}

func (r *ResponseModelRepository) toDomain(m *ResponseModel) *domain.ResponseModel {
	return &domain.ResponseModel{
		ID:         m.ID,
		CreatedAt:  fromTimestamp(m.CreatedAt),
		Name:       m.Name,
		LastSeenAt: fromTimestamp(m.LastSeenAt),
		UseCount:   m.UseCount,
	}
}
