package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
)

func (h *AdminHandler) handleRedemptionCodes(w http.ResponseWriter, r *http.Request, id uint64, parts []string) {
	tenantID := maxxctx.GetTenantID(r.Context())

	switch {
	case len(parts) == 2:
		switch r.Method {
		case http.MethodGet:
			h.handleListRedemptionCodes(w, tenantID)
		case http.MethodPost:
			h.handleCreateRedemptionCodes(w, r, tenantID)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		return
	case len(parts) == 3:
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid redemption code id"})
			return
		}
		switch r.Method {
		case http.MethodGet:
			h.handleGetRedemptionCode(w, tenantID, id)
		case http.MethodPut:
			h.handleUpdateRedemptionCode(w, r, tenantID, id)
		case http.MethodDelete:
			if err := h.svc.DeleteRedemptionCode(tenantID, id); err != nil {
				writeRedemptionCodeError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"success": true})
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
		return
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

func (h *AdminHandler) handleListRedemptionCodes(w http.ResponseWriter, tenantID uint64) {
	codes, err := h.svc.GetRedemptionCodes(tenantID)
	if err != nil {
		log.Printf("[AdminRedemptionCodes] Failed to list redemption codes: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	if codes == nil {
		codes = []*domain.RedemptionCode{}
	}
	writeJSON(w, http.StatusOK, codes)
}

func (h *AdminHandler) handleGetRedemptionCode(w http.ResponseWriter, tenantID uint64, id uint64) {
	code, err := h.svc.GetRedemptionCode(tenantID, id)
	if err != nil {
		writeRedemptionCodeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, code)
}

func (h *AdminHandler) handleCreateRedemptionCodes(w http.ResponseWriter, r *http.Request, tenantID uint64) {
	var body struct {
		Count  int    `json:"count"`
		Amount uint64 `json:"amount"`
		Note   string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	createdBy := maxxctx.GetUserID(r.Context())
	result, err := h.svc.CreateRedemptionCodes(tenantID, createdBy, body.Count, body.Amount, strings.TrimSpace(body.Note))
	if err != nil {
		writeRedemptionCodeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *AdminHandler) handleUpdateRedemptionCode(w http.ResponseWriter, r *http.Request, tenantID uint64, id uint64) {
	existing, err := h.svc.GetRedemptionCode(tenantID, id)
	if err != nil {
		writeRedemptionCodeError(w, err)
		return
	}
	var body struct {
		Status *string `json:"status"`
		Amount *uint64 `json:"amount"`
		Note   *string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if body.Status != nil {
		switch strings.ToLower(strings.TrimSpace(*body.Status)) {
		case string(domain.RedemptionCodeStatusActive), string(domain.RedemptionCodeStatusDisabled):
			existing.Status = domain.RedemptionCodeStatus(strings.ToLower(strings.TrimSpace(*body.Status)))
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
			return
		}
	}
	if body.Amount != nil {
		existing.Amount = *body.Amount
	}
	if body.Note != nil {
		existing.Note = strings.TrimSpace(*body.Note)
	}
	if err := h.svc.UpdateRedemptionCode(tenantID, existing); err != nil {
		writeRedemptionCodeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func writeRedemptionCodeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "redemption code not found"})
	case errors.Is(err, domain.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid redemption code input"})
	case errors.Is(err, domain.ErrInvalidState):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "redemption code cannot be changed after use"})
	case errors.Is(err, domain.ErrRedemptionCodeDisabled):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "redemption code disabled"})
	case errors.Is(err, domain.ErrRedemptionCodeUsed):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "redemption code already used"})
	case errors.Is(err, domain.ErrRedemptionCodeInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "redemption code invalid"})
	default:
		log.Printf("[AdminRedemptionCodes] Failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
}
