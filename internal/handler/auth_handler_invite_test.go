package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

type stubUserRepo struct {
	users     map[string]*domain.User
	nextID    uint64
	createErr error
}

func (r *stubUserRepo) Create(user *domain.User) error {
	if r.createErr != nil {
		return r.createErr
	}
	if _, exists := r.users[user.Username]; exists {
		return domain.ErrAlreadyExists
	}
	r.nextID++
	user.ID = r.nextID
	r.users[user.Username] = user
	return nil
}

func (r *stubUserRepo) Update(user *domain.User) error { return nil }
func (r *stubUserRepo) Delete(tenantID uint64, id uint64) error { return nil }
func (r *stubUserRepo) GetByID(tenantID uint64, id uint64) (*domain.User, error) {
	return nil, domain.ErrNotFound
}
func (r *stubUserRepo) GetByUsername(username string) (*domain.User, error) {
	if u, ok := r.users[username]; ok {
		return u, nil
	}
	return nil, domain.ErrNotFound
}
func (r *stubUserRepo) GetDefault() (*domain.User, error) { return nil, domain.ErrNotFound }
func (r *stubUserRepo) List() ([]*domain.User, error) { return nil, nil }
func (r *stubUserRepo) ListByTenant(tenantID uint64) ([]*domain.User, error) { return nil, nil }
func (r *stubUserRepo) ListByTenantAndStatus(tenantID uint64, status domain.UserStatus) ([]*domain.User, error) {
	return nil, nil
}
func (r *stubUserRepo) CountActive() (int64, error) { return 0, nil }

type stubInviteRepo struct {
	invite        *domain.InviteCode
	consumeErr    error
	consumeCalled bool
	rollbackCount int
}

func (r *stubInviteRepo) Create(code *domain.InviteCode) error { return nil }
func (r *stubInviteRepo) Update(tenantID uint64, code *domain.InviteCode) error { return nil }
func (r *stubInviteRepo) Delete(tenantID uint64, id uint64) error { return nil }
func (r *stubInviteRepo) GetByID(tenantID uint64, id uint64) (*domain.InviteCode, error) {
	return nil, domain.ErrNotFound
}
func (r *stubInviteRepo) GetByCodeHash(tenantID uint64, codeHash string) (*domain.InviteCode, error) {
	return nil, domain.ErrNotFound
}
func (r *stubInviteRepo) List(tenantID uint64) ([]*domain.InviteCode, error) { return nil, nil }
func (r *stubInviteRepo) Consume(tenantID uint64, codeHash string, nowTime time.Time) (*domain.InviteCode, error) {
	r.consumeCalled = true
	if r.consumeErr != nil {
		return nil, r.consumeErr
	}
	return r.invite, nil
}
func (r *stubInviteRepo) RollbackConsume(tenantID uint64, id uint64) error {
	r.rollbackCount++
	return nil
}

type stubInviteUsageRepo struct {
	usages []*domain.InviteCodeUsage
}

func (r *stubInviteUsageRepo) Create(usage *domain.InviteCodeUsage) error {
	r.usages = append(r.usages, usage)
	return nil
}
func (r *stubInviteUsageRepo) ListByCodeID(tenantID uint64, codeID uint64) ([]*domain.InviteCodeUsage, error) {
	return nil, nil
}
func (r *stubInviteUsageRepo) ListByUserID(tenantID uint64, userID uint64) ([]*domain.InviteCodeUsage, error) {
	return nil, nil
}

func TestHandleApply_RollbackOnCreateFailure(t *testing.T) {
	userRepo := &stubUserRepo{users: map[string]*domain.User{}, createErr: errors.New("db down")}
	inviteRepo := &stubInviteRepo{invite: &domain.InviteCode{ID: 7}}
	usageRepo := &stubInviteUsageRepo{}

	h := NewAuthHandler(nil, userRepo, nil, inviteRepo, usageRepo, true)

	payload := map[string]string{
		"username":   "user1",
		"password":   "pass1",
		"inviteCode": "CODE123",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/admin/auth/apply", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if inviteRepo.rollbackCount != 1 {
		t.Fatalf("rollbackCount = %d, want 1", inviteRepo.rollbackCount)
	}
	if len(usageRepo.usages) != 1 {
		t.Fatalf("usage records = %d, want 1", len(usageRepo.usages))
	}
	if usageRepo.usages[0].Result != "failed" {
		t.Fatalf("usage result = %s, want failed", usageRepo.usages[0].Result)
	}
}

func TestHandleApply_InviteCodeExpired(t *testing.T) {
	userRepo := &stubUserRepo{users: map[string]*domain.User{}}
	inviteRepo := &stubInviteRepo{consumeErr: domain.ErrInviteCodeExpired}

	h := NewAuthHandler(nil, userRepo, nil, inviteRepo, nil, true)

	payload := map[string]string{
		"username":   "user2",
		"password":   "pass2",
		"inviteCode": "CODE123",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/admin/auth/apply", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleApply_InviteCodeSystemError(t *testing.T) {
	userRepo := &stubUserRepo{users: map[string]*domain.User{}}
	inviteRepo := &stubInviteRepo{consumeErr: errors.New("db down")}

	h := NewAuthHandler(nil, userRepo, nil, inviteRepo, nil, true)

	payload := map[string]string{
		"username":   "user3",
		"password":   "pass3",
		"inviteCode": "CODE123",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/admin/auth/apply", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}
