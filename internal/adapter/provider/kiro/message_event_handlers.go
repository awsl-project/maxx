package kiro

import (
	"fmt"
	"strings"
)

// Helper: convert input to JSON string.
func convertInputToString(input any) string {
	if input == nil {
		return "{}"
	}
	if str, ok := input.(string); ok {
		return str
	}
	if jsonBytes, err := FastMarshal(input); err == nil {
		return string(jsonBytes)
	}
	return "{}"
}

func isToolCallEvent(payload []byte) bool {
	payloadStr := string(payload)
	return strings.Contains(payloadStr, "\"toolUseId\":") ||
		strings.Contains(payloadStr, "\"tool_use_id\":") ||
		(strings.Contains(payloadStr, "\"name\":") && strings.Contains(payloadStr, "\"input\":"))
}

func isStreamingResponse(event *FullAssistantResponseEvent) bool {
	if event == nil {
		return false
	}
	status := strings.ToUpper(string(event.MessageStatus))
	return status == "IN_PROGRESS" || status == "INPROGRESS" || event.Content != ""
}

// CompletionEventHandler handles completion events.
type CompletionEventHandler struct {
	processor *CompliantMessageProcessor
}

func (h *CompletionEventHandler) Handle(message *EventStreamMessage) ([]SSEEvent, error) {
	var data map[string]any
	if err := jsonUnmarshal(message.Payload, &data); err != nil {
		return nil, err
	}

	content := ""
	if c, ok := data["content"].(string); ok {
		content = c
	}
	finishReason := ""
	if fr, ok := data["finish_reason"].(string); ok {
		finishReason = fr
	}

	return []SSEEvent{{
		Event: "completion",
		Data: map[string]any{
			"type":          "completion",
			"content":       content,
			"finish_reason": finishReason,
			"raw_data":      data,
		},
	}}, nil
}

// CompletionChunkEventHandler handles completion chunk events.
type CompletionChunkEventHandler struct {
	processor *CompliantMessageProcessor
}

func (h *CompletionChunkEventHandler) Handle(message *EventStreamMessage) ([]SSEEvent, error) {
	var data map[string]any
	if err := jsonUnmarshal(message.Payload, &data); err != nil {
		return nil, err
	}

	content := ""
	if c, ok := data["content"].(string); ok {
		content = c
	}
	delta := ""
	if d, ok := data["delta"].(string); ok {
		delta = d
	}
	finishReason := ""
	if fr, ok := data["finish_reason"].(string); ok {
		finishReason = fr
	}

	h.processor.completionBuffer = append(h.processor.completionBuffer, content)

	textDelta := delta
	if textDelta == "" {
		textDelta = content
	}

	events := []SSEEvent{{
		Event: "content_block_delta",
		Data: map[string]any{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]any{
				"type": "text_delta",
				"text": textDelta,
			},
		},
	}}

	if finishReason != "" {
		events = append(events, SSEEvent{
			Event: "content_block_stop",
			Data: map[string]any{
				"type":          "content_block_stop",
				"index":         0,
				"finish_reason": finishReason,
			},
		})
	}

	return events, nil
}

// ToolCallRequestHandler handles tool call requests.
type ToolCallRequestHandler struct {
	toolManager *ToolLifecycleManager
}

func (h *ToolCallRequestHandler) Handle(message *EventStreamMessage) ([]SSEEvent, error) {
	var data map[string]any
	if err := jsonUnmarshal(message.Payload, &data); err != nil {
		return nil, err
	}

	toolCallID, _ := data["toolCallId"].(string)
	toolName, _ := data["toolName"].(string)
	input := map[string]any{}
	if inputData, ok := data["input"].(map[string]any); ok {
		input = inputData
	}

	toolCall := ToolCall{
		ID:   toolCallID,
		Type: "function",
		Function: ToolCallFunction{
			Name:      toolName,
			Arguments: "{}",
		},
	}
	if len(input) > 0 {
		if argsJSON, err := FastMarshal(input); err == nil {
			toolCall.Function.Arguments = string(argsJSON)
		}
	}

	request := ToolCallRequest{ToolCalls: []ToolCall{toolCall}}
	return h.toolManager.HandleToolCallRequest(request), nil
}

// ToolCallErrorHandler handles tool call errors.
type ToolCallErrorHandler struct {
	toolManager *ToolLifecycleManager
}

func (h *ToolCallErrorHandler) Handle(message *EventStreamMessage) ([]SSEEvent, error) {
	var errorInfo ToolCallError
	if err := jsonUnmarshal(message.Payload, &errorInfo); err != nil {
		return nil, err
	}
	return h.toolManager.HandleToolCallError(errorInfo), nil
}

// SessionStartHandler handles session start events.
type SessionStartHandler struct {
	sessionManager *SessionManager
}

