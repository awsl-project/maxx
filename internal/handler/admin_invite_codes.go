package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
)

func (h *AdminHandler) handleInviteCodes(w http.ResponseWriter, r *http.Request, id uint64, parts []string) {
	tenantID := maxxctx.GetTenantID(r.Context())

	// Handle /admin/invite-codes/{id}/usages
	if id > 0 && len(parts) > 3 && parts[3] == "usages" {
		if r.Method == http.MethodGet {
			h.handleInviteCodeUsages(w, tenantID, id)
		} else {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		if id > 0 {
			h.handleGetInviteCode(w, tenantID, id)
		} else {
			h.handleListInviteCodes(w, tenantID)
		}
	case http.MethodPost:
		h.handleCreateInviteCodes(w, r, tenantID)
	case http.MethodPut:
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		h.handleUpdateInviteCode(w, r, tenantID, id)
	case http.MethodDelete:
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		if err := h.svc.DeleteInviteCode(tenantID, id); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *AdminHandler) handleListInviteCodes(w http.ResponseWriter, tenantID uint64) {
	codes, err := h.svc.GetInviteCodes(tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, codes)
}

func (h *AdminHandler) handleGetInviteCode(w http.ResponseWriter, tenantID uint64, id uint64) {
	code, err := h.svc.GetInviteCode(tenantID, id)
	if err != nil {
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
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *AdminHandler) handleUpdateInviteCode(w http.ResponseWriter, r *http.Request, tenantID uint64, id uint64) {
	existing, err := h.svc.GetInviteCode(tenantID, id)
	if err != nil {
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
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (h *AdminHandler) handleInviteCodeUsages(w http.ResponseWriter, tenantID uint64, codeID uint64) {
	usages, err := h.svc.ListInviteCodeUsages(tenantID, codeID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, usages)
}
