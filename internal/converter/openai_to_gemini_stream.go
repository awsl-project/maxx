package converter

import (
	"encoding/json"
	"strings"
)

func (c *openaiToGeminiResponse) TransformChunk(chunk []byte, state *TransformState) ([]byte, error) {
	events, remaining := ParseSSE(state.Buffer + string(chunk))
	state.Buffer = remaining

	var output []byte
	for _, event := range events {
		if event.Event == "done" {
			continue
		}

		var openaiChunk OpenAIStreamChunk
		if err := json.Unmarshal(event.Data, &openaiChunk); err != nil {
			continue
		}

		if len(openaiChunk.Choices) > 0 {
			choice := openaiChunk.Choices[0]
			if choice.Delta != nil {
				if reasoningText := collectReasoningText(choice.Delta.ReasoningContent); strings.TrimSpace(reasoningText) != "" {
					geminiChunk := GeminiStreamChunk{
						Candidates: []GeminiCandidate{{
							Content: GeminiContent{
								Role: "model",
								Parts: []GeminiPart{{
									Text:    reasoningText,
									Thought: true,
								}},
							},
							Index: 0,
						}},
					}
					output = append(output, FormatSSE("", geminiChunk)...)
				}
				if content, ok := choice.Delta.Content.(string); ok && content != "" {
					geminiChunk := GeminiStreamChunk{
						Candidates: []GeminiCandidate{{
							Content: GeminiContent{
								Role:  "model",
								Parts: []GeminiPart{{Text: content}},
							},
							Index: 0,
						}},
					}
					output = append(output, FormatSSE("", geminiChunk)...)
				}

				// Generated images arrive in the delta's non-standard images[]
				// array → emit them as Gemini inlineData parts.
				for _, img := range choice.Delta.Images {
					if img.ImageURL == nil {
						continue
					}
					if inline := parseInlineImage(img.ImageURL.URL); inline != nil {
						geminiChunk := GeminiStreamChunk{
							Candidates: []GeminiCandidate{{
								Content: GeminiContent{
									Role:  "model",
									Parts: []GeminiPart{{InlineData: inline}},
								},
								Index: 0,
							}},
						}
						output = append(output, FormatSSE("", geminiChunk)...)
					}
				}

				if len(choice.Delta.ToolCalls) > 0 {
					if state.ToolCalls == nil {
						state.ToolCalls = make(map[int]*ToolCallState)
					}
					for _, tc := range choice.Delta.ToolCalls {
						toolIndex := tc.Index
						callState, ok := state.ToolCalls[toolIndex]
						if !ok {
							callState = &ToolCallState{ID: tc.ID, Name: tc.Function.Name}
							state.ToolCalls[toolIndex] = callState
						}
						if tc.ID != "" {
							callState.ID = tc.ID
						}
						if tc.Function.Name != "" {
							callState.Name = tc.Function.Name
						}
						if tc.Function.Arguments != "" {
							callState.Arguments += tc.Function.Arguments
						}
					}
				}
			}

			if choice.FinishReason != "" {
				finishReason := "STOP"
				if choice.FinishReason == "length" {
					finishReason = "MAX_TOKENS"
				}
				geminiChunk := GeminiStreamChunk{
					Candidates: []GeminiCandidate{{
						FinishReason: finishReason,
						Index:        0,
					}},
				}
				if len(state.ToolCalls) > 0 {
					var parts []GeminiPart
					for _, tc := range state.ToolCalls {
						var args map[string]interface{}
						_ = json.Unmarshal([]byte(tc.Arguments), &args)
						parts = append(parts, GeminiPart{
							FunctionCall: &GeminiFunctionCall{
								Name: tc.Name,
								Args: args,
							},
						})
					}
					if len(parts) > 0 {
						geminiChunk.Candidates[0].Content = GeminiContent{
							Role:  "model",
							Parts: parts,
						}
					}
					state.ToolCalls = nil
				}
				output = append(output, FormatSSE("", geminiChunk)...)
			}
		}
	}

	return output, nil
}
