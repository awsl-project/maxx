package kiro

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// streamProcessorContext holds streaming state.
type streamProcessorContext struct {
	writer             http.ResponseWriter
	flusher            http.Flusher
	messageID          string
	requestModel       string
	inputTokens        int
	sseStateManager    *SSEStateManager
	stopReasonManager  *StopReasonManager
	tokenEstimator     *TokenEstimator
	compliantParser    *CompliantEventStreamParser
	totalOutputTokens  int
	totalProcessedEvents int
	toolUseIdByBlockIndex map[int]string
	completedToolUseIds   map[string]bool
	jsonBytesByBlockIndex map[int]int
}

func newStreamProcessorContext(w http.ResponseWriter, model string, inputTokens int, writer io.Writer) (*streamProcessorContext, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming not supported")
	}

	ctx := &streamProcessorContext{
		writer:               w,
		flusher:              flusher,
		messageID:            newStreamMessageID(),
		requestModel:         model,
		inputTokens:          inputTokens,
		sseStateManager:      NewSSEStateManager(writer, false),
		stopReasonManager:    NewStopReasonManager(),
		tokenEstimator:       NewTokenEstimator(),
		compliantParser:      NewCompliantEventStreamParser(),
		toolUseIdByBlockIndex: make(map[int]string),
		completedToolUseIds:   make(map[string]bool),
		jsonBytesByBlockIndex: make(map[int]int),
	}

	return ctx, nil
}

func newStreamMessageID() string {
	return fmt.Sprintf("msg_%s", time.Now().Format("20060102150405"))
}

func (ctx *streamProcessorContext) sendInitialEvents() error {
	events := []map[string]any{
		{
			"type": "message_start",
			"message": map[string]any{
				"id":            ctx.messageID,
				"type":          "message",
				"role":          "assistant",
				"content":       []any{},
				"model":         ctx.requestModel,
				"stop_reason":   nil,
				"stop_sequence": nil,
				"usage": map[string]any{
					"input_tokens":  ctx.inputTokens,
					"output_tokens": 0,
				},
			},
		},
		{
			"type": "ping",
		},
	}

	for _, event := range events {
		if err := ctx.sseStateManager.SendEvent(event); err != nil {
			return err
		}
		ctx.flusher.Flush()
	}
	return nil
}

