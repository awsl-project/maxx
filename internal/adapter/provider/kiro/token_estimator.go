package kiro

import (
	"math"
	"strings"

	"github.com/awsl-project/maxx/internal/converter"
)

// TokenEstimator 本地 token 估算器
// 匹配 kiro2api/utils/token_estimator.go
type TokenEstimator struct{}

// NewTokenEstimator 创建 token 估算器实例
func NewTokenEstimator() *TokenEstimator {
	return &TokenEstimator{}
}

// EstimateInputTokens 估算请求的 input token 数量
func (e *TokenEstimator) EstimateInputTokens(req *converter.ClaudeRequest) int {
	totalTokens := 0

	// 1. 系统提示词
	if req.System != nil {
		systemContent := extractSystemContentForTokens(req.System)
		if systemContent != "" {
			totalTokens += e.EstimateTextTokens(systemContent)
			totalTokens += 2 // 系统提示的固定开销
		}
	}

	// 2. 消息内容
	for _, msg := range req.Messages {
		// 角色标记开销
		totalTokens += 3

		// 消息内容
		switch content := msg.Content.(type) {
		case string:
			totalTokens += e.EstimateTextTokens(content)
		case []interface{}:
			for _, block := range content {
				totalTokens += e.estimateContentBlock(block)
			}
		}
	}

	// 3. 工具定义
	// 匹配 kiro2api/utils/token_estimator.go:70-145
	toolCount := len(req.Tools)
	if toolCount > 0 {
		var baseToolsOverhead int
		var perToolOverhead int

		if toolCount == 1 {
			baseToolsOverhead = 0
			perToolOverhead = 320
		} else if toolCount <= 5 {
			baseToolsOverhead = 100
			perToolOverhead = 120
		} else {
			baseToolsOverhead = 180
			perToolOverhead = 60
		}

		totalTokens += baseToolsOverhead

		for _, tool := range req.Tools {
			// 工具名称
			nameTokens := e.estimateToolName(tool.Name)
			totalTokens += nameTokens

			// 工具描述
			totalTokens += e.EstimateTextTokens(tool.Description)

			// 工具 schema（JSON Schema）
			if tool.InputSchema != nil {
				if jsonBytes, err := FastMarshal(tool.InputSchema); err == nil {
					// Schema 编码密度：根据工具数量自适应
					var schemaCharsPerToken float64
					if toolCount == 1 {
						schemaCharsPerToken = 1.9
					} else if toolCount <= 5 {
						schemaCharsPerToken = 2.2
					} else {
						schemaCharsPerToken = 2.5
					}

					schemaLen := len(jsonBytes)
					schemaTokens := int(math.Ceil(float64(schemaLen) / schemaCharsPerToken))

					// $schema 字段 URL 开销
					if strings.Contains(string(jsonBytes), "$schema") {
						if toolCount == 1 {
							schemaTokens += 10
						} else {
							schemaTokens += 5
						}
					}

					// 最小 schema 开销
					minSchemaTokens := 50
					if toolCount > 5 {
						minSchemaTokens = 30
					}
					if schemaTokens < minSchemaTokens {
						schemaTokens = minSchemaTokens
					}

					totalTokens += schemaTokens
				}
			}

			totalTokens += perToolOverhead
		}
	}

	// 4. 基础请求开销
	totalTokens += 4

	return totalTokens
}

// EstimateTextTokens 估算纯文本的 token 数量
// 匹配 kiro2api/utils/token_estimator.go:EstimateTextTokens
func (e *TokenEstimator) EstimateTextTokens(text string) int {
	if text == "" {
		return 0
	}

	runes := []rune(text)
	runeCount := len(runes)

	if runeCount == 0 {
		return 0
	}

	// 统计中文字符数
	chineseChars := 0
	for _, r := range runes {
		if r >= 0x4E00 && r <= 0x9FFF {
			chineseChars++
		}
	}

	nonChineseChars := runeCount - chineseChars
	isPureChinese := (nonChineseChars == 0)

	// 中文 token 计算
	chineseTokens := 0
	if chineseChars > 0 {
		if isPureChinese {
			chineseTokens = 1 + chineseChars
		} else {
			chineseTokens = chineseChars
		}
	}

	// 英文/数字字符
	nonChineseTokens := 0
	if nonChineseChars > 0 {
		var charsPerToken float64
		if nonChineseChars < 50 {
			charsPerToken = 2.8
		} else if nonChineseChars < 100 {
			charsPerToken = 2.6
		} else {
			charsPerToken = 2.5
		}

		nonChineseTokens = int(math.Ceil(float64(nonChineseChars) / charsPerToken))
		if nonChineseTokens < 1 {
			nonChineseTokens = 1
		}
	}

	tokens := chineseTokens + nonChineseTokens

	// 长文本压缩系数
	if runeCount >= 1000 {
		tokens = int(float64(tokens) * 0.60)
	} else if runeCount >= 500 {
		tokens = int(float64(tokens) * 0.70)
	} else if runeCount >= 300 {
		tokens = int(float64(tokens) * 0.80)
	} else if runeCount >= 200 {
		tokens = int(float64(tokens) * 0.85)
	} else if runeCount >= 100 {
		tokens = int(float64(tokens) * 0.90)
	} else if runeCount >= 50 {
		tokens = int(float64(tokens) * 0.95)
	}

	if tokens < 1 {
		tokens = 1
	}

	return tokens
}

