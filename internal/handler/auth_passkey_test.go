package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

type passkeyTestUserRepo struct {
	users map[uint64]*domain.User
}

func newPasskeyTestUserRepo(users ...*domain.User) *passkeyTestUserRepo {
	repo := &passkeyTestUserRepo{users: make(map[uint64]*domain.User, len(users))}
	for _, user := range users {
		cloned := *user
		repo.users[user.ID] = &cloned
	}
	return repo
}

func (r *passkeyTestUserRepo) Create(user *domain.User) error {
	cloned := *user
	r.users[user.ID] = &cloned
	return nil
}

func (r *passkeyTestUserRepo) Update(user *domain.User) error {
	if _, ok := r.users[user.ID]; !ok {
		return domain.ErrNotFound
	}
	cloned := *user
	r.users[user.ID] = &cloned
	return nil
}

func (r *passkeyTestUserRepo) Delete(tenantID uint64, id uint64) error {
	if _, ok := r.users[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.users, id)
	return nil
}

func (r *passkeyTestUserRepo) GetByID(tenantID uint64, id uint64) (*domain.User, error) {
	user, ok := r.users[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	if tenantID > 0 && user.TenantID != tenantID {
		return nil, domain.ErrNotFound
	}
	cloned := *user
	return &cloned, nil
}

func (r *passkeyTestUserRepo) GetByUsername(username string) (*domain.User, error) {
	for _, user := range r.users {
		if user.Username == username {
			cloned := *user
			return &cloned, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *passkeyTestUserRepo) GetDefault() (*domain.User, error) {
	return nil, domain.ErrNotFound
}

func (r *passkeyTestUserRepo) List() ([]*domain.User, error) {
	users := make([]*domain.User, 0, len(r.users))
	for _, user := range r.users {
		cloned := *user
		users = append(users, &cloned)
	}
	return users, nil
}

func (r *passkeyTestUserRepo) ListByTenant(tenantID uint64) ([]*domain.User, error) {
	users := make([]*domain.User, 0, len(r.users))
	for _, user := range r.users {
		if user.TenantID != tenantID {
			continue
		}
		cloned := *user
		users = append(users, &cloned)
	}
	return users, nil
}

func (r *passkeyTestUserRepo) ListByTenantAndStatus(tenantID uint64, status domain.UserStatus) ([]*domain.User, error) {
	users := make([]*domain.User, 0, len(r.users))
	for _, user := range r.users {
		if user.TenantID != tenantID || user.Status != status {
			continue
		}
		cloned := *user
		users = append(users, &cloned)
	}
	return users, nil
}

func (r *passkeyTestUserRepo) CountActive() (int64, error) {
	var count int64
	for _, user := range r.users {
		if user.Status == domain.UserStatusActive {
			count++
		}
	}
	return count, nil
}

type passkeyTestTenantRepo struct{}

func (r *passkeyTestTenantRepo) Create(tenant *domain.Tenant) error { return nil }
func (r *passkeyTestTenantRepo) Update(tenant *domain.Tenant) error { return nil }
func (r *passkeyTestTenantRepo) Delete(id uint64) error             { return nil }
func (r *passkeyTestTenantRepo) GetByID(id uint64) (*domain.Tenant, error) {
	return nil, domain.ErrNotFound
}
func (r *passkeyTestTenantRepo) GetBySlug(slug string) (*domain.Tenant, error) {
	return nil, domain.ErrNotFound
}
func (r *passkeyTestTenantRepo) GetDefault() (*domain.Tenant, error) { return nil, domain.ErrNotFound }
func (r *passkeyTestTenantRepo) List() ([]*domain.Tenant, error)     { return []*domain.Tenant{}, nil }

func newPasskeyTestUser(t *testing.T) *domain.User {
	t.Helper()

	credentials := []webauthn.Credential{
		{
			ID:        []byte("credential-1"),
			Transport: []protocol.AuthenticatorTransport{protocol.AuthenticatorTransport("usb")},
			Flags: webauthn.CredentialFlags{
				BackupEligible: true,
				BackupState:    true,
			},
			Authenticator: webauthn.Authenticator{
				SignCount:  3,
				Attachment: protocol.AuthenticatorAttachment("platform"),
			},
		},
		{
			ID:        []byte("credential-2"),
			Transport: []protocol.AuthenticatorTransport{protocol.AuthenticatorTransport("internal")},
			Authenticator: webauthn.Authenticator{
				SignCount:  8,
				Attachment: protocol.AuthenticatorAttachment("cross-platform"),
			},
		},
	}

	encoded, err := encodePasskeyCredentials(credentials)
	if err != nil {
		t.Fatalf("encode credentials: %v", err)
	}

	now := time.Now()
	return &domain.User{
		ID:                 1,
		TenantID:           domain.DefaultTenantID,
		Username:           "alice",
		PasswordHash:       "hashed",
		PasskeyCredentials: encoded,
		Role:               domain.UserRoleAdmin,
		Status:             domain.UserStatusActive,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

func newPasskeyHandlerAndToken(t *testing.T, user *domain.User) (*AuthHandler, string, *passkeyTestUserRepo) {
	t.Helper()

	userRepo := newPasskeyTestUserRepo(user)
	authMiddleware := NewAuthMiddleware(nil)
	handler := NewAuthHandler(authMiddleware, userRepo, &passkeyTestTenantRepo{}, true)

	token, err := authMiddleware.GenerateToken(user)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	return handler, token, userRepo
}

func TestAuthHandler_PasskeyCredentialList(t *testing.T) {
	user := newPasskeyTestUser(t)
	handler, token, _ := newPasskeyHandlerAndToken(t, user)

	req := httptest.NewRequest(http.MethodGet, "/admin/auth/passkey/credentials", nil)
	req.Header.Set(AuthHeader, "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var response struct {
		Success     bool                    `json:"success"`
		Credentials []passkeyCredentialInfo `json:"credentials"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Success {
		t.Fatalf("success = false")
	}
	if len(response.Credentials) != 2 {
		t.Fatalf("credential count = %d, want 2", len(response.Credentials))
	}
	if response.Credentials[0].ID != encodeCredentialID([]byte("credential-1")) {
		t.Fatalf("first credential ID mismatch: %s", response.Credentials[0].ID)
	}
}

func TestAuthHandler_PasskeyCredentialDelete(t *testing.T) {
	user := newPasskeyTestUser(t)
	handler, token, userRepo := newPasskeyHandlerAndToken(t, user)

	credentialID := encodeCredentialID([]byte("credential-1"))
	req := httptest.NewRequest(http.MethodDelete, "/admin/auth/passkey/credentials/"+credentialID, nil)
	req.Header.Set(AuthHeader, "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	updatedUser, err := userRepo.GetByID(domain.DefaultTenantID, user.ID)
	if err != nil {
		t.Fatalf("get updated user: %v", err)
	}
	remaining, err := parsePasskeyCredentials(updatedUser.PasskeyCredentials)
	if err != nil {
		t.Fatalf("parse remaining credentials: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("remaining credentials = %d, want 1", len(remaining))
	}
	if string(remaining[0].ID) != "credential-2" {
		t.Fatalf("remaining credential = %q, want credential-2", string(remaining[0].ID))
	}
}

func TestAuthHandler_PasskeyCredentialDelete_NotFound(t *testing.T) {
	user := newPasskeyTestUser(t)
	handler, token, _ := newPasskeyHandlerAndToken(t, user)

	req := httptest.NewRequest(http.MethodDelete, "/admin/auth/passkey/credentials/"+encodeCredentialID([]byte("missing")), nil)
	req.Header.Set(AuthHeader, "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}