func (h *SessionStartHandler) Handle(message *EventStreamMessage) ([]SSEEvent, error) {
	var data map[string]any
	if err := jsonUnmarshal(message.Payload, &data); err != nil {
		return nil, err
	}

	sessionID := ""
	if sid, ok := data["sessionId"].(string); ok {
		sessionID = sid
	} else if sid, ok := data["session_id"].(string); ok {
		sessionID = sid
	}

	if sessionID != "" {
		h.sessionManager.SetSessionID(sessionID)
		h.sessionManager.StartSession()
	}

	return []SSEEvent{{
		Event: EventTypes.SessionStart,
		Data:  data,
	}}, nil
}

// SessionEndHandler handles session end events.
type SessionEndHandler struct {
	sessionManager *SessionManager
}

func (h *SessionEndHandler) Handle(message *EventStreamMessage) ([]SSEEvent, error) {
	var data map[string]any
	if err := jsonUnmarshal(message.Payload, &data); err != nil {
		return nil, err
	}

	endEvents := h.sessionManager.EndSession()
	result := []SSEEvent{{
		Event: EventTypes.SessionEnd,
		Data:  data,
	}}
	result = append(result, endEvents...)
	return result, nil
}

// StandardAssistantResponseEventHandler handles assistantResponseEvent messages.
type StandardAssistantResponseEventHandler struct {
	processor *CompliantMessageProcessor
}

func (h *StandardAssistantResponseEventHandler) Handle(message *EventStreamMessage) ([]SSEEvent, error) {
	if isToolCallEvent(message.Payload) {
		return h.handleToolCallEvent(message)
	}

	if fullEvent, err := parseFullAssistantResponseEvent(message.Payload); err == nil {
		if isStreamingResponse(fullEvent) {
			return h.handleStreamingEvent(fullEvent)
		}
		return h.handleFullAssistantEvent(fullEvent)
	}

	return h.handleLegacyFormat(message.Payload)
}

func (h *StandardAssistantResponseEventHandler) handleToolCallEvent(message *EventStreamMessage) ([]SSEEvent, error) {
	var evt toolUseEvent
	if err := jsonUnmarshal(message.Payload, &evt); err != nil {
		return []SSEEvent{}, nil
	}

	toolCall := ToolCall{
		ID:   evt.ToolUseId,
		Type: "function",
		Function: ToolCallFunction{
			Name:      evt.Name,
			Arguments: convertInputToString(evt.Input),
		},
	}
	request := ToolCallRequest{ToolCalls: []ToolCall{toolCall}}
	return h.processor.toolManager.HandleToolCallRequest(request), nil
}

func (h *StandardAssistantResponseEventHandler) handleStreamingEvent(event *FullAssistantResponseEvent) ([]SSEEvent, error) {
	if event.Content == "" {
		return []SSEEvent{}, nil
	}
	return []SSEEvent{{
		Event: "content_block_delta",
		Data: map[string]any{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]any{
				"type": "text_delta",
				"text": event.Content,
			},
		},
	}}, nil
}

func (h *StandardAssistantResponseEventHandler) handleFullAssistantEvent(event *FullAssistantResponseEvent) ([]SSEEvent, error) {
	if event.Content == "" {
		return []SSEEvent{}, nil
	}

	return []SSEEvent{
		{
			Event: "content_block_start",
			Data: map[string]any{
				"type":  "content_block_start",
				"index": 0,
				"content_block": map[string]any{
					"type": "text",
					"text": event.Content,
				},
			},
		},
		{
			Event: "content_block_delta",
			Data: map[string]any{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]any{
					"type": "text_delta",
					"text": event.Content,
				},
			},
		},
		{
			Event: "content_block_stop",
			Data: map[string]any{
				"type":  "content_block_stop",
				"index": 0,
			},
		},
	}, nil
}

func (h *StandardAssistantResponseEventHandler) handleLegacyFormat(payload []byte) ([]SSEEvent, error) {
	payloadStr := strings.TrimSpace(string(payload))
	if payloadStr != "" && !strings.HasPrefix(payloadStr, "{") {
		return []SSEEvent{{
			Event: "content_block_delta",
			Data: map[string]any{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]any{
					"type": "text_delta",
					"text": payloadStr,
				},
			},
		}}, nil
	}

	var data map[string]any
	if err := jsonUnmarshal(payload, &data); err != nil {
		return []SSEEvent{}, nil
	}

	var events []SSEEvent
	if content, ok := data["content"].(string); ok && content != "" {
		events = append(events, SSEEvent{
			Event: "content_block_delta",
			Data: map[string]any{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]any{
					"type": "text_delta",
					"text": content,
				},
			},
		})
	}

	return events, nil
}

