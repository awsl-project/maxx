package converter

import (
	"encoding/json"

	"github.com/awsl-project/maxx/internal/domain"
)

func init() {
	RegisterConverter(domain.ClientTypeOpenAI, domain.ClientTypeClaude, &openaiToClaudeRequest{}, &openaiToClaudeResponse{})
}

type openaiToClaudeRequest struct{}
type openaiToClaudeResponse struct{}

func (c *openaiToClaudeRequest) Transform(body []byte, model string, stream bool) ([]byte, error) {
	var req OpenAIRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	claudeReq := ClaudeRequest{
		Model:       model,
		Stream:      stream,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
	}

	if req.MaxCompletionTokens > 0 && req.MaxTokens == 0 {
		claudeReq.MaxTokens = req.MaxCompletionTokens
	}

	// Convert messages
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			// Extract system message
			switch content := msg.Content.(type) {
			case string:
				claudeReq.System = content
			case []interface{}:
				var systemText string
				for _, part := range content {
					if m, ok := part.(map[string]interface{}); ok {
						if text, ok := m["text"].(string); ok {
							systemText += text
						}
					}
				}
				claudeReq.System = systemText
			}
			continue
		}

		claudeMsg := ClaudeMessage{Role: msg.Role}

		// Handle tool messages
		if msg.Role == "tool" {
			claudeMsg.Role = "user"
			contentStr, _ := msg.Content.(string)
			claudeMsg.Content = []ClaudeContentBlock{{
				Type:      "tool_result",
				ToolUseID: msg.ToolCallID,
				Content:   contentStr,
			}}
			claudeReq.Messages = append(claudeReq.Messages, claudeMsg)
			continue
		}

		// Convert content
		switch content := msg.Content.(type) {
		case string:
			claudeMsg.Content = content
		case []interface{}:
			var blocks []ClaudeContentBlock
			for _, part := range content {
				if m, ok := part.(map[string]interface{}); ok {
					partType, _ := m["type"].(string)
					switch partType {
					case "text":
						text, _ := m["text"].(string)
						blocks = append(blocks, ClaudeContentBlock{Type: "text", Text: text})
					}
				}
			}
			if len(blocks) == 1 && blocks[0].Type == "text" {
				claudeMsg.Content = blocks[0].Text
			} else {
				claudeMsg.Content = blocks
			}
		}

		// Handle tool calls
		if len(msg.ToolCalls) > 0 {
			var blocks []ClaudeContentBlock
			if text, ok := claudeMsg.Content.(string); ok && text != "" {
				blocks = append(blocks, ClaudeContentBlock{Type: "text", Text: text})
			}
			for _, tc := range msg.ToolCalls {
				var input interface{}
				json.Unmarshal([]byte(tc.Function.Arguments), &input)
				blocks = append(blocks, ClaudeContentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Function.Name,
					Input: input,
				})
			}
			claudeMsg.Content = blocks
		}

		claudeReq.Messages = append(claudeReq.Messages, claudeMsg)
	}

	// Convert tools
	for _, tool := range req.Tools {
		claudeReq.Tools = append(claudeReq.Tools, ClaudeTool{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			InputSchema: tool.Function.Parameters,
		})
	}

	// Convert stop
	switch stop := req.Stop.(type) {
	case string:
		claudeReq.StopSequences = []string{stop}
	case []interface{}:
		for _, s := range stop {
			if str, ok := s.(string); ok {
				claudeReq.StopSequences = append(claudeReq.StopSequences, str)
			}
		}
	}

	return json.Marshal(claudeReq)
}

func (c *openaiToClaudeResponse) Transform(body []byte) ([]byte, error) {
	var resp OpenAIResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	claudeResp := ClaudeResponse{
		ID:    resp.ID,
		Type:  "message",
		Role:  "assistant",
		Model: resp.Model,
		Usage: ClaudeUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		},
	}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		if choice.Message != nil {
			// Convert content
			if content, ok := choice.Message.Content.(string); ok && content != "" {
				claudeResp.Content = append(claudeResp.Content, ClaudeContentBlock{
					Type: "text",
					Text: content,
				})
			}

			// Convert tool calls
			for _, tc := range choice.Message.ToolCalls {
				var input interface{}
				json.Unmarshal([]byte(tc.Function.Arguments), &input)
				claudeResp.Content = append(claudeResp.Content, ClaudeContentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Function.Name,
					Input: input,
				})
			}

			// Map finish reason
			switch choice.FinishReason {
			case "stop":
				claudeResp.StopReason = "end_turn"
			case "length":
				claudeResp.StopReason = "max_tokens"
			case "tool_calls":
				claudeResp.StopReason = "tool_use"
			}
		}
	}

	return json.Marshal(claudeResp)
}

