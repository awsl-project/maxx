package antigravity

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OAuth 配置（与 Antigravity Manager 一致）
const (
	GoogleTokenURL    = "https://oauth2.googleapis.com/token"
	GoogleUserInfoURL = "https://www.googleapis.com/oauth2/v2/userinfo"
	LoadCodeAssistURL = "https://cloudcode-pa.googleapis.com/v1internal:loadCodeAssist"
	FetchModelsURL    = "https://cloudcode-pa.googleapis.com/v1internal:fetchAvailableModels"

	// OAuth Client Credentials (from Antigravity Manager)
	OAuthClientID     = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"
	OAuthClientSecret = "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"

	// User-Agent (必须与 Antigravity 客户端一致)
	// loadCodeAssist 使用不带版本号的 User-Agent
	UserAgentLoadCodeAssist = "antigravity/windows/amd64"
	// fetchAvailableModels 使用带版本号的 User-Agent
	UserAgentFetchModels = "antigravity/1.11.3 Darwin/arm64"
	// 代理请求使用的 User-Agent
	AntigravityUserAgent = "antigravity/1.11.9 windows/amd64"

	// 默认 Project ID (当 API 未返回时使用)
	DefaultProjectID = "bamboo-precept-lgxtn"
)

// UserInfo 用户信息
type UserInfo struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

// ModelQuota 单个模型的配额信息
type ModelQuota struct {
	Name       string  `json:"name"`
	Percentage int     `json:"percentage"` // 剩余配额百分比 0-100
	ResetTime  string  `json:"resetTime"`  // 重置时间 ISO8601
}

// QuotaData 配额信息
type QuotaData struct {
	Models           []ModelQuota `json:"models"`
	LastUpdated      int64        `json:"lastUpdated"`      // Unix timestamp
	IsForbidden      bool         `json:"isForbidden"`      // 403 禁止访问
	SubscriptionTier string       `json:"subscriptionTier"` // FREE/PRO/ULTRA
}

// TokenValidationResult token 验证结果
type TokenValidationResult struct {
	Valid       bool      `json:"valid"`
	Error       string    `json:"error,omitempty"`
	UserInfo    *UserInfo `json:"userInfo,omitempty"`
	ProjectID   string    `json:"projectID,omitempty"`
	Quota       *QuotaData `json:"quota,omitempty"`
}

// ValidateRefreshToken 验证 refresh token 并获取用户信息和配额
func ValidateRefreshToken(ctx context.Context, refreshToken string) (*TokenValidationResult, error) {
	result := &TokenValidationResult{}

	// 1. 使用 refresh token 获取 access token
	accessToken, _, err := refreshGoogleToken(ctx, refreshToken)
	if err != nil {
		result.Valid = false
		result.Error = fmt.Sprintf("Token refresh failed: %v", err)
		return result, nil
	}

	// 2. 获取用户信息
	userInfo, err := fetchUserInfo(ctx, accessToken)
	if err != nil {
		result.Valid = false
		result.Error = fmt.Sprintf("Failed to fetch user info: %v", err)
		return result, nil
	}
	result.UserInfo = userInfo

	// 3. 获取 Project ID 和订阅信息
	projectID, tier, err := fetchProjectInfo(ctx, accessToken, userInfo.Email)
	if err != nil {
		// Project info 获取失败不算致命错误，继续
		result.Valid = true
		result.Error = fmt.Sprintf("Warning: Failed to fetch project info: %v", err)
		return result, nil
	}
	result.ProjectID = projectID

	// 4. 获取配额信息
	quota, err := fetchQuota(ctx, accessToken, projectID)
	if err != nil {
		result.Valid = true
		result.Quota = &QuotaData{
			LastUpdated:      time.Now().Unix(),
			IsForbidden:      strings.Contains(err.Error(), "403"),
			SubscriptionTier: tier,
		}
		return result, nil
	}
	quota.SubscriptionTier = tier
	result.Quota = quota
	result.Valid = true

	return result, nil
}

