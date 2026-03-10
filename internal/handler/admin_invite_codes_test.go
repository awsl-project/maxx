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
	code      *domain.InviteCode
	updateErr error
}

func (r *adminTestInviteCodeRepo) Create(code *domain.InviteCode) error { return nil }
func (r *adminTestInviteCodeRepo) Update(tenantID uint64, code *domain.InviteCode) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	return nil
}
func (r *adminTestInviteCodeRepo) Delete(tenantID uint64, id uint64) error { return nil }
func (r *adminTestInviteCodeRepo) GetByID(tenantID uint64, id uint64) (*domain.InviteCode, error) {
	if r.code != nil && r.code.ID == id {
		return r.code, nil
	}
	return nil, domain.ErrNotFound
}
func (r *adminTestInviteCodeRepo) GetByCodeHash(tenantID uint64, codeHash string) (*domain.InviteCode, error) {
	return nil, domain.ErrNotFound
}
func (r *adminTestInviteCodeRepo) List(tenantID uint64) ([]*domain.InviteCode, error) { return nil, nil }
func (r *adminTestInviteCodeRepo) Consume(tenantID uint64, codeHash string, now time.Time) (*domain.InviteCode, error) {
	return nil, domain.ErrInviteCodeInvalid
}
func (r *adminTestInviteCodeRepo) RollbackConsume(tenantID uint64, id uint64) error { return nil }

func newAdminHandlerForInviteCodeTests(inviteRepo *adminTestInviteCodeRepo) *AdminHandler {
	adminSvc := service.NewAdminService(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		inviteRepo,
		nil,
		nil,
		nil,
		nil,
		nil,
		"",
		nil,
		nil,
		nil,
	)
	return NewAdminHandler(adminSvc, nil, "")
}

func TestAdminHandler_UpdateInviteCode_NotFoundReturns404(t *testing.T) {
	inviteRepo := &adminTestInviteCodeRepo{
		code: &domain.InviteCode{ID: 1, TenantID: 1},
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