// estimateToolName 估算工具名称的 token 数量
func (e *TokenEstimator) estimateToolName(name string) int {
	if name == "" {
		return 0
	}

	baseTokens := (len(name) + 1) / 2

	underscoreCount := strings.Count(name, "_")
	underscorePenalty := underscoreCount

	camelCaseCount := 0
	for _, r := range name {
		if r >= 'A' && r <= 'Z' {
			camelCaseCount++
		}
	}
	camelCasePenalty := camelCaseCount / 2

	totalTokens := baseTokens + underscorePenalty + camelCasePenalty
	if totalTokens < 2 {
		totalTokens = 2
	}

	return totalTokens
}

// estimateContentBlock 估算单个内容块的 token 数量
// 匹配 kiro2api/utils/token_estimator.go:estimateContentBlock
func (e *TokenEstimator) estimateContentBlock(block any) int {
	blockMap, ok := block.(map[string]interface{})
	if !ok {
		return 10 // 未知格式，保守估算
	}

	blockType, _ := blockMap["type"].(string)

	switch blockType {
	case "text":
		// 文本块
		if text, ok := blockMap["text"].(string); ok {
			return e.EstimateTextTokens(text)
		}
		return 10

	case "image":
		// 图片：官方文档显示约 1000-2000 tokens
		return 1500

	case "document":
		// 文档：根据大小估算（简化处理）
		return 500

	case "tool_use":
		// 工具调用（在历史消息中的 assistant 消息可能包含）
		toolName, _ := blockMap["name"].(string)
		toolInput, _ := blockMap["input"].(map[string]any)
		return e.EstimateToolUseTokens(toolName, toolInput)

	case "tool_result":
		// 工具执行结果
		content := blockMap["content"]
		switch c := content.(type) {
		case string:
			return e.EstimateTextTokens(c)
		case []any:
			total := 0
			for _, item := range c {
				total += e.estimateContentBlock(item)
			}
			return total
		default:
			return 50
		}

	default:
		// 未知类型：JSON 长度估算
		if jsonBytes, err := FastMarshal(block); err == nil {
			return len(jsonBytes) / 4
		}
		return 10
	}
}

// EstimateToolUseTokens 精确估算工具调用的 token 数量
// 匹配 kiro2api/utils/token_estimator.go:EstimateToolUseTokens
func (e *TokenEstimator) EstimateToolUseTokens(toolName string, toolInput map[string]any) int {
	totalTokens := 0

	// 1. JSON 结构字段开销
	// "type": "tool_use" ≈ 3 tokens
	totalTokens += 3

	// "id": "toolu_01A09q90qw90lq917835lq9" ≈ 8 tokens
	totalTokens += 8

	// "name" 关键字 ≈ 1 token
	totalTokens += 1

	// 2. 工具名称（使用与输入侧相同的精确方法）
	nameTokens := e.estimateToolName(toolName)
	totalTokens += nameTokens

	// 3. "input" 关键字 ≈ 1 token
	totalTokens += 1

	// 4. 参数内容（JSON 序列化）
	// 匹配 kiro2api: 使用标准的 4 字符/token 比率
	if len(toolInput) > 0 {
		if jsonBytes, err := FastMarshal(toolInput); err == nil {
			inputTokens := len(jsonBytes) / 4
			totalTokens += inputTokens
		}
	} else {
		// 空参数对象 {} ≈ 1 token
		totalTokens += 1
	}

	return totalTokens
}

// extractSystemContentForTokens 提取系统消息内容用于 token 计算
func extractSystemContentForTokens(system interface{}) string {
	switch s := system.(type) {
	case string:
		return s
	case []interface{}:
		var parts []string
		for _, item := range s {
			if block, ok := item.(map[string]interface{}); ok {
				if text, ok := block["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}
