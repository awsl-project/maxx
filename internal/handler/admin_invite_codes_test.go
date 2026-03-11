package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/service"
)

type adminTestInviteCodeRepo struct {
	code         *domain.InviteCode
	list         []*domain.InviteCode
	updateErr    error
	deleteCalled bool
	updatedCode  *domain.InviteCode
}

func (r *adminTestInviteCodeRepo) Create(code *domain.InviteCode) error { return nil }
func (r *adminTestInviteCodeRepo) Update(tenantID uint64, code *domain.InviteCode) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.updatedCode = code
	return nil
}
func (r *adminTestInviteCodeRepo) Delete(tenantID uint64, id uint64) error {
	r.deleteCalled = true
	return nil
}
func (r *adminTestInviteCodeRepo) GetByID(tenantID uint64, id uint64) (*domain.InviteCode, error) {
	if r.code != nil && r.code.ID == id {
		return r.code, nil
	}
	return nil, domain.ErrNotFound
}
func (r *adminTestInviteCodeRepo) GetByCodeHash(tenantID uint64, codeHash string) (*domain.InviteCode, error) {
	return nil, domain.ErrNotFound
}
func (r *adminTestInviteCodeRepo) GetByCodeHashAny(codeHash string) (*domain.InviteCode, error) {
	return nil, domain.ErrNotFound
}
func (r *adminTestInviteCodeRepo) List(tenantID uint64) ([]*domain.InviteCode, error) {
	return r.list, nil
}
func (r *adminTestInviteCodeRepo) Consume(tenantID uint64, codeHash string, now time.Time) (*domain.InviteCode, error) {
	return nil, domain.ErrInviteCodeInvalid
}
func (r *adminTestInviteCodeRepo) RollbackConsume(tenantID uint64, usageID uint64) error {
	return nil
}

func newAdminHandlerForInviteCodeTests(inviteRepo *adminTestInviteCodeRepo) *AdminHandler {
	adminSvc := service.NewTestAdminService(inviteRepo)
	return NewAdminHandler(adminSvc, nil, "")
}

func TestAdminHandler_UpdateInviteCode_NotFoundReturns404(t *testing.T) {
	inviteRepo := &adminTestInviteCodeRepo{
		code:      &domain.InviteCode{ID: 1, TenantID: 1},
		updateErr: domain.ErrNotFound,
	}
	h := newAdminHandlerForInviteCodeTests(inviteRepo)

	body, err := json.Marshal(map[string]any{})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/admin/invite-codes/1", bytes.NewReader(body))
	ctx := maxxctx.WithUserRole(req.Context(), string(domain.UserRoleAdmin))
	ctx = maxxctx.WithTenantID(ctx, 1)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestAdminHandler_UpdateInviteCode_AllowsZeroMaxUses(t *testing.T) {
	inviteRepo := &adminTestInviteCodeRepo{
		code: &domain.InviteCode{
			ID:        1,
			TenantID:  1,
			Status:    domain.InviteCodeStatusActive,
			MaxUses:   5,
			UsedCount: 3,
		},
	}
	h := newAdminHandlerForInviteCodeTests(inviteRepo)

	body, err := json.Marshal(map[string]any{"maxUses": 0})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/admin/invite-codes/1", bytes.NewReader(body))
	ctx := maxxctx.WithUserRole(req.Context(), string(domain.UserRoleAdmin))
	ctx = maxxctx.WithTenantID(ctx, 1)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if inviteRepo.updatedCode == nil || inviteRepo.updatedCode.MaxUses != 0 {
		t.Fatalf("updated maxUses = %v, want 0", inviteRepo.updatedCode)
	}
}

func TestAdminHandler_ListInviteCodes_MemberFiltersOwn(t *testing.T) {
	inviteRepo := &adminTestInviteCodeRepo{
		list: []*domain.InviteCode{
			{ID: 1, TenantID: 1, CreatedByUserID: 10},
			{ID: 2, TenantID: 1, CreatedByUserID: 20},
		},
	}
	h := newAdminHandlerForInviteCodeTests(inviteRepo)

	req := httptest.NewRequest(http.MethodGet, "/admin/invite-codes", nil)
	ctx := maxxctx.WithUserRole(req.Context(), string(domain.UserRoleMember))
	ctx = maxxctx.WithTenantID(ctx, 1)
	ctx = maxxctx.WithUserID(ctx, 10)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var result []domain.InviteCode
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(result) != 1 || result[0].ID != 1 {
		t.Fatalf("filtered result = %+v, want only invite code 1", result)
	}
}

func TestAdminHandler_GetInviteCode_MemberOwnAllowed(t *testing.T) {
	inviteRepo := &adminTestInviteCodeRepo{
		code: &domain.InviteCode{ID: 1, TenantID: 1, CreatedByUserID: 10},
	}
	h := newAdminHandlerForInviteCodeTests(inviteRepo)

	req := httptest.NewRequest(http.MethodGet, "/admin/invite-codes/1", nil)
	ctx := maxxctx.WithUserRole(req.Context(), string(domain.UserRoleMember))
	ctx = maxxctx.WithTenantID(ctx, 1)
	ctx = maxxctx.WithUserID(ctx, 10)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestAdminHandler_GetInviteCode_MemberOtherForbidden(t *testing.T) {
	inviteRepo := &adminTestInviteCodeRepo{
		code: &domain.InviteCode{ID: 1, TenantID: 1, CreatedByUserID: 20},
	}
	h := newAdminHandlerForInviteCodeTests(inviteRepo)

	req := httptest.NewRequest(http.MethodGet, "/admin/invite-codes/1", nil)
	ctx := maxxctx.WithUserRole(req.Context(), string(domain.UserRoleMember))
	ctx = maxxctx.WithTenantID(ctx, 1)
	ctx = maxxctx.WithUserID(ctx, 10)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestAdminHandler_DeleteInviteCode_MemberOtherForbidden(t *testing.T) {
	inviteRepo := &adminTestInviteCodeRepo{
		code: &domain.InviteCode{ID: 1, TenantID: 1, CreatedByUserID: 20},
	}
	h := newAdminHandlerForInviteCodeTests(inviteRepo)

	req := httptest.NewRequest(http.MethodDelete, "/admin/invite-codes/1", nil)
	ctx := maxxctx.WithUserRole(req.Context(), string(domain.UserRoleMember))
	ctx = maxxctx.WithTenantID(ctx, 1)
	ctx = maxxctx.WithUserID(ctx, 10)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
	if inviteRepo.deleteCalled {
		t.Fatalf("delete should not be called for unauthorized member")
	}
}

func TestAdminHandler_InviteCodeUsages_MemberOtherForbidden(t *testing.T) {
	inviteRepo := &adminTestInviteCodeRepo{
		code: &domain.InviteCode{ID: 1, TenantID: 1, CreatedByUserID: 20},
	}
	h := newAdminHandlerForInviteCodeTests(inviteRepo)

	req := httptest.NewRequest(http.MethodGet, "/admin/invite-codes/1/usages", nil)
	ctx := maxxctx.WithUserRole(req.Context(), string(domain.UserRoleMember))
	ctx = maxxctx.WithTenantID(ctx, 1)
	ctx = maxxctx.WithUserID(ctx, 10)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}
