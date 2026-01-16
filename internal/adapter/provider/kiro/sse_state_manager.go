package kiro

import (
	"errors"
	"fmt"
	"io"
	"strings"
)

// BlockState 内容块状态
type BlockState struct {
	Index     int    `json:"index"`
	Type      string `json:"type"` // "text" | "tool_use"
	Started   bool   `json:"started"`
	Stopped   bool   `json:"stopped"`
	ToolUseID string `json:"tool_use_id,omitempty"` // 仅用于工具块
}

// SSEStateManager SSE事件状态管理器，确保事件序列符合Claude规范
type SSEStateManager struct {
	messageStarted   bool
	messageDeltaSent bool // 跟踪message_delta是否已发送
	activeBlocks     map[int]*BlockState
	messageEnded     bool
	nextBlockIndex   int
	strictMode       bool

	// 输出写入器
	writer io.Writer
}

// NewSSEStateManager 创建SSE状态管理器
func NewSSEStateManager(writer io.Writer, strictMode bool) *SSEStateManager {
	return &SSEStateManager{
		activeBlocks: make(map[int]*BlockState),
		strictMode:   strictMode,
		writer:       writer,
	}
}

// SetWriter 设置输出写入器（不重置状态）
func (ssm *SSEStateManager) SetWriter(writer io.Writer) {
	ssm.writer = writer
}

// Reset 重置状态管理器
func (ssm *SSEStateManager) Reset() {
	ssm.messageStarted = false
	ssm.messageDeltaSent = false
	ssm.messageEnded = false
	ssm.activeBlocks = make(map[int]*BlockState)
	ssm.nextBlockIndex = 0
}

// SendEvent 受控的事件发送，确保符合Claude规范
func (ssm *SSEStateManager) SendEvent(eventData map[string]interface{}) error {
	eventType, ok := eventData["type"].(string)
	if !ok {
		return errors.New("无效的事件类型")
	}

	// 状态验证和处理
	switch eventType {
	case "message_start":
		return ssm.handleMessageStart(eventData)
	case "content_block_start":
		return ssm.handleContentBlockStart(eventData)
	case "content_block_delta":
		return ssm.handleContentBlockDelta(eventData)
	case "content_block_stop":
		return ssm.handleContentBlockStop(eventData)
	case "message_delta":
		return ssm.handleMessageDelta(eventData)
	case "message_stop":
		return ssm.handleMessageStop(eventData)
	default:
		// 其他事件直接转发
		return ssm.writeEvent(eventData)
	}
}

// writeEvent 写入SSE事件
func (ssm *SSEStateManager) writeEvent(eventData map[string]interface{}) error {
	eventType, _ := eventData["type"].(string)
	formatted := formatSSE(eventData)
	_, err := ssm.writer.Write([]byte(formatted))
	if err != nil {
		return fmt.Errorf("写入事件 %s 失败: %w", eventType, err)
	}
	return nil
}

// handleMessageStart 处理消息开始事件
func (ssm *SSEStateManager) handleMessageStart(eventData map[string]interface{}) error {
	if ssm.messageStarted {
		errMsg := "违规：message_start只能出现一次"
		if ssm.strictMode {
			return errors.New(errMsg)
		}
		return nil // 非严格模式下跳过重复的message_start
	}

	ssm.messageStarted = true
	return ssm.writeEvent(eventData)
}

