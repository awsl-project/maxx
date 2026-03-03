package converter

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/awsl-project/maxx/internal/domain"
)

const geminiFunctionThoughtSignature = "skip_thought_signature_validator"
const geminiSystemOnlyPlaceholderText = " "

func init() {
	RegisterConverter(domain.ClientTypeOpenAI, domain.ClientTypeGemini, &openaiToGeminiRequest{}, &openaiToGeminiResponse{})
}

type openaiToGeminiRequest struct{}

func (c *openaiToGeminiRequest) Transform(body []byte, model string, stream bool) ([]byte, error) {
	var req OpenAIRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	geminiReq := GeminiRequest{
		GenerationConfig: &GeminiGenerationConfig{
			MaxOutputTokens: req.MaxTokens,
			Temperature:     req.Temperature,
			TopP:            req.TopP,
		},
	}

	if req.MaxCompletionTokens > 0 && req.MaxTokens == 0 {
		geminiReq.GenerationConfig.MaxOutputTokens = req.MaxCompletionTokens
	}
	if req.N > 1 {
		geminiReq.GenerationConfig.CandidateCount = req.N
	}

	switch stop := req.Stop.(type) {
	case string:
		geminiReq.GenerationConfig.StopSequences = []string{stop}
	case []interface{}:
		for _, s := range stop {
			if str, ok := s.(string); ok {
				geminiReq.GenerationConfig.StopSequences = append(geminiReq.GenerationConfig.StopSequences, str)
			}
		}
	}

	if len(req.Modalities) > 0 {
		var mods []string
		for _, m := range req.Modalities {
			switch strings.ToLower(strings.TrimSpace(m)) {
			case "text":
				mods = append(mods, "TEXT")
			case "image":
				mods = append(mods, "IMAGE")
			}
		}
		if len(mods) > 0 {
			geminiReq.GenerationConfig.ResponseModalities = mods
		}
	}

	if req.ImageConfig != nil {
		geminiReq.GenerationConfig.ImageConfig = &GeminiImageConfig{
			AspectRatio: req.ImageConfig.AspectRatio,
			ImageSize:   req.ImageConfig.ImageSize,
		}
	}

	if req.ReasoningEffort != "" {
		effort := strings.ToLower(strings.TrimSpace(req.ReasoningEffort))
		if geminiReq.GenerationConfig.ThinkingConfig == nil {
			geminiReq.GenerationConfig.ThinkingConfig = &GeminiThinkingConfig{}
		}
		if effort == "auto" {
			geminiReq.GenerationConfig.ThinkingConfig.ThinkingBudget = -1
			geminiReq.GenerationConfig.ThinkingConfig.IncludeThoughts = true
		} else if effort == "none" {
			geminiReq.GenerationConfig.ThinkingConfig.IncludeThoughts = false
			// ThinkingLevel left empty, omitempty will exclude it
		} else {
			geminiReq.GenerationConfig.ThinkingConfig.ThinkingLevel = effort
			geminiReq.GenerationConfig.ThinkingConfig.IncludeThoughts = true
		}
	}

	toolCallNameByID := map[string]string{}
	toolResponses := map[string]string{}
	hasToolText := false
	for _, msg := range req.Messages {
		if msg.Role == "assistant" {
			for _, tc := range msg.ToolCalls {
				if tc.ID != "" && tc.Function.Name != "" {
					toolCallNameByID[tc.ID] = tc.Function.Name
				}
			}
		}
	}
	for _, msg := range req.Messages {
		if msg.Role != "tool" || msg.ToolCallID == "" {
			continue
		}
		contentStr := stringifyContent(msg.Content)
		if strings.TrimSpace(contentStr) != "" {
			hasToolText = true
		}
		toolResponses[msg.ToolCallID] = contentStr
	}

	var systemParts []GeminiPart
	for _, msg := range req.Messages {
		if msg.Role == "system" || msg.Role == "developer" {
			switch content := msg.Content.(type) {
			case string:
				if content != "" {
					systemParts = append(systemParts, GeminiPart{Text: content})
				}
			case []interface{}:
				for _, part := range content {
					if m, ok := part.(map[string]interface{}); ok {
						if text, ok := m["text"].(string); ok && text != "" {
							systemParts = append(systemParts, GeminiPart{Text: text})
						}
					}
				}
			case map[string]interface{}:
				if typ, _ := content["type"].(string); typ == "text" {
					if text, ok := content["text"].(string); ok && text != "" {
						systemParts = append(systemParts, GeminiPart{Text: text})
					}
				}
			}
			continue
		}
		if msg.Role == "tool" {
			continue
		}

		geminiContent := GeminiContent{}
		switch msg.Role {
		case "user":
			geminiContent.Role = "user"
		case "assistant":
			geminiContent.Role = "model"
		}

		switch content := msg.Content.(type) {
		case string:
			geminiContent.Parts = []GeminiPart{{Text: content}}
		case []interface{}:
			for _, part := range content {
				if m, ok := part.(map[string]interface{}); ok {
					if m["type"] == "text" {
						if text, ok := m["text"].(string); ok {
							geminiContent.Parts = append(geminiContent.Parts, GeminiPart{Text: text})
						}
					}
					if m["type"] == "image_url" {
						if urlObj, ok := m["image_url"].(map[string]interface{}); ok {
							if url, ok := urlObj["url"].(string); ok {
								if inline := parseInlineImage(url); inline != nil {
									geminiContent.Parts = append(geminiContent.Parts, GeminiPart{
										InlineData:       inline,
										ThoughtSignature: geminiFunctionThoughtSignature,
									})
								}
							}
						}
					}
					if m["type"] == "file" {
						if inline := parseFilePart(m); inline != nil {
							geminiContent.Parts = append(geminiContent.Parts, GeminiPart{
								InlineData:       inline,
								ThoughtSignature: geminiFunctionThoughtSignature,
							})
						}
					}
				}
			}
		}

		for _, tc := range msg.ToolCalls {
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				return nil, err
			}
			geminiContent.Parts = append(geminiContent.Parts, GeminiPart{
				FunctionCall: &GeminiFunctionCall{
					Name: tc.Function.Name,
					Args: args,
				},
				ThoughtSignature: geminiFunctionThoughtSignature,
			})
		}

		geminiReq.Contents = append(geminiReq.Contents, geminiContent)
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			var toolParts []GeminiPart
			for _, tc := range msg.ToolCalls {
				if tc.ID == "" {
					continue
				}
				name := tc.Function.Name
				if name == "" {
					name = toolCallNameByID[tc.ID]
				}
				if name == "" {
					continue
				}
				resp := toolResponses[tc.ID]
				if resp == "" {
					resp = "{}"
				}
				toolParts = append(toolParts, GeminiPart{
					FunctionResponse: &GeminiFunctionResponse{
						Name:     name,
						Response: map[string]string{"result": resp},
					},
				})
			}
			if len(toolParts) > 0 {
				geminiReq.Contents = append(geminiReq.Contents, GeminiContent{
					Role:  "user",
					Parts: toolParts,
				})
			}
		}
	}
	if len(geminiReq.Contents) == 0 {
		if len(systemParts) > 0 || hasToolText {
			geminiReq.Contents = append(geminiReq.Contents, GeminiContent{
				Role:  "user",
				Parts: []GeminiPart{{Text: geminiSystemOnlyPlaceholderText}},
			})
		} else {
			return nil, fmt.Errorf("no user/system content available")
		}
	}
	if len(systemParts) > 0 {
		geminiReq.SystemInstruction = &GeminiContent{
			Role:  "user",
			Parts: systemParts,
		}
	}

	if len(req.Tools) > 0 {
		var funcDecls []GeminiFunctionDecl
		for _, tool := range req.Tools {
			params := tool.Function.Parameters
			if params == nil {
				params = map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				}
			}
			funcDecls = append(funcDecls, GeminiFunctionDecl{
				Name:                 tool.Function.Name,
				Description:          tool.Function.Description,
				ParametersJsonSchema: params,
			})
		}
		geminiReq.Tools = []GeminiTool{{FunctionDeclarations: funcDecls}}
	}

	if tc := parseToolChoice(req.ToolChoice); tc != nil {
		geminiReq.ToolConfig = tc
	}

	return json.Marshal(geminiReq)
}
