package converter

import (
	"encoding/json"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

func init() {
	RegisterConverter(domain.ClientTypeClaude, domain.ClientTypeCodex, &claudeToCodexRequest{}, &claudeToCodexResponse{})
}

type claudeToCodexRequest struct{}
type claudeToCodexResponse struct{}

func (c *claudeToCodexRequest) Transform(body []byte, model string, stream bool) ([]byte, error) {
	var req ClaudeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	codexReq := CodexRequest{
		Model:           model,
		Stream:          stream,
		MaxOutputTokens: req.MaxTokens,
		Temperature:     req.Temperature,
		TopP:            req.TopP,
	}

	// Convert system to instructions
	if req.System != nil {
		switch s := req.System.(type) {
		case string:
			codexReq.Instructions = s
		case []interface{}:
			var systemText string
			for _, block := range s {
				if m, ok := block.(map[string]interface{}); ok {
					if text, ok := m["text"].(string); ok {
						systemText += text
					}
				}
			}
			codexReq.Instructions = systemText
		}
	}

	// Convert messages to input
	var input []CodexInputItem
	for _, msg := range req.Messages {
		item := CodexInputItem{Role: msg.Role}
		switch content := msg.Content.(type) {
		case string:
			item.Type = "message"
			item.Content = content
		case []interface{}:
			for _, block := range content {
				if m, ok := block.(map[string]interface{}); ok {
					blockType, _ := m["type"].(string)
					switch blockType {
					case "text":
						item.Type = "message"
						item.Content = m["text"]
					case "tool_use":
						// Convert tool use to function_call output
						name, _ := m["name"].(string)
						id, _ := m["id"].(string)
						inputData := m["input"]
						argJSON, _ := json.Marshal(inputData)
						input = append(input, CodexInputItem{
							Type:      "function_call",
							ID:        id,
							CallID:    id,
							Name:      name,
							Role:      "assistant",
							Arguments: string(argJSON),
						})
						continue
					case "tool_result":
						toolUseID, _ := m["tool_use_id"].(string)
						resultContent, _ := m["content"].(string)
						input = append(input, CodexInputItem{
							Type:   "function_call_output",
							CallID: toolUseID,
							Output: resultContent,
						})
						continue
					}
				}
			}
		}
		if item.Type != "" {
			input = append(input, item)
		}
	}
	codexReq.Input = input

	// Convert tools
	for _, tool := range req.Tools {
		codexReq.Tools = append(codexReq.Tools, CodexTool{
			Type:        "function",
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.InputSchema,
		})
	}

	return json.Marshal(codexReq)
}

func (c *claudeToCodexResponse) Transform(body []byte) ([]byte, error) {
	var resp ClaudeResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	codexResp := CodexResponse{
		ID:        resp.ID,
		Object:    "response",
		CreatedAt: time.Now().Unix(),
		Model:     resp.Model,
		Status:    "completed",
		Usage: CodexUsage{
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
			TotalTokens:  resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}

	// Convert content to output
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			codexResp.Output = append(codexResp.Output, CodexOutput{
				Type:    "message",
				Role:    "assistant",
				Content: block.Text,
			})
		case "tool_use":
			argJSON, _ := json.Marshal(block.Input)
			codexResp.Output = append(codexResp.Output, CodexOutput{
				Type:      "function_call",
				ID:        block.ID,
				CallID:    block.ID,
				Name:      block.Name,
				Arguments: string(argJSON),
				Status:    "completed",
			})
		}
	}

	return json.Marshal(codexResp)
}

func (c *claudeToCodexResponse) TransformChunk(chunk []byte, state *TransformState) ([]byte, error) {
	events, remaining := ParseSSE(state.Buffer + string(chunk))
	state.Buffer = remaining

	var output []byte
	for _, event := range events {
		if event.Event == "done" {
			output = append(output, FormatSSE("", map[string]string{"type": "response.done"})...)
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
			codexEvent := map[string]interface{}{
				"type": "response.created",
				"response": map[string]interface{}{
					"id":     state.MessageID,
					"status": "in_progress",
				},
			}
			output = append(output, FormatSSE("", codexEvent)...)

		case "content_block_delta":
			if claudeEvent.Delta != nil && claudeEvent.Delta.Type == "text_delta" {
				codexEvent := map[string]interface{}{
					"type": "response.output_item.delta",
					"delta": map[string]interface{}{
						"type": "text",
						"text": claudeEvent.Delta.Text,
					},
				}
				output = append(output, FormatSSE("", codexEvent)...)
			}

		case "message_stop":
			codexEvent := map[string]interface{}{
				"type": "response.done",
				"response": map[string]interface{}{
					"id":     state.MessageID,
					"status": "completed",
				},
			}
			output = append(output, FormatSSE("", codexEvent)...)
		}
	}

	return output, nil
}
