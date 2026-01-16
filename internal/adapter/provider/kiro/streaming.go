package kiro

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// ClaudeStreamingState 管理 CodeWhisperer → Claude SSE 转换状态
// 匹配 kiro2api/server/stream_processor.go 架构
type ClaudeStreamingState struct {
	mu sync.Mutex

	// 消息 ID
	messageID    string
	requestModel string

	// Token 跟踪
	inputTokens       int
	totalOutputTokens int
	tokenEstimator    *TokenEstimator

	// 状态管理器
	sseStateManager   *SSEStateManager
	stopReasonManager *StopReasonManager

	// 解析器
	compliantParser *CompliantEventStreamParser

	// 是否已发送初始事件
	sentMessageStart bool

	// 是否已发送结束事件
	sentMessageStop bool

	// 输出缓冲
	outputBuffer strings.Builder

	// 工具调用跟踪 (用于 stop_reason 判断)
	toolUseIdByBlockIndex map[int]string
	completedToolUseIds   map[string]bool

	// JSON 字节累加器（修复分段整除精度损失）
	jsonBytesByBlockIndex map[int]int

	// 统计信息（用于最小 token 保护）
	totalProcessedEvents int
}

// NewClaudeStreamingState 创建新的流式状态
func NewClaudeStreamingState(requestModel string, inputTokens int) *ClaudeStreamingState {
	s := &ClaudeStreamingState{
		messageID:             fmt.Sprintf("msg_%s", uuid.New().String()[:24]),
		requestModel:          requestModel,
		inputTokens:           inputTokens,
		tokenEstimator:        NewTokenEstimator(),
		sseStateManager:       NewSSEStateManager(&strings.Builder{}, false),
		stopReasonManager:     NewStopReasonManager(),
		compliantParser:       NewCompliantEventStreamParser(),
		toolUseIdByBlockIndex: make(map[int]string),
		completedToolUseIds:   make(map[string]bool),
		jsonBytesByBlockIndex: make(map[int]int),
	}
	return s
}

// ProcessEventStreamData 处理 EventStream 二进制数据，返回 Claude SSE 事件
// 匹配 kiro2api/server/stream_processor.go:ProcessEventStream
func (s *ClaudeStreamingState) ProcessEventStreamData(data []byte) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 重置输出缓冲
	s.outputBuffer.Reset()

	// 更新 SSE 状态管理器的 writer（保持状态，只更新 writer）
	s.sseStateManager.SetWriter(&s.outputBuffer)

	// 发送初始事件 (message_start)
	if !s.sentMessageStart {
		s.emitMessageStart()
		s.emitPing() // 匹配 kiro2api: 在 message_start 后发送 ping 事件
		s.sentMessageStart = true
	}

	// 使用 CompliantEventStreamParser 解析并转换事件
	events, err := s.compliantParser.ParseStream(data)
	if err != nil {
		return nil
	}

	// 直传模式：直接转发每个事件
	for _, event := range events {
		s.processEvent(event)
		s.totalProcessedEvents++
	}

	return []byte(s.outputBuffer.String())
}

