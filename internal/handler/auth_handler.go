package handler

import (
	"encoding/json"
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
	case "/verify":
		h.handleVerify(w, r)
	case "/login":
		h.handleLogin(w, r)
	case "/register":
		h.handleRegister(w, r)
	case "/status":
		h.handleStatus(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

// handleVerify verifies the provided password (legacy single-user mode)
// POST /admin/auth/verify
func (h *AuthHandler) handleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if h.authMiddleware.VerifyPassword(body.Password) {
		token, err := h.authMiddleware.GenerateLegacyToken()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"token":   token,
		})
	} else {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"success": false,
			"error":   "invalid password",
		})
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

	// Generate token
	token, err := h.authMiddleware.GenerateToken(user)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
		return
	}

	// Update last login time
	now := time.Now()
	user.LastLoginAt = &now
	h.userRepo.Update(user)

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

	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		TenantID uint64 `json:"tenantID"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if body.Username == "" || body.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username and password are required"})
		return
	}

	// Default to default tenant
	if body.TenantID == 0 {
		body.TenantID = domain.DefaultTenantID
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to hash password"})
		return
	}

	user := &domain.User{
		TenantID:     body.TenantID,
		Username:     body.Username,
		PasswordHash: string(hash),
		Role:         domain.UserRoleMember,
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

// handleStatus returns the authentication status
// GET /admin/auth/status
func (h *AuthHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	multiTenancy := h.authMiddleware.IsMultiTenancyEnabled()

	result := map[string]any{
		"authEnabled":         h.authMiddleware.IsEnabled() || multiTenancy,
		"multiTenancyEnabled": multiTenancy,
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
