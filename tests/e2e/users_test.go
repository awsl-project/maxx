package e2e_test

import (
	"fmt"
	"net/http"
	"testing"
)

func TestListUsers(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/users")
	AssertStatus(t, resp, http.StatusOK)

	var users []map[string]any
	DecodeJSON(t, resp, &users)

	// The admin user created in NewTestEnv should be present
	if len(users) < 1 {
		t.Fatalf("Expected at least 1 user (admin), got %d", len(users))
	}

	found := false
	for _, u := range users {
		if u["username"] == "admin" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Expected to find admin user in list")
	}
}

func TestCreateUser(t *testing.T) {
	env := NewTestEnv(t)

	body := map[string]any{
		"username": "testuser",
		"password": "test-password-123",
		"role":     "member",
	}

	resp := env.AdminPost("/api/admin/users", body)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)

	if created["username"] != "testuser" {
		t.Fatalf("Expected username 'testuser', got %v", created["username"])
	}
	if created["role"] != "member" {
		t.Fatalf("Expected role 'member', got %v", created["role"])
	}

	id, ok := created["id"].(float64)
	if !ok || id == 0 {
		t.Fatalf("Expected non-zero id, got %v", created["id"])
	}

	// Verify the user appears in the list
	resp = env.AdminGet("/api/admin/users")
	AssertStatus(t, resp, http.StatusOK)

	var users []map[string]any
	DecodeJSON(t, resp, &users)

	// Should have admin + testuser
	if len(users) < 2 {
		t.Fatalf("Expected at least 2 users, got %d", len(users))
	}
}

func TestGetUser_ByID(t *testing.T) {
	env := NewTestEnv(t)

	body := map[string]any{
		"username": "getuser",
		"password": "get-password-123",
		"role":     "member",
	}

	resp := env.AdminPost("/api/admin/users", body)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	// Get by ID
	resp = env.AdminGet(fmt.Sprintf("/api/admin/users/%d", int(id)))
	AssertStatus(t, resp, http.StatusOK)

	var fetched map[string]any
	DecodeJSON(t, resp, &fetched)

	if fetched["username"] != "getuser" {
		t.Fatalf("Expected username 'getuser', got %v", fetched["username"])
	}
}

func TestUpdateUser(t *testing.T) {
	env := NewTestEnv(t)

	body := map[string]any{
		"username": "updateuser",
		"password": "update-password-123",
		"role":     "member",
	}

	resp := env.AdminPost("/api/admin/users", body)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	// Update the user
	updateBody := map[string]any{
		"username": "updateduser",
		"role":     "admin",
	}

	resp = env.AdminPut(fmt.Sprintf("/api/admin/users/%d", int(id)), updateBody)
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["username"] != "updateduser" {
		t.Fatalf("Expected username 'updateduser', got %v", result["username"])
	}
	if result["role"] != "admin" {
		t.Fatalf("Expected role 'admin', got %v", result["role"])
	}
}

func TestDeleteUser(t *testing.T) {
	env := NewTestEnv(t)

	body := map[string]any{
		"username": "deleteuser",
		"password": "delete-password-123",
		"role":     "member",
	}

	resp := env.AdminPost("/api/admin/users", body)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	// Delete the user
	resp = env.AdminDelete(fmt.Sprintf("/api/admin/users/%d", int(id)))
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["success"] != "user deleted" {
		t.Fatalf("Expected success message 'user deleted', got %v", result["success"])
	}
}

func TestGetUser_NotFound(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminGet("/api/admin/users/99999")
	AssertStatus(t, resp, http.StatusNotFound)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["error"] != "user not found" {
		t.Fatalf("Expected error 'user not found', got %v", result["error"])
	}
}

func TestCreateUser_EmptyUsername(t *testing.T) {
	env := NewTestEnv(t)

	body := map[string]any{
		"username": "",
		"password": "some-password",
		"role":     "member",
	}

	resp := env.AdminPost("/api/admin/users", body)
	AssertStatus(t, resp, http.StatusBadRequest)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["error"] != "username and password are required" {
		t.Fatalf("Expected error 'username and password are required', got %v", result["error"])
	}
}

func TestCreateUser_EmptyPassword(t *testing.T) {
	env := NewTestEnv(t)

	body := map[string]any{
		"username": "nopassuser",
		"password": "",
		"role":     "member",
	}

	resp := env.AdminPost("/api/admin/users", body)
	AssertStatus(t, resp, http.StatusBadRequest)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["error"] != "username and password are required" {
		t.Fatalf("Expected error 'username and password are required', got %v", result["error"])
	}
}

func TestCreateUser_DuplicateUsername(t *testing.T) {
	env := NewTestEnv(t)

	body := map[string]any{
		"username": "dupuser",
		"password": "dup-password-123",
		"role":     "member",
	}

	// Create the user first time
	resp := env.AdminPost("/api/admin/users", body)
	AssertStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	// Create with same username again
	resp = env.AdminPost("/api/admin/users", body)
	AssertStatus(t, resp, http.StatusConflict)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["error"] != "user already exists or invalid data" {
		t.Fatalf("Expected error 'user already exists or invalid data', got %v", result["error"])
	}
}

func TestCreateUser_InvalidPassword(t *testing.T) {
	env := NewTestEnv(t)

	body := map[string]any{
		"username": "weak-user",
		"password": "weakpass",
		"role":     "member",
	}

	resp := env.AdminPost("/api/admin/users", body)
	AssertStatus(t, resp, http.StatusBadRequest)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["code"] != "PASSWORD_POLICY_VIOLATION" {
		t.Fatalf("Expected PASSWORD_POLICY_VIOLATION code, got %v", result["code"])
	}
}

