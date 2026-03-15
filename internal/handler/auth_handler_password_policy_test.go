package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"golang.org/x/crypto/bcrypt"
)

type registerTestUserRepo struct {
	usersByID       map[uint64]*domain.User
	usersByUsername map[string]*domain.User
	nextID          uint64
	createCalls     int
	updateCalls     int
}

func newRegisterTestUserRepo(users ...*domain.User) *registerTestUserRepo {
	repo := &registerTestUserRepo{
		usersByID:       make(map[uint64]*domain.User, len(users)),
		usersByUsername: make(map[string]*domain.User, len(users)),
		nextID:          uint64(len(users)),
	}
	for _, user := range users {
		cloned := *user
		repo.usersByID[user.ID] = &cloned
		repo.usersByUsername[user.Username] = &cloned
		if user.ID > repo.nextID {
			repo.nextID = user.ID
		}
	}
	return repo
}

func (r *registerTestUserRepo) Create(user *domain.User) error {
	r.createCalls++
	if _, exists := r.usersByUsername[user.Username]; exists {
		return domain.ErrAlreadyExists
	}
	r.nextID++
	cloned := *user
	cloned.ID = r.nextID
	user.ID = cloned.ID
	r.usersByID[cloned.ID] = &cloned
	r.usersByUsername[cloned.Username] = &cloned
	return nil
}

func (r *registerTestUserRepo) Update(user *domain.User) error {
	r.updateCalls++
	existing, ok := r.usersByID[user.ID]
	if !ok {
		return domain.ErrNotFound
	}
	if existing.Username != user.Username {
		delete(r.usersByUsername, existing.Username)
	}
	cloned := *user
	r.usersByID[user.ID] = &cloned
	r.usersByUsername[user.Username] = &cloned
	return nil
}

func (r *registerTestUserRepo) Delete(tenantID uint64, id uint64) error {
	user, ok := r.usersByID[id]
	if !ok {
		return domain.ErrNotFound
	}
	delete(r.usersByID, id)
	delete(r.usersByUsername, user.Username)
	return nil
}

func (r *registerTestUserRepo) GetByID(tenantID uint64, id uint64) (*domain.User, error) {
	user, ok := r.usersByID[id]
	if !ok || (tenantID > 0 && user.TenantID != tenantID) {
		return nil, domain.ErrNotFound
	}
	cloned := *user
	return &cloned, nil
}

func (r *registerTestUserRepo) GetByUsername(username string) (*domain.User, error) {
	user, ok := r.usersByUsername[username]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cloned := *user
	return &cloned, nil
}

func (r *registerTestUserRepo) GetDefault() (*domain.User, error) { return nil, domain.ErrNotFound }
func (r *registerTestUserRepo) List() ([]*domain.User, error)     { return nil, nil }
func (r *registerTestUserRepo) ListByTenant(tenantID uint64) ([]*domain.User, error) {
	return nil, nil
}
func (r *registerTestUserRepo) ListByTenantAndStatus(tenantID uint64, status domain.UserStatus) ([]*domain.User, error) {
	return nil, nil
}
func (r *registerTestUserRepo) CountActive() (int64, error) { return 0, nil }

func TestIsValidRegistrationPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		want     bool
	}{
		{name: "valid", password: "Abcd123!", want: true},
		{name: "missing number", password: "Abcdefg!", want: false},
		{name: "missing letter", password: "1234567!", want: false},
		{name: "missing punctuation", password: "Abcd1234", want: false},
		{name: "too short", password: "Ab1!xyz", want: false},
		{name: "supports slash punctuation", password: "Abcd123/", want: true},
		{name: "supports backslash punctuation", password: "Abcd123\\", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidRegistrationPassword(tt.password); got != tt.want {
				t.Fatalf("isValidRegistrationPassword(%q) = %v, want %v", tt.password, got, tt.want)
			}
		})
	}
}

func TestHandleRegister_InvalidPasswordRejected(t *testing.T) {
	admin := &domain.User{
		ID:       1,
		TenantID: domain.DefaultTenantID,
		Username: "admin",
		Role:     domain.UserRoleAdmin,
		Status:   domain.UserStatusActive,
	}
	userRepo := newRegisterTestUserRepo(admin)
	authMiddleware := NewAuthMiddleware(nil)
	handler := NewAuthHandler(authMiddleware, userRepo, &passkeyTestTenantRepo{}, nil, nil, true)

	token, err := authMiddleware.GenerateToken(admin)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	payload := map[string]string{
		"username": "new-member",
		"password": "weakpass",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/admin/auth/register", bytes.NewReader(body))
	req.Header.Set(AuthHeader, "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["code"] != registrationPasswordValidationCode {
		t.Fatalf("code = %v, want %s", result["code"], registrationPasswordValidationCode)
	}
	if userRepo.createCalls != 0 {
		t.Fatalf("createCalls = %d, want 0", userRepo.createCalls)
	}
	if _, err := userRepo.GetByUsername("new-member"); err == nil {
		t.Fatalf("user should not be created on invalid password")
	}
}

func TestHandleChangePassword_InvalidNewPasswordRejected(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("Oldpass1!"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	admin := &domain.User{
		ID:           1,
		TenantID:     domain.DefaultTenantID,
		Username:     "admin",
		PasswordHash: string(hash),
		Role:         domain.UserRoleAdmin,
		Status:       domain.UserStatusActive,
	}
	userRepo := newRegisterTestUserRepo(admin)
	authMiddleware := NewAuthMiddleware(nil)
	handler := NewAuthHandler(authMiddleware, userRepo, &passkeyTestTenantRepo{}, nil, nil, true)

	token, err := authMiddleware.GenerateToken(admin)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	payload := map[string]string{
		"oldPassword": "Oldpass1!",
		"newPassword": "weakpass",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPut, "/admin/auth/password", bytes.NewReader(body))
	req.Header.Set(AuthHeader, "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["code"] != registrationPasswordValidationCode {
		t.Fatalf("code = %v, want %s", result["code"], registrationPasswordValidationCode)
	}
	if userRepo.updateCalls != 0 {
		t.Fatalf("updateCalls = %d, want 0", userRepo.updateCalls)
	}
	updatedUser, err := userRepo.GetByID(domain.DefaultTenantID, admin.ID)
	if err != nil {
		t.Fatalf("get user after password change: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(updatedUser.PasswordHash), []byte("Oldpass1!")); err != nil {
		t.Fatalf("password hash should remain unchanged")
	}
}