// handleContentBlockStart 处理内容块开始事件
func (ssm *SSEStateManager) handleContentBlockStart(eventData map[string]interface{}) error {
	if !ssm.messageStarted {
		errMsg := "违规：content_block_start必须在message_start之后"
		if ssm.strictMode {
			return errors.New(errMsg)
		}
	}

	if ssm.messageEnded {
		errMsg := "违规：message已结束，不能发送content_block_start"
		if ssm.strictMode {
			return errors.New(errMsg)
		}
		return nil
	}

	// 提取块索引
	index := ssm.extractIndex(eventData)
	if index < 0 {
		index = ssm.nextBlockIndex
	}

	// 检查是否重复启动同一块
	if block, exists := ssm.activeBlocks[index]; exists && block.Started && !block.Stopped {
		errMsg := fmt.Sprintf("违规：索引%d的content_block已经started但未stopped", index)
		if ssm.strictMode {
			return errors.New(errMsg)
		}
		return nil // 跳过重复的start
	}

	// 确定块类型
	blockType := "text"
	if contentBlock, ok := eventData["content_block"].(map[string]interface{}); ok {
		if cbType, ok := contentBlock["type"].(string); ok {
			blockType = cbType
		}
	}

	// 在启动新工具块前，自动关闭文本块
	if blockType == "tool_use" {
		for blockIndex, block := range ssm.activeBlocks {
			if block.Type == "text" && block.Started && !block.Stopped {
				// 自动发送content_block_stop来关闭文本块
				stopEvent := map[string]interface{}{
					"type":  "content_block_stop",
					"index": blockIndex,
				}
				// 立即发送stop事件（在工具块start之前）
				if err := ssm.writeEvent(stopEvent); err == nil {
					block.Stopped = true
				}
			}
		}
	}

	// 创建或更新块状态
	toolUseID := ""
	if blockType == "tool_use" {
		if contentBlock, ok := eventData["content_block"].(map[string]interface{}); ok {
			if id, ok := contentBlock["id"].(string); ok {
				toolUseID = id
			}
		}
	}

	ssm.activeBlocks[index] = &BlockState{
		Index:     index,
		Type:      blockType,
		Started:   true,
		Stopped:   false,
		ToolUseID: toolUseID,
	}

	if index >= ssm.nextBlockIndex {
		ssm.nextBlockIndex = index + 1
	}

	return ssm.writeEvent(eventData)
}

// handleContentBlockDelta 处理内容块增量事件
func (ssm *SSEStateManager) handleContentBlockDelta(eventData map[string]interface{}) error {
	index := ssm.extractIndex(eventData)
	if index < 0 {
		errMsg := "content_block_delta缺少有效索引"
		if ssm.strictMode {
			return errors.New(errMsg)
		}
		return nil
	}

	// 检查块是否已启动，如果没有则自动启动
	block, exists := ssm.activeBlocks[index]
	if !exists || !block.Started {
		// 推断块类型：检查delta内容来确定类型
		blockType := "text" // 默认为文本块
		if delta, ok := eventData["delta"].(map[string]interface{}); ok {
			if deltaType, ok := delta["type"].(string); ok {
				if deltaType == "input_json_delta" {
					blockType = "tool_use"
				}
			}
		}

		// 自动生成并发送content_block_start事件
		startEvent := map[string]interface{}{
			"type":  "content_block_start",
			"index": index,
			"content_block": map[string]interface{}{
				"type": blockType,
			},
		}

		switch blockType {
		case "text":
			startEvent["content_block"].(map[string]interface{})["text"] = ""
		case "tool_use":
			startEvent["content_block"].(map[string]interface{})["id"] = fmt.Sprintf("tooluse_auto_%d", index)
			startEvent["content_block"].(map[string]interface{})["name"] = "auto_detected"
			startEvent["content_block"].(map[string]interface{})["input"] = map[string]interface{}{}
		}

		// 先处理start事件来更新状态
		if err := ssm.handleContentBlockStart(startEvent); err != nil {
			return err
		}

		// 重新获取更新后的block状态
		block = ssm.activeBlocks[index]
	}

	if block != nil && block.Stopped {
		errMsg := fmt.Sprintf("违规：索引%d的content_block已停止，不能发送delta", index)
		if ssm.strictMode {
			return errors.New(errMsg)
		}
		return nil
	}

	return ssm.writeEvent(eventData)
}

// handleContentBlockStop 处理内容块停止事件
func (ssm *SSEStateManager) handleContentBlockStop(eventData map[string]interface{}) error {
	index := ssm.extractIndex(eventData)
	if index < 0 {
		errMsg := "content_block_stop缺少有效索引"
		if ssm.strictMode {
			return errors.New(errMsg)
		}
		return nil
	}

	// 验证块状态
	block, exists := ssm.activeBlocks[index]
	if !exists || !block.Started {
		errMsg := fmt.Sprintf("违规：索引%d的content_block未启动就发送stop", index)
		if ssm.strictMode {
			return errors.New(errMsg)
		}
		return nil
	}

	if block.Stopped {
		errMsg := fmt.Sprintf("违规：索引%d的content_block重复停止", index)
		if ssm.strictMode {
			return errors.New(errMsg)
		}
		return nil
	}

	// 标记为已停止
	block.Stopped = true

	return ssm.writeEvent(eventData)
}

