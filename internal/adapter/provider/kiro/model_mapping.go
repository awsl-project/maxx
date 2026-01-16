package kiro

import (
	"strings"
)

// defaultModelMappingRules is the ordered list of default mapping rules
// Rules are matched in order, first match wins (higher priority first)
// Supports wildcard patterns: * matches any characters
var defaultModelMappingRules = []ModelMappingRule{
	// 精确匹配优先 (匹配 kiro2api 的 ModelMap)
	{"claude-sonnet-4-5", "CLAUDE_SONNET_4_5_20250929_V1_0"},
	{"claude-sonnet-4-5-20250929", "CLAUDE_SONNET_4_5_20250929_V1_0"},
	{"claude-sonnet-4-20250514", "CLAUDE_SONNET_4_20250514_V1_0"},
	{"claude-3-7-sonnet-20250219", "CLAUDE_3_7_SONNET_20250219_V1_0"},
	{"claude-3-5-haiku-20241022", "auto"},
	{"claude-haiku-4-5-20251001", "auto"},

	// 通配符规则 (按优先级排序)
	{"*opus*", "CLAUDE_SONNET_4_5_20250929_V1_0"},    // opus 变体 -> 最强模型
	{"*sonnet-4-5*", "CLAUDE_SONNET_4_5_20250929_V1_0"}, // sonnet 4.5 变体
	{"*sonnet-4*", "CLAUDE_SONNET_4_20250514_V1_0"},     // sonnet 4 变体
	{"*sonnet*", "CLAUDE_3_7_SONNET_20250219_V1_0"},     // 其他 sonnet 变体
	{"*haiku*", "auto"},                                 // haiku 变体 -> auto
}

// AvailableTargetModels is the list of valid target models for Kiro mapping
var AvailableTargetModels = []string{
	"CLAUDE_SONNET_4_5_20250929_V1_0",
	"CLAUDE_SONNET_4_20250514_V1_0",
	"CLAUDE_3_7_SONNET_20250219_V1_0",
	"auto",
}

// MapModel 将 Anthropic 模型名映射到 CodeWhisperer 模型 ID
// 优先级: customMapping > globalSettings > defaultModelMappingRules
func MapModel(model string, customMapping map[string]string) string {
	cleanInput := strings.TrimSpace(strings.ToLower(model))

	// 1. 优先使用自定义映射（精确匹配）
	if customMapping != nil {
		if mapped, ok := customMapping[model]; ok {
			return mapped
		}
		// 也尝试小写匹配
		if mapped, ok := customMapping[cleanInput]; ok {
			return mapped
		}
	}

	// 2. 检查全局设置（用户自定义规则）
	if globalSettings := GetGlobalSettings(); globalSettings != nil {
		if len(globalSettings.ModelMappingRules) > 0 {
			if mapped := MatchRulesInOrder(cleanInput, globalSettings.ModelMappingRules); mapped != "" {
				return mapped
			}
		}
	}

	// 3. 检查默认规则
	if mapped := MatchRulesInOrder(cleanInput, defaultModelMappingRules); mapped != "" {
		return mapped
	}

	// 4. 未找到映射，返回空字符串
	return ""
}

// MatchRulesInOrder matches input against rules in order, first match wins
// Returns the target model or empty string if no match
func MatchRulesInOrder(input string, rules []ModelMappingRule) string {
	for _, rule := range rules {
		if matchPattern(input, rule.Pattern) {
			return rule.Target
		}
	}
	return ""
}

// matchPattern checks if input matches the pattern (supports * wildcard)
func matchPattern(input, pattern string) bool {
	pattern = strings.ToLower(pattern)
	input = strings.ToLower(input)

	// 精确匹配
	if !strings.Contains(pattern, "*") {
		return input == pattern
	}

	// 通配符匹配
	parts := strings.Split(pattern, "*")

	// 处理 *xxx* 模式
	if len(parts) == 3 && parts[0] == "" && parts[2] == "" {
		return strings.Contains(input, parts[1])
	}

	// 处理 xxx* 模式
	if len(parts) == 2 && parts[1] == "" {
		return strings.HasPrefix(input, parts[0])
	}

	// 处理 *xxx 模式
	if len(parts) == 2 && parts[0] == "" {
		return strings.HasSuffix(input, parts[1])
	}

	// 处理 xxx*yyy 模式
	if len(parts) == 2 {
		return strings.HasPrefix(input, parts[0]) && strings.HasSuffix(input, parts[1])
	}

	return false
}

// GetDefaultModelMapping returns the default mapping as a map (for API compatibility)
// Note: The map loses ordering, use defaultModelMappingRules for ordered matching
func GetDefaultModelMapping() map[string]string {
	result := make(map[string]string, len(defaultModelMappingRules))
	for _, rule := range defaultModelMappingRules {
		result[rule.Pattern] = rule.Target
	}
	return result
}

// API 端点常量
const (
	// RefreshTokenURL Social 认证方式的 Token 刷新 URL
	RefreshTokenURL = "https://prod.us-east-1.auth.desktop.kiro.dev/refreshToken"

	// IdcRefreshTokenURL IdC 认证方式的 Token 刷新 URL (硬编码匹配 kiro2api)
	IdcRefreshTokenURL = "https://oidc.us-east-1.amazonaws.com/token"

	// CodeWhispererURLTemplate CodeWhisperer API URL 模板
	// 使用时需要替换 {region}
	CodeWhispererURLTemplate = "https://codewhisperer.%s.amazonaws.com/generateAssistantResponse"

	// DefaultRegion 默认 AWS 区域
	DefaultRegion = "us-east-1"
)

// MaxToolDescriptionLength 工具描述的最大长度（字符数）
const MaxToolDescriptionLength = 10000
