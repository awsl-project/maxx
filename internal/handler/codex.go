package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider/codex"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/event"
	"github.com/awsl-project/maxx/internal/repository"
	"github.com/awsl-project/maxx/internal/service"
)

// CodexHandler handles Codex-specific API requests
type CodexHandler struct {
	svc          *service.AdminService
	quotaRepo    repository.CodexQuotaRepository
	oauthManager *codex.OAuthManager
	taskSvc      *service.CodexTaskService
}

// NewCodexHandler creates a new Codex handler
func NewCodexHandler(svc *service.AdminService, quotaRepo repository.CodexQuotaRepository, broadcaster event.Broadcaster) *CodexHandler {
	return &CodexHandler{
		svc:          svc,
		quotaRepo:    quotaRepo,
		oauthManager: codex.NewOAuthManager(broadcaster),
	}
}

// SetTaskService sets the CodexTaskService for background task operations
func (h *CodexHandler) SetTaskService(taskSvc *service.CodexTaskService) {
	h.taskSvc = taskSvc
}

// ServeHTTP routes Codex requests
// Routes:
//
//	POST /codex/validate-token - Validate refresh token
//	POST /codex/oauth/start - Start OAuth flow
//	GET  /codex/oauth/callback - OAuth callback
//	POST /codex/provider/:id/refresh - Refresh provider info
//	GET  /codex/provider/:id/usage - Get provider usage/quota
//	POST /codex/refresh-quotas - Force refresh all Codex quotas
//	POST /codex/sort-routes - Manually sort Codex routes
//	GET  /codex/providers/quotas - Batch get all Codex provider quotas
func (h *CodexHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/codex")
	path = strings.TrimSuffix(path, "/")

	parts := strings.Split(path, "/")

	// POST /codex/validate-token
	if len(parts) >= 2 && parts[1] == "validate-token" && r.Method == http.MethodPost {
		h.handleValidateToken(w, r)
		return
	}

	// POST /codex/oauth/start
	if len(parts) >= 3 && parts[1] == "oauth" && parts[2] == "start" && r.Method == http.MethodPost {
		h.handleOAuthStart(w, r)
		return
	}

	// GET /codex/oauth/callback
	if len(parts) >= 3 && parts[1] == "oauth" && parts[2] == "callback" && r.Method == http.MethodGet {
		h.handleOAuthCallback(w, r)
		return
	}

	// POST /codex/refresh-quotas - Force refresh all quotas
	if len(parts) >= 2 && parts[1] == "refresh-quotas" && r.Method == http.MethodPost {
		h.handleForceRefreshQuotas(w, r)
		return
	}

	// POST /codex/sort-routes - Manually sort routes
	if len(parts) >= 2 && parts[1] == "sort-routes" && r.Method == http.MethodPost {
		h.handleSortRoutes(w, r)
		return
	}

	// GET /codex/providers/quotas - Batch get quotas (before single provider route)
	if len(parts) >= 3 && parts[1] == "providers" && parts[2] == "quotas" && r.Method == http.MethodGet {
		h.handleGetBatchQuotas(w, r)
		return
	}

	// POST /codex/provider/:id/refresh
	if len(parts) >= 4 && parts[1] == "provider" && parts[3] == "refresh" && r.Method == http.MethodPost {
		h.handleRefreshProviderInfo(w, r, parts[2])
		return
	}

	// GET /codex/provider/:id/usage
	if len(parts) >= 4 && parts[1] == "provider" && parts[3] == "usage" && r.Method == http.MethodGet {
		h.handleGetProviderUsage(w, r, parts[2])
		return
	}

	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

// ============================================================================
// Public methods (shared by HTTP handler and Wails)
// ============================================================================

// ValidateToken validates a refresh token
func (h *CodexHandler) ValidateToken(ctx context.Context, refreshToken string) (*codex.CodexTokenValidationResult, error) {
	if refreshToken == "" {
		return nil, fmt.Errorf("refreshToken is required")
	}

	return codex.ValidateRefreshToken(ctx, refreshToken)
}

// OAuthStartResult OAuth start result
type CodexOAuthStartResult struct {
	AuthURL string `json:"authURL"`
	State   string `json:"state"`
}

// StartOAuth starts the OAuth authorization flow
func (h *CodexHandler) StartOAuth() (*CodexOAuthStartResult, error) {
	// Generate random state token
	state, err := h.oauthManager.GenerateState()
	if err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
	}

	// Create OAuth session with PKCE
	_, pkce, err := h.oauthManager.CreateSession(state)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Build OpenAI OAuth authorization URL (uses fixed localhost redirect)
	authURL := codex.GetAuthURL(state, pkce)

	return &CodexOAuthStartResult{
		AuthURL: authURL,
		State:   state,
	}, nil
}

