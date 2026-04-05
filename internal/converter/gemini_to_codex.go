package converter

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/awsl-project/maxx/internal/domain"
)

func init() {
	// Response transformer is codexToGeminiResponse (reads Gemini body → outputs Codex body)
	// because responses[from][to] must convert FROM from-format TO to-format
	RegisterConverter(domain.ClientTypeGemini, domain.ClientTypeCodex, &geminiToCodexRequest{}, &codexToGeminiResponse{})
}

type geminiToCodexRequest struct{}
type geminiToCodexResponse struct{}

func (c *geminiToCodexRequest) Transform(body []byte, model string, stream bool) ([]byte, error) {
	userAgent := ExtractCodexUserAgent(body)
	var req GeminiRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	codexReq := CodexRequest{
		Model:  model,
		Stream: stream,
	}

	// Convert generation config
	if req.GenerationConfig != nil {
		codexReq.MaxOutputTokens = req.GenerationConfig.MaxOutputTokens
		codexReq.Temperature = req.GenerationConfig.Temperature
		codexReq.TopP = req.GenerationConfig.TopP
		if req.GenerationConfig.ThinkingConfig != nil {
			effort := ""
			if req.GenerationConfig.ThinkingConfig.ThinkingLevel != "" {
				effort = strings.ToLower(req.GenerationConfig.ThinkingConfig.ThinkingLevel)
			} else {
				effort = mapBudgetToEffort(req.GenerationConfig.ThinkingConfig.ThinkingBudget)
			}
			if effort != "" {
				codexReq.Reasoning = &CodexReasoning{
					Effort: effort,
				}
			}
		}
	}

	// Convert contents to input
	shortMap := map[string]string{}
	if len(req.Tools) > 0 {
		var names []string
		for _, tool := range req.Tools {
			for _, decl := range tool.FunctionDeclarations {
				if decl.Name != "" {
					names = append(names, decl.Name)
				}
			}
		}
		if len(names) > 0 {
			shortMap = buildShortNameMap(names)
		}
	}
	var inputItems []map[string]interface{}
	if req.SystemInstruction != nil {
		var sysParts []map[string]interface{}
		for _, part := range req.SystemInstruction.Parts {
			if part.Text != "" {
				sysParts = append(sysParts, map[string]interface{}{
					"type": "input_text",
					"text": part.Text,
				})
			}
		}
		if len(sysParts) > 0 {
			inputItems = append(inputItems, map[string]interface{}{
				"type":    "message",
				"role":    "developer",
				"content": sysParts,
			})
		}
	}
	var pendingCallIDs []string
	callCounter := 0
	newCallID := func() string {
		callCounter++
		return fmt.Sprintf("call_%d", callCounter)
	}
	for _, content := range req.Contents {
		role := mapGeminiRoleToCodex(content.Role)
		var contentParts []map[string]interface{}

		for _, part := range content.Parts {
			if part.Text != "" {
				partType := "input_text"
				if role == "assistant" {
					partType = "output_text"
				}
				contentParts = append(contentParts, map[string]interface{}{
					"type": partType,
					"text": part.Text,
				})
			}
			if part.FunctionCall != nil {
				argsJSON, _ := json.Marshal(part.FunctionCall.Args)
				// Extract call_id from name if present
				name := part.FunctionCall.Name
				if short, ok := shortMap[name]; ok {
					name = short
				} else {
					name = shortenNameIfNeeded(name)
				}
				callID := newCallID()
				pendingCallIDs = append(pendingCallIDs, callID)
				inputItems = append(inputItems, map[string]interface{}{
					"type":      "function_call",
					"name":      name,
					"call_id":   callID,
					"arguments": string(argsJSON),
				})
				continue
			}
			if part.FunctionResponse != nil {
				callID := ""
				if len(pendingCallIDs) > 0 {
					callID = pendingCallIDs[0]
					pendingCallIDs = pendingCallIDs[1:]
				} else {
					callID = newCallID()
				}
				output := ""
				switch resp := part.FunctionResponse.Response.(type) {
				case map[string]interface{}:
					if val, ok := resp["result"]; ok {
						switch v := val.(type) {
						case string:
							output = v
						default:
							if b, err := json.Marshal(v); err == nil {
								output = string(b)
							}
						}
					} else if b, err := json.Marshal(resp); err == nil {
						output = string(b)
					}
				default:
					if resp != nil {
						if b, err := json.Marshal(resp); err == nil {
							output = string(b)
						}
					}
				}
				inputItems = append(inputItems, map[string]interface{}{
					"type":    "function_call_output",
					"call_id": callID,
					"output":  output,
				})
				continue
			}
		}

		if len(contentParts) > 0 {
			inputItems = append(inputItems, map[string]interface{}{
				"type":    "message",
				"role":    role,
				"content": contentParts,
			})
		}
	}

	if len(inputItems) == 1 {
		// Check if single text message from user
		item := inputItems[0]
		if item["type"] == "message" && item["role"] == "user" {
			if content, ok := item["content"].([]map[string]interface{}); ok {
				if len(content) == 1 && content[0]["type"] == "input_text" {
					codexReq.Input = content[0]["text"]
					goto skipInputItems
				}
			}
		}
	}
	codexReq.Input = inputItems
skipInputItems:

	// Convert tools
	for _, tool := range req.Tools {
		for _, funcDecl := range tool.FunctionDeclarations {
			name := funcDecl.Name
			if short, ok := shortMap[name]; ok {
				name = short
			} else {
				name = shortenNameIfNeeded(name)
			}
			params := funcDecl.Parameters
			if params == nil {
				params = funcDecl.ParametersJsonSchema
			}
			params = sanitizeGeminiToolParameters(params)
			codexReq.Tools = append(codexReq.Tools, CodexTool{
				Type:        "function",
				Name:        name,
				Description: funcDecl.Description,
				Parameters:  params,
			})
		}
	}
	if len(codexReq.Tools) > 0 {
		codexReq.ToolChoice = "auto"
	}

	if codexReq.Reasoning == nil {
		codexReq.Reasoning = &CodexReasoning{
			Effort:  "medium",
			Summary: "auto",
		}
	} else {
		codexReq.Reasoning.Effort = strings.TrimSpace(codexReq.Reasoning.Effort)
		if codexReq.Reasoning.Effort == "" {
			codexReq.Reasoning.Effort = "medium"
		}
		if codexReq.Reasoning.Summary == "" {
			codexReq.Reasoning.Summary = "auto"
		}
	}

	parallel := true
	codexReq.ParallelToolCalls = &parallel
	codexReq.Include = []string{"reasoning.encrypted_content"}
	codexReq.Store = false
	codexReq.Stream = stream
	if instructions := CodexInstructionsForModel(model, userAgent); instructions != "" {
		codexReq.Instructions = instructions
	}

	return json.Marshal(codexReq)
}