// handleMessageDelta 处理消息增量事件
func (ssm *SSEStateManager) handleMessageDelta(eventData map[string]interface{}) error {
	if !ssm.messageStarted {
		errMsg := "违规：message_delta必须在message_start之后"
		if ssm.strictMode {
			return errors.New(errMsg)
		}
	}

	// 防止重复的message_delta事件
	if ssm.messageDeltaSent {
		errMsg := "违规：message_delta只能出现一次"
		if ssm.strictMode {
			return errors.New(errMsg)
		}
		return nil // 非严格模式下跳过重复的message_delta
	}

	// 在发送message_delta之前，确保所有content_block都已关闭
	var unclosedBlocks []int
	for index, block := range ssm.activeBlocks {
		if block.Started && !block.Stopped {
			unclosedBlocks = append(unclosedBlocks, index)
		}
	}

	if len(unclosedBlocks) > 0 && !ssm.strictMode {
		// 自动关闭未关闭的块
		for _, index := range unclosedBlocks {
			stopEvent := map[string]interface{}{
				"type":  "content_block_stop",
				"index": index,
			}
			ssm.writeEvent(stopEvent)
			ssm.activeBlocks[index].Stopped = true
		}
	}

	// 标记message_delta已发送
	ssm.messageDeltaSent = true

	return ssm.writeEvent(eventData)
}

// handleMessageStop 处理消息停止事件
func (ssm *SSEStateManager) handleMessageStop(eventData map[string]interface{}) error {
	if !ssm.messageStarted {
		errMsg := "违规：message_stop必须在message_start之后"
		if ssm.strictMode {
			return errors.New(errMsg)
		}
	}

	if ssm.messageEnded {
		errMsg := "违规：message_stop只能出现一次"
		if ssm.strictMode {
			return errors.New(errMsg)
		}
		return nil
	}

	ssm.messageEnded = true
	return ssm.writeEvent(eventData)
}

// extractIndex 从数据映射中提取索引
func (ssm *SSEStateManager) extractIndex(eventData map[string]interface{}) int {
	if v, ok := eventData["index"].(int); ok {
		return v
	}
	if f, ok := eventData["index"].(float64); ok {
		return int(f)
	}
	return -1
}

// GetActiveBlocks 获取所有活跃块
func (ssm *SSEStateManager) GetActiveBlocks() map[int]*BlockState {
	return ssm.activeBlocks
}

// IsMessageStarted 检查消息是否已开始
func (ssm *SSEStateManager) IsMessageStarted() bool {
	return ssm.messageStarted
}

// IsMessageEnded 检查消息是否已结束
func (ssm *SSEStateManager) IsMessageEnded() bool {
	return ssm.messageEnded
}

// IsMessageDeltaSent 检查message_delta是否已发送
func (ssm *SSEStateManager) IsMessageDeltaSent() bool {
	return ssm.messageDeltaSent
}

// GetCompletedToolUseIDs 获取所有已完成的工具使用ID
func (ssm *SSEStateManager) GetCompletedToolUseIDs() []string {
	var ids []string
	for _, block := range ssm.activeBlocks {
		if block.Type == "tool_use" && block.ToolUseID != "" && block.Stopped {
			ids = append(ids, block.ToolUseID)
		}
	}
	return ids
}

// HasToolUseBlocks 检查是否有工具使用块（活跃或已完成）
func (ssm *SSEStateManager) HasToolUseBlocks() bool {
	for _, block := range ssm.activeBlocks {
		if block.Type == "tool_use" {
			return true
		}
	}
	return false
}

// CloseAllActiveBlocks 关闭所有活跃的内容块
func (ssm *SSEStateManager) CloseAllActiveBlocks() error {
	var errs []string
	for index, block := range ssm.activeBlocks {
		if block.Started && !block.Stopped {
			stopEvent := map[string]interface{}{
				"type":  "content_block_stop",
				"index": index,
			}
			if err := ssm.writeEvent(stopEvent); err != nil {
				errs = append(errs, fmt.Sprintf("关闭块%d失败: %v", index, err))
			} else {
				block.Stopped = true
			}
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}
