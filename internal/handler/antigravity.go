package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Bowl42/maxx-next/internal/adapter/provider/antigravity"
	"github.com/Bowl42/maxx-next/internal/domain"
	"github.com/Bowl42/maxx-next/internal/event"
	"github.com/Bowl42/maxx-next/internal/repository"
	"github.com/Bowl42/maxx-next/internal/service"
)

// AntigravityHandler handles Antigravity-specific API requests
type AntigravityHandler struct {
	svc          *service.AdminService
	quotaRepo    repository.AntigravityQuotaRepository
	oauthManager *antigravity.OAuthManager
}

// NewAntigravityHandler creates a new Antigravity handler
func NewAntigravityHandler(svc *service.AdminService, quotaRepo repository.AntigravityQuotaRepository, broadcaster event.Broadcaster) *AntigravityHandler {
	return &AntigravityHandler{
		svc:          svc,
		quotaRepo:    quotaRepo,
		oauthManager: antigravity.NewOAuthManager(broadcaster),
	}
}

// ServeHTTP routes Antigravity requests
// Routes:
//   POST /antigravity/validate-token - 验证单个 refresh token
//   POST /antigravity/validate-tokens - 批量验证 refresh tokens
//   GET  /antigravity/providers/{id}/quota - 获取 provider 的配额信息
//   POST /antigravity/oauth/start - 启动 OAuth 流程
//   GET  /antigravity/oauth/callback - OAuth 回调
func (h *AntigravityHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/antigravity")
	path = strings.TrimSuffix(path, "/")

	parts := strings.Split(path, "/")

	// POST /antigravity/validate-token
	if len(parts) >= 2 && parts[1] == "validate-token" && r.Method == http.MethodPost {
		h.handleValidateToken(w, r)
		return
	}

	// POST /antigravity/validate-tokens
	if len(parts) >= 2 && parts[1] == "validate-tokens" && r.Method == http.MethodPost {
		h.handleValidateTokens(w, r)
		return
	}

	// GET /antigravity/providers/{id}/quota
	if len(parts) >= 4 && parts[1] == "providers" && parts[3] == "quota" {
		id, _ := strconv.ParseUint(parts[2], 10, 64)
		if id > 0 {
			h.handleGetQuota(w, r, id)
			return
		}
	}

	// POST /antigravity/oauth/start
	if len(parts) >= 3 && parts[1] == "oauth" && parts[2] == "start" && r.Method == http.MethodPost {
		h.handleOAuthStart(w, r)
		return
	}

	// GET /antigravity/oauth/callback
	if len(parts) >= 3 && parts[1] == "oauth" && parts[2] == "callback" && r.Method == http.MethodGet {
		h.handleOAuthCallback(w, r)
		return
	}

	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

// handleValidateToken 验证单个 refresh token
func (h *AntigravityHandler) handleValidateToken(w http.ResponseWriter, r *http.Request) {
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

	result, err := antigravity.ValidateRefreshToken(r.Context(), req.RefreshToken)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// 保存配额到数据库（基于邮箱）
	if result.Valid && result.UserInfo != nil && result.UserInfo.Email != "" {
		h.saveQuotaToDB(result.UserInfo.Email, result.UserInfo.Name, result.UserInfo.Picture, result.ProjectID, result.Quota)
	}

	writeJSON(w, http.StatusOK, result)
}

// saveQuotaToDB 保存配额到数据库
func (h *AntigravityHandler) saveQuotaToDB(email, name, picture, projectID string, quota *antigravity.QuotaData) {
	if h.quotaRepo == nil || email == "" {
		return
	}

	var models []domain.AntigravityModelQuota
	var subscriptionTier string
	var isForbidden bool
	var lastUpdated int64

	if quota != nil {
		models = make([]domain.AntigravityModelQuota, len(quota.Models))
		for i, m := range quota.Models {
			models[i] = domain.AntigravityModelQuota{
				Name:       m.Name,
				Percentage: m.Percentage,
				ResetTime:  m.ResetTime,
			}
		}
		subscriptionTier = quota.SubscriptionTier
		isForbidden = quota.IsForbidden
		lastUpdated = quota.LastUpdated
	} else {
		lastUpdated = time.Now().Unix()
	}

	domainQuota := &domain.AntigravityQuota{
		Email:            email,
		Name:             name,
		Picture:          picture,
		ProjectID:        projectID,
		SubscriptionTier: subscriptionTier,
		IsForbidden:      isForbidden,
		Models:           models,
		LastUpdated:      lastUpdated,
	}

	h.quotaRepo.Upsert(domainQuota)
}

// handleValidateTokens 批量验证 refresh tokens
func (h *AntigravityHandler) handleValidateTokens(w http.ResponseWriter, r *http.Request) {
	var req struct {
		// 可以是 tokens 数组或多行文本
		Tokens    []string `json:"tokens,omitempty"`
		TokenText string   `json:"tokenText,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	var tokens []string
	if len(req.Tokens) > 0 {
		tokens = req.Tokens
	} else if req.TokenText != "" {
		tokens = antigravity.ParseRefreshTokens(req.TokenText)
	}

	if len(tokens) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no valid tokens provided"})
		return
	}

	// 限制批量验证数量
	if len(tokens) > 50 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "too many tokens (max 50)"})
		return
	}

	results := antigravity.BatchValidateRefreshTokens(r.Context(), tokens)

	// 保存每个有效的验证结果到数据库
	for _, result := range results {
		if result.Valid && result.UserInfo != nil && result.UserInfo.Email != "" {
			h.saveQuotaToDB(result.UserInfo.Email, result.UserInfo.Name, result.UserInfo.Picture, result.ProjectID, result.Quota)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"results": results,
		"total":   len(results),
	})
}

// handleGetQuota 获取 provider 的配额信息
func (h *AntigravityHandler) handleGetQuota(w http.ResponseWriter, r *http.Request, providerID uint64) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	// 获取 provider
	provider, err := h.svc.GetProvider(providerID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "provider not found"})
		return
	}

	// 检查是否为 Antigravity provider
	if provider.Type != "antigravity" || provider.Config == nil || provider.Config.Antigravity == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "not an Antigravity provider"})
		return
	}

	config := provider.Config.Antigravity
	email := config.Email

	// 检查 refresh 参数 - 是否强制刷新
	forceRefresh := r.URL.Query().Get("refresh") == "true"

	// 尝试从数据库获取缓存的配额（如果不是强制刷新）
	if !forceRefresh && email != "" && h.quotaRepo != nil {
		cachedQuota, err := h.quotaRepo.GetByEmail(email)
		if err == nil && cachedQuota != nil {
			// 检查是否过期（5分钟）
			if time.Now().Unix()-cachedQuota.LastUpdated < 300 {
				writeJSON(w, http.StatusOK, h.domainQuotaToResponse(cachedQuota))
				return
			}
		}
	}

	// 从 API 获取最新配额
	quota, err := antigravity.FetchQuotaForProvider(r.Context(), config.RefreshToken, config.ProjectID)
	if err != nil {
		// 如果 API 失败，尝试返回缓存数据
		if email != "" && h.quotaRepo != nil {
			cachedQuota, _ := h.quotaRepo.GetByEmail(email)
			if cachedQuota != nil {
				writeJSON(w, http.StatusOK, h.domainQuotaToResponse(cachedQuota))
				return
			}
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// 保存到数据库（从缓存获取已有的 name 和 picture，因为 FetchQuotaForProvider 不返回这些）
	if email != "" {
		// 尝试保留已有的用户信息
		var name, picture string
		if cachedQuota, _ := h.quotaRepo.GetByEmail(email); cachedQuota != nil {
			name = cachedQuota.Name
			picture = cachedQuota.Picture
		}
		h.saveQuotaToDB(email, name, picture, config.ProjectID, quota)
	}

	writeJSON(w, http.StatusOK, quota)
}

// domainQuotaToResponse 将数据库模型转换为 API 响应
func (h *AntigravityHandler) domainQuotaToResponse(quota *domain.AntigravityQuota) *antigravity.QuotaData {
	models := make([]antigravity.ModelQuota, len(quota.Models))
	for i, m := range quota.Models {
		models[i] = antigravity.ModelQuota{
			Name:       m.Name,
			Percentage: m.Percentage,
			ResetTime:  m.ResetTime,
		}
	}

	return &antigravity.QuotaData{
		Models:           models,
		LastUpdated:      quota.LastUpdated,
		IsForbidden:      quota.IsForbidden,
		SubscriptionTier: quota.SubscriptionTier,
	}
}

// ============================================================================
// OAuth 授权处理函数
// ============================================================================

// handleOAuthStart 启动 OAuth 授权流程
func (h *AntigravityHandler) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	// 生成随机 state token
	state, err := h.oauthManager.GenerateState()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate state"})
		return
	}

	// 创建 OAuth 会话
	h.oauthManager.CreateSession(state)

	// 构建回调 URL（使用当前请求的 host）
	redirectURI := fmt.Sprintf("%s://%s/antigravity/oauth/callback", getScheme(r), r.Host)

	// 构建 Google OAuth 授权 URL
	authURL := antigravity.GetAuthURL(redirectURI, state)

	// 返回授权 URL 和 state
	writeJSON(w, http.StatusOK, map[string]string{
		"authURL": authURL,
		"state":   state,
	})
}

// handleOAuthCallback 处理 Google OAuth 回调
func (h *AntigravityHandler) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	// 获取 code 和 state
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		h.sendOAuthErrorResult(w, state, "Missing code or state parameter")
		return
	}

	// 验证 state
	session, ok := h.oauthManager.GetSession(state)
	if !ok {
		h.sendOAuthErrorResult(w, state, "Invalid or expired state")
		return
	}

	_ = session // session 可用于将来扩展

	// 构建回调 URL
	redirectURI := fmt.Sprintf("%s://%s/antigravity/oauth/callback", getScheme(r), r.Host)

	// 使用 code 交换 tokens
	accessToken, refreshToken, _, err := antigravity.ExchangeCodeForTokens(r.Context(), code, redirectURI)
	if err != nil {
		h.sendOAuthErrorResult(w, state, fmt.Sprintf("Token exchange failed: %v", err))
		return
	}

	// 获取用户信息
	userInfo, err := antigravity.FetchUserInfo(r.Context(), accessToken)
	if err != nil {
		h.sendOAuthErrorResult(w, state, fmt.Sprintf("Failed to fetch user info: %v", err))
		return
	}

	// 获取项目信息和订阅等级
	projectID, tier, err := antigravity.FetchProjectInfo(r.Context(), accessToken, userInfo.Email)
	if err != nil {
		// Project info 获取失败不算致命错误
		projectID = antigravity.DefaultProjectID
		tier = "FREE"
	}

	// 获取配额信息
	quota, err := antigravity.FetchQuota(r.Context(), accessToken, projectID)
	if err != nil {
		// 配额获取失败也不算致命错误
		quota = &antigravity.QuotaData{
			SubscriptionTier: tier,
			LastUpdated:      time.Now().Unix(),
		}
	} else {
		quota.SubscriptionTier = tier
	}

	// 保存配额到数据库
	h.saveQuotaToDB(userInfo.Email, userInfo.Name, userInfo.Picture, projectID, quota)

	// 推送成功结果到前端
	result := &antigravity.OAuthResult{
		State:        state,
		Success:      true,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Email:        userInfo.Email,
		ProjectID:    projectID,
		UserInfo:     userInfo,
		Quota:        quota,
	}

	h.oauthManager.CompleteSession(state, result)

	// 返回成功页面
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(oauthSuccessHTML))
}

// sendOAuthErrorResult 发送 OAuth 错误结果并返回错误页面
func (h *AntigravityHandler) sendOAuthErrorResult(w http.ResponseWriter, state, errorMsg string) {
	// 推送错误结果到前端
	result := &antigravity.OAuthResult{
		State:   state,
		Success: false,
		Error:   errorMsg,
	}

	h.oauthManager.CompleteSession(state, result)

	// 返回错误页面
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte(oauthErrorHTML))
}

// getScheme 从请求中获取协议 (http 或 https)
func getScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if scheme := r.Header.Get("X-Forwarded-Proto"); scheme != "" {
		return scheme
	}
	return "http"
}

// OAuth 成功页面 HTML
const oauthSuccessHTML = `<!DOCTYPE html>
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
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
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
            border-top: 4px solid #667eea;
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

// OAuth 错误页面 HTML
const oauthErrorHTML = `<!DOCTYPE html>
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

