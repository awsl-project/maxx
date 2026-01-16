package kiro

import (
	"encoding/json"
	"strings"
	"sync"
)

// CompliantMessageProcessor 符合规范的消息处理器
// 匹配 kiro2api/parser/compliant_message_processor.go
type CompliantMessageProcessor struct {
	mu sync.Mutex

	// 工具调用状态
	activeTools       map[string]*ToolState // toolUseId -> ToolState
	completedToolIds  map[string]bool
	nextBlockIndex    int
	currentTextIndex  int  // 当前文本块索引
	hasTextBlock      bool // 是否已有文本块

	// 内容跟踪
	sentMessageStart bool
}

// ToolState 工具调用状态
type ToolState struct {
	ToolUseId  string
	Name       string
	BlockIndex int
	Started    bool
	Stopped    bool
}

// NewCompliantMessageProcessor 创建消息处理器
func NewCompliantMessageProcessor() *CompliantMessageProcessor {
	return &CompliantMessageProcessor{
		activeTools:      make(map[string]*ToolState),
		completedToolIds: make(map[string]bool),
		currentTextIndex: 0,
	}
}

// Reset 重置处理器状态
func (cmp *CompliantMessageProcessor) Reset() {
	cmp.mu.Lock()
	defer cmp.mu.Unlock()

	cmp.activeTools = make(map[string]*ToolState)
	cmp.completedToolIds = make(map[string]bool)
	cmp.nextBlockIndex = 0
	cmp.currentTextIndex = 0
	cmp.hasTextBlock = false
	cmp.sentMessageStart = false
}

// ProcessMessage 处理单个 EventStreamMessage，返回 Claude SSE 事件
// 匹配 kiro2api/parser/compliant_message_processor.go:ProcessMessage
func (cmp *CompliantMessageProcessor) ProcessMessage(msg *EventStreamMessage) ([]SSEEvent, error) {
	cmp.mu.Lock()
	defer cmp.mu.Unlock()

	messageType := msg.GetMessageType()
	eventType := msg.GetEventType()

	switch messageType {
	case KiroMessageTypes.EVENT:
		return cmp.processEventMessage(msg, eventType)
	case KiroMessageTypes.ERROR:
		return cmp.processErrorMessage(msg)
	case KiroMessageTypes.EXCEPTION:
		return cmp.processExceptionMessage(msg)
	default:
		return []SSEEvent{}, nil
	}
}

// processEventMessage 处理事件消息
func (cmp *CompliantMessageProcessor) processEventMessage(msg *EventStreamMessage, eventType string) ([]SSEEvent, error) {
	switch eventType {
	case KiroEventTypes.ASSISTANT_RESPONSE_EVENT:
		return cmp.handleAssistantResponseEvent(msg.Payload)
	case KiroEventTypes.TOOL_USE_EVENT:
		return cmp.handleToolUseEvent(msg.Payload)
	case KiroEventTypes.END_OF_TURN_EVENT:
		return cmp.handleEndOfTurnEvent(msg.Payload)
	case "codeEvent":
		// codeEvent 也作为文本内容处理
		return cmp.handleCodeEvent(msg.Payload)
	default:
		// 尝试检测嵌套格式
		return cmp.handleNestedEvent(msg.Payload)
	}
}

// handleAssistantResponseEvent 处理助手响应事件
// 匹配 kiro2api/parser/message_event_handlers.go:handleStreamingEvent
// 关键修复：不再手动发送 content_block_start，由 sse_state_manager 自动处理
func (cmp *CompliantMessageProcessor) handleAssistantResponseEvent(payload []byte) ([]SSEEvent, error) {
	// 检查是否为工具调用事件
	if isToolCallPayload(payload) {
		return cmp.handleToolUseEvent(payload)
	}

	// 解析内容
	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		return []SSEEvent{}, nil
	}

	// 尝试提取嵌套的 assistantResponseEvent
	if nested, ok := data["assistantResponseEvent"].(map[string]any); ok {
		data = nested
	}

	content, _ := data["content"].(string)
	if content == "" {
		return []SSEEvent{}, nil
	}

	// 标记有文本块（用于后续状态跟踪）
	cmp.hasTextBlock = true

	// 只发送 content_block_delta，不发送 content_block_start
	// sse_state_manager 会在收到 delta 时自动创建 content_block_start
	// 这样可以避免重复创建空的 text 块
	return []SSEEvent{
		{
			Event: "content_block_delta",
			Data: map[string]any{
				"type":  "content_block_delta",
				"index": cmp.currentTextIndex,
				"delta": map[string]any{
					"type": "text_delta",
					"text": content,
				},
			},
		},
	}, nil
}

