package kiro

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/awsl-project/maxx/internal/converter"
)

// ConvertClaudeToCodeWhisperer 将 Claude 请求转换为 CodeWhisperer 请求
// req 参数用于生成稳定的会话ID (匹配 kiro2api)
func ConvertClaudeToCodeWhisperer(requestBody []byte, modelMapping map[string]string, req *http.Request) ([]byte, string, error) {
	var claudeReq converter.ClaudeRequest
	if err := FastUnmarshal(requestBody, &claudeReq); err != nil {
		return nil, "", fmt.Errorf("解析 Claude 请求失败: %w", err)
	}

	// 映射模型
	mappedModel := MapModel(claudeReq.Model, modelMapping)
	if mappedModel == "" {
		return nil, "", fmt.Errorf("不支持的模型: %s", claudeReq.Model)
	}

	// 构建 CodeWhisperer 请求
	cwReq := CodeWhispererRequest{}

	// 设置代理相关字段 (使用稳定的ID生成器，匹配 kiro2api)
	cwReq.ConversationState.AgentContinuationId = GenerateStableAgentContinuationID(req)
	cwReq.ConversationState.AgentTaskType = "vibe"
	cwReq.ConversationState.ChatTriggerType = determineChatTriggerType(claudeReq)
	cwReq.ConversationState.ConversationId = GenerateStableConversationID(req)

	// 处理消息
	if len(claudeReq.Messages) == 0 {
		return nil, "", fmt.Errorf("消息列表为空")
	}

	// 处理最后一条消息作为 currentMessage
	lastMessage := claudeReq.Messages[len(claudeReq.Messages)-1]
	textContent, images, toolResults, err := processMessageContent(lastMessage.Content)
	if err != nil {
		return nil, "", fmt.Errorf("处理消息内容失败: %w", err)
	}

	// 设置当前消息
	cwReq.ConversationState.CurrentMessage.UserInputMessage.Content = textContent
	// 确保 Images 字段始终是数组，即使为空 (matching kiro2api)
	if len(images) > 0 {
		cwReq.ConversationState.CurrentMessage.UserInputMessage.Images = images
	} else {
		cwReq.ConversationState.CurrentMessage.UserInputMessage.Images = []CodeWhispererImage{}
	}
	cwReq.ConversationState.CurrentMessage.UserInputMessage.ModelId = mappedModel
	cwReq.ConversationState.CurrentMessage.UserInputMessage.Origin = "AI_EDITOR"

	// 如果有工具结果，设置到 context 中
	if len(toolResults) > 0 {
		cwReq.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext.ToolResults = toolResults
		// 对于包含 tool_result 的请求，content 应该为空字符串
		cwReq.ConversationState.CurrentMessage.UserInputMessage.Content = ""
	}

	// 处理工具定义
	if len(claudeReq.Tools) > 0 {
		tools := convertTools(claudeReq.Tools)
		cwReq.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext.Tools = tools
	}

	// 构建历史消息 (匹配 kiro2api 的条件判断逻辑)
	// 只在有系统消息、多条消息或有工具时才设置 History
	if claudeReq.System != nil || len(claudeReq.Messages) > 1 || len(claudeReq.Tools) > 0 {
		history := buildHistory(claudeReq, mappedModel)
		cwReq.ConversationState.History = history
	}
	// 否则 History 保持 nil (序列化为 null，匹配 kiro2api)

	// 验证请求完整性 (matching kiro2api validateCodeWhispererRequest)
	if err := validateCodeWhispererRequest(&cwReq); err != nil {
		return nil, "", fmt.Errorf("请求验证失败: %w", err)
	}

	// 序列化请求 (使用 SafeMarshal 匹配 kiro2api)
	result, err := SafeMarshal(cwReq)
	if err != nil {
		return nil, "", fmt.Errorf("序列化 CodeWhisperer 请求失败: %w", err)
	}

	return result, mappedModel, nil
}

// determineChatTriggerType 确定聊天触发类型
func determineChatTriggerType(req converter.ClaudeRequest) string {
	// 如果有工具调用，检查 tool_choice
	if len(req.Tools) > 0 && req.ToolChoice != nil {
		switch tc := req.ToolChoice.(type) {
		case map[string]interface{}:
			if tcType, ok := tc["type"].(string); ok {
				if tcType == "any" || tcType == "tool" {
					return "AUTO"
				}
			}
		case string:
			if tc == "any" || tc == "tool" {
				return "AUTO"
			}
		}
	}
	return "MANUAL"
}

