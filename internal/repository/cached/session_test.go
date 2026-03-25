package cached

import (
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

type sessionTestRepo struct {
	session       *domain.Session
	lastTouchedAt time.Time
}

func (r *sessionTestRepo) Create(session *domain.Session) error {
	if session.ID == 0 {
		session.ID = 1
	}
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = session.CreatedAt
	}

	clone := *session
	r.session = &clone
	return nil
}

func (r *sessionTestRepo) Update(session *domain.Session) error {
	clone := *session
	r.session = &clone
	return nil
}

func (r *sessionTestRepo) Touch(tenantID uint64, sessionID string, touchedAt time.Time) error {
	if r.session == nil || r.session.TenantID != tenantID || r.session.SessionID != sessionID {
		return domain.ErrNotFound
	}

	r.lastTouchedAt = touchedAt
	r.session.UpdatedAt = touchedAt
	return nil
}

func (r *sessionTestRepo) GetBySessionID(tenantID uint64, sessionID string) (*domain.Session, error) {
	if r.session == nil || r.session.TenantID != tenantID || r.session.SessionID != sessionID {
		return nil, domain.ErrNotFound
	}

	clone := *r.session
	return &clone, nil
}

func (r *sessionTestRepo) List(tenantID uint64) ([]*domain.Session, error) {
	if r.session == nil || r.session.TenantID != tenantID {
		return nil, nil
	}

	clone := *r.session
	return []*domain.Session{&clone}, nil
}

func (r *sessionTestRepo) DeleteOlderThan(before time.Time) (int64, error) {
	return 0, nil
}

func TestSessionRepositoryTouchNormalizesZeroTimestamp(t *testing.T) {
	baseRepo := &sessionTestRepo{}
	repo := NewSessionRepository(baseRepo)
	session := &domain.Session{
		TenantID:   1,
		SessionID:  "session-touch-zero",
		ClientType: domain.ClientTypeCodex,
	}

	if err := repo.Create(session); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := repo.Touch(session.TenantID, session.SessionID, time.Time{}); err != nil {
		t.Fatalf("Touch() error = %v", err)
	}
	if baseRepo.lastTouchedAt.IsZero() {
		t.Fatal("Touch() forwarded a zero timestamp to the backing repository")
	}

	cachedSession, err := repo.GetBySessionID(session.TenantID, session.SessionID)
	if err != nil {
		t.Fatalf("GetBySessionID() error = %v", err)
	}
	if cachedSession.UpdatedAt.IsZero() {
		t.Fatal("cached session UpdatedAt should be normalized to a non-zero timestamp")
	}
	if !cachedSession.UpdatedAt.Equal(baseRepo.lastTouchedAt) {
		t.Fatalf("cached UpdatedAt = %v, want %v", cachedSession.UpdatedAt, baseRepo.lastTouchedAt)
	}
}
