package e2e_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestAuthStatus_NoAuth(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.UnauthGet("/api/admin/auth/status")
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["authEnabled"] != true {
		t.Fatalf("Expected authEnabled=true, got %v", result["authEnabled"])
	}
	// Without token, there should be no user info
	if _, ok := result["user"]; ok {
		t.Fatalf("Expected no user info without auth, got %v", result["user"])
	}
}

func TestLogin_Success(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.UnauthPost("/api/admin/auth/login", map[string]string{
		"username": "admin",
		"password": "test-admin-password",
	})
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["success"] != true {
		t.Fatalf("Expected success=true, got %v", result["success"])
	}
	token, ok := result["token"].(string)
	if !ok || token == "" {
		t.Fatalf("Expected non-empty token, got %v", result["token"])
	}
	user, ok := result["user"].(map[string]any)
	if !ok {
		t.Fatalf("Expected user object, got %v", result["user"])
	}
	if user["username"] != "admin" {
		t.Fatalf("Expected username 'admin', got %v", user["username"])
	}
	if user["role"] != "admin" {
		t.Fatalf("Expected role 'admin', got %v", user["role"])
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.UnauthPost("/api/admin/auth/login", map[string]string{
		"username": "admin",
		"password": "wrong-password",
	})
	AssertStatus(t, resp, http.StatusUnauthorized)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if _, ok := result["error"]; !ok {
		t.Fatalf("Expected error in response, got %v", result)
	}
}

func TestRegister_Success(t *testing.T) {
	env := NewTestEnv(t)

	// Admin registers a new user
	resp := env.AdminPost("/api/admin/auth/register", map[string]string{
		"username": "newuser",
		"password": "Newuser1!",
	})
	AssertStatus(t, resp, http.StatusCreated)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["success"] != true {
		t.Fatalf("Expected success=true, got %v", result["success"])
	}
	token, ok := result["token"].(string)
	if !ok || token == "" {
		t.Fatalf("Expected non-empty token for new user, got %v", result["token"])
	}
	user, ok := result["user"].(map[string]any)
	if !ok {
		t.Fatalf("Expected user object, got %v", result["user"])
	}
	if user["username"] != "newuser" {
		t.Fatalf("Expected username 'newuser', got %v", user["username"])
	}
	if user["role"] != "member" {
		t.Fatalf("Expected role 'member', got %v", user["role"])
	}

	// Verify the new user can login
	resp = env.UnauthPost("/api/admin/auth/login", map[string]string{
		"username": "newuser",
		"password": "Newuser1!",
	})
	AssertStatus(t, resp, http.StatusOK)
}

func TestChangePassword_Success(t *testing.T) {
	env := NewTestEnv(t)

	// Change admin password
	resp := env.RequestWithToken(http.MethodPut, "/api/admin/auth/password", map[string]string{
		"oldPassword": "test-admin-password",
		"newPassword": "Newadmin1!",
	}, env.Token)
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["success"] != true {
		t.Fatalf("Expected success=true, got %v", result["success"])
	}

	// Verify old password no longer works
	resp = env.UnauthPost("/api/admin/auth/login", map[string]string{
		"username": "admin",
		"password": "test-admin-password",
	})
	AssertStatus(t, resp, http.StatusUnauthorized)

	// Verify new password works
	resp = env.UnauthPost("/api/admin/auth/login", map[string]string{
		"username": "admin",
		"password": "Newadmin1!",
	})
	AssertStatus(t, resp, http.StatusOK)
}

func TestLogin_EmptyUsername(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.UnauthPost("/api/admin/auth/login", map[string]string{
		"username": "",
		"password": "some-password",
	})
	AssertStatus(t, resp, http.StatusBadRequest)
}

func TestLogin_EmptyPassword(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.UnauthPost("/api/admin/auth/login", map[string]string{
		"username": "admin",
		"password": "",
	})
	AssertStatus(t, resp, http.StatusBadRequest)
}

func TestLogin_InvalidJSON(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.RawPost("/api/admin/auth/login", "{invalid json!!!")
	AssertStatus(t, resp, http.StatusBadRequest)
}

func TestLogin_NonExistentUser(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.UnauthPost("/api/admin/auth/login", map[string]string{
		"username": "nonexistent-user-xyz",
		"password": "some-password",
	})
	AssertStatus(t, resp, http.StatusUnauthorized)
}

func TestLogin_PendingUser(t *testing.T) {
	env := NewTestEnv(t)

	// Create a pending user via the apply endpoint
	env.CreatePendingUser("pending-user", "Pending1!")

	// Try to login as pending user
	resp := env.UnauthPost("/api/admin/auth/login", map[string]string{
		"username": "pending-user",
		"password": "Pending1!",
	})
	AssertStatus(t, resp, http.StatusForbidden)
}

func TestRegister_MemberForbidden(t *testing.T) {
	env := NewTestEnv(t)
	memberToken := getMemberToken(t, env)

	resp := env.RequestWithToken(http.MethodPost, "/api/admin/auth/register", map[string]string{
		"username": "another-user",
		"password": "Another1!",
	}, memberToken)
	AssertStatus(t, resp, http.StatusForbidden)
}

func TestRegister_NoAuth(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.UnauthPost("/api/admin/auth/register", map[string]string{
		"username": "noauth-user",
		"password": "Noauth1!",
	})
	AssertStatus(t, resp, http.StatusUnauthorized)
}

func TestRegister_DuplicateUsername(t *testing.T) {
	env := NewTestEnv(t)

	// Register first user
	resp := env.AdminPost("/api/admin/auth/register", map[string]string{
		"username": "duplicate-user",
		"password": "Duplicate1!",
	})
	AssertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Register same username again
	resp = env.AdminPost("/api/admin/auth/register", map[string]string{
		"username": "duplicate-user",
		"password": "Duplicate2!",
	})
	AssertStatus(t, resp, http.StatusConflict)
}

func TestRegister_EmptyFields(t *testing.T) {
	env := NewTestEnv(t)

	// Empty username
	resp := env.AdminPost("/api/admin/auth/register", map[string]string{
		"username": "",
		"password": "some-password",
	})
	AssertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()

	// Empty password
	resp = env.AdminPost("/api/admin/auth/register", map[string]string{
		"username": "some-user",
		"password": "",
	})
	AssertStatus(t, resp, http.StatusBadRequest)
}

func TestRegister_InvalidPassword(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminPost("/api/admin/auth/register", map[string]string{
		"username": "weak-user",
		"password": "weakpass",
	})
	AssertStatus(t, resp, http.StatusBadRequest)

	var result map[string]any
	DecodeJSON(t, resp, &result)
	if _, ok := result["error"]; !ok {
		t.Fatalf("Expected error in response, got %v", result)
	}
	if result["code"] != "PASSWORD_POLICY_VIOLATION" {
		t.Fatalf("Expected PASSWORD_POLICY_VIOLATION code, got %v", result["code"])
	}
}

func TestApply_Success(t *testing.T) {
	env := NewTestEnv(t)

	inviteCode := env.CreateInviteCode()
	resp := env.UnauthPost("/api/admin/auth/apply", map[string]string{
		"username":   "apply-user",
		"password":   "Apply123!",
		"inviteCode": inviteCode,
	})
	AssertStatus(t, resp, http.StatusCreated)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["success"] != true {
		t.Fatalf("Expected success=true, got %v", result["success"])
	}
	msg, _ := result["message"].(string)
	if !strings.Contains(msg, "pending") && !strings.Contains(msg, "approval") {
		t.Fatalf("Expected pending/approval message, got %v", msg)
	}
}

func TestApply_DuplicateUsername(t *testing.T) {
	env := NewTestEnv(t)

	// First apply
	inviteCode1 := env.CreateInviteCode()
	resp := env.UnauthPost("/api/admin/auth/apply", map[string]string{
		"username":   "dup-apply-user",
		"password":   "Dupapply1!",
		"inviteCode": inviteCode1,
	})
	AssertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Duplicate apply
	inviteCode2 := env.CreateInviteCode()
	resp = env.UnauthPost("/api/admin/auth/apply", map[string]string{
		"username":   "dup-apply-user",
		"password":   "Dupapply2!",
		"inviteCode": inviteCode2,
	})
	AssertStatus(t, resp, http.StatusConflict)

	resp = env.UnauthPost("/api/admin/auth/apply", map[string]string{
		"username":   "new-apply-user",
		"password":   "Newapply3!",
		"inviteCode": inviteCode2,
	})
	AssertStatus(t, resp, http.StatusCreated)
}

func TestApply_InvalidPassword(t *testing.T) {
	env := NewTestEnv(t)

	inviteCode := env.CreateInviteCode()
	resp := env.UnauthPost("/api/admin/auth/apply", map[string]string{
		"username":   "weak-apply-user",
		"password":   "weakpass",
		"inviteCode": inviteCode,
	})
	AssertStatus(t, resp, http.StatusBadRequest)

	var result map[string]any
	DecodeJSON(t, resp, &result)
	if _, ok := result["error"]; !ok {
		t.Fatalf("Expected error in response, got %v", result)
	}
	if result["code"] != "PASSWORD_POLICY_VIOLATION" {
		t.Fatalf("Expected PASSWORD_POLICY_VIOLATION code, got %v", result["code"])
	}
}

func TestChangePassword_WrongOldPassword(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.RequestWithToken(http.MethodPut, "/api/admin/auth/password", map[string]string{
		"oldPassword": "wrong-old-password",
		"newPassword": "Newpass1!",
	}, env.Token)
	AssertStatus(t, resp, http.StatusUnauthorized)
}

func TestChangePassword_InvalidNewPassword(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.RequestWithToken(http.MethodPut, "/api/admin/auth/password", map[string]string{
		"oldPassword": "test-admin-password",
		"newPassword": "weakpass",
	}, env.Token)
	AssertStatus(t, resp, http.StatusBadRequest)

	var result map[string]any
	DecodeJSON(t, resp, &result)
	if _, ok := result["error"]; !ok {
		t.Fatalf("Expected error in response, got %v", result)
	}
	if result["code"] != "PASSWORD_POLICY_VIOLATION" {
		t.Fatalf("Expected PASSWORD_POLICY_VIOLATION code, got %v", result["code"])
	}
}

func TestChangePassword_NoAuth(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.UnauthPut("/api/admin/auth/password", map[string]string{
		"oldPassword": "test-admin-password",
		"newPassword": "Newpass1!",
	})
	AssertStatus(t, resp, http.StatusUnauthorized)
}

func TestAuthStatus_WithValidToken(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.RequestWithToken(http.MethodGet, "/api/admin/auth/status", nil, env.Token)
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["authEnabled"] != true {
		t.Fatalf("Expected authEnabled=true, got %v", result["authEnabled"])
	}
	user, ok := result["user"].(map[string]any)
	if !ok {
		t.Fatalf("Expected user info with valid token, got %v", result["user"])
	}
	if user["role"] != "admin" {
		t.Fatalf("Expected role 'admin', got %v", user["role"])
	}
	if user["username"] != "admin" {
		t.Fatalf("Expected username 'admin', got %v", user["username"])
	}
}

func TestLogin_SQLInjection(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.UnauthPost("/api/admin/auth/login", map[string]string{
		"username": "' OR 1=1 --",
		"password": "anything",
	})
	AssertStatus(t, resp, http.StatusUnauthorized)
}
