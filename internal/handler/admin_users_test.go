package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
)

type stubUserRepo struct {
	users map[uint64]*domain.User
}

func newStubUserRepo() *stubUserRepo {
	return &stubUserRepo{users: map[uint64]*domain.User{}}
}

func (r *stubUserRepo) Create(user *domain.User) error { return nil }
func (r *stubUserRepo) Update(user *domain.User) error {
	if _, ok := r.users[user.ID]; ok {
		r.users[user.ID] = user
		return nil
	}
	return domain.ErrNotFound
}
func (r *stubUserRepo) Delete(tenantID uint64, id uint64) error { return nil }
func (r *stubUserRepo) GetByID(tenantID uint64, id uint64) (*domain.User, error) {
	if user, ok := r.users[id]; ok {
		return user, nil
	}
	return nil, domain.ErrNotFound
}
func (r *stubUserRepo) GetByUsername(username string) (*domain.User, error) {
	return nil, domain.ErrNotFound
}
func (r *stubUserRepo) GetDefault() (*domain.User, error)                    { return nil, domain.ErrNotFound }
func (r *stubUserRepo) List() ([]*domain.User, error)                        { return nil, nil }
func (r *stubUserRepo) ListByTenant(tenantID uint64) ([]*domain.User, error) { return nil, nil }
func (r *stubUserRepo) ListByTenantAndStatus(tenantID uint64, status domain.UserStatus) ([]*domain.User, error) {
	return nil, nil
}
func (r *stubUserRepo) CountActive() (int64, error) { return 0, nil }

func TestAdminHandler_ApproveUser_AllowsWithoutInviteCode(t *testing.T) {
	repo := newStubUserRepo()
	h := NewAdminHandler(nil, nil, "")
	h.SetUserRepo(repo)

	user := &domain.User{
		ID:       7,
		TenantID: 1,
		Username: "pending-no-invite",
		Role:     domain.UserRoleMember,
		Status:   domain.UserStatusPending,
	}
	repo.users[user.ID] = user

	req := httptest.NewRequest(http.MethodPut, "/admin/users/7/approve", nil)
	ctx := maxxctx.WithUserRole(req.Context(), string(domain.UserRoleAdmin))
	ctx = maxxctx.WithTenantID(ctx, 1)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if repo.users[user.ID].Status != domain.UserStatusActive {
		t.Fatalf("status = %s, want %s", repo.users[user.ID].Status, domain.UserStatusActive)
	}
}
