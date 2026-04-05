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

type stubInviteUserRepo struct {
	users     map[string]*domain.User
	nextID    uint64
	createErr error
}

func (r *stubInviteUserRepo) Create(user *domain.User) error {
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

func (r *stubInviteUserRepo) Update(user *domain.User) error          { return nil }
func (r *stubInviteUserRepo) Delete(tenantID uint64, id uint64) error { return nil }
func (r *stubInviteUserRepo) GetByID(tenantID uint64, id uint64) (*domain.User, error) {
	return nil, domain.ErrNotFound
}
func (r *stubInviteUserRepo) GetByUsername(username string) (*domain.User, error) {
	if u, ok := r.users[username]; ok {
		return u, nil
	}
	return nil, domain.ErrNotFound
}
func (r *stubInviteUserRepo) GetDefault() (*domain.User, error)                    { return nil, domain.ErrNotFound }
func (r *stubInviteUserRepo) List() ([]*domain.User, error)                        { return nil, nil }
func (r *stubInviteUserRepo) ListByTenant(tenantID uint64) ([]*domain.User, error) { return nil, nil }
func (r *stubInviteUserRepo) ListByTenantAndStatus(tenantID uint64, status domain.UserStatus) ([]*domain.User, error) {
	return nil, nil
}
func (r *stubInviteUserRepo) CountActive() (int64, error) { return 0, nil }

type stubInviteRepo struct {
	invite              *domain.InviteCode
	consumeErr          error
	consumeCalled       bool
	lastConsumeTenantID uint64
	rollbackCount       int
	lastRollbackUsageID uint64
}

func (r *stubInviteRepo) Create(code *domain.InviteCode) error                  { return nil }
func (r *stubInviteRepo) Update(tenantID uint64, code *domain.InviteCode) error { return nil }
func (r *stubInviteRepo) Delete(tenantID uint64, id uint64) error               { return nil }
func (r *stubInviteRepo) GetByID(tenantID uint64, id uint64) (*domain.InviteCode, error) {
	return nil, domain.ErrNotFound
}
func (r *stubInviteRepo) GetByCodeHash(tenantID uint64, codeHash string) (*domain.InviteCode, error) {
	return nil, domain.ErrNotFound
}
func (r *stubInviteRepo) GetByCodeHashAny(codeHash string) (*domain.InviteCode, error) {
	if r.invite != nil && r.invite.CodeHash == codeHash {
		return r.invite, nil
	}
	return nil, domain.ErrNotFound
}
func (r *stubInviteRepo) List(tenantID uint64) ([]*domain.InviteCode, error) { return nil, nil }
func (r *stubInviteRepo) Consume(tenantID uint64, codeHash string, nowTime time.Time) (*domain.InviteCode, error) {
	r.consumeCalled = true
	r.lastConsumeTenantID = tenantID
	if r.consumeErr != nil {
		return nil, r.consumeErr
	}
	return r.invite, nil
}
func (r *stubInviteRepo) RollbackConsume(tenantID uint64, usageID uint64) error {
	r.rollbackCount++
	r.lastRollbackUsageID = usageID
	return nil
}

type stubInviteUsageRepo struct {
	usages    []*domain.InviteCodeUsage
	nextID    uint64
	createErr error
}

func (r *stubInviteUsageRepo) Create(usage *domain.InviteCodeUsage) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.nextID++
	usage.ID = r.nextID
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
	userRepo := &stubInviteUserRepo{users: map[string]*domain.User{}, createErr: errors.New("db down")}
	codeHash := domain.HashInviteCode("CODE123")
	inviteRepo := &stubInviteRepo{invite: &domain.InviteCode{ID: 7, TenantID: 1, CodeHash: codeHash}}
	usageRepo := &stubInviteUsageRepo{}

	h := NewAuthHandler(nil, userRepo, nil, inviteRepo, usageRepo, true)

	payload := map[string]string{
		"username":   "user1",
		"password":   "Pass1!aa",
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
	if inviteRepo.lastRollbackUsageID != usageRepo.usages[0].ID {
		t.Fatalf("lastRollbackUsageID = %d, want %d", inviteRepo.lastRollbackUsageID, usageRepo.usages[0].ID)
	}
	if usageRepo.usages[0].Result != "failed" {
		t.Fatalf("usage result = %s, want failed", usageRepo.usages[0].Result)
	}
}

func TestHandleApply_InviteCodeExpired(t *testing.T) {
	userRepo := &stubInviteUserRepo{users: map[string]*domain.User{}}
	codeHash := domain.HashInviteCode("CODE123")
	inviteRepo := &stubInviteRepo{
		invite:     &domain.InviteCode{ID: 7, TenantID: 1, CodeHash: codeHash},
		consumeErr: domain.ErrInviteCodeExpired,
	}

	h := NewAuthHandler(nil, userRepo, nil, inviteRepo, nil, true)

	payload := map[string]string{
		"username":   "user2",
		"password":   "Pass2!aa",
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
	userRepo := &stubInviteUserRepo{users: map[string]*domain.User{}}
	codeHash := domain.HashInviteCode("CODE123")
	inviteRepo := &stubInviteRepo{
		invite:     &domain.InviteCode{ID: 7, TenantID: 1, CodeHash: codeHash},
		consumeErr: errors.New("db down"),
	}

	h := NewAuthHandler(nil, userRepo, nil, inviteRepo, nil, true)

	payload := map[string]string{
		"username":   "user3",
		"password":   "Pass3!aa",
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

func TestHandleApply_RollbackWithoutUsageRepo(t *testing.T) {
	userRepo := &stubInviteUserRepo{users: map[string]*domain.User{}, createErr: errors.New("db down")}
	codeHash := domain.HashInviteCode("CODE123")
	inviteRepo := &stubInviteRepo{invite: &domain.InviteCode{ID: 9, TenantID: 1, CodeHash: codeHash}}

	h := NewAuthHandler(nil, userRepo, nil, inviteRepo, nil, true)

	payload := map[string]string{
		"username":   "user4",
		"password":   "Pass4!aa",
		"inviteCode": "CODE123",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/admin/auth/apply", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if inviteRepo.rollbackCount != 0 {
		t.Fatalf("rollbackCount = %d, want 0", inviteRepo.rollbackCount)
	}
}

func TestHandleApply_RollbackWhenUsageCreateFails(t *testing.T) {
	userRepo := &stubInviteUserRepo{users: map[string]*domain.User{}, createErr: errors.New("db down")}
	codeHash := domain.HashInviteCode("CODE123")
	inviteRepo := &stubInviteRepo{invite: &domain.InviteCode{ID: 10, TenantID: 1, CodeHash: codeHash}}
	usageRepo := &stubInviteUsageRepo{createErr: errors.New("usage down")}

	h := NewAuthHandler(nil, userRepo, nil, inviteRepo, usageRepo, true)

	payload := map[string]string{
		"username":   "user6",
		"password":   "Pass6!aa",
		"inviteCode": "CODE123",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/admin/auth/apply", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	if inviteRepo.rollbackCount != 0 {
		t.Fatalf("rollbackCount = %d, want 0", inviteRepo.rollbackCount)
	}
}

func TestHandleApply_ResolveTenantFromInvite(t *testing.T) {
	userRepo := &stubInviteUserRepo{users: map[string]*domain.User{}}
	codeHash := domain.HashInviteCode("CODE123")
	inviteRepo := &stubInviteRepo{invite: &domain.InviteCode{ID: 11, TenantID: 42, CodeHash: codeHash}}

	h := NewAuthHandler(nil, userRepo, nil, inviteRepo, nil, true)

	payload := map[string]string{
		"username":   "user5",
		"password":   "Pass5!aa",
		"inviteCode": "CODE123",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/admin/auth/apply", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	created := userRepo.users["user5"]
	if created == nil {
		t.Fatalf("user not created")
	}
	if created.TenantID != 42 {
		t.Fatalf("tenantID = %d, want 42", created.TenantID)
	}
	if inviteRepo.lastConsumeTenantID != 42 {
		t.Fatalf("consume tenantID = %d, want 42", inviteRepo.lastConsumeTenantID)
	}
}

func TestHandleApply_InvalidPasswordRejectedBeforeInviteConsume(t *testing.T) {
	userRepo := &stubInviteUserRepo{users: map[string]*domain.User{}}
	codeHash := domain.HashInviteCode("CODE123")
	inviteRepo := &stubInviteRepo{invite: &domain.InviteCode{ID: 12, TenantID: 42, CodeHash: codeHash}}

	h := NewAuthHandler(nil, userRepo, nil, inviteRepo, nil, true)

	payload := map[string]string{
		"username":   "user7",
		"password":   "weakpass",
		"inviteCode": "CODE123",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/admin/auth/apply", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if inviteRepo.consumeCalled {
		t.Fatalf("invite consume should not be called for invalid password")
	}
	if len(userRepo.users) != 0 {
		t.Fatalf("unexpected users created: %d", len(userRepo.users))
	}
}