// ============================================================================
// HTTP handler methods
// ============================================================================

// handleValidateToken validates a refresh token
func (h *CodexHandler) handleValidateToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	result, err := h.ValidateToken(r.Context(), req.RefreshToken)
	if err != nil {
		if strings.Contains(err.Error(), "required") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handleOAuthStart starts the OAuth authorization flow
func (h *CodexHandler) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	result, err := h.StartOAuth()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handleOAuthCallback handles the OAuth callback from OpenAI
// This is called on localhost:1455/auth/callback
func (h *CodexHandler) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	// Get code and state
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		h.sendOAuthErrorResult(w, state, "Missing code or state parameter")
		return
	}

	// Validate state and get session
	session, ok := h.oauthManager.GetSession(state)
	if !ok {
		h.sendOAuthErrorResult(w, state, "Invalid or expired state")
		return
	}

	// Exchange code for tokens (using fixed redirect URI)
	tokenResp, err := codex.ExchangeCodeForTokens(r.Context(), code, codex.OAuthRedirectURI, session.CodeVerifier)
	if err != nil {
		h.sendOAuthErrorResult(w, state, fmt.Sprintf("Token exchange failed: %v", err))
		return
	}

	// Parse ID token to get user info
	var email, name, picture, accountID, userID, planType, subscriptionStart, subscriptionEnd string
	if tokenResp.IDToken != "" {
		claims, err := codex.ParseIDToken(tokenResp.IDToken)
		if err == nil {
			email = claims.Email
			name = claims.Name
			picture = claims.Picture
			accountID = claims.GetAccountID()
			userID = claims.GetUserID()
			planType = claims.GetPlanType()
			subscriptionStart = claims.GetSubscriptionStart()
			subscriptionEnd = claims.GetSubscriptionEnd()
		}
	}

	// Calculate expiration time
	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Format(time.RFC3339)

	// Push success result to frontend
	result := &codex.OAuthResult{
		State:             state,
		Success:           true,
		AccessToken:       tokenResp.AccessToken,
		RefreshToken:      tokenResp.RefreshToken,
		ExpiresAt:         expiresAt,
		Email:             email,
		Name:              name,
		Picture:           picture,
		AccountID:         accountID,
		UserID:            userID,
		PlanType:          planType,
		SubscriptionStart: subscriptionStart,
		SubscriptionEnd:   subscriptionEnd,
	}

	h.oauthManager.CompleteSession(state, result)

	// Return success page
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(codexOAuthSuccessHTML))
}

// sendOAuthErrorResult sends OAuth error result and returns error page
func (h *CodexHandler) sendOAuthErrorResult(w http.ResponseWriter, state, errorMsg string) {
	// Push error result to frontend
	result := &codex.OAuthResult{
		State:   state,
		Success: false,
		Error:   errorMsg,
	}

	h.oauthManager.CompleteSession(state, result)

	// Return error page
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte(codexOAuthErrorHTML))
}

// RefreshProviderInfo refreshes the Codex provider info by re-validating the refresh token
func (h *CodexHandler) RefreshProviderInfo(ctx context.Context, providerID int) (*codex.CodexTokenValidationResult, error) {
	// Get the provider
	provider, err := h.svc.GetProvider(uint64(providerID))
	if err != nil {
		return nil, fmt.Errorf("provider not found: %w", err)
	}

	if provider.Type != "codex" || provider.Config == nil || provider.Config.Codex == nil {
		return nil, fmt.Errorf("provider %s is not a codex provider", provider.Name)
	}

	refreshToken := provider.Config.Codex.RefreshToken
	if refreshToken == "" {
		return nil, fmt.Errorf("provider %s has no refresh token", provider.Name)
	}

	// Validate and refresh the token
	result, err := codex.ValidateRefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}

	if !result.Valid {
		return result, nil
	}

	// Update provider config with new info
	provider.Config.Codex.Email = result.Email
	provider.Config.Codex.Name = result.Name
	provider.Config.Codex.Picture = result.Picture
	provider.Config.Codex.AccessToken = result.AccessToken
	provider.Config.Codex.ExpiresAt = result.ExpiresAt
	provider.Config.Codex.AccountID = result.AccountID
	provider.Config.Codex.UserID = result.UserID
	provider.Config.Codex.PlanType = result.PlanType
	provider.Config.Codex.SubscriptionStart = result.SubscriptionStart
	provider.Config.Codex.SubscriptionEnd = result.SubscriptionEnd

	// Update refresh token if a new one was issued
	if result.RefreshToken != "" && result.RefreshToken != refreshToken {
		provider.Config.Codex.RefreshToken = result.RefreshToken
	}

	// Save the updated provider
	if err := h.svc.UpdateProvider(provider); err != nil {
		return nil, fmt.Errorf("failed to update provider: %w", err)
	}

	return result, nil
}

