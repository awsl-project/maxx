package sqlite

import (
	"errors"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
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

func (r *SessionRepository) Delete(id uint64) error {
	now := time.Now().UnixMilli()
	return r.db.gorm.Model(&Session{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"deleted_at": now,
			"updated_at": now,
		}).Error
}

func (r *SessionRepository) GetBySessionID(sessionID string) (*domain.Session, error) {
	var model Session
	if err := r.db.gorm.Where("session_id = ? AND deleted_at = 0", sessionID).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *SessionRepository) List() ([]*domain.Session, error) {
	var models []Session
	if err := r.db.gorm.Where("deleted_at = 0").Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}

	sessions := make([]*domain.Session, len(models))
	for i, m := range models {
		sessions[i] = r.toDomain(&m)
	}
	return sessions, nil
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
		SessionID:  m.SessionID,
		ClientType: domain.ClientType(m.ClientType),
		ProjectID:  m.ProjectID,
		RejectedAt: fromTimestampPtr(m.RejectedAt),
	}
}
