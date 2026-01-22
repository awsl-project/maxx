package kiro

import "fmt"

// CompliantEventStreamParser parses AWS EventStream and converts to Claude SSE events.
type CompliantEventStreamParser struct {
	robustParser     *RobustEventStreamParser
	messageProcessor *CompliantMessageProcessor
}

// NewCompliantEventStreamParser creates a compliant parser.
func NewCompliantEventStreamParser() *CompliantEventStreamParser {
	return &CompliantEventStreamParser{
		robustParser:     NewRobustEventStreamParser(),
		messageProcessor: NewCompliantMessageProcessor(),
	}
}

// SetMaxErrors overrides the max error count.
func (cesp *CompliantEventStreamParser) SetMaxErrors(maxErrors int) {
	cesp.robustParser.SetMaxErrors(maxErrors)
}

// Reset clears parser state.
func (cesp *CompliantEventStreamParser) Reset() {
	cesp.robustParser.Reset()
	cesp.messageProcessor.Reset()
}

// ParseResponse parses a full response body.
func (cesp *CompliantEventStreamParser) ParseResponse(streamData []byte) (*ParseResult, error) {
	messages, err := cesp.robustParser.ParseStream(streamData)
	if err != nil {
		_ = err // Continue with partial messages.
	}

	var allEvents []SSEEvent
	var errs []error

	for i, message := range messages {
		events, processErr := cesp.messageProcessor.ProcessMessage(message)
		if processErr != nil {
			errs = append(errs, fmt.Errorf("process message %d: %w", i, processErr))
			continue
		}
		allEvents = append(allEvents, events...)
	}

	result := &ParseResult{
		Messages:       messages,
		Events:         allEvents,
		ToolExecutions: cesp.messageProcessor.toolManager.GetCompletedTools(),
		ActiveTools:    cesp.messageProcessor.toolManager.GetActiveTools(),
		SessionInfo:    cesp.messageProcessor.sessionManager.GetSessionInfo(),
		Summary:        cesp.generateSummary(messages, allEvents),
		Errors:         errs,
	}

	return result, nil
}

// ParseStream parses incremental data and returns SSE events.
func (cesp *CompliantEventStreamParser) ParseStream(data []byte) ([]SSEEvent, error) {
	messages, err := cesp.robustParser.ParseStream(data)
	if err != nil {
		_ = err // Continue with partial messages.
	}

	var allEvents []SSEEvent
	for _, message := range messages {
		events, processErr := cesp.messageProcessor.ProcessMessage(message)
		if processErr != nil {
			continue
		}
		allEvents = append(allEvents, events...)
	}

	return allEvents, nil
}

// GetToolManager returns the tool manager for aggregation.
func (cesp *CompliantEventStreamParser) GetToolManager() *ToolLifecycleManager {
	return cesp.messageProcessor.GetToolManager()
}

func (cesp *CompliantEventStreamParser) generateSummary(messages []*EventStreamMessage, events []SSEEvent) *ParseSummary {
	summary := &ParseSummary{
		TotalMessages:    len(messages),
		TotalEvents:      len(events),
		MessageTypes:     make(map[string]int),
		EventTypes:       make(map[string]int),
		HasToolCalls:     false,
		HasCompletions:   false,
		HasErrors:        false,
		HasSessionEvents: false,
	}

	for _, message := range messages {
		msgType := message.GetMessageType()
		summary.MessageTypes[msgType]++
		if msgType == MessageTypes.Error || msgType == MessageTypes.Exception {
			summary.HasErrors = true
		}

		eventType := message.GetEventType()
		if eventType == "" {
			continue
		}
		summary.EventTypes[eventType]++
		switch eventType {
		case EventTypes.ToolCallRequest, EventTypes.ToolCallError:
			summary.HasToolCalls = true
		case EventTypes.Completion, EventTypes.CompletionChunk, EventTypes.AssistantEvent:
			summary.HasCompletions = true
		case EventTypes.SessionStart, EventTypes.SessionEnd:
			summary.HasSessionEvents = true
		}
	}

	for _, event := range events {
		summary.EventTypes[event.Event]++
		if event.Event == "content_block_start" || event.Event == "content_block_stop" || event.Event == "content_block_delta" {
			data, ok := event.Data.(map[string]any)
			if !ok {
				continue
			}
			if contentBlock, exists := data["content_block"]; exists {
				if block, ok := contentBlock.(map[string]any); ok {
					if blockType, ok := block["type"].(string); ok && blockType == "tool_use" {
						summary.HasToolCalls = true
					}
				}
			}
		}
	}

	summary.ToolSummary = cesp.messageProcessor.toolManager.GenerateToolSummary()

	return summary
}

// ParseResult contains parse output and summary.
type ParseResult struct {
	Messages       []*EventStreamMessage     `json:"messages"`
	Events         []SSEEvent                `json:"events"`
	ToolExecutions map[string]*ToolExecution `json:"tool_executions"`
	ActiveTools    map[string]*ToolExecution `json:"active_tools"`
	SessionInfo    SessionInfo               `json:"session_info"`
	Summary        *ParseSummary             `json:"summary"`
	Errors         []error                   `json:"errors,omitempty"`
}

// ParseSummary contains statistics for parsed messages.
type ParseSummary struct {
	TotalMessages    int            `json:"total_messages"`
	TotalEvents      int            `json:"total_events"`
	MessageTypes     map[string]int `json:"message_types"`
	EventTypes       map[string]int `json:"event_types"`
	HasToolCalls     bool           `json:"has_tool_calls"`
	HasCompletions   bool           `json:"has_completions"`
	HasErrors        bool           `json:"has_errors"`
	HasSessionEvents bool           `json:"has_session_events"`
	ToolSummary      map[string]any `json:"tool_summary"`
}

// GetCompletionText extracts concatenated text deltas.
func (pr *ParseResult) GetCompletionText() string {
	text := ""
	for _, event := range pr.Events {
		if event.Event != "content_block_delta" {
			continue
		}
		data, ok := event.Data.(map[string]any)
		if !ok {
			continue
		}
		if delta, ok := data["delta"].(map[string]any); ok {
			if deltaText, ok := delta["text"].(string); ok {
				text += deltaText
			}
		}
	}
	return text
}