// handleToolUseEvent 处理工具使用事件
// 匹配 kiro2api/parser/message_event_handlers.go:LegacyToolUseEventHandler
func (cmp *CompliantMessageProcessor) handleToolUseEvent(payload []byte) ([]SSEEvent, error) {
	var evt ToolUseEvent
	if err := json.Unmarshal(payload, &evt); err != nil {
		// 尝试解析嵌套格式
		var nested map[string]any
		if err := json.Unmarshal(payload, &nested); err != nil {
			return []SSEEvent{}, nil
		}
		if toolEvt, ok := nested["toolUseEvent"].(map[string]any); ok {
			evt.ToolUseId, _ = toolEvt["toolUseId"].(string)
			evt.Name, _ = toolEvt["name"].(string)
			evt.Input = toolEvt["input"]
			evt.Stop, _ = toolEvt["stop"].(bool)
		}
	}

	if evt.ToolUseId == "" || evt.Name == "" {
		return []SSEEvent{}, nil
	}

	var events []SSEEvent

	// 检查是否是新的工具调用
	toolState, exists := cmp.activeTools[evt.ToolUseId]
	if !exists {
		// 如果有活跃的文本块，先关闭它
		if cmp.hasTextBlock {
			events = append(events, SSEEvent{
				Event: "content_block_stop",
				Data: map[string]any{
					"type":  "content_block_stop",
					"index": cmp.currentTextIndex,
				},
			})
			cmp.nextBlockIndex = cmp.currentTextIndex + 1
			cmp.hasTextBlock = false
		}

		// 创建新的工具状态
		toolState = &ToolState{
			ToolUseId:  evt.ToolUseId,
			Name:       evt.Name,
			BlockIndex: cmp.nextBlockIndex,
			Started:    true,
		}
		cmp.activeTools[evt.ToolUseId] = toolState
		cmp.nextBlockIndex++

		// 发送 content_block_start
		events = append(events, SSEEvent{
			Event: "content_block_start",
			Data: map[string]any{
				"type":  "content_block_start",
				"index": toolState.BlockIndex,
				"content_block": map[string]any{
					"type":  "tool_use",
					"id":    evt.ToolUseId,
					"name":  evt.Name,
					"input": map[string]any{},
				},
			},
		})
	}

	// 发送输入增量
	if evt.Input != nil {
		inputJSON := convertInputToJSON(evt.Input)
		if inputJSON != "" && inputJSON != "{}" {
			events = append(events, SSEEvent{
				Event: "content_block_delta",
				Data: map[string]any{
					"type":  "content_block_delta",
					"index": toolState.BlockIndex,
					"delta": map[string]any{
						"type":         "input_json_delta",
						"partial_json": inputJSON,
					},
				},
			})
		}
	}

	// 如果工具调用结束
	if evt.Stop && !toolState.Stopped {
		toolState.Stopped = true
		cmp.completedToolIds[evt.ToolUseId] = true

		events = append(events, SSEEvent{
			Event: "content_block_stop",
			Data: map[string]any{
				"type":  "content_block_stop",
				"index": toolState.BlockIndex,
			},
		})
	}

	return events, nil
}

// handleEndOfTurnEvent 处理结束事件
func (cmp *CompliantMessageProcessor) handleEndOfTurnEvent(payload []byte) ([]SSEEvent, error) {
	// endOfTurnEvent 不生成任何 SSE 事件
	// 结束逻辑在 streaming.go 中由 EmitForceStop 处理
	return []SSEEvent{}, nil
}

// handleCodeEvent 处理代码事件
// codeEvent 的内容也作为文本增量处理
// 关键修复：不再手动发送 content_block_start，由 sse_state_manager 自动处理
func (cmp *CompliantMessageProcessor) handleCodeEvent(payload []byte) ([]SSEEvent, error) {
	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		return []SSEEvent{}, nil
	}

	// 尝试提取嵌套的 codeEvent
	if nested, ok := data["codeEvent"].(map[string]any); ok {
		data = nested
	}

	content, _ := data["content"].(string)
	if content == "" {
		return []SSEEvent{}, nil
	}

	// 标记有文本块
	cmp.hasTextBlock = true

	// 只发送 content_block_delta
	return []SSEEvent{
		{
			Event: "content_block_delta",
			Data: map[string]any{
				"type":  "content_block_delta",
				"index": cmp.currentTextIndex,
				"delta": map[string]any{
					"type": "text_delta",
					"text": content,
				},
			},
		},
	}, nil
}

