package cached

import (
	"errors"
	"sync"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

// SessionRepository caches session records around a backing repository.
type SessionRepository struct {
	repo  repository.SessionRepository
	cache map[string]*domain.Session
	mu    sync.RWMutex
}

func NewSessionRepository(repo repository.SessionRepository) *SessionRepository {
	return &SessionRepository{
		repo:  repo,
		cache: make(map[string]*domain.Session),
	}
}

func (r *SessionRepository) Create(s *domain.Session) error {
	if err := r.repo.Create(s); err != nil {
		return err
	}
	r.mu.Lock()
	r.cache[s.SessionID] = s
	r.mu.Unlock()
	return nil
}

func (r *SessionRepository) Update(s *domain.Session) error {
	if err := r.repo.Update(s); err != nil {
		return err
	}
	r.mu.Lock()
	r.cache[s.SessionID] = s
	r.mu.Unlock()
	return nil
}

func (r *SessionRepository) GetBySessionID(tenantID uint64, sessionID string) (*domain.Session, error) {
	r.mu.RLock()
	if s, ok := r.cache[sessionID]; ok && (tenantID == 0 || s.TenantID == tenantID) {
		r.mu.RUnlock()
		return s, nil
	}
	r.mu.RUnlock()

	s, err := r.repo.GetBySessionID(tenantID, sessionID)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.cache[sessionID] = s
	r.mu.Unlock()
	return s, nil
}

func (r *SessionRepository) GetOrCreate(tenantID uint64, sessionID string, clientType domain.ClientType) (*domain.Session, error) {
	r.mu.RLock()
	if s, ok := r.cache[sessionID]; ok && (tenantID == 0 || s.TenantID == tenantID) {
		r.mu.RUnlock()
		return s, nil
	}
	r.mu.RUnlock()

	s, err := r.repo.GetBySessionID(tenantID, sessionID)
	if err == nil {
		r.mu.Lock()
		r.cache[sessionID] = s
		r.mu.Unlock()
		return s, nil
	}

	if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
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
	r.cache[sessionID] = s
	r.mu.Unlock()
	return s, nil
}

func (r *SessionRepository) List(tenantID uint64) ([]*domain.Session, error) {
	return r.repo.List(tenantID)
}
