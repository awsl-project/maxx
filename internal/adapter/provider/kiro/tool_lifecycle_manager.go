package kiro

import (
	"time"
)

// ToolLifecycleManager tracks tool call lifecycle and block indices.
type ToolLifecycleManager struct {
	activeTools        map[string]*ToolExecution
	completedTools     map[string]*ToolExecution
	blockIndexMap      map[string]int
	nextBlockIndex     int
	textIntroGenerated bool
}

// NewToolLifecycleManager creates a tool lifecycle manager.
func NewToolLifecycleManager() *ToolLifecycleManager {
	return &ToolLifecycleManager{
		activeTools:    make(map[string]*ToolExecution),
		completedTools: make(map[string]*ToolExecution),
		blockIndexMap:  make(map[string]int),
		nextBlockIndex: 1,
	}
}

// Reset clears tool state.
func (tlm *ToolLifecycleManager) Reset() {
	tlm.activeTools = make(map[string]*ToolExecution)
	tlm.completedTools = make(map[string]*ToolExecution)
	tlm.blockIndexMap = make(map[string]int)
	tlm.nextBlockIndex = 1
	tlm.textIntroGenerated = false
}

// HandleToolCallRequest registers tool calls and emits SSE events.
func (tlm *ToolLifecycleManager) HandleToolCallRequest(request ToolCallRequest) []SSEEvent {
	events := make([]SSEEvent, 0, len(request.ToolCalls)*3)

	if !tlm.textIntroGenerated && len(request.ToolCalls) > 0 {
		events = append(events, tlm.generateTextIntroduction(request.ToolCalls[0])...)
		tlm.textIntroGenerated = true
	}

	for _, toolCall := range request.ToolCalls {
		if existing, exists := tlm.activeTools[toolCall.ID]; exists {
			var arguments map[string]any
			_ = FastUnmarshal([]byte(toolCall.Function.Arguments), &arguments)
			if len(arguments) > 0 {
				existing.Arguments = arguments
			}
			continue
		}

		var arguments map[string]any
		if err := FastUnmarshal([]byte(toolCall.Function.Arguments), &arguments); err != nil {
			arguments = make(map[string]any)
		}

		execution := &ToolExecution{
			ID:         toolCall.ID,
			Name:       toolCall.Function.Name,
			StartTime:  time.Now(),
			Status:     ToolStatusPending,
			Arguments:  arguments,
			BlockIndex: tlm.getOrAssignBlockIndex(toolCall.ID),
		}

		tlm.activeTools[toolCall.ID] = execution

		events = append(events, SSEEvent{
			Event: "content_block_start",
			Data: map[string]any{
				"type":  "content_block_start",
				"index": execution.BlockIndex,
				"content_block": map[string]any{
					"type":  "tool_use",
					"id":    toolCall.ID,
					"name":  toolCall.Function.Name,
					"input": map[string]any{},
				},
			},
		})

		if len(arguments) > 0 {
			argsJSON, _ := FastMarshal(arguments)
			events = append(events, SSEEvent{
				Event: "content_block_delta",
				Data: map[string]any{
					"type":  "content_block_delta",
					"index": execution.BlockIndex,
					"delta": map[string]any{
						"type":         "input_json_delta",
						"partial_json": string(argsJSON),
					},
				},
			})
		}

		execution.Status = ToolStatusRunning
	}

	return events
}

// HandleToolCallResult finalizes tool calls.
func (tlm *ToolLifecycleManager) HandleToolCallResult(result ToolCallResult) []SSEEvent {
	events := make([]SSEEvent, 0, 1)

	execution, exists := tlm.activeTools[result.ToolCallID]
	if !exists {
		return events
	}

	now := time.Now()
	execution.EndTime = &now
	execution.Result = result.Result
	execution.Status = ToolStatusCompleted

	events = append(events, SSEEvent{
		Event: "content_block_stop",
		Data: map[string]any{
			"type":  "content_block_stop",
			"index": execution.BlockIndex,
		},
	})

	tlm.completedTools[result.ToolCallID] = execution
	delete(tlm.activeTools, result.ToolCallID)

	return events
}

