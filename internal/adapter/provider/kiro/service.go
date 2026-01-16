package kiro

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// KiroTokenValidationResult Kiro token 验证结果
type KiroTokenValidationResult struct {
	Valid            bool   `json:"valid"`
	Error            string `json:"error,omitempty"`
	Email            string `json:"email,omitempty"`
	UserId           string `json:"userId,omitempty"`
	SubscriptionType string `json:"subscriptionType,omitempty"`
	UsageLimit       int    `json:"usageLimit,omitempty"`
	CurrentUsage     int    `json:"currentUsage,omitempty"`
	DaysUntilReset   int    `json:"daysUntilReset,omitempty"`
	IsBanned         bool   `json:"isBanned"`
	BanReason        string `json:"banReason,omitempty"`
	ProfileArn       string `json:"profileArn,omitempty"`
	AccessToken      string `json:"accessToken,omitempty"`
	RefreshToken     string `json:"refreshToken,omitempty"`
}

// KiroQuotaData Kiro 配额数据（用于 API 响应，匹配 kiro2api）
type KiroQuotaData struct {
	TotalLimit       float64 `json:"total_limit"`                 // 总额度（包括基础+免费试用）
	Available        float64 `json:"available"`                   // 可用额度
	Used             float64 `json:"used"`                        // 已使用额度
	DaysUntilReset   int     `json:"days_until_reset"`            // 距离重置的天数
	SubscriptionType string  `json:"subscription_type,omitempty"` // 订阅类型
	FreeTrialStatus  string  `json:"free_trial_status,omitempty"` // 免费试用状态
	Email            string  `json:"email,omitempty"`             // 用户邮箱
	IsBanned         bool    `json:"is_banned"`                   // 是否被封禁
	BanReason        string  `json:"ban_reason,omitempty"`
	LastUpdated      int64   `json:"last_updated"`
}

// SocialRefreshResponse Social token 刷新响应
type SocialRefreshResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int    `json:"expiresIn"`
	ProfileArn   string `json:"profileArn"`
	CsrfToken    string `json:"csrfToken,omitempty"`
}

// UsageLimitsResponse 配额响应 (用于 service.go 的 getUsageLimits)
type UsageLimitsResponse struct {
	DaysUntilReset     int              `json:"daysUntilReset"`
	NextDateReset      float64          `json:"nextDateReset"`
	UserInfo           *UsageLimitsUser `json:"userInfo"`
	SubscriptionInfo   *SubscriptionInfo `json:"subscriptionInfo"`
	UsageBreakdownList []UsageBreakdown `json:"usageBreakdownList"`
}

// UsageLimitsUser 用户信息
type UsageLimitsUser struct {
	Email  string `json:"email"`
	UserId string `json:"userId"`
}

// ValidateSocialToken 验证 Social refresh token
func ValidateSocialToken(ctx context.Context, refreshToken string) (*KiroTokenValidationResult, error) {
	result := &KiroTokenValidationResult{
		Valid:        false,
		RefreshToken: refreshToken,
	}

	// 1. 刷新 token 获取 access token
	refreshResp, err := refreshSocialToken(ctx, refreshToken)
	if err != nil {
		result.Error = fmt.Sprintf("Token 刷新失败: %v", err)
		return result, nil
	}

	result.AccessToken = refreshResp.AccessToken
	result.RefreshToken = refreshResp.RefreshToken
	result.ProfileArn = refreshResp.ProfileArn

	// 2. 获取配额和用户信息
	usageResp, err := getUsageLimits(ctx, refreshResp.AccessToken, refreshResp.ProfileArn)
	if err != nil {
		// 检测封禁状态
		if strings.HasPrefix(err.Error(), "BANNED:") {
			result.IsBanned = true
			result.BanReason = strings.TrimPrefix(err.Error(), "BANNED:")
			result.Valid = true // Token 有效，但账号被封禁
			return result, nil
		}
		result.Error = fmt.Sprintf("获取配额失败: %v", err)
		return result, nil
	}

	// 3. 填充验证结果
	result.Valid = true
	result.DaysUntilReset = usageResp.DaysUntilReset

	if usageResp.UserInfo != nil {
		result.Email = usageResp.UserInfo.Email
		result.UserId = usageResp.UserInfo.UserId
	}

	if usageResp.SubscriptionInfo != nil {
		result.SubscriptionType = usageResp.SubscriptionInfo.Type
	}

	if len(usageResp.UsageBreakdownList) > 0 {
		result.UsageLimit = usageResp.UsageBreakdownList[0].UsageLimit
		result.CurrentUsage = usageResp.UsageBreakdownList[0].CurrentUsage
	}

	return result, nil
}