// FetchQuotaForProvider 为现有 provider 获取配额信息
func FetchQuotaForProvider(ctx context.Context, refreshToken, projectID string) (*QuotaData, error) {
	// 获取 access token
	accessToken, _, err := refreshGoogleToken(ctx, refreshToken)
	if err != nil {
		return nil, fmt.Errorf("token refresh failed: %w", err)
	}

	// 获取订阅信息
	_, tier, _ := fetchProjectInfoByID(ctx, accessToken, projectID)

	// 获取配额
	quota, err := fetchQuota(ctx, accessToken, projectID)
	if err != nil {
		if strings.Contains(err.Error(), "403") {
			return &QuotaData{
				LastUpdated:      time.Now().Unix(),
				IsForbidden:      true,
				SubscriptionTier: tier,
			}, nil
		}
		return nil, err
	}
	quota.SubscriptionTier = tier
	return quota, nil
}

// FetchUserInfo 获取用户信息 (导出版本供 handler 使用)
func FetchUserInfo(ctx context.Context, accessToken string) (*UserInfo, error) {
	return fetchUserInfo(ctx, accessToken)
}

// fetchUserInfo 获取用户信息
func fetchUserInfo(ctx context.Context, accessToken string) (*UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", GoogleUserInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var userInfo UserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, err
	}
	return &userInfo, nil
}

// FetchProjectInfo 获取 Project ID 和订阅信息 (导出版本供 handler 使用)
func FetchProjectInfo(ctx context.Context, accessToken, email string) (projectID, tier string, err error) {
	return fetchProjectInfo(ctx, accessToken, email)
}

// fetchProjectInfo 获取 Project ID 和订阅信息
func fetchProjectInfo(ctx context.Context, accessToken, email string) (projectID, tier string, err error) {
	payload := map[string]interface{}{
		"metadata": map[string]string{
			"ideType": "ANTIGRAVITY",
		},
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", LoadCodeAssistURL, strings.NewReader(string(body)))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgentLoadCodeAssist)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		CloudaicompanionProject string `json:"cloudaicompanionProject"`
		CurrentTier             struct {
			ID string `json:"id"`
		} `json:"currentTier"`
		PaidTier struct {
			ID string `json:"id"`
		} `json:"paidTier"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", err
	}

	projectID = result.CloudaicompanionProject
	// 如果 API 没有返回 projectID，使用默认值
	if projectID == "" {
		projectID = DefaultProjectID
	}
	// 优先使用 paidTier
	tier = result.PaidTier.ID
	if tier == "" {
		tier = result.CurrentTier.ID
	}
	if tier == "" {
		tier = "FREE"
	}

	return projectID, tier, nil
}

// fetchProjectInfoByID 通过已知 projectID 获取订阅信息
func fetchProjectInfoByID(ctx context.Context, accessToken, projectID string) (string, string, error) {
	payload := map[string]interface{}{
		"metadata": map[string]string{
			"ideType": "ANTIGRAVITY",
		},
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", LoadCodeAssistURL, strings.NewReader(string(body)))
	if err != nil {
		return projectID, "FREE", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	// loadCodeAssist 使用不带版本号的 User-Agent
	req.Header.Set("User-Agent", UserAgentLoadCodeAssist)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return projectID, "FREE", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return projectID, "FREE", nil
	}

	var result struct {
		CurrentTier struct {
			ID string `json:"id"`
		} `json:"currentTier"`
		PaidTier struct {
			ID string `json:"id"`
		} `json:"paidTier"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return projectID, "FREE", nil
	}

	tier := result.PaidTier.ID
	if tier == "" {
		tier = result.CurrentTier.ID
	}
	if tier == "" {
		tier = "FREE"
	}

	return projectID, tier, nil
}

// FetchQuota 获取配额信息 (导出版本供 handler 使用)
func FetchQuota(ctx context.Context, accessToken, projectID string) (*QuotaData, error) {
	return fetchQuota(ctx, accessToken, projectID)
}