// HandleToolCallError handles tool call errors.
func (tlm *ToolLifecycleManager) HandleToolCallError(errorInfo ToolCallError) []SSEEvent {
	events := make([]SSEEvent, 0, 2)

	execution, exists := tlm.activeTools[errorInfo.ToolCallID]
	if !exists {
		return events
	}

	now := time.Now()
	execution.EndTime = &now
	execution.Error = errorInfo.Error
	execution.Status = ToolStatusError

	events = append(events, SSEEvent{
		Event: "error",
		Data: map[string]any{
			"type": "error",
			"error": map[string]any{
				"type":         "tool_error",
				"message":      errorInfo.Error,
				"tool_call_id": errorInfo.ToolCallID,
			},
		},
	})

	events = append(events, SSEEvent{
		Event: "content_block_stop",
		Data: map[string]any{
			"type":  "content_block_stop",
			"index": execution.BlockIndex,
		},
	})

	tlm.completedTools[errorInfo.ToolCallID] = execution
	delete(tlm.activeTools, errorInfo.ToolCallID)

	return events
}

// GetActiveTools returns active tools.
func (tlm *ToolLifecycleManager) GetActiveTools() map[string]*ToolExecution {
	result := make(map[string]*ToolExecution, len(tlm.activeTools))
	for id, tool := range tlm.activeTools {
		result[id] = tool
	}
	return result
}

// GetCompletedTools returns completed tools.
func (tlm *ToolLifecycleManager) GetCompletedTools() map[string]*ToolExecution {
	result := make(map[string]*ToolExecution, len(tlm.completedTools))
	for id, tool := range tlm.completedTools {
		result[id] = tool
	}
	return result
}

func (tlm *ToolLifecycleManager) getOrAssignBlockIndex(toolID string) int {
	if index, exists := tlm.blockIndexMap[toolID]; exists {
		return index
	}
	index := tlm.nextBlockIndex
	tlm.blockIndexMap[toolID] = index
	tlm.nextBlockIndex++
	return index
}

// GetBlockIndex returns the tool block index.
func (tlm *ToolLifecycleManager) GetBlockIndex(toolID string) int {
	if index, exists := tlm.blockIndexMap[toolID]; exists {
		return index
	}
	return -1
}

// GenerateToolSummary returns tool statistics.
func (tlm *ToolLifecycleManager) GenerateToolSummary() map[string]any {
	activeCount := len(tlm.activeTools)
	completedCount := len(tlm.completedTools)
	errorCount := 0
	totalExecutionTime := int64(0)

	for _, tool := range tlm.completedTools {
		if tool.Status == ToolStatusError {
			errorCount++
		}
		if tool.EndTime != nil {
			totalExecutionTime += tool.EndTime.Sub(tool.StartTime).Milliseconds()
		}
	}

	successRate := 0.0
	if completedCount+activeCount > 0 {
		successRate = float64(completedCount-errorCount) / float64(completedCount+activeCount)
	}

	return map[string]any{
		"active_tools":         activeCount,
		"completed_tools":      completedCount,
		"error_tools":          errorCount,
		"total_execution_time": totalExecutionTime,
		"success_rate":         successRate,
	}
}

// UpdateToolArguments updates tool arguments.
func (tlm *ToolLifecycleManager) UpdateToolArguments(toolID string, arguments map[string]any) {
	if execution, exists := tlm.activeTools[toolID]; exists {
		execution.Arguments = arguments
		return
	}
	if execution, exists := tlm.completedTools[toolID]; exists {
		execution.Arguments = arguments
		return
	}
}

// UpdateToolArgumentsFromJSON updates tool arguments from JSON string.
func (tlm *ToolLifecycleManager) UpdateToolArgumentsFromJSON(toolID string, jsonArgs string) {
	var arguments map[string]any
	if err := FastUnmarshal([]byte(jsonArgs), &arguments); err != nil {
		return
	}
	tlm.UpdateToolArguments(toolID, arguments)
}

func (tlm *ToolLifecycleManager) generateTextIntroduction(_ ToolCall) []SSEEvent {
	return []SSEEvent{}
}