func TestCreateUser_InvalidJSON(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminRawPost("/api/admin/users", "{invalid json")
	AssertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestUpdateUser_NotFound(t *testing.T) {
	env := NewTestEnv(t)

	updateBody := map[string]any{
		"username": "ghost",
		"role":     "member",
	}

	resp := env.AdminPut("/api/admin/users/99999", updateBody)
	AssertStatus(t, resp, http.StatusNotFound)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["error"] != "user not found" {
		t.Fatalf("Expected error 'user not found', got %v", result["error"])
	}
}

func TestDeleteUser_NotFound(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminDelete("/api/admin/users/99999")
	AssertStatus(t, resp, http.StatusNotFound)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["error"] != "user not found" {
		t.Fatalf("Expected error 'user not found', got %v", result["error"])
	}
}

func TestUpdateUserPassword_Success(t *testing.T) {
	env := NewTestEnv(t)

	// Create a user first
	body := map[string]any{
		"username": "pwduser",
		"password": "old-password-123",
		"role":     "member",
	}
	resp := env.AdminPost("/api/admin/users", body)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	// Update password
	pwdBody := map[string]any{
		"password": "new-password-456",
	}
	resp = env.AdminPut(fmt.Sprintf("/api/admin/users/%d/password", int(id)), pwdBody)
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["success"] != "password updated" {
		t.Fatalf("Expected success 'password updated', got %v", result["success"])
	}
}

func TestUpdateUserPassword_EmptyPassword(t *testing.T) {
	env := NewTestEnv(t)

	// Create a user first
	body := map[string]any{
		"username": "pwdempty",
		"password": "old-password-123",
		"role":     "member",
	}
	resp := env.AdminPost("/api/admin/users", body)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	// Update with empty password
	pwdBody := map[string]any{
		"password": "",
	}
	resp = env.AdminPut(fmt.Sprintf("/api/admin/users/%d/password", int(id)), pwdBody)
	AssertStatus(t, resp, http.StatusBadRequest)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["error"] != "password is required" {
		t.Fatalf("Expected error 'password is required', got %v", result["error"])
	}
}

func TestUpdateUserPassword_InvalidPassword(t *testing.T) {
	env := NewTestEnv(t)

	body := map[string]any{
		"username": "pwdinvalid",
		"password": "Oldpass1!",
		"role":     "member",
	}
	resp := env.AdminPost("/api/admin/users", body)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	pwdBody := map[string]any{
		"password": "weakpass",
	}
	resp = env.AdminPut(fmt.Sprintf("/api/admin/users/%d/password", int(id)), pwdBody)
	AssertStatus(t, resp, http.StatusBadRequest)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["code"] != "PASSWORD_POLICY_VIOLATION" {
		t.Fatalf("Expected PASSWORD_POLICY_VIOLATION code, got %v", result["code"])
	}
}

func TestUpdateUserPassword_NotFound(t *testing.T) {
	env := NewTestEnv(t)

	pwdBody := map[string]any{
		"password": "new-password-456",
	}
	resp := env.AdminPut("/api/admin/users/99999/password", pwdBody)
	AssertStatus(t, resp, http.StatusNotFound)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["error"] != "user not found" {
		t.Fatalf("Expected error 'user not found', got %v", result["error"])
	}
}

func TestApproveUser_Success(t *testing.T) {
	env := NewTestEnv(t)

	// Create a pending user via the apply endpoint
	env.CreatePendingUser("pendinguser", "Pending1!")

	// Find the pending user in the list
	resp := env.AdminGet("/api/admin/users")
	AssertStatus(t, resp, http.StatusOK)

	var users []map[string]any
	DecodeJSON(t, resp, &users)

	var pendingID float64
	for _, u := range users {
		if u["username"] == "pendinguser" {
			pendingID = u["id"].(float64)
			break
		}
	}
	if pendingID == 0 {
		t.Fatal("Could not find pending user in list")
	}

	// Approve the user
	resp = env.AdminPut(fmt.Sprintf("/api/admin/users/%d/approve", int(pendingID)), nil)
	AssertStatus(t, resp, http.StatusOK)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["status"] != "active" {
		t.Fatalf("Expected status 'active' after approval, got %v", result["status"])
	}
}

func TestApproveUser_AlreadyActive(t *testing.T) {
	env := NewTestEnv(t)

	// Create a normal (active) user
	body := map[string]any{
		"username": "activeuser",
		"password": "active-password-123",
		"role":     "member",
	}
	resp := env.AdminPost("/api/admin/users", body)
	AssertStatus(t, resp, http.StatusCreated)

	var created map[string]any
	DecodeJSON(t, resp, &created)
	id := created["id"].(float64)

	// Try to approve an already active user
	resp = env.AdminPut(fmt.Sprintf("/api/admin/users/%d/approve", int(id)), nil)
	AssertStatus(t, resp, http.StatusBadRequest)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["error"] != "user is not pending approval" {
		t.Fatalf("Expected error 'user is not pending approval', got %v", result["error"])
	}
}

func TestApproveUser_NotFound(t *testing.T) {
	env := NewTestEnv(t)

	resp := env.AdminPut("/api/admin/users/99999/approve", nil)
	AssertStatus(t, resp, http.StatusNotFound)

	var result map[string]any
	DecodeJSON(t, resp, &result)

	if result["error"] != "user not found" {
		t.Fatalf("Expected error 'user not found', got %v", result["error"])
	}
}
