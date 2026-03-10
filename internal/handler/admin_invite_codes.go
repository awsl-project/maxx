package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
)

func (h *AdminHandler) handleInviteCodes(w http.ResponseWriter, r *http.Request, id uint64, parts []string) {
	tenantID := maxxctx.GetTenantID(r.Context())

	switch {
	case len(parts) == 2:
		switch r.Method {
		case http.MethodGet:
			h.handleListInviteCodes(w, r, tenantID)
		case http.MethodPost:
			h.handleCreateInviteCodes(w, r, tenantID)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		return
	case len(parts) == 3:
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid invite code id"})
			return
		}
		switch r.Method {
		case http.MethodGet:
			h.handleGetInviteCode(w, r, tenantID, id)
		case http.MethodPut:
			h.handleUpdateInviteCode(w, r, tenantID, id)
		case http.MethodDelete:
			code, err := h.svc.GetInviteCode(tenantID, id)
			if err != nil {
				if errors.Is(err, domain.ErrNotFound) {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "invite code not found"})
					return
				}
				log.Printf("[AdminInviteCodes] Failed to load invite code %d: %v", id, err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
				return
			}
			if !canAccessInviteCode(r, code, h.authEnabled) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "invite code not found"})
				return
			}
			if err := h.svc.DeleteInviteCode(tenantID, id); err != nil {
				if errors.Is(err, domain.ErrNotFound) {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "invite code not found"})
					return
				}
				if errors.Is(err, domain.ErrInvalidState) {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid invite code state"})
					return
				}
				log.Printf("[AdminInviteCodes] Failed to delete invite code %d: %v", id, err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"success": true})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		return
	case len(parts) == 4 && parts[3] == "usages":
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid invite code id"})
			return
		}
		if r.Method == http.MethodGet {
			h.handleInviteCodeUsages(w, r, tenantID, id)
		} else {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		return
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

func (h *AdminHandler) handleListInviteCodes(w http.ResponseWriter, r *http.Request, tenantID uint64) {
	codes, err := h.svc.GetInviteCodes(tenantID)
	if err != nil {
		log.Printf("[AdminInviteCodes] Failed to list invite codes: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	if maxxctx.GetUserRole(r.Context()) == string(domain.UserRoleMember) {
		userID := maxxctx.GetUserID(r.Context())
		filtered := make([]*domain.InviteCode, 0, len(codes))
		for _, code := range codes {
			if code.CreatedByUserID == userID {
				filtered = append(filtered, code)
			}
		}
		codes = filtered
	}
	writeJSON(w, http.StatusOK, codes)
}

func (h *AdminHandler) handleGetInviteCode(w http.ResponseWriter, r *http.Request, tenantID uint64, id uint64) {
	code, err := h.svc.GetInviteCode(tenantID, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "invite code not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	if !canAccessInviteCode(r, code, h.authEnabled) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "invite code not found"})
		return
	}
	writeJSON(w, http.StatusOK, code)
}

func (h *AdminHandler) handleCreateInviteCodes(w http.ResponseWriter, r *http.Request, tenantID uint64) {
	var body struct {
		Count     int     `json:"count"`
		MaxUses   *uint64 `json:"maxUses"`
		ExpiresAt *string `json:"expiresAt"`
		Note      string  `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	maxUses := uint64(1)
	if body.MaxUses != nil {
		maxUses = *body.MaxUses
	}

	var expiresAt *time.Time
	if body.ExpiresAt != nil && strings.TrimSpace(*body.ExpiresAt) != "" {
		t, err := time.Parse(time.RFC3339, *body.ExpiresAt)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid expiresAt format, use RFC3339"})
			return
		}
		expiresAt = &t
	}

	createdBy := maxxctx.GetUserID(r.Context())
	result, err := h.svc.CreateInviteCodes(tenantID, createdBy, body.Count, maxUses, expiresAt, strings.TrimSpace(body.Note))
	if err != nil {
		log.Printf("[AdminInviteCodes] Failed to create invite codes: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *AdminHandler) handleUpdateInviteCode(w http.ResponseWriter, r *http.Request, tenantID uint64, id uint64) {
	existing, err := h.svc.GetInviteCode(tenantID, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "invite code not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	if !canAccessInviteCode(r, existing, h.authEnabled) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "invite code not found"})
		return
	}

	var body struct {
		Status    *string `json:"status"`
		MaxUses   *uint64 `json:"maxUses"`
		ExpiresAt *string `json:"expiresAt"`
		Note      *string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if body.Status != nil {
		switch strings.ToLower(strings.TrimSpace(*body.Status)) {
		case string(domain.InviteCodeStatusActive), string(domain.InviteCodeStatusDisabled):
			existing.Status = domain.InviteCodeStatus(strings.ToLower(strings.TrimSpace(*body.Status)))
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
			return
		}
	}
	if body.MaxUses != nil {
		if *body.MaxUses != 0 && *body.MaxUses < existing.UsedCount {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "maxUses cannot be less than usedCount"})
			return
		}
		existing.MaxUses = *body.MaxUses
	}
	if body.ExpiresAt != nil {
		if strings.TrimSpace(*body.ExpiresAt) == "" {
			existing.ExpiresAt = nil
		} else {
			t, err := time.Parse(time.RFC3339, *body.ExpiresAt)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid expiresAt format, use RFC3339"})
				return
			}
			existing.ExpiresAt = &t
		}
	}
	if body.Note != nil {
		existing.Note = strings.TrimSpace(*body.Note)
	}

	if err := h.svc.UpdateInviteCode(tenantID, existing); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "invite code not found"})
			return
		}
		if errors.Is(err, domain.ErrInvalidState) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid invite code state"})
			return
		}
		log.Printf("[AdminInviteCodes] Failed to update invite code %d: %v", existing.ID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (h *AdminHandler) handleInviteCodeUsages(w http.ResponseWriter, r *http.Request, tenantID uint64, codeID uint64) {
	code, err := h.svc.GetInviteCode(tenantID, codeID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "invite code not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	if !canAccessInviteCode(r, code, h.authEnabled) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "invite code not found"})
		return
	}
	usages, err := h.svc.ListInviteCodeUsages(tenantID, codeID)
	if err != nil {
		log.Printf("[AdminInviteCodes] Failed to list invite code usages %d: %v", codeID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	if maxxctx.GetUserRole(r.Context()) != string(domain.UserRoleAdmin) {
		type inviteCodeUsageRedacted struct {
			ID           uint64    `json:"id"`
			CreatedAt    time.Time `json:"createdAt"`
			UpdatedAt    time.Time `json:"updatedAt"`
			TenantID     uint64    `json:"tenantID"`
			InviteCodeID uint64    `json:"inviteCodeID"`
			UserID       uint64    `json:"userID"`
			Username     string    `json:"username"`
			UsedAt       time.Time `json:"usedAt"`
			Result       string    `json:"result"`
			Reason       string    `json:"reason,omitempty"`
			RolledBack   bool      `json:"rolledBack,omitempty"`
		}
		redacted := make([]inviteCodeUsageRedacted, 0, len(usages))
		for _, usage := range usages {
			redacted = append(redacted, inviteCodeUsageRedacted{
				ID:           usage.ID,
				CreatedAt:    usage.CreatedAt,
				UpdatedAt:    usage.UpdatedAt,
				TenantID:     usage.TenantID,
				InviteCodeID: usage.InviteCodeID,
				UserID:       usage.UserID,
				Username:     usage.Username,
				UsedAt:       usage.UsedAt,
				Result:       usage.Result,
				Reason:       usage.Reason,
				RolledBack:   usage.RolledBack,
			})
		}
		writeJSON(w, http.StatusOK, redacted)
		return
	}
	writeJSON(w, http.StatusOK, usages)
}

func canAccessInviteCode(r *http.Request, code *domain.InviteCode, authEnabled bool) bool {
	if !authEnabled {
		return true
	}
	role := maxxctx.GetUserRole(r.Context())
	if role == string(domain.UserRoleAdmin) {
		return true
	}
	if role == string(domain.UserRoleMember) {
		userID := maxxctx.GetUserID(r.Context())
		return code.CreatedByUserID == userID
	}
	return false
}
