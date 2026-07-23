package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository/cached"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
	"github.com/awsl-project/maxx/internal/service"
)

func newAdminHandlerForAPITokenTests(t *testing.T) (*AdminHandler, *sqlite.APITokenRepository) {
	t.Helper()
	db, err := sqlite.NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("NewDBWithDSN() error = %v", err)
	}
	baseRepo := sqlite.NewAPITokenRepository(db)
	apiTokenRepo := cached.NewAPITokenRepository(baseRepo)
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
		apiTokenRepo,
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
	return NewAdminHandler(adminSvc, nil, ""), baseRepo
}

func newAdminAPITokenRequest(method string, path string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	ctx := maxxctx.WithUserRole(req.Context(), string(domain.UserRoleAdmin))
	ctx = maxxctx.WithTenantID(ctx, 1)
	return req.WithContext(ctx)
}

func TestAdminHandlerResetAPITokenValidityClearsExpiryAndActivityState(t *testing.T) {
	h, repo := newAdminHandlerForAPITokenTests(t)
	now := time.Now().UTC()
	expiredAt := now.Add(-time.Hour)
	lastUsedAt := now.Add(-domain.APITokenInactiveExpiry - time.Minute)
	lastIPAt := now.Add(-2 * time.Hour)
	token := &domain.APIToken{
		TenantID:    1,
		Token:       "maxx_expired_reset_validity",
		TokenPrefix: "maxx_exp",
		Name:        "expired-token",
		ProjectID:   7,
		IsEnabled:   false,
		ExpiresAt:   &expiredAt,
		LastUsedAt:  &lastUsedAt,
		LastIP:      "203.0.113.9",
		LastIPAt:    &lastIPAt,
		UseCount:    12,
	}
	if err := repo.Create(token); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := newAdminAPITokenRequest(http.MethodPut, fmt.Sprintf("/admin/api-tokens/%d", token.ID))
	req.Body = io.NopCloser(strings.NewReader(`{"resetValidity":true}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	updated, err := repo.GetByID(1, token.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if !updated.IsEnabled {
		t.Fatalf("token IsEnabled = false, want true")
	}
	if updated.ExpiresAt != nil || updated.LastUsedAt != nil || updated.LastIPAt != nil || updated.LastIP != "" {
		t.Fatalf("activity/expiry state = expiresAt:%v lastUsedAt:%v lastIP:%q lastIPAt:%v, want cleared", updated.ExpiresAt, updated.LastUsedAt, updated.LastIP, updated.LastIPAt)
	}
	if updated.UseCount != 12 || updated.ProjectID != 7 || updated.Token != "maxx_expired_reset_validity" {
		t.Fatalf("stable fields changed unexpectedly: %+v", updated)
	}
}

func TestAdminHandlerResetAPITokenValidityClearsInactiveOnlyExpiry(t *testing.T) {
	h, repo := newAdminHandlerForAPITokenTests(t)
	lastUsedAt := time.Now().UTC().Add(-domain.APITokenInactiveExpiry - time.Minute)
	token := &domain.APIToken{
		TenantID:    1,
		Token:       "maxx_inactive_reset_validity",
		TokenPrefix: "maxx_ina",
		Name:        "inactive-token",
		IsEnabled:   true,
		LastUsedAt:  &lastUsedAt,
	}
	if err := repo.Create(token); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	rec := httptest.NewRecorder()
	req := newAdminAPITokenRequest(http.MethodPut, fmt.Sprintf("/admin/api-tokens/%d", token.ID))
	req.Body = io.NopCloser(strings.NewReader(`{"resetValidity":true}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	updated, err := repo.GetByID(1, token.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if updated.LastUsedAt != nil || !updated.IsEnabled {
		t.Fatalf("updated token = %+v, want active token with cleared lastUsedAt", updated)
	}
}

func TestAdminHandlerCleanupExpiredAPITokens(t *testing.T) {
	h, repo := newAdminHandlerForAPITokenTests(t)
	now := time.Now().UTC()
	expiredAt := now.Add(-time.Hour)
	futureAt := now.Add(time.Hour)
	oldLastUsedAt := now.Add(-domain.APITokenInactiveExpiry - time.Minute)

	seed := []*domain.APIToken{
		{TenantID: 1, Token: "maxx_expired_by_date", TokenPrefix: "maxx_exp", Name: "expired-by-date", IsEnabled: true, ExpiresAt: &expiredAt},
		{TenantID: 1, Token: "maxx_expired_by_inactivity", TokenPrefix: "maxx_ina", Name: "expired-by-inactivity", IsEnabled: true, LastUsedAt: &oldLastUsedAt},
		{TenantID: 1, Token: "maxx_active", TokenPrefix: "maxx_act", Name: "active", IsEnabled: true, ExpiresAt: &futureAt},
	}
	for _, token := range seed {
		if err := repo.Create(token); err != nil {
			t.Fatalf("Create(%s) error = %v", token.Name, err)
		}
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newAdminAPITokenRequest(http.MethodPost, "/admin/api-tokens/cleanup-expired"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var result domain.APITokenCleanupResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.DeletedCount != 2 || len(result.Tokens) != 2 {
		t.Fatalf("cleanup result = %#v, want two deleted tokens", result)
	}

	remaining, err := repo.List(1)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(remaining) != 1 || remaining[0].Name != "active" {
		t.Fatalf("remaining = %#v, want only active token", remaining)
	}
}

func TestAdminHandlerCleanupExpiredAPITokensRejectsNonPost(t *testing.T) {
	h, _ := newAdminHandlerForAPITokenTests(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newAdminAPITokenRequest(http.MethodGet, "/admin/api-tokens/cleanup-expired"))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusMethodNotAllowed, rec.Body.String())
	}
	if got := rec.Header().Get("Allow"); got != http.MethodPost {
		t.Fatalf("Allow = %q, want %q", got, http.MethodPost)
	}
}