// handleRefreshProviderInfo handles POST /codex/provider/:id/refresh
func (h *CodexHandler) handleRefreshProviderInfo(w http.ResponseWriter, r *http.Request, idStr string) {
	providerID, err := strconv.Atoi(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid provider ID"})
		return
	}

	result, err := h.RefreshProviderInfo(r.Context(), providerID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// GetProviderUsage fetches the usage/quota information for a Codex provider
func (h *CodexHandler) GetProviderUsage(ctx context.Context, providerID int) (*codex.CodexUsageResponse, error) {
	// Get the provider
	provider, err := h.svc.GetProvider(uint64(providerID))
	if err != nil {
		return nil, fmt.Errorf("provider not found: %w", err)
	}

	if provider.Type != "codex" || provider.Config == nil || provider.Config.Codex == nil {
		return nil, fmt.Errorf("provider %s is not a codex provider", provider.Name)
	}

	codexConfig := provider.Config.Codex

	// Ensure we have an access token
	accessToken := codexConfig.AccessToken
	if accessToken == "" {
		// Need to refresh to get an access token
		if codexConfig.RefreshToken == "" {
			return nil, fmt.Errorf("provider %s has no refresh token", provider.Name)
		}

		result, err := codex.ValidateRefreshToken(ctx, codexConfig.RefreshToken)
		if err != nil {
			return nil, fmt.Errorf("failed to refresh token: %w", err)
		}
		if !result.Valid {
			return nil, fmt.Errorf("refresh token is invalid")
		}
		accessToken = result.AccessToken

		// Update provider with new access token
		codexConfig.AccessToken = result.AccessToken
		codexConfig.ExpiresAt = result.ExpiresAt
		if result.RefreshToken != "" && result.RefreshToken != codexConfig.RefreshToken {
			codexConfig.RefreshToken = result.RefreshToken
		}
		_ = h.svc.UpdateProvider(provider) // Best effort update
	} else {
		// Check if access token is expired
		if codexConfig.ExpiresAt != "" {
			expiresAt, err := time.Parse(time.RFC3339, codexConfig.ExpiresAt)
			if err == nil && time.Now().After(expiresAt.Add(-60*time.Second)) {
				// Token expired or about to expire, refresh it
				if codexConfig.RefreshToken != "" {
					result, err := codex.ValidateRefreshToken(ctx, codexConfig.RefreshToken)
					if err == nil && result.Valid {
						accessToken = result.AccessToken
						codexConfig.AccessToken = result.AccessToken
						codexConfig.ExpiresAt = result.ExpiresAt
						if result.RefreshToken != "" && result.RefreshToken != codexConfig.RefreshToken {
							codexConfig.RefreshToken = result.RefreshToken
						}
						_ = h.svc.UpdateProvider(provider)
					}
				}
			}
		}
	}

	// Fetch usage
	accountID := codexConfig.AccountID
	usage, err := codex.FetchUsage(ctx, accessToken, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch usage: %w", err)
	}

	return usage, nil
}

// handleGetProviderUsage handles GET /codex/provider/:id/usage
func (h *CodexHandler) handleGetProviderUsage(w http.ResponseWriter, r *http.Request, idStr string) {
	providerID, err := strconv.Atoi(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid provider ID"})
		return
	}

	usage, err := h.GetProviderUsage(r.Context(), providerID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, usage)
}

// handleForceRefreshQuotas handles POST /codex/refresh-quotas
func (h *CodexHandler) handleForceRefreshQuotas(w http.ResponseWriter, r *http.Request) {
	if h.taskSvc == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "task service not initialized"})
		return
	}

	refreshed := h.taskSvc.ForceRefreshQuotas(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"success":   true,
		"refreshed": refreshed,
	})
}