func mapGeminiRoleToCodex(role string) string {
	switch role {
	case "user":
		return "user"
	case "model":
		return "assistant"
	default:
		return "user"
	}
}

func sanitizeGeminiToolParameters(params interface{}) interface{} {
	if params == nil {
		return nil
	}
	m, ok := params.(map[string]interface{})
	if !ok {
		return params
	}
	cleaned := map[string]interface{}{}
	for k, val := range m {
		if k == "$schema" {
			continue
		}
		cleaned[k] = val
	}
	cleaned["additionalProperties"] = false
	return cleaned
}

func (c *geminiToCodexResponse) Transform(body []byte) ([]byte, error) {
	var resp CodexResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	geminiResp := GeminiResponse{
		UsageMetadata: &GeminiUsageMetadata{
			PromptTokenCount:     resp.Usage.InputTokens,
			CandidatesTokenCount: resp.Usage.OutputTokens,
			TotalTokenCount:      resp.Usage.TotalTokens,
		},
	}

	// Convert output to candidates
	var parts []GeminiPart
	for _, out := range resp.Output {
		switch out.Type {
		case "message":
			switch content := out.Content.(type) {
			case string:
				parts = append(parts, GeminiPart{Text: content})
			case []interface{}:
				for _, c := range content {
					if cm, ok := c.(map[string]interface{}); ok {
						if text, ok := cm["text"].(string); ok {
							parts = append(parts, GeminiPart{Text: text})
						}
					}
				}
			}
		case "function_call":
			var args map[string]interface{}
			json.Unmarshal([]byte(out.Arguments), &args)
			// Embed call_id in name for round-trip
			name := out.Name
			if out.CallID != "" {
				name = out.Name + "_" + out.CallID
			}
			parts = append(parts, GeminiPart{
				FunctionCall: &GeminiFunctionCall{
					Name: name,
					Args: args,
				},
			})
		}
	}

	finishReason := "STOP"
	if resp.Status == "incomplete" {
		finishReason = "MAX_TOKENS"
	}
	// Check if there are function calls
	for _, part := range parts {
		if part.FunctionCall != nil {
			finishReason = "STOP"
			break
		}
	}

	geminiResp.Candidates = []GeminiCandidate{{
		Content: GeminiContent{
			Role:  "model",
			Parts: parts,
		},
		FinishReason: finishReason,
		Index:        0,
	}}

	return json.Marshal(geminiResp)
}

