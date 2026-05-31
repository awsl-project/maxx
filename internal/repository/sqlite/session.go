package sqlite

import (
	"errors"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
	"gorm.io/gorm"
)

type SessionRepository struct {
	db *DB
}

func NewSessionRepository(db *DB) *SessionRepository {
	return &SessionRepository{db: db}
}

func (r *SessionRepository) Create(s *domain.Session) error {
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

func (r *SessionRepository) Update(s *domain.Session) error {
	s.UpdatedAt = time.Now()
	model := r.toModel(s)
	return r.db.gorm.Save(model).Error
}

func (r *SessionRepository) Touch(tenantID uint64, sessionID string, touchedAt time.Time) error {
	if touchedAt.IsZero() {
		touchedAt = time.Now()
	}

	result := tenantScope(r.db.gorm.Model(&Session{}), tenantID).
		Where("session_id = ? AND deleted_at = 0", sessionID).
		Update("updated_at", toTimestamp(touchedAt))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}

	var count int64
	if err := tenantScope(r.db.gorm.Model(&Session{}), tenantID).
		Where("session_id = ? AND deleted_at = 0", sessionID).
		Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *SessionRepository) Delete(id uint64) error {
	now := time.Now().UnixMilli()
	return r.db.gorm.Model(&Session{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"deleted_at": now,
			"updated_at": now,
		}).Error
}

func (r *SessionRepository) GetBySessionID(tenantID uint64, sessionID string) (*domain.Session, error) {
	var model Session
	if err := tenantScope(r.db.gorm, tenantID).Where("session_id = ? AND deleted_at = 0", sessionID).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *SessionRepository) List(tenantID uint64) ([]*domain.Session, error) {
	var models []Session
	if err := tenantScope(r.db.gorm, tenantID).
		Where("deleted_at = 0").
		Order("updated_at DESC, created_at DESC").
		Find(&models).Error; err != nil {
		return nil, err
	}

	sessions := make([]*domain.Session, len(models))
	for i, m := range models {
		sessions[i] = r.toDomain(&m)
	}
	return sessions, nil
}

func (r *SessionRepository) DeleteOlderThan(before time.Time) (int64, error) {
	if before.IsZero() {
		return 0, nil
	}

	result := r.db.gorm.
		Where("updated_at < ?", toTimestamp(before)).
		Delete(&Session{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// ListExpiredKeys 返回 updated_at < before 的 session 标识。
// 仅 SELECT tenant_id/session_id 两列,避免拉整行;调用方拿到 keys 后
// 通常会立刻 DeleteOlderThan,所以这里没必要做分页。
func (r *SessionRepository) ListExpiredKeys(before time.Time) ([]repository.SessionKey, error) {
	if before.IsZero() {
		return nil, nil
	}
	var rows []struct {
		TenantID  uint64
		SessionID string
	}
	if err := r.db.gorm.
		Model(&Session{}).
		Select("tenant_id, session_id").
		Where("updated_at < ?", toTimestamp(before)).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]repository.SessionKey, len(rows))
	for i, row := range rows {
		out[i] = repository.SessionKey{TenantID: row.TenantID, SessionID: row.SessionID}
	}
	return out, nil
}

func (r *SessionRepository) toModel(s *domain.Session) *Session {
	return &Session{
		SoftDeleteModel: SoftDeleteModel{
			BaseModel: BaseModel{
				ID:        s.ID,
				CreatedAt: toTimestamp(s.CreatedAt),
				UpdatedAt: toTimestamp(s.UpdatedAt),
			},
			DeletedAt: toTimestampPtr(s.DeletedAt),
		},
		TenantID:   s.TenantID,
		SessionID:  s.SessionID,
		ClientType: string(s.ClientType),
		ProjectID:  s.ProjectID,
		RejectedAt: toTimestampPtr(s.RejectedAt),
	}
}

func (r *SessionRepository) toDomain(m *Session) *domain.Session {
	return &domain.Session{
		ID:         m.ID,
		CreatedAt:  fromTimestamp(m.CreatedAt),
		UpdatedAt:  fromTimestamp(m.UpdatedAt),
		DeletedAt:  fromTimestampPtr(m.DeletedAt),
		TenantID:   m.TenantID,
		SessionID:  m.SessionID,
		ClientType: domain.ClientType(m.ClientType),
		ProjectID:  m.ProjectID,
		RejectedAt: fromTimestampPtr(m.RejectedAt),
	}
}