// validateCodeWhispererRequest 验证 CodeWhisperer 请求的完整性 (matching kiro2api)
func validateCodeWhispererRequest(cwReq *CodeWhispererRequest) error {
	// 验证必需字段
	if cwReq.ConversationState.CurrentMessage.UserInputMessage.ModelId == "" {
		return fmt.Errorf("ModelId 不能为空")
	}

	if cwReq.ConversationState.ConversationId == "" {
		return fmt.Errorf("ConversationId 不能为空")
	}

	// 验证内容完整性
	trimmedContent := strings.TrimSpace(cwReq.ConversationState.CurrentMessage.UserInputMessage.Content)
	hasImages := len(cwReq.ConversationState.CurrentMessage.UserInputMessage.Images) > 0
	hasTools := len(cwReq.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext.Tools) > 0
	hasToolResults := len(cwReq.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext.ToolResults) > 0

	// 如果有工具结果，允许内容为空（这是工具执行后的反馈请求）
	if hasToolResults {
		return nil
	}

	// 如果没有内容但有工具，注入占位内容
	if trimmedContent == "" && !hasImages && hasTools {
		cwReq.ConversationState.CurrentMessage.UserInputMessage.Content = "执行工具任务"
		trimmedContent = "执行工具任务"
	}

	// 验证至少有内容或图片
	if trimmedContent == "" && !hasImages {
		return fmt.Errorf("用户消息内容和图片都为空")
	}

	return nil
}

// processMessageContent 处理消息内容，提取文本、图片和工具结果 (匹配 kiro2api)
func processMessageContent(content interface{}) (string, []CodeWhispererImage, []ToolResult, error) {
	var textParts []string
	var images []CodeWhispererImage
	var toolResults []ToolResult

	switch v := content.(type) {
	case string:
		return v, nil, nil, nil

	case []interface{}: // []any 和 []interface{} 是相同类型
		for _, item := range v {
			block, ok := item.(map[string]interface{})
			if !ok {
				continue
			}

			blockType, _ := block["type"].(string)
			switch blockType {
			case "text":
				if text, ok := block["text"].(string); ok {
					textParts = append(textParts, text)
				}

			case "image":
				if source, ok := block["source"].(map[string]interface{}); ok {
					img := convertImage(source)
					if img != nil {
						images = append(images, *img)
					}
				}

			case "tool_result":
				tr := extractToolResult(block)
				if tr != nil {
					toolResults = append(toolResults, *tr)
				}
			}
		}

	default:
		return "", nil, nil, fmt.Errorf("不支持的内容类型: %T", content)
	}

	return strings.Join(textParts, ""), images, toolResults, nil
}

// convertImage 转换图片格式
func convertImage(source map[string]interface{}) *CodeWhispererImage {
	mediaType, _ := source["media_type"].(string)
	data, _ := source["data"].(string)

	if data == "" {
		return nil
	}

	// 从 media_type 提取格式
	format := "png"
	if strings.Contains(mediaType, "jpeg") || strings.Contains(mediaType, "jpg") {
		format = "jpeg"
	} else if strings.Contains(mediaType, "gif") {
		format = "gif"
	} else if strings.Contains(mediaType, "webp") {
		format = "webp"
	}

	img := &CodeWhispererImage{
		Format: format,
	}
	img.Source.Bytes = data
	return img
}

// extractToolResult 提取工具结果 (匹配 kiro2api)
func extractToolResult(block map[string]interface{}) *ToolResult {
	toolUseId, _ := block["tool_use_id"].(string)
	if toolUseId == "" {
		return nil
	}

	tr := &ToolResult{
		ToolUseId: toolUseId,
		Status:    "success",
	}

	// 处理 is_error
	if isError, ok := block["is_error"].(bool); ok && isError {
		tr.Status = "error"
		tr.IsError = true
	}

	// 处理 content - 转换为数组格式 (匹配 kiro2api)
	if content, exists := block["content"]; exists {
		tr.Content = convertToolResultContent(content)
	}

	return tr
}

// convertToolResultContent 转换工具结果内容 (匹配 kiro2api)
func convertToolResultContent(content interface{}) []map[string]any {
	switch c := content.(type) {
	case string:
		return []map[string]any{{"text": c}}
	case []interface{}: // []any 和 []interface{} 是相同类型
		var result []map[string]any
		for _, item := range c {
			if m, ok := item.(map[string]interface{}); ok {
				result = append(result, convertMapInterface(m))
			}
		}
		return result
	case map[string]interface{}: // map[string]any 和 map[string]interface{} 是相同类型
		return []map[string]any{convertMapInterface(c)}
	default:
		return []map[string]any{{"text": fmt.Sprintf("%v", c)}}
	}
}

// convertMapInterface 转换 map[string]interface{} 到 map[string]any
func convertMapInterface(m map[string]interface{}) map[string]any {
	result := make(map[string]any)
	for k, v := range m {
		result[k] = v
	}
	return result
}

// convertTools 转换工具定义
func convertTools(tools []converter.ClaudeTool) []CodeWhispererTool {
	var result []CodeWhispererTool

	for _, tool := range tools {
		// 过滤 web_search 工具
		if tool.IsWebSearch() {
			continue
		}

		if tool.Name == "" {
			continue
		}

		cwTool := CodeWhispererTool{}
		cwTool.ToolSpecification.Name = tool.Name

		// 限制描述长度
		desc := tool.Description
		if len(desc) > MaxToolDescriptionLength {
			desc = desc[:MaxToolDescriptionLength]
		}
		cwTool.ToolSpecification.Description = desc

		// 转换 InputSchema (直接使用原始值，匹配 kiro2api)
		// kiro2api 在 converter/codewhisperer.go:337-339 直接使用 tool.InputSchema
		if tool.InputSchema != nil {
			if schema, ok := tool.InputSchema.(map[string]any); ok {
				cwTool.ToolSpecification.InputSchema = InputSchema{
					Json: schema, // 直接使用，不做浅拷贝
				}
			}
		}

		result = append(result, cwTool)
	}

	return result
}