func (c *geminiToCodexResponse) TransformChunk(chunk []byte, state *TransformState) ([]byte, error) {
	events, remaining := ParseSSE(state.Buffer + string(chunk))
	state.Buffer = remaining

	var output []byte
	for _, event := range events {
		if event.Event == "done" {
			continue
		}

		var codexEvent CodexStreamEvent
		if err := json.Unmarshal(event.Data, &codexEvent); err != nil {
			continue
		}

		switch codexEvent.Type {
		case "response.created":
			if codexEvent.Response != nil {
				state.MessageID = codexEvent.Response.ID
			}

		case "response.output_text.delta":
			if codexEvent.Delta != nil && codexEvent.Delta.Text != "" {
				geminiChunk := GeminiStreamChunk{
					Candidates: []GeminiCandidate{{
						Content: GeminiContent{
							Role:  "model",
							Parts: []GeminiPart{{Text: codexEvent.Delta.Text}},
						},
						Index: 0,
					}},
				}
				output = append(output, FormatSSE("", geminiChunk)...)
			}

		case "response.output_item.added":
			if codexEvent.Item != nil && codexEvent.Item.Type == "function_call" {
				var args map[string]interface{}
				json.Unmarshal([]byte(codexEvent.Item.Arguments), &args)
				name := codexEvent.Item.Name
				if codexEvent.Item.CallID != "" {
					name = codexEvent.Item.Name + "_" + codexEvent.Item.CallID
				}
				geminiChunk := GeminiStreamChunk{
					Candidates: []GeminiCandidate{{
						Content: GeminiContent{
							Role: "model",
							Parts: []GeminiPart{{
								FunctionCall: &GeminiFunctionCall{
									Name: name,
									Args: args,
								},
							}},
						},
						Index: 0,
					}},
				}
				output = append(output, FormatSSE("", geminiChunk)...)
			}

		case "response.completed":
			if codexEvent.Response != nil {
				finishReason := "STOP"
				geminiChunk := GeminiStreamChunk{
					Candidates: []GeminiCandidate{{
						Content:      GeminiContent{Role: "model", Parts: []GeminiPart{}},
						FinishReason: finishReason,
						Index:        0,
					}},
					UsageMetadata: &GeminiUsageMetadata{
						PromptTokenCount:     codexEvent.Response.Usage.InputTokens,
						CandidatesTokenCount: codexEvent.Response.Usage.OutputTokens,
						TotalTokenCount:      codexEvent.Response.Usage.TotalTokens,
					},
				}
				output = append(output, FormatSSE("", geminiChunk)...)
			}
		}
	}

	return output, nil
}
