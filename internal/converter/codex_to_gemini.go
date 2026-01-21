package converter

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

func init() {
	RegisterConverter(domain.ClientTypeCodex, domain.ClientTypeGemini, &codexToGeminiRequest{}, &codexToGeminiResponse{})
}

type codexToGeminiRequest struct{}
type codexToGeminiResponse struct{}

func (c *codexToGeminiRequest) Transform(body []byte, model string, stream bool) ([]byte, error) {
	var req CodexRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	geminiReq := GeminiRequest{
		GenerationConfig: &GeminiGenerationConfig{
			MaxOutputTokens: req.MaxOutputTokens,
			Temperature:     req.Temperature,
			TopP:            req.TopP,
		},
	}

	// Convert instructions to system instruction
	if req.Instructions != "" {
		// [FIX] Set role to "user" for systemInstruction (like CLIProxyAPI)
		geminiReq.SystemInstruction = &GeminiContent{
			Role:  "user",
			Parts: []GeminiPart{{Text: req.Instructions}},
		}
	}

	// Convert input to contents
	switch input := req.Input.(type) {
	case string:
		geminiReq.Contents = append(geminiReq.Contents, GeminiContent{
			Role:  "user",
			Parts: []GeminiPart{{Text: input}},
		})
	case []interface{}:
		for _, item := range input {
			if m, ok := item.(map[string]interface{}); ok {
				itemType, _ := m["type"].(string)
				switch itemType {
				case "message":
					role := mapCodexRoleToGemini(m["role"])
					content := m["content"]
					var parts []GeminiPart
					switch c := content.(type) {
					case string:
						parts = append(parts, GeminiPart{Text: c})
					case []interface{}:
						for _, part := range c {
							if pm, ok := part.(map[string]interface{}); ok {
								partType, _ := pm["type"].(string)
								if partType == "input_text" || partType == "output_text" {
									if text, ok := pm["text"].(string); ok {
										parts = append(parts, GeminiPart{Text: text})
									}
								}
							}
						}
					}
					if len(parts) > 0 {
						geminiReq.Contents = append(geminiReq.Contents, GeminiContent{
							Role:  role,
							Parts: parts,
						})
					}
				case "function_call":
					name, _ := m["name"].(string)
					callID, _ := m["call_id"].(string)
					arguments, _ := m["arguments"].(string)
					var args map[string]interface{}
					json.Unmarshal([]byte(arguments), &args)
					geminiReq.Contents = append(geminiReq.Contents, GeminiContent{
						Role: "model",
						Parts: []GeminiPart{{
							FunctionCall: &GeminiFunctionCall{
								Name: name + "_" + callID,
								Args: args,
							},
						}},
					})
				case "function_call_output":
					callID, _ := m["call_id"].(string)
					output, _ := m["output"].(string)
					// Find the function name from previous content
					funcName := "function_" + callID
					geminiReq.Contents = append(geminiReq.Contents, GeminiContent{
						Role: "user",
						Parts: []GeminiPart{{
							FunctionResponse: &GeminiFunctionResponse{
								Name:     funcName,
								Response: map[string]interface{}{"result": output},
							},
						}},
					})
				}
			}
		}
	}

	// Convert tools
	if len(req.Tools) > 0 {
		var funcDecls []GeminiFunctionDecl
		for _, tool := range req.Tools {
			if tool.Type == "function" {
				funcDecls = append(funcDecls, GeminiFunctionDecl{
					Name:        tool.Name,
					Description: tool.Description,
					Parameters:  tool.Parameters,
				})
			}
		}
		if len(funcDecls) > 0 {
			geminiReq.Tools = []GeminiTool{{FunctionDeclarations: funcDecls}}
		}
	}

	return json.Marshal(geminiReq)
}

func mapCodexRoleToGemini(role interface{}) string {
	r, _ := role.(string)
	switch r {
	case "user":
		return "user"
	case "assistant", "system":
		return "model"
	default:
		return "user"
	}
}