func (c *openaiToClaudeResponse) TransformChunk(chunk []byte, state *TransformState) ([]byte, error) {
	events, remaining := ParseSSE(state.Buffer + string(chunk))
	state.Buffer = remaining

	var output []byte
	for _, event := range events {
		if event.Event == "done" {
			// Close all open blocks and send message_stop
			// Skip if no message was started (upstream returned no valid data)
			if state.MessageID != "" {
				output = append(output, c.handleFinish(state)...)
			}
			continue
		}

		var openaiChunk OpenAIStreamChunk
		if err := json.Unmarshal(event.Data, &openaiChunk); err != nil {
			continue
		}

		// Handle usage from stream (when stream_options.include_usage is true)
		if openaiChunk.Usage != nil {
			state.Usage.InputTokens = openaiChunk.Usage.PromptTokens
			state.Usage.OutputTokens = openaiChunk.Usage.CompletionTokens
		}

		if len(openaiChunk.Choices) == 0 {
			continue
		}

		choice := openaiChunk.Choices[0]

		// First chunk - send message_start (but not content_block_start yet)
		if state.MessageID == "" {
			state.MessageID = openaiChunk.ID
			msgStart := map[string]interface{}{
				"type": "message_start",
				"message": map[string]interface{}{
					"id":            openaiChunk.ID,
					"type":          "message",
					"role":          "assistant",
					"model":         openaiChunk.Model,
					"content":       []interface{}{},
					"stop_reason":   nil,
					"stop_sequence": nil,
					"usage":         map[string]int{"input_tokens": 0, "output_tokens": 0},
				},
			}
			output = append(output, FormatSSE("message_start", msgStart)...)
		}

		if choice.Delta != nil {
			// Handle text content
			if content, ok := choice.Delta.Content.(string); ok && content != "" {
				// Ensure text block is started
				if state.CurrentBlockType != "text" {
					blockStart := map[string]interface{}{
						"type":  "content_block_start",
						"index": state.CurrentIndex,
						"content_block": map[string]interface{}{
							"type": "text",
							"text": "",
						},
					}
					output = append(output, FormatSSE("content_block_start", blockStart)...)
					state.CurrentBlockType = "text"
				}

				delta := map[string]interface{}{
					"type":  "content_block_delta",
					"index": state.CurrentIndex,
					"delta": map[string]interface{}{
						"type": "text_delta",
						"text": content,
					},
				}
				output = append(output, FormatSSE("content_block_delta", delta)...)
			}

			// Handle tool calls
			if len(choice.Delta.ToolCalls) > 0 {
				for _, toolCall := range choice.Delta.ToolCalls {
					toolIndex := toolCall.Index

					// Initialize tool call state if needed
					if state.ToolCalls == nil {
						state.ToolCalls = make(map[int]*ToolCallState)
					}

					tc, exists := state.ToolCalls[toolIndex]
					if !exists {
						// Close previous text block if any
						if state.CurrentBlockType == "text" {
							blockStop := map[string]interface{}{
								"type":  "content_block_stop",
								"index": state.CurrentIndex,
							}
							output = append(output, FormatSSE("content_block_stop", blockStop)...)
							state.CurrentIndex++
							state.CurrentBlockType = ""
						}

						// New tool call - send content_block_start
						tc = &ToolCallState{
							ID:           toolCall.ID,
							Name:         toolCall.Function.Name,
							ContentIndex: state.CurrentIndex,
						}
						state.ToolCalls[toolIndex] = tc

						blockStart := map[string]interface{}{
							"type":  "content_block_start",
							"index": tc.ContentIndex,
							"content_block": map[string]interface{}{
								"type":  "tool_use",
								"id":    tc.ID,
								"name":  tc.Name,
								"input": map[string]interface{}{},
							},
						}
						output = append(output, FormatSSE("content_block_start", blockStart)...)
						state.CurrentIndex++
					} else {
						// Update existing tool call state
						if toolCall.ID != "" {
							tc.ID = toolCall.ID
						}
						if toolCall.Function.Name != "" {
							tc.Name = toolCall.Function.Name
						}
					}

					// Send arguments delta if present
					if toolCall.Function.Arguments != "" {
						tc.Arguments += toolCall.Function.Arguments
						delta := map[string]interface{}{
							"type":  "content_block_delta",
							"index": tc.ContentIndex,
							"delta": map[string]interface{}{
								"type":         "input_json_delta",
								"partial_json": toolCall.Function.Arguments,
							},
						}
						output = append(output, FormatSSE("content_block_delta", delta)...)
					}
				}
			}
		}

		// Handle finish reason - just record, actual close happens on [DONE]
		if choice.FinishReason != "" {
			state.StopReason = choice.FinishReason
		}
	}

	return output, nil
}

// handleFinish closes all open blocks and sends final events
func (c *openaiToClaudeResponse) handleFinish(state *TransformState) []byte {
	var output []byte

	// Close text block if open
	if state.CurrentBlockType == "text" {
		blockStop := map[string]interface{}{
			"type":  "content_block_stop",
			"index": state.CurrentIndex,
		}
		output = append(output, FormatSSE("content_block_stop", blockStop)...)
		state.CurrentIndex++
		state.CurrentBlockType = ""
	}

	// Close all tool blocks
	for _, tc := range state.ToolCalls {
		blockStop := map[string]interface{}{
			"type":  "content_block_stop",
			"index": tc.ContentIndex,
		}
		output = append(output, FormatSSE("content_block_stop", blockStop)...)
	}

	// Map finish reason
	stopReason := "end_turn"
	switch state.StopReason {
	case "length":
		stopReason = "max_tokens"
	case "tool_calls":
		stopReason = "tool_use"
	}

	// Send message_delta
	msgDelta := map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]interface{}{
			"stop_reason": stopReason,
		},
		"usage": map[string]int{"output_tokens": state.Usage.OutputTokens},
	}
	output = append(output, FormatSSE("message_delta", msgDelta)...)

	// Send message_stop
	output = append(output, FormatSSE("message_stop", map[string]string{"type": "message_stop"})...)

	// Clear tool calls to prevent double closing
	state.ToolCalls = nil

	return output
}
