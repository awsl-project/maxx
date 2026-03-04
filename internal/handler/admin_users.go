package handler

import (
	"encoding/json"
	"net/http"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"golang.org/x/crypto/bcrypt"
)

// handleUsers handles /admin/users endpoints
func (h *AdminHandler) handleUsers(w http.ResponseWriter, r *http.Request, id uint64, parts []string) {
	if h.userRepo == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "user management not available"})
		return
	}

	tenantID := maxxctx.GetTenantID(r.Context())

	// Handle /admin/users/{id}/password
	if id > 0 && len(parts) > 3 && parts[3] == "password" {
		if r.Method == http.MethodPut {
			h.handleUpdateUserPassword(w, r, tenantID, id)
		} else {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		return
	}

	// Handle /admin/users/{id}/approve
	if id > 0 && len(parts) > 3 && parts[3] == "approve" {
		if r.Method == http.MethodPut {
			h.handleApproveUser(w, tenantID, id)
		} else {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		if id > 0 {
			h.handleGetUser(w, tenantID, id)
		} else {
			h.handleListUsers(w, tenantID)
		}
	case http.MethodPost:
		h.handleCreateUser(w, r, tenantID)
	case http.MethodPut:
		if id > 0 {
			h.handleUpdateUser(w, r, tenantID, id)
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user ID is required"})
		}
	case http.MethodDelete:
		if id > 0 {
			h.handleDeleteUser(w, tenantID, id)
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user ID is required"})
		}
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *AdminHandler) handleListUsers(w http.ResponseWriter, tenantID uint64) {
	users, err := h.userRepo.ListByTenant(tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list users"})
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (h *AdminHandler) handleGetUser(w http.ResponseWriter, tenantID uint64, id uint64) {
	user, err := h.userRepo.GetByID(tenantID, id)
	if err != nil {
		if err == domain.ErrNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (h *AdminHandler) handleCreateUser(w http.ResponseWriter, r *http.Request, tenantID uint64) {
	var body struct {
		Username string          `json:"username"`
		Password string          `json:"password"`
		Role     domain.UserRole `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if body.Username == "" || body.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username and password are required"})
		return
	}

	if body.Role == "" {
		body.Role = domain.UserRoleMember
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to hash password"})
		return
	}

	user := &domain.User{
		TenantID:     tenantID,
		Username:     body.Username,
		PasswordHash: string(hash),
		Role:         body.Role,
		Status:       domain.UserStatusActive,
	}

	if err := h.userRepo.Create(user); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "user already exists or invalid data"})
		return
	}

	writeJSON(w, http.StatusCreated, user)
}

func (h *AdminHandler) handleUpdateUser(w http.ResponseWriter, r *http.Request, tenantID uint64, id uint64) {
	user, err := h.userRepo.GetByID(tenantID, id)
	if err != nil {
		if err == domain.ErrNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	var body struct {
		Username string            `json:"username"`
		Role     domain.UserRole   `json:"role"`
		Status   domain.UserStatus `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if body.Username != "" {
		user.Username = body.Username
	}
	if body.Role != "" {
		user.Role = body.Role
	}
	if body.Status != "" {
		switch body.Status {
		case domain.UserStatusPending, domain.UserStatusActive:
			user.Status = body.Status
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
			return
		}
	}

	if err := h.userRepo.Update(user); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update user"})
		return
	}

	writeJSON(w, http.StatusOK, user)
}

func (h *AdminHandler) handleApproveUser(w http.ResponseWriter, tenantID uint64, id uint64) {
	user, err := h.userRepo.GetByID(tenantID, id)
	if err != nil {
		if err == domain.ErrNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	if user.Status != domain.UserStatusPending {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user is not pending approval"})
		return
	}

	user.Status = domain.UserStatusActive
	if err := h.userRepo.Update(user); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to approve user"})
		return
	}

	writeJSON(w, http.StatusOK, user)
}

func (h *AdminHandler) handleUpdateUserPassword(w http.ResponseWriter, r *http.Request, tenantID uint64, id uint64) {
	user, err := h.userRepo.GetByID(tenantID, id)
	if err != nil {
		if err == domain.ErrNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if body.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password is required"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to hash password"})
		return
	}

	user.PasswordHash = string(hash)
	if err := h.userRepo.Update(user); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update password"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"success": "password updated"})
}

func (h *AdminHandler) handleDeleteUser(w http.ResponseWriter, tenantID uint64, id uint64) {
	if err := h.userRepo.Delete(tenantID, id); err != nil {
		if err == domain.ErrNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete user"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"success": "user deleted"})
}
