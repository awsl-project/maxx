package handler

import (
	"encoding/json"
	"net/http"
	"strings"
)

// AuthHandler handles authentication-related endpoints
type AuthHandler struct {
	authMiddleware *AuthMiddleware
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(authMiddleware *AuthMiddleware) *AuthHandler {
	return &AuthHandler{
		authMiddleware: authMiddleware,
	}
}

// ServeHTTP routes auth requests
func (h *AuthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/auth")
	path = strings.TrimSuffix(path, "/")

	switch path {
	case "/verify":
		h.handleVerify(w, r)
	case "/status":
		h.handleStatus(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

// handleVerify verifies the provided password
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
		token, err := h.authMiddleware.GenerateToken()
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

// handleStatus returns the authentication status
// GET /admin/auth/status
func (h *AuthHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"authEnabled": h.authMiddleware.IsEnabled(),
	})
}
