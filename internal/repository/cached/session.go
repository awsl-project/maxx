package cached

import (
	"errors"
	"sync"

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
	r.cache[sessionCacheKey{TenantID: s.TenantID, SessionID: s.SessionID}] = s
	r.mu.Unlock()
	return nil
}

func (r *SessionRepository) Update(s *domain.Session) error {
	if err := r.repo.Update(s); err != nil {
		return err
	}
	r.mu.Lock()
	r.cache[sessionCacheKey{TenantID: s.TenantID, SessionID: s.SessionID}] = s
	r.mu.Unlock()
	return nil
}

func (r *SessionRepository) GetBySessionID(tenantID uint64, sessionID string) (*domain.Session, error) {
	r.mu.RLock()
	if tenantID == domain.TenantIDAll {
		for key, s := range r.cache {
			if key.SessionID == sessionID {
				r.mu.RUnlock()
				return s, nil
			}
		}
	} else if s, ok := r.cache[sessionCacheKey{TenantID: tenantID, SessionID: sessionID}]; ok {
		r.mu.RUnlock()
		return s, nil
	}
	r.mu.RUnlock()

	s, err := r.repo.GetBySessionID(tenantID, sessionID)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.cache[sessionCacheKey{TenantID: s.TenantID, SessionID: s.SessionID}] = s
	r.mu.Unlock()
	return s, nil
}

func (r *SessionRepository) GetOrCreate(tenantID uint64, sessionID string, clientType domain.ClientType) (*domain.Session, error) {
	r.mu.RLock()
	if tenantID == domain.TenantIDAll {
		for key, s := range r.cache {
			if key.SessionID == sessionID {
				r.mu.RUnlock()
				return s, nil
			}
		}
	} else if s, ok := r.cache[sessionCacheKey{TenantID: tenantID, SessionID: sessionID}]; ok {
		r.mu.RUnlock()
		return s, nil
	}
	r.mu.RUnlock()

	s, err := r.repo.GetBySessionID(tenantID, sessionID)
	if err == nil {
		r.mu.Lock()
		r.cache[sessionCacheKey{TenantID: s.TenantID, SessionID: s.SessionID}] = s
		r.mu.Unlock()
		return s, nil
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

	r.mu.Lock()
	r.cache[sessionCacheKey{TenantID: s.TenantID, SessionID: s.SessionID}] = s
	r.mu.Unlock()
	return s, nil
}

func (r *SessionRepository) List(tenantID uint64) ([]*domain.Session, error) {
	return r.repo.List(tenantID)
}