// handleNestedEvent 处理嵌套格式的事件
func (cmp *CompliantMessageProcessor) handleNestedEvent(payload []byte) ([]SSEEvent, error) {
	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		return []SSEEvent{}, nil
	}

	// 检查各种嵌套格式
	if _, ok := data["assistantResponseEvent"]; ok {
		return cmp.handleAssistantResponseEvent(payload)
	}
	if _, ok := data["toolUseEvent"]; ok {
		return cmp.handleToolUseEvent(payload)
	}
	if _, ok := data["endOfTurnEvent"]; ok {
		return cmp.handleEndOfTurnEvent(payload)
	}
	if _, ok := data["codeEvent"]; ok {
		return cmp.handleCodeEvent(payload)
	}

	// 检查是否有 content 字段（直接格式）
	if content, ok := data["content"].(string); ok && content != "" {
		return cmp.handleAssistantResponseEvent(payload)
	}

	// 检查是否有 toolUseId 字段（直接格式）
	if _, ok := data["toolUseId"].(string); ok {
		return cmp.handleToolUseEvent(payload)
	}

	return []SSEEvent{}, nil
}

// processErrorMessage 处理错误消息
func (cmp *CompliantMessageProcessor) processErrorMessage(msg *EventStreamMessage) ([]SSEEvent, error) {
	var errorData map[string]any
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &errorData); err != nil {
			errorData = map[string]any{
				"message": string(msg.Payload),
			}
		}
	}

	errorCode := ""
	errorMessage := ""
	if errorData != nil {
		if code, ok := errorData["__type"].(string); ok {
			errorCode = code
		}
		if m, ok := errorData["message"].(string); ok {
			errorMessage = m
		}
	}

	return []SSEEvent{
		{
			Event: "error",
			Data: map[string]any{
				"type":          "error",
				"error_code":    errorCode,
				"error_message": errorMessage,
				"raw_data":      errorData,
			},
		},
	}, nil
}

// processExceptionMessage 处理异常消息
func (cmp *CompliantMessageProcessor) processExceptionMessage(msg *EventStreamMessage) ([]SSEEvent, error) {
	var exceptionData map[string]any
	if len(msg.Payload) > 0 {
		if err := json.Unmarshal(msg.Payload, &exceptionData); err != nil {
			exceptionData = map[string]any{
				"message": string(msg.Payload),
			}
		}
	}

	exceptionType := ""
	exceptionMessage := ""
	if exceptionData != nil {
		if eType, ok := exceptionData["__type"].(string); ok {
			exceptionType = eType
		}
		if m, ok := exceptionData["message"].(string); ok {
			exceptionMessage = m
		}
	}

	return []SSEEvent{
		{
			Event: "exception",
			Data: map[string]any{
				"type":              "exception",
				"exception_type":    exceptionType,
				"exception_message": exceptionMessage,
				"raw_data":          exceptionData,
			},
		},
	}, nil
}

// GetActiveTools 获取活跃的工具
func (cmp *CompliantMessageProcessor) GetActiveTools() map[string]*ToolState {
	cmp.mu.Lock()
	defer cmp.mu.Unlock()
	return cmp.activeTools
}

// GetCompletedToolIds 获取已完成的工具ID
func (cmp *CompliantMessageProcessor) GetCompletedToolIds() map[string]bool {
	cmp.mu.Lock()
	defer cmp.mu.Unlock()
	return cmp.completedToolIds
}

// HasTextBlock 检查是否有活跃的文本块
func (cmp *CompliantMessageProcessor) HasTextBlock() bool {
	cmp.mu.Lock()
	defer cmp.mu.Unlock()
	return cmp.hasTextBlock
}

// GetCurrentTextIndex 获取当前文本块索引
func (cmp *CompliantMessageProcessor) GetCurrentTextIndex() int {
	cmp.mu.Lock()
	defer cmp.mu.Unlock()
	return cmp.currentTextIndex
}

// 辅助函数

// isToolCallPayload 检查是否为工具调用事件
func isToolCallPayload(payload []byte) bool {
	payloadStr := string(payload)
	return strings.Contains(payloadStr, `"toolUseId":`) ||
		strings.Contains(payloadStr, `"tool_use_id":`) ||
		(strings.Contains(payloadStr, `"name":`) && strings.Contains(payloadStr, `"input":`))
}

// convertInputToJSON 将 input 转换为 JSON 字符串
func convertInputToJSON(input any) string {
	if input == nil {
		return "{}"
	}

	// 如果已经是字符串，直接返回
	if str, ok := input.(string); ok {
		return str
	}

	// 将对象转换为 JSON 字符串
	jsonBytes, err := json.Marshal(input)
	if err != nil {
		return "{}"
	}

	return string(jsonBytes)
}
