package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/awsl-project/maxx/internal/adapter/provider/kiro"
	"github.com/awsl-project/maxx/internal/service"
)

// KiroHandler handles Kiro-specific API requests
type KiroHandler struct {
	svc *service.AdminService
}

// NewKiroHandler creates a new Kiro handler
func NewKiroHandler(svc *service.AdminService) *KiroHandler {
	return &KiroHandler{svc: svc}
}

// TokenValidationResult is an alias for kiro.KiroTokenValidationResult
type TokenValidationResult = kiro.KiroTokenValidationResult

// ServeHTTP routes Kiro requests
// Routes:
//
//	POST /kiro/validate-social-token - 验证 Social refresh token
//	GET  /kiro/providers/{id}/quota - 获取 provider 的配额信息
func (h *KiroHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/kiro")
	path = strings.TrimSuffix(path, "/")

	parts := strings.Split(path, "/")

	// POST /kiro/validate-social-token
	if len(parts) >= 2 && parts[1] == "validate-social-token" && r.Method == http.MethodPost {
		h.handleValidateSocialToken(w, r)
		return
	}

	// GET /kiro/providers/{id}/quota
	if len(parts) >= 4 && parts[1] == "providers" && parts[3] == "quota" {
		id, _ := strconv.ParseUint(parts[2], 10, 64)
		if id > 0 {
			h.handleGetQuota(w, r, id)
			return
		}
	}

	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

// ValidateSocialToken 验证 Social refresh token
func (h *KiroHandler) ValidateSocialToken(ctx context.Context, refreshToken string) (*kiro.KiroTokenValidationResult, error) {
	return kiro.ValidateSocialToken(ctx, refreshToken)
}

// handleValidateSocialToken 处理验证 Social token 的 HTTP 请求
func (h *KiroHandler) handleValidateSocialToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if req.RefreshToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "refreshToken is required"})
		return
	}

	result, err := h.ValidateSocialToken(r.Context(), req.RefreshToken)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// GetProviderQuota 获取 Kiro provider 的配额信息
func (h *KiroHandler) GetProviderQuota(ctx context.Context, providerID uint64) (*kiro.KiroQuotaData, error) {
	// 获取 provider
	provider, err := h.svc.GetProvider(providerID)
	if err != nil {
		return nil, fmt.Errorf("provider not found: %w", err)
	}

	// 检查是否为 Kiro provider
	if provider.Type != "kiro" || provider.Config == nil || provider.Config.Kiro == nil {
		return nil, fmt.Errorf("not a Kiro provider")
	}

	config := provider.Config.Kiro

	// 获取配额
	quota, err := kiro.FetchQuota(ctx, config.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch quota: %w", err)
	}

	return quota, nil
}

// handleGetQuota 获取 provider 的配额信息
func (h *KiroHandler) handleGetQuota(w http.ResponseWriter, r *http.Request, providerID uint64) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	quota, err := h.GetProviderQuota(r.Context(), providerID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		} else if strings.Contains(err.Error(), "not a Kiro") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}

	writeJSON(w, http.StatusOK, quota)
}