// LegacyToolUseEventHandler handles legacy toolUseEvent messages.
type LegacyToolUseEventHandler struct {
	toolManager *ToolLifecycleManager
	aggregator  *StreamingJSONAggregator
}

func (h *LegacyToolUseEventHandler) Handle(message *EventStreamMessage) ([]SSEEvent, error) {
	return h.handleToolCallEvent(message)
}

func (h *LegacyToolUseEventHandler) handleToolCallEvent(message *EventStreamMessage) ([]SSEEvent, error) {
	var evt toolUseEvent
	if err := jsonUnmarshal(message.Payload, &evt); err != nil {
		return []SSEEvent{}, nil
	}

	if evt.Name == "" || evt.ToolUseId == "" {
		if evt.Name == "" && evt.ToolUseId == "" {
			return []SSEEvent{}, nil
		}
	}

	inputStr := convertInputToString(evt.Input)

	_, toolExists := h.toolManager.GetActiveTools()[evt.ToolUseId]
	if !toolExists {
		toolCall := ToolCall{
			ID:   evt.ToolUseId,
			Type: "function",
			Function: ToolCallFunction{
				Name:      evt.Name,
				Arguments: inputStr,
			},
		}
		request := ToolCallRequest{ToolCalls: []ToolCall{toolCall}}
		events := h.toolManager.HandleToolCallRequest(request)
		if evt.Stop {
			return events, nil
		}
		return events, nil
	}

	if evt.Stop {
		complete, fullInput := h.aggregator.ProcessToolData(evt.ToolUseId, evt.Name, "", evt.Stop, -1)
		if complete {
			if fullInput != "" && fullInput != "{}" {
				var testArgs map[string]any
				if err := FastUnmarshal([]byte(fullInput), &testArgs); err == nil {
					h.toolManager.UpdateToolArguments(evt.ToolUseId, testArgs)
				}
			}
			result := ToolCallResult{
				ToolCallID: evt.ToolUseId,
				Result:     "tool use event completed",
			}
			return h.toolManager.HandleToolCallResult(result), nil
		}
	}

	if inputStr == "" || inputStr == "{}" {
		return []SSEEvent{}, nil
	}

	complete, _ := h.aggregator.ProcessToolData(evt.ToolUseId, evt.Name, inputStr, evt.Stop, -1)
	if !complete {
		if inputStr != "" && inputStr != "{}" {
			toolIndex := h.toolManager.GetBlockIndex(evt.ToolUseId)
			if toolIndex >= 0 {
				return []SSEEvent{{
					Event: "content_block_delta",
					Data: map[string]any{
						"type":  "content_block_delta",
						"index": toolIndex,
						"delta": map[string]any{
							"type":         "input_json_delta",
							"partial_json": inputStr,
						},
					},
				}}, nil
			}
		}
		return []SSEEvent{}, nil
	}

	return []SSEEvent{}, nil
}

// Legacy event payloads and helpers.

type toolUseEvent struct {
	Name      string `json:"name"`
	ToolUseId string `json:"toolUseId"`
	Input     any    `json:"input"`
	Stop      bool   `json:"stop"`
}

type FullAssistantResponseEvent struct {
	AssistantResponseEvent
}

func parseFullAssistantResponseEvent(payload []byte) (*FullAssistantResponseEvent, error) {
	var data map[string]any
	if err := jsonUnmarshal(payload, &data); err != nil {
		return nil, err
	}

	if eventData, ok := data["assistantResponseEvent"].(map[string]any); ok {
		data = eventData
	}

	isToolFragment := false
	hasMainFields := false

	if _, hasToolUseId := data["toolUseId"]; hasToolUseId {
		if _, hasName := data["name"]; hasName {
			isToolFragment = true
		}
	}

	if content, hasContent := data["content"]; hasContent && content != "" {
		hasMainFields = true
	}
	if convID, hasConv := data["conversationId"]; hasConv && convID != "" {
		hasMainFields = true
	}
	if msgID, hasMsg := data["messageId"]; hasMsg && msgID != "" {
		hasMainFields = true
	}

	if isToolFragment && !hasMainFields {
		return nil, fmt.Errorf("tool fragment without main fields")
	}

	event := &FullAssistantResponseEvent{}
	if convID, ok := data["conversationId"].(string); ok {
		event.ConversationID = convID
	}
	if msgID, ok := data["messageId"].(string); ok {
		event.MessageID = msgID
	}
	if content, ok := data["content"].(string); ok {
		event.Content = content
	}
	if contentType, ok := data["contentType"].(string); ok {
		event.ContentType = ContentType(contentType)
	} else {
		event.ContentType = ContentTypeMarkdown
	}
	if msgStatus, ok := data["messageStatus"].(string); ok {
		event.MessageStatus = MessageStatus(msgStatus)
	} else {
		event.MessageStatus = MessageStatusCompleted
	}

	return event, nil
}