// handleSortRoutes handles POST /codex/sort-routes
func (h *CodexHandler) handleSortRoutes(w http.ResponseWriter, r *http.Request) {
	if h.taskSvc == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "task service not initialized"})
		return
	}

	h.taskSvc.SortRoutes(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// CodexBatchQuotaResult 批量配额查询结果
type CodexBatchQuotaResult struct {
	Quotas map[uint64]*codex.CodexQuotaResponse `json:"quotas"` // providerId -> quota
}

// GetBatchQuotas 批量获取所有 Codex provider 的配额信息（供 HTTP handler 和 Wails 共用）
// 优先从数据库返回缓存数据，即使过期也会返回（避免 API 请求阻塞）
// 配额刷新由后台任务负责
func (h *CodexHandler) GetBatchQuotas(ctx context.Context) (*CodexBatchQuotaResult, error) {
	// 获取所有 providers
	providers, err := h.svc.GetProviders()
	if err != nil {
		return nil, fmt.Errorf("failed to list providers: %w", err)
	}

	result := &CodexBatchQuotaResult{
		Quotas: make(map[uint64]*codex.CodexQuotaResponse),
	}

	// 过滤出 Codex providers 并获取配额
	for _, provider := range providers {
		if provider.Type != "codex" || provider.Config == nil || provider.Config.Codex == nil {
			continue
		}

		config := provider.Config.Codex
		email := config.Email

		// 优先从数据库获取缓存的配额（无论是否过期）
		if email != "" && h.quotaRepo != nil {
			cachedQuota, err := h.quotaRepo.GetByEmail(email)
			if err == nil && cachedQuota != nil {
				result.Quotas[provider.ID] = h.domainQuotaToResponse(cachedQuota)
				continue
			}
		}

		// 数据库没有缓存，尝试从 API 获取
		if config.RefreshToken == "" {
			continue
		}

		// 获取或刷新 access token
		accessToken := config.AccessToken
		if accessToken == "" || h.isTokenExpired(config.ExpiresAt) {
			tokenResp, err := codex.RefreshAccessToken(ctx, config.RefreshToken)
			if err != nil {
				// API 失败，跳过此 provider
				continue
			}
			accessToken = tokenResp.AccessToken

			// 更新 provider config
			config.AccessToken = tokenResp.AccessToken
			config.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).Format(time.RFC3339)
			if tokenResp.RefreshToken != "" && tokenResp.RefreshToken != config.RefreshToken {
				config.RefreshToken = tokenResp.RefreshToken
			}
			_ = h.svc.UpdateProvider(provider)
		}

		// 获取配额
		usage, err := codex.FetchUsage(ctx, accessToken, config.AccountID)
		if err != nil {
			// API 失败，跳过此 provider
			continue
		}

		// 保存到数据库
		if email != "" && h.quotaRepo != nil {
			h.saveQuotaToDB(email, config.AccountID, usage.PlanType, usage, false)
		}

		result.Quotas[provider.ID] = h.usageToResponse(email, config.AccountID, usage)
	}

	return result, nil
}

