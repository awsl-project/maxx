package kiro

// UsageLimits 使用限制响应结构 (匹配 kiro2api/types/usage_limits.go)
type UsageLimits struct {
	Limits               []any            `json:"limits"`
	UsageBreakdownList   []UsageBreakdown `json:"usageBreakdownList"`
	UserInfo             UsageUserInfo    `json:"userInfo"`
	DaysUntilReset       int              `json:"daysUntilReset"`
	OverageConfiguration OverageConfig    `json:"overageConfiguration"`
	NextDateReset        float64          `json:"nextDateReset"`
	SubscriptionInfo     SubscriptionInfo `json:"subscriptionInfo"`
	UsageBreakdown       any              `json:"usageBreakdown"`
}

// UsageBreakdown 使用详细信息
type UsageBreakdown struct {
	NextDateReset                float64        `json:"nextDateReset"`
	OverageCharges               float64        `json:"overageCharges"`
	ResourceType                 string         `json:"resourceType"`
	Unit                         string         `json:"unit"`
	UsageLimit                   int            `json:"usageLimit"`
	UsageLimitWithPrecision      float64        `json:"usageLimitWithPrecision"`
	OverageRate                  float64        `json:"overageRate"`
	CurrentUsage                 int            `json:"currentUsage"`
	CurrentUsageWithPrecision    float64        `json:"currentUsageWithPrecision"`
	OverageCap                   int            `json:"overageCap"`
	OverageCapWithPrecision      float64        `json:"overageCapWithPrecision"`
	Currency                     string         `json:"currency"`
	CurrentOverages              int            `json:"currentOverages"`
	CurrentOveragesWithPrecision float64        `json:"currentOveragesWithPrecision"`
	FreeTrialInfo                *FreeTrialInfo `json:"freeTrialInfo,omitempty"`
	DisplayName                  string         `json:"displayName"`
	DisplayNamePlural            string         `json:"displayNamePlural"`
}

// FreeTrialInfo 免费试用信息
type FreeTrialInfo struct {
	FreeTrialExpiry           float64 `json:"freeTrialExpiry"`
	FreeTrialStatus           string  `json:"freeTrialStatus"`
	UsageLimit                int     `json:"usageLimit"`
	UsageLimitWithPrecision   float64 `json:"usageLimitWithPrecision"`
	CurrentUsage              int     `json:"currentUsage"`
	CurrentUsageWithPrecision float64 `json:"currentUsageWithPrecision"`
}

// UsageUserInfo 用户信息
type UsageUserInfo struct {
	Email  string `json:"email"`
	UserID string `json:"userId"`
}

// OverageConfig 超额配置
type OverageConfig struct {
	OverageStatus string `json:"overageStatus"`
}

// SubscriptionInfo 订阅信息
type SubscriptionInfo struct {
	SubscriptionManagementTarget string `json:"subscriptionManagementTarget"`
	OverageCapability            string `json:"overageCapability"`
	SubscriptionTitle            string `json:"subscriptionTitle"`
	Type                         string `json:"type"`
	UpgradeCapability            string `json:"upgradeCapability"`
}

// UsageInfo 简化的额度信息 (用于 API 返回)
type UsageInfo struct {
	TotalLimit       float64 `json:"total_limit"`
	Available        float64 `json:"available"`
	Used             float64 `json:"used"`
	DaysUntilReset   int     `json:"days_until_reset"`
	Email            string  `json:"email,omitempty"`
	SubscriptionType string  `json:"subscription_type,omitempty"`
	FreeTrialStatus  string  `json:"free_trial_status,omitempty"`
}

// CalculateUsageInfo 从 UsageLimits 计算简化的额度信息
func CalculateUsageInfo(limits *UsageLimits) *UsageInfo {
	if limits == nil {
		return nil
	}

	info := &UsageInfo{
		DaysUntilReset:   limits.DaysUntilReset,
		Email:            limits.UserInfo.Email,
		SubscriptionType: limits.SubscriptionInfo.Type,
	}

	for _, breakdown := range limits.UsageBreakdownList {
		if breakdown.ResourceType == "CREDIT" {
			// 基础额度
			info.TotalLimit = breakdown.UsageLimitWithPrecision
			info.Used = breakdown.CurrentUsageWithPrecision

			// 免费试用额度
			if breakdown.FreeTrialInfo != nil && breakdown.FreeTrialInfo.FreeTrialStatus == "ACTIVE" {
				info.TotalLimit += breakdown.FreeTrialInfo.UsageLimitWithPrecision
				info.Used += breakdown.FreeTrialInfo.CurrentUsageWithPrecision
				info.FreeTrialStatus = breakdown.FreeTrialInfo.FreeTrialStatus
			}

			info.Available = info.TotalLimit - info.Used
			if info.Available < 0 {
				info.Available = 0
			}
			break
		}
	}

	return info
}
