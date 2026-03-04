package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

// AuthHandler handles authentication-related endpoints
type AuthHandler struct {
	authMiddleware *AuthMiddleware
	userRepo       repository.UserRepository
	tenantRepo     repository.TenantRepository
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(authMiddleware *AuthMiddleware, userRepo repository.UserRepository, tenantRepo repository.TenantRepository) *AuthHandler {
	return &AuthHandler{
		authMiddleware: authMiddleware,
		userRepo:       userRepo,
		tenantRepo:     tenantRepo,
	}
}

// ServeHTTP routes auth requests
func (h *AuthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/auth")
	path = strings.TrimSuffix(path, "/")

	switch path {
	case "/login":
		h.handleLogin(w, r)
	case "/register":
		h.handleRegister(w, r)
	case "/apply":
		h.handleApply(w, r)
	case "/password":
		h.handleChangePassword(w, r)
	case "/status":
		h.handleStatus(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

// handleLogin handles username+password login
// POST /admin/auth/login
func (h *AuthHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if body.Username == "" || body.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username and password are required"})
		return
	}

	// Look up user by username
	user, err := h.userRepo.GetByUsername(body.Username)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(body.Password)); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	// Only active users can login
	if user.Status != domain.UserStatusActive {
		if user.Status == domain.UserStatusPending {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "account pending approval"})
		} else {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "account is not active"})
		}
		return
	}

	// Generate token
	token, err := h.authMiddleware.GenerateToken(user)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
		return
	}

	// Update last login time
	now := time.Now()
	user.LastLoginAt = &now
	if err := h.userRepo.Update(user); err != nil {
		log.Printf("[Auth] Failed to update last login time for user %s: %v", user.Username, err)
	}

	// Get tenant info
	var tenantName string
	if tenant, err := h.tenantRepo.GetByID(user.TenantID); err == nil {
		tenantName = tenant.Name
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"token":   token,
		"user": map[string]any{
			"id":         user.ID,
			"username":   user.Username,
			"tenantID":   user.TenantID,
			"tenantName": tenantName,
			"role":       user.Role,
		},
	})
}

// handleRegister handles user registration (admin only)
// POST /admin/auth/register
func (h *AuthHandler) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	// Require authenticated admin user
	authHeader := r.Header.Get(AuthHeader)
	if !strings.HasPrefix(authHeader, "Bearer ") {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	claims, valid := h.authMiddleware.ValidateToken(strings.TrimPrefix(authHeader, "Bearer "))
	if !valid {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
		return
	}
	if claims.Role != string(domain.UserRoleAdmin) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin access required"})
		return
	}

	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if body.Username == "" || body.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username and password are required"})
		return
	}

	// Use tenant from the authenticated admin's token
	tenantID := claims.TenantID
	if tenantID == 0 {
		tenantID = domain.DefaultTenantID
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to hash password"})
		return
	}

	user := &domain.User{
		TenantID:     tenantID,
		Username:     body.Username,
		PasswordHash: string(hash),
		Role:         domain.UserRoleMember,
		Status:       domain.UserStatusActive,
	}

	if err := h.userRepo.Create(user); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "user already exists or invalid data"})
		return
	}

	// Generate token
	token, err := h.authMiddleware.GenerateToken(user)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"success": true,
		"token":   token,
		"user": map[string]any{
			"id":       user.ID,
			"username": user.Username,
			"tenantID": user.TenantID,
			"role":     user.Role,
		},
	})
}

// handleApply handles public user registration (no auth required)
// POST /admin/auth/apply
func (h *AuthHandler) handleApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if body.Username == "" || body.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username and password are required"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to hash password"})
		return
	}

	user := &domain.User{
		TenantID:     domain.DefaultTenantID,
		Username:     body.Username,
		PasswordHash: string(hash),
		Role:         domain.UserRoleMember,
		Status:       domain.UserStatusPending,
	}

	if err := h.userRepo.Create(user); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "username already exists"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"success": true,
		"message": "registration submitted, waiting for admin approval",
	})
}

// handleChangePassword handles self-service password change
// PUT /admin/auth/password
func (h *AuthHandler) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	// Require authenticated user (manual token validation since this is under /admin/auth/)
	authHeader := r.Header.Get(AuthHeader)
	if !strings.HasPrefix(authHeader, "Bearer ") {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	claims, valid := h.authMiddleware.ValidateToken(strings.TrimPrefix(authHeader, "Bearer "))
	if !valid {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
		return
	}

	var body struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if body.OldPassword == "" || body.NewPassword == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "oldPassword and newPassword are required"})
		return
	}

	user, err := h.userRepo.GetByID(claims.TenantID, claims.UserID)
	if err != nil {
		if err == domain.ErrNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(body.OldPassword)); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "incorrect old password"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to hash password"})
		return
	}

	user.PasswordHash = string(hash)
	if err := h.userRepo.Update(user); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update password"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "password updated"})
}

// handleStatus returns the authentication status
// GET /admin/auth/status
func (h *AuthHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	result := map[string]any{
		"authEnabled": true,
	}

	// If authenticated, return user info
	authHeader := r.Header.Get(AuthHeader)
	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if claims, valid := h.authMiddleware.ValidateToken(token); valid {
			userInfo := map[string]any{
				"id":       claims.UserID,
				"tenantID": claims.TenantID,
				"role":     claims.Role,
			}
			// Try to get user details
			if h.userRepo != nil {
				if user, err := h.userRepo.GetByID(claims.TenantID, claims.UserID); err == nil {
					userInfo["username"] = user.Username
				}
			}
			// Try to get tenant details
			if h.tenantRepo != nil {
				if tenant, err := h.tenantRepo.GetByID(claims.TenantID); err == nil {
					userInfo["tenantName"] = tenant.Name
				}
			}
			result["user"] = userInfo
		}
	}

	writeJSON(w, http.StatusOK, result)
}
