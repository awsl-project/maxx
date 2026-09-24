package sqlite

import (
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

type ModelHealthCheckRepository struct{ db *DB }

const modelHealthTargetBatchSize = 100

func NewModelHealthCheckRepository(db *DB) *ModelHealthCheckRepository {
	return &ModelHealthCheckRepository{db: db}
}

func (r *ModelHealthCheckRepository) Create(check *domain.ModelHealthCheck) error {
	if check == nil {
		return domain.ErrInvalidInput
	}
	now := time.Now()
	if check.CheckedAt.IsZero() {
		check.CheckedAt = now
	}
	check.CreatedAt = now
	check.UpdatedAt = now
	model := r.toModel(check)
	if err := r.db.gorm.Create(model).Error; err != nil {
		return err
	}
	check.ID = model.ID
	return nil
}

func (r *ModelHealthCheckRepository) LatestByTargets(tenantID uint64, targets []domain.ModelHealthCheckTarget) (map[string]*domain.ModelHealthCheck, error) {
	out := make(map[string]*domain.ModelHealthCheck, len(targets))
	if len(targets) == 0 {
		return out, nil
	}
	for _, batch := range chunkModelHealthTargets(targets, modelHealthTargetBatchSize) {
		query := tenantScope(r.db.gorm.Model(&ModelHealthCheck{}), tenantID)
		query = query.Where(r.targetWhere(batch), r.targetArgs(batch)...)
		var models []ModelHealthCheck
		if err := query.Order("checked_at DESC, id DESC").Find(&models).Error; err != nil {
			return nil, err
		}
		for i := range models {
			model := &models[i]
			key := repository.ModelHealthTargetKey(model.Model, domain.ClientType(model.ClientType), model.RouteID, model.ProviderID)
			if _, ok := out[key]; ok {
				continue
			}
			out[key] = r.toDomain(model)
		}
	}
	return out, nil
}

func (r *ModelHealthCheckRepository) ListByTargetsSince(tenantID uint64, targets []domain.ModelHealthCheckTarget, since time.Time) ([]*domain.ModelHealthCheck, error) {
	if len(targets) == 0 {
		return []*domain.ModelHealthCheck{}, nil
	}
	out := make([]*domain.ModelHealthCheck, 0, len(targets))
	for _, batch := range chunkModelHealthTargets(targets, modelHealthTargetBatchSize) {
		query := tenantScope(r.db.gorm.Model(&ModelHealthCheck{}), tenantID).Where("checked_at >= ?", toTimestamp(since))
		query = query.Where(r.targetWhere(batch), r.targetArgs(batch)...)
		var models []ModelHealthCheck
		if err := query.Order("checked_at ASC, id ASC").Find(&models).Error; err != nil {
			return nil, err
		}
		for i := range models {
			out = append(out, r.toDomain(&models[i]))
		}
	}
	return out, nil
}

func (r *ModelHealthCheckRepository) targetWhere(targets []domain.ModelHealthCheckTarget) string {
	parts := make([]byte, 0, len(targets)*48)
	parts = append(parts, '(')
	for i := range targets {
		if i > 0 {
			parts = append(parts, " OR "...)
		}
		parts = append(parts, "(model = ? AND client_type = ? AND route_id = ? AND provider_id = ?)"...)
	}
	parts = append(parts, ')')
	return string(parts)
}

func (r *ModelHealthCheckRepository) targetArgs(targets []domain.ModelHealthCheckTarget) []any {
	args := make([]any, 0, len(targets)*4)
	for _, t := range targets {
		args = append(args, t.Model, string(t.ClientType), t.RouteID, t.ProviderID)
	}
	return args
}

func (r *ModelHealthCheckRepository) toModel(c *domain.ModelHealthCheck) *ModelHealthCheck {
	return &ModelHealthCheck{BaseModel: BaseModel{ID: c.ID, CreatedAt: toTimestamp(c.CreatedAt), UpdatedAt: toTimestamp(c.UpdatedAt)}, TenantID: c.TenantID, Model: c.Model, ClientType: string(c.ClientType), RouteID: c.RouteID, ProviderID: c.ProviderID, ProviderName: c.ProviderName, Status: string(c.Status), LatencyMs: c.LatencyMs, Error: LongText(c.Error), CheckedAt: toTimestamp(c.CheckedAt)}
}
func (r *ModelHealthCheckRepository) toDomain(m *ModelHealthCheck) *domain.ModelHealthCheck {
	return &domain.ModelHealthCheck{ID: m.ID, TenantID: m.TenantID, Model: m.Model, ClientType: domain.ClientType(m.ClientType), RouteID: m.RouteID, ProviderID: m.ProviderID, ProviderName: m.ProviderName, Status: domain.ModelHealthStatus(m.Status), LatencyMs: m.LatencyMs, Error: string(m.Error), CheckedAt: fromTimestamp(m.CheckedAt), CreatedAt: fromTimestamp(m.CreatedAt), UpdatedAt: fromTimestamp(m.UpdatedAt)}
}

func chunkModelHealthTargets(targets []domain.ModelHealthCheckTarget, size int) [][]domain.ModelHealthCheckTarget {
	if size <= 0 || len(targets) <= size {
		return [][]domain.ModelHealthCheckTarget{targets}
	}
	chunks := make([][]domain.ModelHealthCheckTarget, 0, (len(targets)+size-1)/size)
	for start := 0; start < len(targets); start += size {
		end := start + size
		if end > len(targets) {
			end = len(targets)
		}
		chunks = append(chunks, targets[start:end])
	}
	return chunks
}
