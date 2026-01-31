package converter

import (
	"encoding/json"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

func init() {
	RegisterConverter(domain.ClientTypeClaude, domain.ClientTypeOpenAI, &claudeToOpenAIRequest{}, &claudeToOpenAIResponse{})
}

type claudeToOpenAIRequest struct{}
type claudeToOpenAIResponse struct{}

func (c *claudeToOpenAIRequest) Transform(body []byte, model string, stream bool) ([]byte, error) {
	var req ClaudeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	openaiReq := OpenAIRequest{
		Model:       model,
		Stream:      stream,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
	}

	// Convert system to first message
	if req.System != nil {
		switch s := req.System.(type) {
		case string:
			openaiReq.Messages = append(openaiReq.Messages, OpenAIMessage{
				Role:    "system",
				Content: s,
			})
		case []interface{}:
			var systemText string
			for _, block := range s {
				if m, ok := block.(map[string]interface{}); ok {
					if text, ok := m["text"].(string); ok {
						systemText += text
					}
				}
			}
			if systemText != "" {
				openaiReq.Messages = append(openaiReq.Messages, OpenAIMessage{
					Role:    "system",
					Content: systemText,
				})
			}
		}
	}

	// Convert messages
	for _, msg := range req.Messages {
		openaiMsg := OpenAIMessage{Role: msg.Role}
		switch content := msg.Content.(type) {
		case string:
			openaiMsg.Content = content
		case []interface{}:
			var parts []OpenAIContentPart
			var toolCalls []OpenAIToolCall
			for _, block := range content {
				if m, ok := block.(map[string]interface{}); ok {
					blockType, _ := m["type"].(string)
					switch blockType {
					case "text":
						if text, ok := m["text"].(string); ok {
							parts = append(parts, OpenAIContentPart{Type: "text", Text: text})
						}
					case "tool_use":
						id, _ := m["id"].(string)
						name, _ := m["name"].(string)
						input := m["input"]
						inputJSON, _ := json.Marshal(input)
						toolCalls = append(toolCalls, OpenAIToolCall{
							ID:       id,
							Type:     "function",
							Function: OpenAIFunctionCall{Name: name, Arguments: string(inputJSON)},
						})
					case "tool_result":
						toolUseID, _ := m["tool_use_id"].(string)
						content, _ := m["content"].(string)
						openaiReq.Messages = append(openaiReq.Messages, OpenAIMessage{
							Role:       "tool",
							Content:    content,
							ToolCallID: toolUseID,
						})
						continue
					}
				}
			}
			if len(toolCalls) > 0 {
				openaiMsg.ToolCalls = toolCalls
			}
			if len(parts) == 1 && parts[0].Type == "text" {
				openaiMsg.Content = parts[0].Text
			} else if len(parts) > 0 {
				openaiMsg.Content = parts
			}
		}
		// Only add message if it has actual content (avoid empty user messages)
		if openaiMsg.Content != nil || len(openaiMsg.ToolCalls) > 0 {
			openaiReq.Messages = append(openaiReq.Messages, openaiMsg)
		}
	}

	// Convert tools
	for _, tool := range req.Tools {
		openaiReq.Tools = append(openaiReq.Tools, OpenAITool{
			Type: "function",
			Function: OpenAIFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema,
			},
		})
	}

	// Convert stop sequences
	if len(req.StopSequences) > 0 {
		openaiReq.Stop = req.StopSequences
	}

	return json.Marshal(openaiReq)
}

func (c *claudeToOpenAIResponse) Transform(body []byte) ([]byte, error) {
	var resp ClaudeResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	openaiResp := OpenAIResponse{
		ID:      resp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   resp.Model,
		Usage: OpenAIUsage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}

	// Convert content to message
	msg := OpenAIMessage{Role: "assistant"}
	var textContent string
	var toolCalls []OpenAIToolCall

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			textContent += block.Text
		case "tool_use":
			inputJSON, _ := json.Marshal(block.Input)
			toolCalls = append(toolCalls, OpenAIToolCall{
				ID:       block.ID,
				Type:     "function",
				Function: OpenAIFunctionCall{Name: block.Name, Arguments: string(inputJSON)},
			})
		}
	}

	if textContent != "" {
		msg.Content = textContent
	}
	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	}

	// Map stop reason
	finishReason := "stop"
	switch resp.StopReason {
	case "end_turn":
		finishReason = "stop"
	case "max_tokens":
		finishReason = "length"
	case "tool_use":
		finishReason = "tool_calls"
	}

	openaiResp.Choices = []OpenAIChoice{{
		Index:        0,
		Message:      &msg,
		FinishReason: finishReason,
	}}

	return json.Marshal(openaiResp)
}