// processEvent 处理单个 SSE 事件
// 匹配 kiro2api/server/stream_processor.go:processEvent
func (s *ClaudeStreamingState) processEvent(event SSEEvent) {
	eventType, _ := event.Data["type"].(string)

	// 处理不同类型的事件
	switch eventType {
	case "content_block_start":
		s.processToolUseStart(event.Data)
		// 统计工具块的结构开销
		if contentBlock, ok := event.Data["content_block"].(map[string]any); ok {
			blockType, _ := contentBlock["type"].(string)
			if blockType == "tool_use" {
				s.totalOutputTokens += 12 // 结构字段固定开销
				if toolName, ok := contentBlock["name"].(string); ok {
					s.totalOutputTokens += s.tokenEstimator.EstimateTextTokens(toolName)
				}
			}
		}

	case "content_block_delta":
		// 统计输出 token
		if delta, ok := event.Data["delta"].(map[string]any); ok {
			deltaType, _ := delta["type"].(string)
			switch deltaType {
			case "text_delta":
				if text, ok := delta["text"].(string); ok {
					s.totalOutputTokens += s.tokenEstimator.EstimateTextTokens(text)
				}
			case "input_json_delta":
				// 累加 JSON 字节数，延迟到 content_block_stop 时统一计算
				if partialJSON, ok := delta["partial_json"].(string); ok {
					index := extractBlockIndex(event.Data)
					s.jsonBytesByBlockIndex[index] += len(partialJSON)
				}
			}
		}

	case "content_block_stop":
		s.processToolUseStop(event.Data)
		// 在块结束时计算累加的 JSON 字节数的 token
		idx := extractBlockIndex(event.Data)
		if jsonBytes, exists := s.jsonBytesByBlockIndex[idx]; exists && jsonBytes > 0 {
			tokens := (jsonBytes + 3) / 4 // 进一法: ceil(jsonBytes / 4)
			s.totalOutputTokens += tokens
			delete(s.jsonBytesByBlockIndex, idx)
		}

	case "exception":
		// 检查是否需要映射为 max_tokens
		if s.handleExceptionEvent(event.Data) {
			return // 已处理，不转发原始事件
		}
	}

	// 使用状态管理器发送事件（直传）
	_ = s.sseStateManager.SendEvent(event.Data)
}

// processToolUseStart 处理工具使用开始事件
func (s *ClaudeStreamingState) processToolUseStart(dataMap map[string]any) {
	cb, ok := dataMap["content_block"].(map[string]any)
	if !ok {
		return
	}

	cbType, _ := cb["type"].(string)
	if cbType != "tool_use" {
		return
	}

	// 提取索引
	idx := extractBlockIndex(dataMap)
	if idx < 0 {
		return
	}

	// 提取 tool_use_id
	id, _ := cb["id"].(string)
	if id == "" {
		return
	}

	// 记录索引到 tool_use_id 的映射
	s.toolUseIdByBlockIndex[idx] = id
}

// processToolUseStop 处理工具使用结束事件
func (s *ClaudeStreamingState) processToolUseStop(dataMap map[string]any) {
	idx := extractBlockIndex(dataMap)
	if idx < 0 {
		return
	}

	if toolId, exists := s.toolUseIdByBlockIndex[idx]; exists && toolId != "" {
		// 记录到已完成工具集合
		s.completedToolUseIds[toolId] = true
		delete(s.toolUseIdByBlockIndex, idx)
	}
}

// handleExceptionEvent 处理上游异常事件
func (s *ClaudeStreamingState) handleExceptionEvent(dataMap map[string]any) bool {
	exceptionType, _ := dataMap["exception_type"].(string)

	// 检查是否为内容长度超限异常
	if exceptionType == "ContentLengthExceededException" ||
		strings.Contains(exceptionType, "CONTENT_LENGTH_EXCEEDS") {

		// 关闭所有活跃的 content_block
		activeBlocks := s.sseStateManager.GetActiveBlocks()
		for index, block := range activeBlocks {
			if block.Started && !block.Stopped {
				stopEvent := map[string]any{
					"type":  "content_block_stop",
					"index": index,
				}
				_ = s.sseStateManager.SendEvent(stopEvent)
			}
		}

		// 发送 max_tokens 响应（匹配 kiro2api/server/stream_processor.go:500-510）
		maxTokensEvent := map[string]any{
			"type": "message_delta",
			"delta": map[string]any{
				"stop_reason":   "max_tokens",
				"stop_sequence": nil,
			},
			"usage": map[string]any{
				"input_tokens":  s.inputTokens,
				"output_tokens": s.totalOutputTokens,
			},
		}
		_ = s.sseStateManager.SendEvent(maxTokensEvent)

		// 发送 message_stop
		stopEvent := map[string]any{
			"type": "message_stop",
		}
		_ = s.sseStateManager.SendEvent(stopEvent)
		s.sentMessageStop = true

		return true
	}

	return false
}

