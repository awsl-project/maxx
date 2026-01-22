package converter

import (
	"encoding/json"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

func init() {
	RegisterConverter(domain.ClientTypeGemini, domain.ClientTypeOpenAI, &geminiToOpenAIRequest{}, &geminiToOpenAIResponse{})
}

type geminiToOpenAIRequest struct{}
type geminiToOpenAIResponse struct{}

func (c *geminiToOpenAIRequest) Transform(body []byte, model string, stream bool) ([]byte, error) {
	var req GeminiRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	openaiReq := OpenAIRequest{
		Model:  model,
		Stream: stream,
	}

	if req.GenerationConfig != nil {
		openaiReq.MaxTokens = req.GenerationConfig.MaxOutputTokens
		openaiReq.Temperature = req.GenerationConfig.Temperature
		openaiReq.TopP = req.GenerationConfig.TopP
		if len(req.GenerationConfig.StopSequences) > 0 {
			openaiReq.Stop = req.GenerationConfig.StopSequences
		}
	}

	// Convert systemInstruction
	if req.SystemInstruction != nil {
		var systemText string
		for _, part := range req.SystemInstruction.Parts {
			systemText += part.Text
		}
		if systemText != "" {
			openaiReq.Messages = append(openaiReq.Messages, OpenAIMessage{
				Role:    "system",
				Content: systemText,
			})
		}
	}

	// Convert contents to messages
	for _, content := range req.Contents {
		openaiMsg := OpenAIMessage{}
		switch content.Role {
		case "user":
			openaiMsg.Role = "user"
		case "model":
			openaiMsg.Role = "assistant"
		default:
			openaiMsg.Role = "user"
		}

		var textContent string
		var toolCalls []OpenAIToolCall

		for _, part := range content.Parts {
			if part.Text != "" {
				textContent += part.Text
			}
			if part.FunctionCall != nil {
				argsJSON, _ := json.Marshal(part.FunctionCall.Args)
				toolCalls = append(toolCalls, OpenAIToolCall{
					ID:   "call_" + part.FunctionCall.Name,
					Type: "function",
					Function: OpenAIFunctionCall{
						Name:      part.FunctionCall.Name,
						Arguments: string(argsJSON),
					},
				})
			}
			if part.FunctionResponse != nil {
				respJSON, _ := json.Marshal(part.FunctionResponse.Response)
				openaiReq.Messages = append(openaiReq.Messages, OpenAIMessage{
					Role:       "tool",
					Content:    string(respJSON),
					ToolCallID: part.FunctionResponse.Name,
				})
				continue
			}
		}

		if textContent != "" {
			openaiMsg.Content = textContent
		}
		if len(toolCalls) > 0 {
			openaiMsg.ToolCalls = toolCalls
		}

		if openaiMsg.Content != nil || len(openaiMsg.ToolCalls) > 0 {
			openaiReq.Messages = append(openaiReq.Messages, openaiMsg)
		}
	}

	// Convert tools
	for _, tool := range req.Tools {
		for _, decl := range tool.FunctionDeclarations {
			openaiReq.Tools = append(openaiReq.Tools, OpenAITool{
				Type:     "function",
				Function: OpenAIFunction(decl),
			})
		}
	}

	return json.Marshal(openaiReq)
}

func (c *geminiToOpenAIResponse) Transform(body []byte) ([]byte, error) {
	var resp GeminiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}

	openaiResp := OpenAIResponse{
		ID:      "chatcmpl-gemini",
		Object:  "chat.completion",
		Created: time.Now().Unix(),
	}

	if resp.UsageMetadata != nil {
		openaiResp.Usage = OpenAIUsage{
			PromptTokens:     resp.UsageMetadata.PromptTokenCount,
			CompletionTokens: resp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      resp.UsageMetadata.TotalTokenCount,
		}
	}

	msg := OpenAIMessage{Role: "assistant"}
	var textContent string
	var toolCalls []OpenAIToolCall
	finishReason := "stop"

	if len(resp.Candidates) > 0 {
		candidate := resp.Candidates[0]
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				textContent += part.Text
			}
			if part.FunctionCall != nil {
				argsJSON, _ := json.Marshal(part.FunctionCall.Args)
				toolCalls = append(toolCalls, OpenAIToolCall{
					ID:   "call_" + part.FunctionCall.Name,
					Type: "function",
					Function: OpenAIFunctionCall{
						Name:      part.FunctionCall.Name,
						Arguments: string(argsJSON),
					},
				})
			}
		}

		switch candidate.FinishReason {
		case "STOP":
			if len(toolCalls) > 0 {
				finishReason = "tool_calls"
			} else {
				finishReason = "stop"
			}
		case "MAX_TOKENS":
			finishReason = "length"
		}
	}

	if textContent != "" {
		msg.Content = textContent
	}
	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	}

	openaiResp.Choices = []OpenAIChoice{{
		Index:        0,
		Message:      &msg,
		FinishReason: finishReason,
	}}

	return json.Marshal(openaiResp)
}

func (c *geminiToOpenAIResponse) TransformChunk(chunk []byte, state *TransformState) ([]byte, error) {
	events, remaining := ParseSSE(state.Buffer + string(chunk))
	state.Buffer = remaining

	var output []byte
	for _, event := range events {
		var geminiChunk GeminiStreamChunk
		if err := json.Unmarshal(event.Data, &geminiChunk); err != nil {
			continue
		}

		// First chunk
		if state.MessageID == "" {
			state.MessageID = "chatcmpl-gemini"
			openaiChunk := OpenAIStreamChunk{
				ID:      state.MessageID,
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Choices: []OpenAIChoice{{
					Index: 0,
					Delta: &OpenAIMessage{Role: "assistant", Content: ""},
				}},
			}
			output = append(output, FormatSSE("", openaiChunk)...)
		}

		if len(geminiChunk.Candidates) > 0 {
			candidate := geminiChunk.Candidates[0]
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					openaiChunk := OpenAIStreamChunk{
						ID:      state.MessageID,
						Object:  "chat.completion.chunk",
						Created: time.Now().Unix(),
						Choices: []OpenAIChoice{{
							Index: 0,
							Delta: &OpenAIMessage{Content: part.Text},
						}},
					}
					output = append(output, FormatSSE("", openaiChunk)...)
				}
			}

			if candidate.FinishReason != "" {
				finishReason := "stop"
				if candidate.FinishReason == "MAX_TOKENS" {
					finishReason = "length"
				}
				openaiChunk := OpenAIStreamChunk{
					ID:      state.MessageID,
					Object:  "chat.completion.chunk",
					Created: time.Now().Unix(),
					Choices: []OpenAIChoice{{
						Index:        0,
						Delta:        &OpenAIMessage{},
						FinishReason: finishReason,
					}},
				}
				output = append(output, FormatSSE("", openaiChunk)...)
				output = append(output, FormatDone()...)
			}
		}
	}

	return output, nil
}
