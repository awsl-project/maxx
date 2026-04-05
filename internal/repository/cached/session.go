package cached

import (
	"errors"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

type sessionCacheKey struct {
	TenantID  uint64
	SessionID string
}

// SessionRepository caches session records around a backing repository.
type SessionRepository struct {
	repo  repository.SessionRepository
	cache map[sessionCacheKey]*domain.Session
	mu    sync.RWMutex
}

func NewSessionRepository(repo repository.SessionRepository) *SessionRepository {
	return &SessionRepository{
		repo:  repo,
		cache: make(map[sessionCacheKey]*domain.Session),
	}
}

func (r *SessionRepository) Create(s *domain.Session) error {
	if err := r.repo.Create(s); err != nil {
		return err
	}
	r.mu.Lock()
	r.cache[sessionCacheKey{TenantID: s.TenantID, SessionID: s.SessionID}] = cloneSession(s)
	r.mu.Unlock()
	return nil
}

func (r *SessionRepository) Update(s *domain.Session) error {
	if err := r.repo.Update(s); err != nil {
		return err
	}
	r.mu.Lock()
	r.cache[sessionCacheKey{TenantID: s.TenantID, SessionID: s.SessionID}] = cloneSession(s)
	r.mu.Unlock()
	return nil
}

func (r *SessionRepository) Touch(tenantID uint64, sessionID string, touchedAt time.Time) error {
	if touchedAt.IsZero() {
		touchedAt = time.Now()
	}

	if err := r.repo.Touch(tenantID, sessionID, touchedAt); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if tenantID == domain.TenantIDAll {
		for key, session := range r.cache {
			if key.SessionID == sessionID {
				session.UpdatedAt = touchedAt
			}
		}
		return nil
	}

	if session, ok := r.cache[sessionCacheKey{TenantID: tenantID, SessionID: sessionID}]; ok {
		session.UpdatedAt = touchedAt
	}
	return nil
}

func (r *SessionRepository) GetBySessionID(tenantID uint64, sessionID string) (*domain.Session, error) {
	r.mu.RLock()
	if tenantID == domain.TenantIDAll {
		for key, s := range r.cache {
			if key.SessionID == sessionID {
				clonedSession := cloneSession(s)
				r.mu.RUnlock()
				return clonedSession, nil
			}
		}
	} else if s, ok := r.cache[sessionCacheKey{TenantID: tenantID, SessionID: sessionID}]; ok {
		clonedSession := cloneSession(s)
		r.mu.RUnlock()
		return clonedSession, nil
	}
	r.mu.RUnlock()

	s, err := r.repo.GetBySessionID(tenantID, sessionID)
	if err != nil {
		return nil, err
	}

	cachedSession := cloneSession(s)
	r.mu.Lock()
	r.cache[sessionCacheKey{TenantID: s.TenantID, SessionID: s.SessionID}] = cachedSession
	r.mu.Unlock()
	return cloneSession(cachedSession), nil
}

func (r *SessionRepository) GetOrCreate(tenantID uint64, sessionID string, clientType domain.ClientType) (*domain.Session, error) {
	r.mu.RLock()
	if tenantID == domain.TenantIDAll {
		for key, s := range r.cache {
			if key.SessionID == sessionID {
				clonedSession := cloneSession(s)
				r.mu.RUnlock()
				return clonedSession, nil
			}
		}
	} else if s, ok := r.cache[sessionCacheKey{TenantID: tenantID, SessionID: sessionID}]; ok {
		clonedSession := cloneSession(s)
		r.mu.RUnlock()
		return clonedSession, nil
	}
	r.mu.RUnlock()

	s, err := r.repo.GetBySessionID(tenantID, sessionID)
	if err == nil {
		cachedSession := cloneSession(s)
		r.mu.Lock()
		r.cache[sessionCacheKey{TenantID: s.TenantID, SessionID: s.SessionID}] = cachedSession
		r.mu.Unlock()
		return cloneSession(cachedSession), nil
	}

	if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	// Reject creation with TenantIDAll — it would store TenantID=0
	if tenantID == domain.TenantIDAll {
		return nil, domain.ErrNotFound
	}

	s = &domain.Session{
		TenantID:   tenantID,
		SessionID:  sessionID,
		ClientType: clientType,
		ProjectID:  0,
	}
	if err := r.repo.Create(s); err != nil {
		return nil, err
	}

	cachedSession := cloneSession(s)
	r.mu.Lock()
	r.cache[sessionCacheKey{TenantID: s.TenantID, SessionID: s.SessionID}] = cachedSession
	r.mu.Unlock()
	return cloneSession(cachedSession), nil
}

func (r *SessionRepository) List(tenantID uint64) ([]*domain.Session, error) {
	return r.repo.List(tenantID)
}

func (r *SessionRepository) DeleteOlderThan(before time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	deleted, err := r.repo.DeleteOlderThan(before)
	if err != nil {
		return 0, err
	}
	if deleted > 0 {
		r.cache = make(map[sessionCacheKey]*domain.Session)
	}
	return deleted, nil
}

func cloneSession(session *domain.Session) *domain.Session {
	if session == nil {
		return nil
	}

	clone := *session
	if session.DeletedAt != nil {
		deletedAt := *session.DeletedAt
		clone.DeletedAt = &deletedAt
	}
	if session.RejectedAt != nil {
		rejectedAt := *session.RejectedAt
		clone.RejectedAt = &rejectedAt
	}
	return &clone
}
