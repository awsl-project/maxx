package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"golang.org/x/crypto/bcrypt"
)

func newCookieAuthTestUser(t *testing.T) *domain.User {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret-pass"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	now := time.Now()
	return &domain.User{
		ID:           7,
		TenantID:     domain.DefaultTenantID,
		Username:     "admin",
		PasswordHash: string(hash),
		Role:         domain.UserRoleAdmin,
		Status:       domain.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func TestAuthMiddleware_WrapAcceptsHttpOnlyCookie(t *testing.T) {
	user := newCookieAuthTestUser(t)
	authMiddleware := NewAuthMiddleware(nil)
	token, err := authMiddleware.GenerateToken(user)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/projects", nil)
	req.AddCookie(&http.Cookie{Name: AuthCookieName, Value: token})
	rec := httptest.NewRecorder()

	wrapped := authMiddleware.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tenantID := maxxctx.GetTenantID(r.Context()); tenantID != user.TenantID {
			t.Fatalf("tenant ID = %d, want %d", tenantID, user.TenantID)
		}
		if userID := maxxctx.GetUserID(r.Context()); userID != user.ID {
			t.Fatalf("user ID = %d, want %d", userID, user.ID)
		}
		if role := maxxctx.GetUserRole(r.Context()); role != string(user.Role) {
			t.Fatalf("role = %q, want %q", role, user.Role)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
}

func TestAuthHandler_LoginSetsHttpOnlyCookieAndOmitsTokenBody(t *testing.T) {
	user := newCookieAuthTestUser(t)
	userRepo := newPasskeyTestUserRepo(user)
	authMiddleware := NewAuthMiddleware(nil)
	handler := NewAuthHandler(authMiddleware, userRepo, &passkeyTestTenantRepo{}, true)

	body, err := json.Marshal(map[string]string{
		"username": user.Username,
		"password": "s3cret-pass",
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/auth/login", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	setCookie := rec.Header().Get("Set-Cookie")
	if !strings.Contains(setCookie, AuthCookieName+"=") {
		t.Fatalf("Set-Cookie missing auth cookie: %q", setCookie)
	}
	if !strings.Contains(setCookie, "HttpOnly") {
		t.Fatalf("Set-Cookie should be HttpOnly: %q", setCookie)
	}
	if !strings.Contains(setCookie, "Path="+AuthCookiePath) {
		t.Fatalf("Set-Cookie should scope cookie to %s: %q", AuthCookiePath, setCookie)
	}
	if !strings.Contains(setCookie, "SameSite=Strict") {
		t.Fatalf("Set-Cookie should be SameSite=Strict: %q", setCookie)
	}

	var response struct {
		Success bool           `json:"success"`
		Token   *string        `json:"token"`
		User    map[string]any `json:"user"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Success {
		t.Fatalf("success = false")
	}
	if response.Token != nil {
		t.Fatalf("token should not be returned in response body")
	}
	if got := response.User["username"]; got != user.Username {
		t.Fatalf("user.username = %v, want %s", got, user.Username)
	}
}

func TestAuthHandler_StatusAcceptsCookie(t *testing.T) {
	user := newCookieAuthTestUser(t)
	userRepo := newPasskeyTestUserRepo(user)
	authMiddleware := NewAuthMiddleware(nil)
	handler := NewAuthHandler(authMiddleware, userRepo, &passkeyTestTenantRepo{}, true)

	token, err := authMiddleware.GenerateToken(user)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/auth/status", nil)
	req.AddCookie(&http.Cookie{Name: AuthCookieName, Value: token})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var response struct {
		AuthEnabled bool `json:"authEnabled"`
		User        *struct {
			ID       uint64 `json:"id"`
			Username string `json:"username"`
		} `json:"user"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.AuthEnabled {
		t.Fatalf("authEnabled = false, want true")
	}
	if response.User == nil {
		t.Fatalf("user missing from status response")
	}
	if response.User.ID != user.ID {
		t.Fatalf("user.id = %d, want %d", response.User.ID, user.ID)
	}
	if response.User.Username != user.Username {
		t.Fatalf("user.username = %q, want %q", response.User.Username, user.Username)
	}
}

func TestAuthHandler_LogoutClearsHttpOnlyCookie(t *testing.T) {
	handler := NewAuthHandler(NewAuthMiddleware(nil), newPasskeyTestUserRepo(), &passkeyTestTenantRepo{}, true)

	req := httptest.NewRequest(http.MethodPost, "/admin/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: AuthCookieName, Value: "token-value"})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	setCookie := rec.Header().Get("Set-Cookie")
	if !strings.Contains(setCookie, AuthCookieName+"=") {
		t.Fatalf("Set-Cookie missing auth cookie: %q", setCookie)
	}
	if !strings.Contains(setCookie, "HttpOnly") {
		t.Fatalf("Set-Cookie should be HttpOnly: %q", setCookie)
	}
	if !strings.Contains(setCookie, "Max-Age=0") {
		t.Fatalf("Set-Cookie should clear cookie: %q", setCookie)
	}
}
