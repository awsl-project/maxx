package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
	"github.com/awsl-project/maxx/internal/service"
)

func newAdminHandlerWithRequestRepos(t *testing.T) (*AdminHandler, *sqlite.APITokenRepository, *sqlite.ProxyRequestRepository) {
	t.Helper()

	db, err := sqlite.NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("NewDBWithDSN() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	settingsRepo := sqlite.NewSystemSettingRepository(db)
	if err := settingsRepo.Set("ui_multitenant_enabled", "true"); err != nil {
		t.Fatalf("enable multitenant setting: %v", err)
	}
	if err := settingsRepo.Set("ui_multitenant_layout", "user_panel"); err != nil {
		t.Fatalf("enable user panel layout setting: %v", err)
	}

	apiTokenRepo := sqlite.NewAPITokenRepository(db)
	proxyRequestRepo := sqlite.NewProxyRequestRepository(db)
	attemptRepo := sqlite.NewProxyUpstreamAttemptRepository(db)

	adminSvc := service.NewAdminService(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		proxyRequestRepo,
		attemptRepo,
		settingsRepo,
		apiTokenRepo,
		nil,
		nil,
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

	return NewAdminHandler(adminSvc, nil, ""), apiTokenRepo, proxyRequestRepo
}

func createUserPanelTokenForTest(t *testing.T, repo *sqlite.APITokenRepository, tenantID uint64, userID uint64) *domain.APIToken {
	t.Helper()
	token := &domain.APIToken{
		TenantID:    tenantID,
		Name:        userPanelAPITokenName(userID),
		Description: userPanelAPITokenDescription(userID),
		Token:       "test-token",
		TokenPrefix: "test",
		IsEnabled:   true,
	}
	if err := repo.Create(token); err != nil {
		t.Fatalf("create user panel token: %v", err)
	}
	return token
}

func createRegularTokenForTest(t *testing.T, repo *sqlite.APITokenRepository, tenantID uint64) *domain.APIToken {
	t.Helper()
	token := &domain.APIToken{
		TenantID:    tenantID,
		Name:        "regular",
		Description: "regular token",
		Token:       "regular-token",
		TokenPrefix: "regular",
		IsEnabled:   true,
	}
	if err := repo.Create(token); err != nil {
		t.Fatalf("create regular token: %v", err)
	}
	return token
}

func createProxyRequestForTokenTest(t *testing.T, repo *sqlite.ProxyRequestRepository, tenantID uint64, apiTokenID uint64, requestID string) *domain.ProxyRequest {
	t.Helper()
	now := time.Now().UTC()
	req := &domain.ProxyRequest{
		TenantID:     tenantID,
		InstanceID:   "test-instance",
		RequestID:    requestID,
		SessionID:    requestID + "-session",
		ClientType:   domain.ClientTypeOpenAI,
		RequestModel: "gpt-test",
		StartTime:    now,
		EndTime:      now.Add(time.Second),
		Duration:     time.Second,
		Status:       "COMPLETED",
		StatusCode:   http.StatusOK,
		APITokenID:   apiTokenID,
	}
	if err := repo.Create(req); err != nil {
		t.Fatalf("create proxy request: %v", err)
	}
	return req
}

func newUserPanelMemberAdminRequest(method, path string, userID uint64) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	ctx := maxxctx.WithTenantID(req.Context(), 1)
	ctx = maxxctx.WithUserID(ctx, userID)
	ctx = maxxctx.WithUserRole(ctx, string(domain.UserRoleMember))
	return req.WithContext(ctx)
}

func TestAdminHandler_UserPanelMemberRequestsAreScopedToCurrentUserToken(t *testing.T) {
	handler, tokenRepo, requestRepo := newAdminHandlerWithRequestRepos(t)
	memberToken := createUserPanelTokenForTest(t, tokenRepo, 1, 9)
	regularToken := createRegularTokenForTest(t, tokenRepo, 1)
	memberReq := createProxyRequestForTokenTest(t, requestRepo, 1, memberToken.ID, "member-request")
	otherReq := createProxyRequestForTokenTest(t, requestRepo, 1, regularToken.ID, "other-request")

	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, newUserPanelMemberAdminRequest(http.MethodGet, "/admin/requests?limit=100", 9))
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d, body = %s", listRec.Code, http.StatusOK, listRec.Body.String())
	}
	var list service.CursorPaginationResult
	if err := json.Unmarshal(listRec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].ID != memberReq.ID {
		t.Fatalf("member list items = %+v, want only request %d", list.Items, memberReq.ID)
	}

	countRec := httptest.NewRecorder()
	handler.ServeHTTP(countRec, newUserPanelMemberAdminRequest(http.MethodGet, "/admin/requests/count", 9))
	if countRec.Code != http.StatusOK {
		t.Fatalf("count status = %d, want %d, body = %s", countRec.Code, http.StatusOK, countRec.Body.String())
	}
	var count int64
	if err := json.Unmarshal(countRec.Body.Bytes(), &count); err != nil {
		t.Fatalf("decode count: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}

	detailRec := httptest.NewRecorder()
	handler.ServeHTTP(detailRec, newUserPanelMemberAdminRequest(http.MethodGet, "/admin/requests/"+jsonNumber(otherReq.ID), 9))
	if detailRec.Code != http.StatusNotFound {
		t.Fatalf("other detail status = %d, want %d, body = %s", detailRec.Code, http.StatusNotFound, detailRec.Body.String())
	}
}

func jsonNumber(n uint64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
