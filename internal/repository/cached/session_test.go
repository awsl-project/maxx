package cached

import (
	"sync"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

type sessionTestRepo struct {
	mu            sync.Mutex
	session       *domain.Session
	lastTouchedAt time.Time
}

type blockingDeleteSessionRepo struct {
	*sessionTestRepo
	deleteStarted chan struct{}
	allowDelete   chan struct{}
	startOnce     sync.Once
}

func (r *sessionTestRepo) Create(session *domain.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()

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
	r.mu.Lock()
	defer r.mu.Unlock()

	clone := *session
	r.session = &clone
	return nil
}

func (r *sessionTestRepo) Touch(tenantID uint64, sessionID string, touchedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.session == nil || r.session.TenantID != tenantID || r.session.SessionID != sessionID {
		return domain.ErrNotFound
	}

	r.lastTouchedAt = touchedAt
	r.session.UpdatedAt = touchedAt
	return nil
}

func (r *sessionTestRepo) GetBySessionID(tenantID uint64, sessionID string) (*domain.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.session == nil || r.session.TenantID != tenantID || r.session.SessionID != sessionID {
		return nil, domain.ErrNotFound
	}

	clone := *r.session
	return &clone, nil
}

func (r *sessionTestRepo) List(tenantID uint64) ([]*domain.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.session == nil || r.session.TenantID != tenantID {
		return nil, nil
	}

	clone := *r.session
	return []*domain.Session{&clone}, nil
}

func (r *sessionTestRepo) DeleteOlderThan(before time.Time) (int64, error) {
	return 0, nil
}

func (r *blockingDeleteSessionRepo) DeleteOlderThan(before time.Time) (int64, error) {
	r.startOnce.Do(func() {
		close(r.deleteStarted)
	})
	<-r.allowDelete

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.session == nil {
		return 0, nil
	}
	r.session = nil
	return 1, nil
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

func TestSessionRepositoryGetBySessionIDReturnsDetachedCopy(t *testing.T) {
	baseRepo := &sessionTestRepo{}
	repo := NewSessionRepository(baseRepo)
	rejectedAt := time.Unix(1710000300, 0).UTC()
	deletedAt := rejectedAt.Add(-time.Minute)
	session := &domain.Session{
		TenantID:   1,
		SessionID:  "session-detached-copy",
		ClientType: domain.ClientTypeCodex,
		ProjectID:  7,
		DeletedAt:  &deletedAt,
		RejectedAt: &rejectedAt,
	}

	if err := repo.Create(session); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	first, err := repo.GetBySessionID(session.TenantID, session.SessionID)
	if err != nil {
		t.Fatalf("GetBySessionID(first) error = %v", err)
	}
	first.ProjectID = 99
	first.DeletedAt = nil
	firstRejectedAt := time.Unix(1710000900, 0).UTC()
	first.RejectedAt = &firstRejectedAt

	second, err := repo.GetBySessionID(session.TenantID, session.SessionID)
	if err != nil {
		t.Fatalf("GetBySessionID(second) error = %v", err)
	}
	if second.ProjectID != 7 {
		t.Fatalf("ProjectID = %d, want 7", second.ProjectID)
	}
	if second.DeletedAt == nil || !second.DeletedAt.Equal(deletedAt) {
		t.Fatalf("DeletedAt = %v, want %v", second.DeletedAt, deletedAt)
	}
	if second.RejectedAt == nil || !second.RejectedAt.Equal(rejectedAt) {
		t.Fatalf("RejectedAt = %v, want %v", second.RejectedAt, rejectedAt)
	}
}

func TestSessionRepositoryTouchDoesNotMutatePreviouslyReturnedCopy(t *testing.T) {
	baseRepo := &sessionTestRepo{}
	repo := NewSessionRepository(baseRepo)
	session := &domain.Session{
		TenantID:   1,
		SessionID:  "session-touch-detached",
		ClientType: domain.ClientTypeCodex,
	}

	if err := repo.Create(session); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	first, err := repo.GetBySessionID(session.TenantID, session.SessionID)
	if err != nil {
		t.Fatalf("GetBySessionID(first) error = %v", err)
	}
	originalUpdatedAt := first.UpdatedAt

	touchedAt := time.Unix(1710001200, 0).UTC()
	if err := repo.Touch(session.TenantID, session.SessionID, touchedAt); err != nil {
		t.Fatalf("Touch() error = %v", err)
	}

	if !first.UpdatedAt.Equal(originalUpdatedAt) {
		t.Fatalf("previously returned copy was mutated to %v, want %v", first.UpdatedAt, originalUpdatedAt)
	}

	second, err := repo.GetBySessionID(session.TenantID, session.SessionID)
	if err != nil {
		t.Fatalf("GetBySessionID(second) error = %v", err)
	}
	if !second.UpdatedAt.Equal(touchedAt) {
		t.Fatalf("UpdatedAt = %v, want %v", second.UpdatedAt, touchedAt)
	}
}

func TestSessionRepositoryConcurrentGetAndTouch(t *testing.T) {
	baseRepo := &sessionTestRepo{}
	repo := NewSessionRepository(baseRepo)
	session := &domain.Session{
		TenantID:   1,
		SessionID:  "session-concurrent",
		ClientType: domain.ClientTypeCodex,
	}

	if err := repo.Create(session); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				if _, err := repo.GetBySessionID(session.TenantID, session.SessionID); err != nil {
					t.Errorf("GetBySessionID() error = %v", err)
					return
				}
				if err := repo.Touch(session.TenantID, session.SessionID, time.Unix(int64(worker*1000+j), 0).UTC()); err != nil {
					t.Errorf("Touch() error = %v", err)
					return
				}
			}
		}(i)
	}
	wg.Wait()
}

func TestSessionRepositoryDeleteOlderThanBlocksReadersUntilCacheCleared(t *testing.T) {
	baseRepo := &blockingDeleteSessionRepo{
		sessionTestRepo: &sessionTestRepo{},
		deleteStarted:   make(chan struct{}),
		allowDelete:     make(chan struct{}),
	}
	repo := NewSessionRepository(baseRepo)
	session := &domain.Session{
		TenantID:   1,
		SessionID:  "session-delete-race",
		ClientType: domain.ClientTypeCodex,
		ProjectID:  42,
	}

	if err := repo.Create(session); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	deleteDone := make(chan error, 1)
	go func() {
		_, err := repo.DeleteOlderThan(time.Now())
		deleteDone <- err
	}()

	select {
	case <-baseRepo.deleteStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("DeleteOlderThan() did not start in time")
	}

	getResult := make(chan error, 1)
	go func() {
		_, err := repo.GetBySessionID(session.TenantID, session.SessionID)
		getResult <- err
	}()

	select {
	case err := <-getResult:
		t.Fatalf("GetBySessionID() returned before cache cleanup finished: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	close(baseRepo.allowDelete)

	select {
	case err := <-deleteDone:
		if err != nil {
			t.Fatalf("DeleteOlderThan() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("DeleteOlderThan() did not finish in time")
	}

	select {
	case err := <-getResult:
		if err != domain.ErrNotFound {
			t.Fatalf("GetBySessionID() error = %v, want %v", err, domain.ErrNotFound)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("GetBySessionID() did not finish after cache cleanup")
	}
}