// EmitForceStop 强制发送结束事件
// 匹配 kiro2api/server/stream_processor.go:sendFinalEvents
func (s *ClaudeStreamingState) EmitForceStop() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sentMessageStop {
		return nil
	}

	// 重置输出缓冲
	s.outputBuffer.Reset()
	s.sseStateManager.SetWriter(&s.outputBuffer)

	// 关闭所有未关闭的 content_block
	// 匹配 kiro2api: 使用 sseStateManager.GetActiveBlocks()
	activeBlocks := s.sseStateManager.GetActiveBlocks()
	for index, block := range activeBlocks {
		if block.Started && !block.Stopped {
			stopEvent := map[string]any{
				"type":  "content_block_stop",
				"index": index,
			}
			_ = s.sseStateManager.SendEvent(stopEvent)
		}
	}

	// 也检查 message processor 中跟踪的文本块（兼容处理）
	msgProcessor := s.compliantParser.GetMessageProcessor()
	if msgProcessor.HasTextBlock() {
		textIdx := msgProcessor.GetCurrentTextIndex()
		// 检查是否已经在上面关闭过
		if block, exists := activeBlocks[textIdx]; !exists || !block.Stopped {
			s.emitContentBlockStop(textIdx)
		}
	}

	// 更新工具调用状态
	hasActiveTools := len(s.toolUseIdByBlockIndex) > 0
	hasCompletedTools := len(s.completedToolUseIds) > 0

	// 也检查 message processor 中的状态
	if len(msgProcessor.GetActiveTools()) > 0 {
		hasActiveTools = true
	}
	if len(msgProcessor.GetCompletedToolIds()) > 0 {
		hasCompletedTools = true
	}

	s.stopReasonManager.UpdateToolCallStatus(hasActiveTools, hasCompletedTools)
	stopReason := s.stopReasonManager.DetermineStopReason()

	s.emitMessageDelta(stopReason)
	s.emitMessageStop()
	s.sentMessageStop = true

	return []byte(s.outputBuffer.String())
}

// SSE 事件生成函数

func (s *ClaudeStreamingState) emitMessageStart() {
	event := map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            s.messageID,
			"type":          "message",
			"role":          "assistant",
			"content":       []any{},
			"model":         s.requestModel,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]any{
				"input_tokens":  s.inputTokens,
				"output_tokens": 0,
			},
		},
	}
	s.writeSSE(event)
}

func (s *ClaudeStreamingState) emitPing() {
	event := map[string]any{
		"type": "ping",
	}
	s.writeSSE(event)
}

func (s *ClaudeStreamingState) emitContentBlockStop(index int) {
	event := map[string]any{
		"type":  "content_block_stop",
		"index": index,
	}
	s.writeSSE(event)
}

func (s *ClaudeStreamingState) emitMessageDelta(stopReason string) {
	// 最小 token 保护（匹配 kiro2api/server/stream_processor.go:251-264）
	outputTokens := s.totalOutputTokens
	if outputTokens < 1 {
		// 检查是否有任何内容被发送
		hasContent := len(s.completedToolUseIds) > 0 ||
			len(s.toolUseIdByBlockIndex) > 0 ||
			s.totalProcessedEvents > 0

		if hasContent {
			outputTokens = 1 // 最小保护：至少 1 token
		}
	}

	event := map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": map[string]any{
			"output_tokens": outputTokens,
			"input_tokens":  s.inputTokens,
		},
	}
	s.writeSSE(event)
}

func (s *ClaudeStreamingState) emitMessageStop() {
	event := map[string]any{
		"type": "message_stop",
	}
	s.writeSSE(event)
}

// writeSSE 写入 SSE 事件到输出缓冲
func (s *ClaudeStreamingState) writeSSE(event map[string]any) {
	formatted := formatSSE(event)
	s.outputBuffer.WriteString(formatted)
}

// formatSSE 格式化 SSE 事件
func formatSSE(event map[string]any) string {
	eventType, _ := event["type"].(string)
	data, _ := json.Marshal(event)
	return fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, string(data))
}

// extractBlockIndex 从数据映射中提取索引
func extractBlockIndex(dataMap map[string]any) int {
	if v, ok := dataMap["index"].(int); ok {
		return v
	}
	if f, ok := dataMap["index"].(float64); ok {
		return int(f)
	}
	return -1
}