func (ctx *streamProcessorContext) processEventStream(reqCtx context.Context, reader io.Reader) error {
	buf := make([]byte, 1024)
	for {
		select {
		case <-reqCtx.Done():
			return reqCtx.Err()
		default:
		}

		n, err := reader.Read(buf)
		if n > 0 {
			events, _ := ctx.compliantParser.ParseStream(buf[:n])
			ctx.totalProcessedEvents += len(events)

			for _, event := range events {
				if err := ctx.processEvent(event); err != nil {
					return err
				}
			}
		}

		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func (ctx *streamProcessorContext) processEvent(event SSEEvent) error {
	dataMap, ok := event.Data.(map[string]any)
	if !ok {
		return nil
	}

	eventType, _ := dataMap["type"].(string)

	switch eventType {
	case "content_block_start":
		ctx.processToolUseStart(dataMap)
		if contentBlock, ok := dataMap["content_block"].(map[string]any); ok {
			if blockType, _ := contentBlock["type"].(string); blockType == "tool_use" {
				ctx.totalOutputTokens += 12
				if toolName, ok := contentBlock["name"].(string); ok {
					ctx.totalOutputTokens += ctx.tokenEstimator.EstimateTextTokens(toolName)
				}
			}
		}
	case "content_block_delta":
		if delta, ok := dataMap["delta"].(map[string]any); ok {
			deltaType, _ := delta["type"].(string)
			switch deltaType {
			case "text_delta":
				if text, ok := delta["text"].(string); ok {
					ctx.totalOutputTokens += ctx.tokenEstimator.EstimateTextTokens(text)
				}
			case "input_json_delta":
				if partialJSON, ok := delta["partial_json"].(string); ok {
					index := extractBlockIndex(dataMap)
					ctx.jsonBytesByBlockIndex[index] += len(partialJSON)
				}
			}
		}
	case "content_block_stop":
		ctx.processToolUseStop(dataMap)
		idx := extractBlockIndex(dataMap)
		if jsonBytes, exists := ctx.jsonBytesByBlockIndex[idx]; exists && jsonBytes > 0 {
			tokens := (jsonBytes + 3) / 4
			ctx.totalOutputTokens += tokens
			delete(ctx.jsonBytesByBlockIndex, idx)
		}
	case "exception":
		if ctx.handleExceptionEvent(dataMap) {
			return nil
		}
	}

	if err := ctx.sseStateManager.SendEvent(dataMap); err != nil {
		return err
	}
	ctx.flusher.Flush()
	return nil
}

func (ctx *streamProcessorContext) processToolUseStart(dataMap map[string]any) {
	cb, ok := dataMap["content_block"].(map[string]any)
	if !ok {
		return
	}
	cbType, _ := cb["type"].(string)
	if cbType != "tool_use" {
		return
	}
	idx := extractBlockIndex(dataMap)
	if idx < 0 {
		return
	}
	id, _ := cb["id"].(string)
	if id == "" {
		return
	}
	ctx.toolUseIdByBlockIndex[idx] = id
}

func (ctx *streamProcessorContext) processToolUseStop(dataMap map[string]any) {
	idx := extractBlockIndex(dataMap)
	if idx < 0 {
		return
	}
	if toolID, exists := ctx.toolUseIdByBlockIndex[idx]; exists && toolID != "" {
		ctx.completedToolUseIds[toolID] = true
		delete(ctx.toolUseIdByBlockIndex, idx)
	}
}

func (ctx *streamProcessorContext) handleExceptionEvent(dataMap map[string]any) bool {
	exceptionType, _ := dataMap["exception_type"].(string)
	if exceptionType == "ContentLengthExceededException" || strings.Contains(exceptionType, "CONTENT_LENGTH_EXCEEDS") {
		activeBlocks := ctx.sseStateManager.GetActiveBlocks()
		for index, block := range activeBlocks {
			if block.Started && !block.Stopped {
				stopEvent := map[string]any{
					"type":  "content_block_stop",
					"index": index,
				}
				_ = ctx.sseStateManager.SendEvent(stopEvent)
			}
		}

		maxTokensEvent := map[string]any{
			"type": "message_delta",
			"delta": map[string]any{
				"stop_reason":   "max_tokens",
				"stop_sequence": nil,
			},
			"usage": map[string]any{
				"input_tokens":  ctx.inputTokens,
				"output_tokens": ctx.totalOutputTokens,
			},
		}
		_ = ctx.sseStateManager.SendEvent(maxTokensEvent)

		stopEvent := map[string]any{
			"type": "message_stop",
		}
		_ = ctx.sseStateManager.SendEvent(stopEvent)
		ctx.flusher.Flush()
		return true
	}
	return false
}

func (ctx *streamProcessorContext) sendFinalEvents() error {
	activeBlocks := ctx.sseStateManager.GetActiveBlocks()
	for index, block := range activeBlocks {
		if block.Started && !block.Stopped {
			stopEvent := map[string]any{
				"type":  "content_block_stop",
				"index": index,
			}
			_ = ctx.sseStateManager.SendEvent(stopEvent)
		}
	}

	hasActiveTools := len(ctx.toolUseIdByBlockIndex) > 0
	hasCompletedTools := len(ctx.completedToolUseIds) > 0
	ctx.stopReasonManager.UpdateToolCallStatus(hasActiveTools, hasCompletedTools)

	outputTokens := ctx.totalOutputTokens
	if outputTokens < 1 {
		hasContent := hasCompletedTools || hasActiveTools || ctx.totalProcessedEvents > 0
		if hasContent {
			outputTokens = 1
		}
	}

	stopReason := ctx.stopReasonManager.DetermineStopReason()

	finalEvents := []map[string]any{
		{
			"type": "message_delta",
			"delta": map[string]any{
				"stop_reason":   stopReason,
				"stop_sequence": nil,
			},
			"usage": map[string]any{
				"output_tokens": outputTokens,
				"input_tokens":  ctx.inputTokens,
			},
		},
		{
			"type": "message_stop",
		},
	}

	for _, event := range finalEvents {
		if err := ctx.sseStateManager.SendEvent(event); err != nil {
			return err
		}
		ctx.flusher.Flush()
	}

	return nil
}

// formatSSE formats a single SSE event.
func formatSSE(event map[string]any) string {
	eventType, _ := event["type"].(string)
	data, _ := FastMarshal(event)
	return fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, string(data))
}

func extractBlockIndex(dataMap map[string]any) int {
	if v, ok := dataMap["index"].(int); ok {
		return v
	}
	if f, ok := dataMap["index"].(float64); ok {
		return int(f)
	}
	return -1
}

// GetTokenCounts returns the input and output token counts
func (ctx *streamProcessorContext) GetTokenCounts() (inputTokens int, outputTokens int) {
	outputTokens = ctx.totalOutputTokens
	if outputTokens < 1 {
		hasContent := len(ctx.completedToolUseIds) > 0 || len(ctx.toolUseIdByBlockIndex) > 0 || ctx.totalProcessedEvents > 0
		if hasContent {
			outputTokens = 1
		}
	}
	return ctx.inputTokens, outputTokens
}