// handleGetBatchQuotas 批量获取所有 Codex provider 的配额信息
func (h *CodexHandler) handleGetBatchQuotas(w http.ResponseWriter, r *http.Request) {
	result, err := h.GetBatchQuotas(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// isTokenExpired checks if the access token is expired or about to expire
func (h *CodexHandler) isTokenExpired(expiresAt string) bool {
	if expiresAt == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return true
	}
	return time.Now().After(t.Add(-60 * time.Second))
}

// saveQuotaToDB saves Codex quota to database
func (h *CodexHandler) saveQuotaToDB(email, accountID, planType string, usage *codex.CodexUsageResponse, isForbidden bool) {
	if h.quotaRepo == nil || email == "" {
		return
	}

	quota := &domain.CodexQuota{
		Email:       email,
		AccountID:   accountID,
		PlanType:    planType,
		IsForbidden: isForbidden,
	}

	if usage != nil {
		if usage.RateLimit != nil {
			quota.PrimaryWindow = h.convertWindow(usage.RateLimit.PrimaryWindow)
			quota.SecondaryWindow = h.convertWindow(usage.RateLimit.SecondaryWindow)
		}
		if usage.CodeReviewRateLimit != nil {
			quota.CodeReviewWindow = h.convertWindow(usage.CodeReviewRateLimit.PrimaryWindow)
		}
	}

	h.quotaRepo.Upsert(quota)
}

// convertWindow converts codex package window to domain window
func (h *CodexHandler) convertWindow(w *codex.CodexUsageWindow) *domain.CodexQuotaWindow {
	if w == nil {
		return nil
	}
	return &domain.CodexQuotaWindow{
		UsedPercent:        w.UsedPercent,
		LimitWindowSeconds: w.LimitWindowSeconds,
		ResetAfterSeconds:  w.ResetAfterSeconds,
		ResetAt:            w.ResetAt,
	}
}

// usageToResponse converts usage response to quota response
func (h *CodexHandler) usageToResponse(email, accountID string, usage *codex.CodexUsageResponse) *codex.CodexQuotaResponse {
	resp := &codex.CodexQuotaResponse{
		Email:       email,
		AccountID:   accountID,
		IsForbidden: false,
		LastUpdated: time.Now().Unix(),
	}

	if usage != nil {
		resp.PlanType = usage.PlanType
		if usage.RateLimit != nil {
			resp.PrimaryWindow = usage.RateLimit.PrimaryWindow
			resp.SecondaryWindow = usage.RateLimit.SecondaryWindow
		}
		if usage.CodeReviewRateLimit != nil {
			resp.CodeReviewWindow = usage.CodeReviewRateLimit.PrimaryWindow
		}
	}

	return resp
}

// domainQuotaToResponse converts domain.CodexQuota to response format
func (h *CodexHandler) domainQuotaToResponse(q *domain.CodexQuota) *codex.CodexQuotaResponse {
	resp := &codex.CodexQuotaResponse{
		Email:       q.Email,
		AccountID:   q.AccountID,
		PlanType:    q.PlanType,
		IsForbidden: q.IsForbidden,
		LastUpdated: q.UpdatedAt.Unix(),
	}

	if q.PrimaryWindow != nil {
		resp.PrimaryWindow = &codex.CodexUsageWindow{
			UsedPercent:        q.PrimaryWindow.UsedPercent,
			LimitWindowSeconds: q.PrimaryWindow.LimitWindowSeconds,
			ResetAfterSeconds:  q.PrimaryWindow.ResetAfterSeconds,
			ResetAt:            q.PrimaryWindow.ResetAt,
		}
	}
	if q.SecondaryWindow != nil {
		resp.SecondaryWindow = &codex.CodexUsageWindow{
			UsedPercent:        q.SecondaryWindow.UsedPercent,
			LimitWindowSeconds: q.SecondaryWindow.LimitWindowSeconds,
			ResetAfterSeconds:  q.SecondaryWindow.ResetAfterSeconds,
			ResetAt:            q.SecondaryWindow.ResetAt,
		}
	}
	if q.CodeReviewWindow != nil {
		resp.CodeReviewWindow = &codex.CodexUsageWindow{
			UsedPercent:        q.CodeReviewWindow.UsedPercent,
			LimitWindowSeconds: q.CodeReviewWindow.LimitWindowSeconds,
			ResetAfterSeconds:  q.CodeReviewWindow.ResetAfterSeconds,
			ResetAt:            q.CodeReviewWindow.ResetAt,
		}
	}

	return resp
}

// OAuth success page HTML
const codexOAuthSuccessHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Authorization Successful</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
            margin: 0;
            background: linear-gradient(135deg, #10a37f 0%, #1a7f64 100%);
        }
        .container {
            background: white;
            padding: 3rem;
            border-radius: 1rem;
            box-shadow: 0 20px 60px rgba(0,0,0,0.3);
            text-align: center;
            max-width: 400px;
        }
        .icon {
            font-size: 4rem;
            margin-bottom: 1rem;
        }
        h1 {
            color: #2d3748;
            margin: 0 0 0.5rem 0;
            font-size: 1.5rem;
        }
        p {
            color: #718096;
            margin: 0;
            font-size: 0.95rem;
        }
        .spinner {
            width: 40px;
            height: 40px;
            margin: 1.5rem auto 0;
            border: 4px solid #e2e8f0;
            border-top: 4px solid #10a37f;
            border-radius: 50%;
            animation: spin 1s linear infinite;
        }
        @keyframes spin {
            0% { transform: rotate(0deg); }
            100% { transform: rotate(360deg); }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="icon">✅</div>
        <h1>Authorization Successful!</h1>
        <p>You can now close this window and return to the application.</p>
        <div class="spinner"></div>
    </div>
    <script>
        setTimeout(function() {
            window.close();
        }, 2000);
    </script>
</body>
</html>`

// OAuth error page HTML
const codexOAuthErrorHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Authorization Failed</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
            margin: 0;
            background: linear-gradient(135deg, #f093fb 0%, #f5576c 100%);
        }
        .container {
            background: white;
            padding: 3rem;
            border-radius: 1rem;
            box-shadow: 0 20px 60px rgba(0,0,0,0.3);
            text-align: center;
            max-width: 400px;
        }
        .icon {
            font-size: 4rem;
            margin-bottom: 1rem;
        }
        h1 {
            color: #2d3748;
            margin: 0 0 0.5rem 0;
            font-size: 1.5rem;
        }
        p {
            color: #718096;
            margin: 0;
            font-size: 0.95rem;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="icon">❌</div>
        <h1>Authorization Failed</h1>
        <p>Please return to the application and try again.</p>
    </div>
</body>
</html>`