// buildHistory 构建历史消息
func buildHistory(req converter.ClaudeRequest, modelId string) []any {
	var history []any

	// 处理系统消息
	if req.System != nil {
		systemContent := extractSystemContent(req.System)
		if systemContent != "" {
			userMsg := HistoryUserMessage{}
			userMsg.UserInputMessage.Content = systemContent
			userMsg.UserInputMessage.ModelId = modelId
			userMsg.UserInputMessage.Origin = "AI_EDITOR"
			history = append(history, userMsg)

			assistantMsg := HistoryAssistantMessage{}
			assistantMsg.AssistantResponseMessage.Content = "OK"
			history = append(history, assistantMsg)
		}
	}

	// 处理消息历史（除了最后一条）
	if len(req.Messages) <= 1 {
		return history
	}

	// 收集 user 消息并与 assistant 配对
	var userBuffer []converter.ClaudeMessage
	lastMessage := req.Messages[len(req.Messages)-1]

	// 确定历史消息的边界
	historyEndIndex := len(req.Messages) - 1
	if lastMessage.Role == "assistant" {
		historyEndIndex = len(req.Messages)
	}

	for i := 0; i < historyEndIndex; i++ {
		msg := req.Messages[i]

		if msg.Role == "user" {
			userBuffer = append(userBuffer, msg)
			continue
		}

		if msg.Role == "assistant" && len(userBuffer) > 0 {
			// 合并 user 消息
			userMsg := mergeUserMessages(userBuffer, modelId)
			history = append(history, userMsg)
			userBuffer = nil

			// 添加 assistant 消息
			assistantMsg := convertAssistantMessage(msg)
			history = append(history, assistantMsg)
		}
	}

	// 处理末尾孤立的 user 消息
	if len(userBuffer) > 0 {
		userMsg := mergeUserMessages(userBuffer, modelId)
		history = append(history, userMsg)

		// 自动配对 "OK" 响应
		assistantMsg := HistoryAssistantMessage{}
		assistantMsg.AssistantResponseMessage.Content = "OK"
		history = append(history, assistantMsg)
	}

	return history
}

// extractSystemContent 提取系统消息内容
func extractSystemContent(system interface{}) string {
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

// mergeUserMessages 合并多个 user 消息
func mergeUserMessages(messages []converter.ClaudeMessage, modelId string) HistoryUserMessage {
	var contentParts []string
	var allImages []CodeWhispererImage
	var allToolResults []ToolResult

	for _, msg := range messages {
		text, images, toolResults, _ := processMessageContent(msg.Content)
		if text != "" {
			contentParts = append(contentParts, text)
		}
		allImages = append(allImages, images...)
		allToolResults = append(allToolResults, toolResults...)
	}

	userMsg := HistoryUserMessage{}
	userMsg.UserInputMessage.Content = strings.Join(contentParts, "\n")
	userMsg.UserInputMessage.ModelId = modelId
	userMsg.UserInputMessage.Origin = "AI_EDITOR"

	if len(allImages) > 0 {
		userMsg.UserInputMessage.Images = allImages
	}

	if len(allToolResults) > 0 {
		userMsg.UserInputMessage.UserInputMessageContext.ToolResults = allToolResults
		userMsg.UserInputMessage.Content = ""
	}

	return userMsg
}

// convertAssistantMessage 转换 assistant 消息
func convertAssistantMessage(msg converter.ClaudeMessage) HistoryAssistantMessage {
	assistantMsg := HistoryAssistantMessage{}

	// 提取文本内容
	text, _, _, _ := processMessageContent(msg.Content)
	assistantMsg.AssistantResponseMessage.Content = text

	// 提取工具调用
	toolUses := extractToolUses(msg.Content)
	if len(toolUses) > 0 {
		assistantMsg.AssistantResponseMessage.ToolUses = toolUses
	}

	return assistantMsg
}

// extractToolUses 从 assistant 消息中提取工具调用
func extractToolUses(content interface{}) []ToolUseEntry {
	var toolUses []ToolUseEntry

	blocks, ok := content.([]interface{})
	if !ok {
		return nil
	}

	for _, item := range blocks {
		block, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		blockType, _ := block["type"].(string)
		if blockType != "tool_use" {
			continue
		}

		toolUse := ToolUseEntry{}

		if id, ok := block["id"].(string); ok {
			toolUse.ToolUseId = id
		}

		if name, ok := block["name"].(string); ok {
			// 过滤 web_search
			if name == "web_search" || name == "websearch" {
				continue
			}
			toolUse.Name = name
		}

		if input, ok := block["input"].(map[string]interface{}); ok {
			toolUse.Input = convertMapInterface(input)
		} else {
			toolUse.Input = map[string]any{}
		}

		toolUses = append(toolUses, toolUse)
	}

	return toolUses
}