func (c *claudeToOpenAIResponse) TransformChunk(chunk []byte, state *TransformState) ([]byte, error) {
	events, remaining := ParseSSE(state.Buffer + string(chunk))
	state.Buffer = remaining

	var output []byte
	for _, event := range events {
		if event.Event == "done" {
			output = append(output, FormatDone()...)
			continue
		}

		var claudeEvent ClaudeStreamEvent
		if err := json.Unmarshal(event.Data, &claudeEvent); err != nil {
			continue
		}

		switch claudeEvent.Type {
		case "message_start":
			if claudeEvent.Message != nil {
				state.MessageID = claudeEvent.Message.ID
			}
			chunk := OpenAIStreamChunk{
				ID:      state.MessageID,
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Choices: []OpenAIChoice{{
					Index: 0,
					Delta: &OpenAIMessage{Role: "assistant", Content: ""},
				}},
			}
			output = append(output, FormatSSE("", chunk)...)

		case "content_block_start":
			if claudeEvent.ContentBlock != nil {
				state.CurrentBlockType = claudeEvent.ContentBlock.Type
				state.CurrentIndex = claudeEvent.Index
				if claudeEvent.ContentBlock.Type == "tool_use" {
					state.ToolCalls[claudeEvent.Index] = &ToolCallState{
						ID:   claudeEvent.ContentBlock.ID,
						Name: claudeEvent.ContentBlock.Name,
					}
				}
			}

		case "content_block_delta":
			if claudeEvent.Delta != nil {
				switch claudeEvent.Delta.Type {
				case "text_delta":
					chunk := OpenAIStreamChunk{
						ID:      state.MessageID,
						Object:  "chat.completion.chunk",
						Created: time.Now().Unix(),
						Choices: []OpenAIChoice{{
							Index: 0,
							Delta: &OpenAIMessage{Content: claudeEvent.Delta.Text},
						}},
					}
					output = append(output, FormatSSE("", chunk)...)
				case "input_json_delta":
					if tc, ok := state.ToolCalls[state.CurrentIndex]; ok {
						tc.Arguments += claudeEvent.Delta.PartialJSON
						chunk := OpenAIStreamChunk{
							ID:      state.MessageID,
							Object:  "chat.completion.chunk",
							Created: time.Now().Unix(),
							Choices: []OpenAIChoice{{
								Index: 0,
								Delta: &OpenAIMessage{
									ToolCalls: []OpenAIToolCall{{
										Index:    state.CurrentIndex,
										ID:       tc.ID,
										Type:     "function",
										Function: OpenAIFunctionCall{Name: tc.Name, Arguments: claudeEvent.Delta.PartialJSON},
									}},
								},
							}},
						}
						output = append(output, FormatSSE("", chunk)...)
					}
				}
			}

		case "message_delta":
			if claudeEvent.Delta != nil {
				state.StopReason = claudeEvent.Delta.StopReason
			}
			if claudeEvent.Usage != nil {
				state.Usage.OutputTokens = claudeEvent.Usage.OutputTokens
			}

		case "message_stop":
			finishReason := "stop"
			switch state.StopReason {
			case "end_turn":
				finishReason = "stop"
			case "max_tokens":
				finishReason = "length"
			case "tool_use":
				finishReason = "tool_calls"
			}
			chunk := OpenAIStreamChunk{
				ID:      state.MessageID,
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Choices: []OpenAIChoice{{
					Index:        0,
					Delta:        &OpenAIMessage{},
					FinishReason: finishReason,
				}},
			}
			output = append(output, FormatSSE("", chunk)...)
			output = append(output, FormatDone()...)
		}
	}

	return output, nil
}

// Add Index field to OpenAIToolCall for streaming
type OpenAIToolCallWithIndex struct {
	Index    int                `json:"index"`
	ID       string             `json:"id,omitempty"`
	Type     string             `json:"type,omitempty"`
	Function OpenAIFunctionCall `json:"function,omitempty"`
}