// fetchQuota 获取配额信息
func fetchQuota(ctx context.Context, accessToken, projectID string) (*QuotaData, error) {
	payload := map[string]interface{}{
		"project": projectID,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", FetchModelsURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	// fetchAvailableModels 使用带版本号的 User-Agent
	req.Header.Set("User-Agent", UserAgentFetchModels)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("403 Forbidden")
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Models map[string]struct {
			QuotaInfo struct {
				RemainingFraction float64 `json:"remainingFraction"`
				ResetTime         string  `json:"resetTime"`
			} `json:"quotaInfo"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// 转换为我们的格式
	quota := &QuotaData{
		Models:      make([]ModelQuota, 0),
		LastUpdated: time.Now().Unix(),
	}

	// 关注的模型列表
	trackedModels := []string{
		"gemini-3-pro-high",
		"gemini-3-flash",
		"gemini-3-pro-image",
		"claude-sonnet-4-5-thinking",
	}

	for _, modelName := range trackedModels {
		if model, ok := result.Models[modelName]; ok {
			quota.Models = append(quota.Models, ModelQuota{
				Name:       modelName,
				Percentage: int(model.QuotaInfo.RemainingFraction * 100),
				ResetTime:  model.QuotaInfo.ResetTime,
			})
		}
	}

	return quota, nil
}

// BatchValidateRefreshTokens 批量验证 refresh tokens
func BatchValidateRefreshTokens(ctx context.Context, tokens []string) []*TokenValidationResult {
	results := make([]*TokenValidationResult, len(tokens))

	for i, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			results[i] = &TokenValidationResult{
				Valid: false,
				Error: "Empty token",
			}
			continue
		}

		result, err := ValidateRefreshToken(ctx, token)
		if err != nil {
			results[i] = &TokenValidationResult{
				Valid: false,
				Error: err.Error(),
			}
		} else {
			results[i] = result
		}
	}

	return results
}

// GenerateDefaultEndpoint 生成默认的 Antigravity 端点
func GenerateDefaultEndpoint(projectID string) string {
	return fmt.Sprintf("https://us-central1-aiplatform.googleapis.com/v1/projects/%s/locations/us-central1", projectID)
}

// ParseRefreshTokens 解析多行 refresh tokens
func ParseRefreshTokens(input string) []string {
	lines := strings.Split(input, "\n")
	tokens := make([]string, 0, len(lines))
	for _, line := range lines {
		token := strings.TrimSpace(line)
		if token != "" && strings.HasPrefix(token, "1//") {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

// ImportFromVSCodeDB 从 VSCode 数据库导入 tokens (简化版)
// 完整实现需要读取 SQLite 数据库
func ImportFromVSCodeDB(dbPath string) ([]string, error) {
	// VSCode Antigravity 数据库路径通常在:
	// macOS: ~/Library/Application Support/Code/User/globalStorage/*/auth.db
	// 这里返回空实现，完整版需要 SQLite 支持
	return nil, fmt.Errorf("VSCode DB import not implemented in server mode")
}

// ============================================================================
// OAuth 授权流程相关函数（参考 Antigravity-Manager oauth.rs）
// ============================================================================

const (
	GoogleAuthURL = "https://accounts.google.com/o/oauth2/v2/auth"
)

// GetAuthURL 构建 Google OAuth 授权 URL
// 参考: oauth.rs line 52-73
func GetAuthURL(redirectURI, state string) string {
	scopes := []string{
		"https://www.googleapis.com/auth/cloud-platform",
		"https://www.googleapis.com/auth/userinfo.email",
		"https://www.googleapis.com/auth/userinfo.profile",
		"https://www.googleapis.com/auth/cclog",
		"https://www.googleapis.com/auth/experimentsandconfigs",
	}

	params := make(map[string]string)
	params["client_id"] = OAuthClientID
	params["redirect_uri"] = redirectURI
	params["response_type"] = "code"
	params["scope"] = strings.Join(scopes, " ")
	params["state"] = state
	params["access_type"] = "offline"
	params["prompt"] = "consent"
	params["include_granted_scopes"] = "true"

	// 构建 URL
	queryParts := make([]string, 0, len(params))
	for k, v := range params {
		queryParts = append(queryParts, fmt.Sprintf("%s=%s", url.QueryEscape(k), url.QueryEscape(v)))
	}
	return GoogleAuthURL + "?" + strings.Join(queryParts, "&")
}

// ExchangeCodeForTokens 使用 authorization code 交换 access_token 和 refresh_token
// 参考: oauth.rs line 75-121
func ExchangeCodeForTokens(ctx context.Context, code, redirectURI string) (string, string, int, error) {
	data := url.Values{}
	data.Set("client_id", OAuthClientID)
	data.Set("client_secret", OAuthClientSecret)
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, "POST", GoogleTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", 0, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", "", 0, fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
		RefreshToken string `json:"refresh_token"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", "", 0, fmt.Errorf("failed to parse token response: %w", err)
	}

	// 检查 refresh_token 是否存在
	if tokenResp.RefreshToken == "" {
		return "", "", 0, fmt.Errorf("no refresh_token returned (user may have already authorized this app)")
	}

	return tokenResp.AccessToken, tokenResp.RefreshToken, tokenResp.ExpiresIn, nil
}