func (c *codexToGeminiResponse) Transform(body []byte) ([]byte, error) {
	var resp GeminiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	codexResp := CodexResponse{
		ID:        "resp_" + time.Now().Format("20060102150405"),
		Object:    "response",
		CreatedAt: time.Now().Unix(),
		Status:    "completed",
	}

	if resp.UsageMetadata != nil {
		codexResp.Usage = CodexUsage{
			InputTokens:  resp.UsageMetadata.PromptTokenCount,
			OutputTokens: resp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:  resp.UsageMetadata.TotalTokenCount,
		}
	}

	// Convert candidates to output
	for _, candidate := range resp.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				codexResp.Output = append(codexResp.Output, CodexOutput{
					Type:    "message",
					ID:      "msg_" + time.Now().Format("20060102150405"),
					Role:    "assistant",
					Content: []map[string]interface{}{{"type": "output_text", "text": part.Text}},
				})
			}
			if part.FunctionCall != nil {
				argsJSON, _ := json.Marshal(part.FunctionCall.Args)
				// Extract call_id from name if present
				name := part.FunctionCall.Name
				callID := "call_" + time.Now().Format("20060102150405")
				if idx := strings.LastIndex(name, "_"); idx > 0 {
					callID = name[idx+1:]
					name = name[:idx]
				}
				codexResp.Output = append(codexResp.Output, CodexOutput{
					Type:      "function_call",
					ID:        "fc_" + time.Now().Format("20060102150405"),
					Name:      name,
					CallID:    callID,
					Arguments: string(argsJSON),
					Status:    "completed",
				})
			}
		}
	}

	return json.Marshal(codexResp)
}

func (c *codexToGeminiResponse) TransformChunk(chunk []byte, state *TransformState) ([]byte, error) {
	events, remaining := ParseSSE(state.Buffer + string(chunk))
	state.Buffer = remaining

	var output []byte
	for _, event := range events {
		if event.Event == "done" {
			// Send response.completed event
			completedEvent := CodexStreamEvent{
				Type: "response.completed",
				Response: &CodexResponse{
					ID:        state.MessageID,
					Object:    "response",
					CreatedAt: time.Now().Unix(),
					Status:    "completed",
					Usage: CodexUsage{
						InputTokens:  state.Usage.InputTokens,
						OutputTokens: state.Usage.OutputTokens,
						TotalTokens:  state.Usage.InputTokens + state.Usage.OutputTokens,
					},
				},
			}
			output = append(output, FormatSSE("response.completed", completedEvent)...)
			output = append(output, FormatDone()...)
			continue
		}

		var geminiChunk GeminiStreamChunk
		if err := json.Unmarshal(event.Data, &geminiChunk); err != nil {
			continue
		}

		// Initialize on first chunk
		if state.MessageID == "" {
			state.MessageID = "resp_" + time.Now().Format("20060102150405")
			createdEvent := CodexStreamEvent{
				Type: "response.created",
				Response: &CodexResponse{
					ID:        state.MessageID,
					Object:    "response",
					CreatedAt: time.Now().Unix(),
					Status:    "in_progress",
				},
			}
			output = append(output, FormatSSE("response.created", createdEvent)...)
		}

		// Update usage
		if geminiChunk.UsageMetadata != nil {
			state.Usage.InputTokens = geminiChunk.UsageMetadata.PromptTokenCount
			state.Usage.OutputTokens = geminiChunk.UsageMetadata.CandidatesTokenCount
		}

		// Process candidates
		for _, candidate := range geminiChunk.Candidates {
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					deltaEvent := CodexStreamEvent{
						Type: "response.output_text.delta",
						Delta: &CodexDelta{
							Type: "output_text_delta",
							Text: part.Text,
						},
					}
					output = append(output, FormatSSE("response.output_text.delta", deltaEvent)...)
				}
				if part.FunctionCall != nil {
					argsJSON, _ := json.Marshal(part.FunctionCall.Args)
					name := part.FunctionCall.Name
					callID := "call_" + time.Now().Format("20060102150405")
					if idx := strings.LastIndex(name, "_"); idx > 0 {
						callID = name[idx+1:]
						name = name[:idx]
					}
					itemEvent := CodexStreamEvent{
						Type: "response.output_item.added",
						Item: &CodexOutput{
							Type:      "function_call",
							ID:        "fc_" + time.Now().Format("20060102150405"),
							Name:      name,
							CallID:    callID,
							Arguments: string(argsJSON),
							Status:    "completed",
						},
					}
					output = append(output, FormatSSE("response.output_item.added", itemEvent)...)
				}
			}
		}
	}

	return output, nil
}