// refreshSocialToken 刷新 Social token
func refreshSocialToken(ctx context.Context, refreshToken string) (*SocialRefreshResponse, error) {
	reqBody, err := FastMarshal(RefreshRequest{RefreshToken: refreshToken})
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", RefreshTokenURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("刷新失败: 状态码 %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result SocialRefreshResponse
	if err := FastUnmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return &result, nil
}

// getUsageLimits 获取配额信息
func getUsageLimits(ctx context.Context, accessToken, profileArn string) (*UsageLimitsResponse, error) {
	// 构建 URL
	baseURL := fmt.Sprintf("https://codewhisperer.%s.amazonaws.com/getUsageLimits", DefaultRegion)
	params := url.Values{}
	params.Set("isEmailRequired", "true")
	params.Set("origin", "AI_EDITOR")
	params.Set("profileArn", profileArn)

	fullURL := baseURL + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// 解析错误响应，检测封禁
		var errResp map[string]interface{}
		if err := FastUnmarshal(body, &errResp); err == nil {
			if reason, ok := errResp["reason"].(string); ok {
				return nil, fmt.Errorf("BANNED:%s", reason)
			}
		}
		return nil, fmt.Errorf("API 错误: 状态码 %d, 响应: %s", resp.StatusCode, string(body))
	}

	var result UsageLimitsResponse
	if err := FastUnmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return &result, nil
}

// FetchQuota 获取 Kiro 配额（基于 refresh token，匹配 kiro2api 计算逻辑）
func FetchQuota(ctx context.Context, refreshToken string) (*KiroQuotaData, error) {
	// 1. 刷新 token 获取 access token
	refreshResp, err := refreshSocialToken(ctx, refreshToken)
	if err != nil {
		return nil, fmt.Errorf("token 刷新失败: %w", err)
	}

	// 2. 获取配额信息
	usageResp, err := getUsageLimits(ctx, refreshResp.AccessToken, refreshResp.ProfileArn)
	if err != nil {
		// 检测封禁状态
		if strings.HasPrefix(err.Error(), "BANNED:") {
			return &KiroQuotaData{
				IsBanned:    true,
				BanReason:   strings.TrimPrefix(err.Error(), "BANNED:"),
				LastUpdated: time.Now().Unix(),
			}, nil
		}
		return nil, err
	}

	// 3. 构建配额数据 (匹配 kiro2api 计算逻辑)
	quota := &KiroQuotaData{
		DaysUntilReset: usageResp.DaysUntilReset,
		LastUpdated:    time.Now().Unix(),
	}

	if usageResp.UserInfo != nil {
		quota.Email = usageResp.UserInfo.Email
	}

	if usageResp.SubscriptionInfo != nil {
		quota.SubscriptionType = usageResp.SubscriptionInfo.Type
	}

	// 计算配额 (匹配 kiro2api: 查找 CREDIT 类型的 breakdown)
	for _, breakdown := range usageResp.UsageBreakdownList {
		if breakdown.ResourceType == "CREDIT" {
			// 基础额度
			quota.TotalLimit = breakdown.UsageLimitWithPrecision
			quota.Used = breakdown.CurrentUsageWithPrecision

			// 免费试用额度
			if breakdown.FreeTrialInfo != nil && breakdown.FreeTrialInfo.FreeTrialStatus == "ACTIVE" {
				quota.TotalLimit += breakdown.FreeTrialInfo.UsageLimitWithPrecision
				quota.Used += breakdown.FreeTrialInfo.CurrentUsageWithPrecision
				quota.FreeTrialStatus = breakdown.FreeTrialInfo.FreeTrialStatus
			}

			// 计算可用额度
			quota.Available = quota.TotalLimit - quota.Used
			if quota.Available < 0 {
				quota.Available = 0
			}
			break
		}
	}

	return quota, nil
}
