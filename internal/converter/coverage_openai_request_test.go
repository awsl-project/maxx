package converter

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOpenAIToGeminiRequestToolChoice(t *testing.T) {
	req := OpenAIRequest{
		Model:      "gpt",
		ToolChoice: "required",
		Tools: []OpenAITool{{
			Type: "function",
			Function: OpenAIFunction{
				Name: "tool",
			},
		}},
		Messages: []OpenAIMessage{{Role: "user", Content: "hi"}},
	}
	body, _ := json.Marshal(req)
	conv := &openaiToGeminiRequest{}
	out, err := conv.Transform(body, "gemini", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var geminiReq GeminiRequest
	if err := json.Unmarshal(out, &geminiReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if geminiReq.ToolConfig == nil || geminiReq.ToolConfig.FunctionCallingConfig == nil {
		t.Fatalf("tool config missing")
	}
}

func TestOpenAIToGeminiRequestBranches(t *testing.T) {
	req := OpenAIRequest{
		Model:           "gpt",
		Modalities:      []string{"text", "image"},
		MaxTokens:       3,
		ReasoningEffort: "none",
		Stop:            "stop",
		ImageConfig:     &OpenAIImageConfig{AspectRatio: "1:1", ImageSize: "512x512"},
		ToolChoice: map[string]interface{}{
			"type":     "function",
			"function": map[string]interface{}{"name": "tool"},
		},
		Messages: []OpenAIMessage{{
			Role: "user",
			Content: []interface{}{map[string]interface{}{
				"type": "text",
				"text": "hi",
			}, map[string]interface{}{
				"type":      "image_url",
				"image_url": map[string]interface{}{"url": "data:image/png;base64,Zm9v"},
			}, map[string]interface{}{
				"type": "file",
				"file": map[string]interface{}{"filename": "a.txt", "file_data": "Zg=="},
			}},
		}},
		Tools: []OpenAITool{{Type: "function", Function: OpenAIFunction{Name: "tool"}}},
	}
	body, _ := json.Marshal(req)
	conv := &openaiToGeminiRequest{}
	out, err := conv.Transform(body, "gemini", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var geminiReq GeminiRequest
	if err := json.Unmarshal(out, &geminiReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if geminiReq.GenerationConfig == nil || geminiReq.GenerationConfig.ImageConfig == nil {
		t.Fatalf("generation config missing")
	}
	if geminiReq.ToolConfig == nil || geminiReq.ToolConfig.FunctionCallingConfig == nil {
		t.Fatalf("tool config missing")
	}
}

func TestOpenAIToClaudeRequestArrayContent(t *testing.T) {
	req := OpenAIRequest{Messages: []OpenAIMessage{{
		Role:    "user",
		Content: []interface{}{map[string]interface{}{"type": "text", "text": "hi"}},
	}}}
	body, _ := json.Marshal(req)
	conv := &openaiToClaudeRequest{}
	out, err := conv.Transform(body, "claude", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var claudeReq ClaudeRequest
	if err := json.Unmarshal(out, &claudeReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(claudeReq.Messages) == 0 {
		t.Fatalf("messages missing")
	}
}

func TestOpenAIToGeminiRequestToolRoleMapping(t *testing.T) {
	req := OpenAIRequest{
		Messages: []OpenAIMessage{{
			Role: "assistant",
			ToolCalls: []OpenAIToolCall{{
				ID:       "call_1",
				Type:     "function",
				Function: OpenAIFunctionCall{Name: "tool", Arguments: `{"x":1}`},
			}},
		}, {
			Role:       "tool",
			ToolCallID: "call_1",
			Content:    "ok",
		}},
		Stop: []interface{}{"s1", "s2"},
	}
	body, _ := json.Marshal(req)
	conv := &openaiToGeminiRequest{}
	out, err := conv.Transform(body, "gemini", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var geminiReq GeminiRequest
	if err := json.Unmarshal(out, &geminiReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if geminiReq.SystemInstruction != nil {
		// no system messages in this test
	}
	found := false
	for _, content := range geminiReq.Contents {
		for _, part := range content.Parts {
			if part.FunctionResponse != nil && part.FunctionResponse.Name == "tool" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("expected function response name mapping")
	}
	if geminiReq.GenerationConfig == nil || len(geminiReq.GenerationConfig.StopSequences) != 2 {
		t.Fatalf("stop sequences missing")
	}
}

func TestOpenAIToGeminiRequestSystemDeveloper(t *testing.T) {
	req := OpenAIRequest{Messages: []OpenAIMessage{{Role: "system", Content: "sys"}, {Role: "developer", Content: "dev"}, {Role: "user", Content: "hi"}}}
	body, _ := json.Marshal(req)
	conv := &openaiToGeminiRequest{}
	out, err := conv.Transform(body, "gemini", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var geminiReq GeminiRequest
	if err := json.Unmarshal(out, &geminiReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if geminiReq.SystemInstruction == nil {
		t.Fatalf("systemInstruction missing")
	}
}

func TestOpenAIToGeminiRequestSystemArrayParts(t *testing.T) {
	req := OpenAIRequest{Messages: []OpenAIMessage{{Role: "system", Content: []interface{}{
		map[string]interface{}{"type": "text", "text": "sys1"},
		map[string]interface{}{"type": "text", "text": "sys2"},
	}}, {Role: "user", Content: "hi"}}}
	body, _ := json.Marshal(req)
	conv := &openaiToGeminiRequest{}
	out, err := conv.Transform(body, "gemini", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var geminiReq GeminiRequest
	if err := json.Unmarshal(out, &geminiReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if geminiReq.SystemInstruction == nil || len(geminiReq.SystemInstruction.Parts) != 2 {
		t.Fatalf("expected system parts")
	}
}

func TestOpenAIToGeminiRequestSystemMapPart(t *testing.T) {
	req := OpenAIRequest{Messages: []OpenAIMessage{{Role: "system", Content: map[string]interface{}{"type": "text", "text": "sys"}}, {Role: "user", Content: "hi"}}}
	body, _ := json.Marshal(req)
	conv := &openaiToGeminiRequest{}
	out, err := conv.Transform(body, "gemini", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var geminiReq GeminiRequest
	if err := json.Unmarshal(out, &geminiReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if geminiReq.SystemInstruction == nil || len(geminiReq.SystemInstruction.Parts) != 1 {
		t.Fatalf("expected system part")
	}
}

func TestOpenAIToGeminiRequestSystemOnlyAsUser(t *testing.T) {
	req := OpenAIRequest{Messages: []OpenAIMessage{{Role: "system", Content: "sys"}}}
	body, _ := json.Marshal(req)
	conv := &openaiToGeminiRequest{}
	out, err := conv.Transform(body, "gemini", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var geminiReq GeminiRequest
	if err := json.Unmarshal(out, &geminiReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if geminiReq.SystemInstruction == nil || len(geminiReq.SystemInstruction.Parts) != 1 {
		t.Fatalf("expected systemInstruction")
	}
	if len(geminiReq.Contents) != 1 || geminiReq.Contents[0].Role != "user" {
		t.Fatalf("expected user content")
	}
	if len(geminiReq.Contents[0].Parts) != 1 || geminiReq.Contents[0].Parts[0].Text != geminiSystemOnlyPlaceholderText {
		t.Fatalf("expected space user part")
	}
}

func TestOpenAIToGeminiRequestToolParametersSchema(t *testing.T) {
	req := OpenAIRequest{
		Messages: []OpenAIMessage{{Role: "user", Content: "hi"}},
		Tools: []OpenAITool{{
			Type: "function",
			Function: OpenAIFunction{
				Name:        "tool",
				Description: "d",
				Parameters:  map[string]interface{}{"type": "object"},
			},
		}},
	}
	body, _ := json.Marshal(req)
	conv := &openaiToGeminiRequest{}
	out, err := conv.Transform(body, "gemini", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var geminiReq GeminiRequest
	if err := json.Unmarshal(out, &geminiReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(geminiReq.Tools) == 0 || len(geminiReq.Tools[0].FunctionDeclarations) == 0 {
		t.Fatalf("expected tool declarations")
	}
	decl := geminiReq.Tools[0].FunctionDeclarations[0]
	if decl.ParametersJsonSchema == nil {
		t.Fatalf("expected parametersJsonSchema")
	}
	if decl.Parameters != nil {
		t.Fatalf("expected parameters empty")
	}
}

func TestOpenAIToGeminiRequestToolDefaultSchema(t *testing.T) {
	req := OpenAIRequest{
		Messages: []OpenAIMessage{{Role: "user", Content: "hi"}},
		Tools: []OpenAITool{{
			Type: "function",
			Function: OpenAIFunction{
				Name: "tool",
			},
		}},
	}
	body, _ := json.Marshal(req)
	conv := &openaiToGeminiRequest{}
	out, err := conv.Transform(body, "gemini", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var geminiReq GeminiRequest
	if err := json.Unmarshal(out, &geminiReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(geminiReq.Tools) == 0 || len(geminiReq.Tools[0].FunctionDeclarations) == 0 {
		t.Fatalf("expected tool declarations")
	}
	decl := geminiReq.Tools[0].FunctionDeclarations[0]
	params, ok := decl.ParametersJsonSchema.(map[string]interface{})
	if !ok {
		t.Fatalf("expected schema map")
	}
	if params["type"] != "object" {
		t.Fatalf("expected object schema")
	}
	if _, ok := params["properties"]; !ok {
		t.Fatalf("expected properties")
	}
}

func TestOpenAIToGeminiRequestToolResponseEmpty(t *testing.T) {
	req := OpenAIRequest{
		Messages: []OpenAIMessage{{
			Role: "assistant",
			ToolCalls: []OpenAIToolCall{{
				ID:       "call_1",
				Type:     "function",
				Function: OpenAIFunctionCall{Name: "tool", Arguments: `{"x":1}`},
			}},
		}, {
			Role:       "tool",
			ToolCallID: "call_1",
			Content:    "",
		}},
	}
	body, _ := json.Marshal(req)
	conv := &openaiToGeminiRequest{}
	out, err := conv.Transform(body, "gemini", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var geminiReq GeminiRequest
	if err := json.Unmarshal(out, &geminiReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	found := false
	for _, content := range geminiReq.Contents {
		for _, part := range content.Parts {
			if part.FunctionResponse != nil && part.FunctionResponse.Name == "tool" {
				if resp, ok := part.FunctionResponse.Response.(map[string]interface{}); ok {
					if result, ok := resp["result"].(string); ok && result == "{}" {
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Fatalf("expected empty tool response to default")
	}
}

func TestOpenAIToGeminiRequestToolCallEmptyID(t *testing.T) {
	req := OpenAIRequest{
		Messages: []OpenAIMessage{{
			Role: "assistant",
			ToolCalls: []OpenAIToolCall{{
				ID:       "",
				Type:     "function",
				Function: OpenAIFunctionCall{Name: "tool", Arguments: `{"x":1}`},
			}},
		}},
	}
	body, _ := json.Marshal(req)
	conv := &openaiToGeminiRequest{}
	if _, err := conv.Transform(body, "gemini", false); err != nil {
		t.Fatalf("Transform: %v", err)
	}
}

func TestOpenAIToGeminiRequestToolCallEmptyName(t *testing.T) {
	req := OpenAIRequest{
		Messages: []OpenAIMessage{{
			Role: "assistant",
			ToolCalls: []OpenAIToolCall{{
				ID:       "call_1",
				Type:     "function",
				Function: OpenAIFunctionCall{Name: "", Arguments: `{"x":1}`},
			}},
		}, {
			Role:       "tool",
			ToolCallID: "call_1",
			Content:    "ok",
		}},
	}
	body, _ := json.Marshal(req)
	conv := &openaiToGeminiRequest{}
	out, err := conv.Transform(body, "gemini", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var geminiReq GeminiRequest
	if err := json.Unmarshal(out, &geminiReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, content := range geminiReq.Contents {
		for _, part := range content.Parts {
			if part.FunctionResponse != nil && part.FunctionResponse.Name == "" {
				t.Fatalf("unexpected function response for empty name")
			}
		}
	}
}

func TestClaudeToOpenAIRequestRedactedThinking(t *testing.T) {
	req := ClaudeRequest{Messages: []ClaudeMessage{{
		Role:    "assistant",
		Content: []interface{}{map[string]interface{}{"type": "redacted_thinking", "text": "x"}, map[string]interface{}{"type": "text", "text": "ok"}},
	}}}
	body, _ := json.Marshal(req)
	conv := &claudeToOpenAIRequest{}
	out, err := conv.Transform(body, "gpt", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var openaiReq OpenAIRequest
	if err := json.Unmarshal(out, &openaiReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(openaiReq.Messages) == 0 {
		t.Fatalf("messages missing")
	}
}

func TestCodexToOpenAIRequestFunctionOutput(t *testing.T) {
	req := CodexRequest{Input: []interface{}{
		map[string]interface{}{"type": "function_call", "name": "tool", "call_id": "call_1", "arguments": "{}"},
		map[string]interface{}{"type": "function_call_output", "call_id": "call_1", "output": "ok"},
	}}
	body, _ := json.Marshal(req)
	conv := &codexToOpenAIRequest{}
	out, err := conv.Transform(body, "gpt", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var openaiReq OpenAIRequest
	if err := json.Unmarshal(out, &openaiReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(openaiReq.Messages) < 2 {
		t.Fatalf("messages missing")
	}
}

func TestClaudeToOpenAIRequestSystemArrayAndToolResultArray(t *testing.T) {
	req := ClaudeRequest{
		System: []interface{}{map[string]interface{}{"text": "sys"}},
		Messages: []ClaudeMessage{{
			Role: "assistant",
			Content: []interface{}{map[string]interface{}{ // tool_result with array content
				"type":        "tool_result",
				"tool_use_id": "call_1",
				"content":     []interface{}{map[string]interface{}{"text": "a"}, map[string]interface{}{"text": "b"}},
			}},
		}},
	}
	body, _ := json.Marshal(req)
	conv := &claudeToOpenAIRequest{}
	out, err := conv.Transform(body, "gpt", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var openaiReq OpenAIRequest
	if err := json.Unmarshal(out, &openaiReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(openaiReq.Messages) == 0 {
		t.Fatalf("messages missing")
	}
}

func TestOpenAIToGeminiRequestMoreBranches(t *testing.T) {
	req := OpenAIRequest{
		Model:           "gpt",
		Stop:            []interface{}{"s1"},
		Modalities:      []string{"text"},
		ReasoningEffort: "auto",
		ImageConfig:     &OpenAIImageConfig{AspectRatio: "1:1"},
		ToolChoice:      "none",
		Messages:        []OpenAIMessage{{Role: "system", Content: "sys"}, {Role: "user", Content: "hi"}},
	}
	body, _ := json.Marshal(req)
	conv := &openaiToGeminiRequest{}
	out, err := conv.Transform(body, "gemini", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var geminiReq GeminiRequest
	if err := json.Unmarshal(out, &geminiReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if geminiReq.GenerationConfig == nil || geminiReq.GenerationConfig.ThinkingConfig == nil {
		t.Fatalf("thinking config missing")
	}
}

func TestOpenAIToGeminiRequestCandidateCount(t *testing.T) {
	req := OpenAIRequest{N: 2, Messages: []OpenAIMessage{{Role: "user", Content: "hi"}}}
	body, _ := json.Marshal(req)
	conv := &openaiToGeminiRequest{}
	out, err := conv.Transform(body, "gemini", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var geminiReq GeminiRequest
	if err := json.Unmarshal(out, &geminiReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if geminiReq.GenerationConfig.CandidateCount != 2 {
		t.Fatalf("expected candidate count")
	}
}

func TestOpenAIToGeminiRequestThoughtSignatureParts(t *testing.T) {
	req := OpenAIRequest{Messages: []OpenAIMessage{{
		Role: "user",
		Content: []interface{}{
			map[string]interface{}{"type": "image_url", "image_url": map[string]interface{}{"url": "data:image/png;base64,Zm9v"}},
		},
	}, {
		Role: "assistant",
		ToolCalls: []OpenAIToolCall{{
			ID:   "call_1",
			Type: "function",
			Function: OpenAIFunctionCall{
				Name:      "tool",
				Arguments: "{}",
			},
		}},
	}}}
	body, _ := json.Marshal(req)
	conv := &openaiToGeminiRequest{}
	out, err := conv.Transform(body, "gemini", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var geminiReq GeminiRequest
	if err := json.Unmarshal(out, &geminiReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	foundSignature := false
	for _, content := range geminiReq.Contents {
		for _, part := range content.Parts {
			if part.ThoughtSignature == geminiFunctionThoughtSignature {
				foundSignature = true
			}
		}
	}
	if !foundSignature {
		t.Fatalf("expected thought signature")
	}
}

func TestCodexToOpenAIRequestMessageDefaultRole(t *testing.T) {
	req := CodexRequest{Input: []interface{}{map[string]interface{}{"type": "message", "content": "hi"}}}
	body, _ := json.Marshal(req)
	conv := &codexToOpenAIRequest{}
	out, err := conv.Transform(body, "gpt", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var openaiReq OpenAIRequest
	if err := json.Unmarshal(out, &openaiReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(openaiReq.Messages) == 0 || openaiReq.Messages[0].Role != "user" {
		t.Fatalf("expected default role user")
	}
}

func TestOpenAIToClaudeRequestSystemArray(t *testing.T) {
	req := OpenAIRequest{Messages: []OpenAIMessage{{Role: "system", Content: []interface{}{map[string]interface{}{"text": "sys"}}}}}
	body, _ := json.Marshal(req)
	conv := &openaiToClaudeRequest{}
	out, err := conv.Transform(body, "claude", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "sys") {
		t.Fatalf("expected system text")
	}
}

func TestGeminiToOpenAIRequestInlineAndTextParts2(t *testing.T) {
	req := GeminiRequest{Contents: []GeminiContent{{
		Role:  "user",
		Parts: []GeminiPart{{Text: "hi"}, {InlineData: &GeminiInlineData{MimeType: "image/png", Data: "Zm9v"}}},
	}}}
	body, _ := json.Marshal(req)
	conv := &geminiToOpenAIRequest{}
	out, err := conv.Transform(body, "gpt", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "image_url") {
		t.Fatalf("expected image_url")
	}
}

func TestGeminiToOpenAIRequestThinkingBudget(t *testing.T) {
	req := GeminiRequest{
		GenerationConfig:  &GeminiGenerationConfig{ThinkingConfig: &GeminiThinkingConfig{ThinkingBudget: 0}, StopSequences: []string{"s"}},
		SystemInstruction: &GeminiContent{Parts: []GeminiPart{{Text: "sys"}}},
		Contents:          []GeminiContent{{Role: "user", Parts: []GeminiPart{{Text: "hi"}}}},
	}
	body, _ := json.Marshal(req)
	conv := &geminiToOpenAIRequest{}
	out, err := conv.Transform(body, "gpt", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "reasoning_effort") {
		t.Fatalf("expected reasoning_effort")
	}
	if !strings.Contains(string(out), "stop") {
		t.Fatalf("expected stop sequences")
	}
}

func TestGeminiToOpenAIRequestUnknownRoleAndToolCallID(t *testing.T) {
	req := GeminiRequest{Contents: []GeminiContent{{
		Role:  "unknown",
		Parts: []GeminiPart{{FunctionCall: &GeminiFunctionCall{Name: "tool", Args: map[string]interface{}{"x": 1}}}},
	}}}
	body, _ := json.Marshal(req)
	conv := &geminiToOpenAIRequest{}
	out, err := conv.Transform(body, "gpt", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "call_tool") {
		t.Fatalf("expected call_tool id")
	}
}

func TestCodexToOpenAIRequestToolOutput(t *testing.T) {
	req := CodexRequest{Input: []interface{}{map[string]interface{}{"type": "function_call_output", "call_id": "call_1", "output": "ok"}}}
	body, _ := json.Marshal(req)
	conv := &codexToOpenAIRequest{}
	out, err := conv.Transform(body, "gpt", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "\"role\":\"tool\"") {
		t.Fatalf("expected tool role")
	}
}

func TestGeminiToOpenAIRequestToolCallIDAndContentParts(t *testing.T) {
	req := GeminiRequest{Contents: []GeminiContent{{
		Role:  "model",
		Parts: []GeminiPart{{Text: "hello"}, {FunctionCall: &GeminiFunctionCall{ID: "call_1", Name: "tool", Args: map[string]interface{}{"x": 1}}}},
	}}}
	body, _ := json.Marshal(req)
	conv := &geminiToOpenAIRequest{}
	out, err := conv.Transform(body, "gpt", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "tool_calls") {
		t.Fatalf("expected tool_calls")
	}
	if !strings.Contains(string(out), "call_1") {
		t.Fatalf("expected tool_call id")
	}
}

func TestGeminiToOpenAIRequestStopAndThinkingLevel(t *testing.T) {
	req := GeminiRequest{GenerationConfig: &GeminiGenerationConfig{StopSequences: []string{"s"}, ThinkingConfig: &GeminiThinkingConfig{ThinkingLevel: "low"}}, Contents: []GeminiContent{{Role: "user", Parts: []GeminiPart{{Text: "hi"}}}}}
	body, _ := json.Marshal(req)
	conv := &geminiToOpenAIRequest{}
	out, err := conv.Transform(body, "gpt", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "reasoning_effort") {
		t.Fatalf("expected reasoning_effort")
	}
	if !strings.Contains(string(out), "stop") {
		t.Fatalf("expected stop")
	}
}

func TestGeminiToOpenAIRequestInlineOnly(t *testing.T) {
	req := GeminiRequest{Contents: []GeminiContent{{
		Role:  "user",
		Parts: []GeminiPart{{InlineData: &GeminiInlineData{MimeType: "image/png", Data: "Zm9v"}}},
	}}}
	body, _ := json.Marshal(req)
	conv := &geminiToOpenAIRequest{}
	out, err := conv.Transform(body, "gpt", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "image_url") {
		t.Fatalf("expected image_url")
	}
}

func TestGeminiToOpenAIRequestToolCallNoID(t *testing.T) {
	req := GeminiRequest{Contents: []GeminiContent{{
		Role:  "model",
		Parts: []GeminiPart{{FunctionCall: &GeminiFunctionCall{Name: "tool", Args: map[string]interface{}{"x": 1}}}},
	}}}
	body, _ := json.Marshal(req)
	conv := &geminiToOpenAIRequest{}
	out, err := conv.Transform(body, "gpt", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "call_tool") {
		t.Fatalf("expected call_tool id")
	}
}

func TestCodexToOpenAIRequestFunctionCallID(t *testing.T) {
	req := CodexRequest{Input: []interface{}{map[string]interface{}{"type": "function_call", "id": "id_1", "name": "tool", "arguments": "{}"}}}
	body, _ := json.Marshal(req)
	conv := &codexToOpenAIRequest{}
	out, err := conv.Transform(body, "gpt", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "id_1") {
		t.Fatalf("expected id_1")
	}
}

func TestGeminiToOpenAIRequestNoReasoningEffort(t *testing.T) {
	req := GeminiRequest{GenerationConfig: &GeminiGenerationConfig{ThinkingConfig: &GeminiThinkingConfig{ThinkingBudget: -2}}, Contents: []GeminiContent{{Role: "user", Parts: []GeminiPart{{Text: "hi"}}}}}
	body, _ := json.Marshal(req)
	conv := &geminiToOpenAIRequest{}
	out, err := conv.Transform(body, "gpt", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if strings.Contains(string(out), "reasoning_effort") {
		t.Fatalf("expected no reasoning_effort")
	}
}

func TestGeminiToOpenAIRequestInlineAndTextParts(t *testing.T) {
	req := GeminiRequest{Contents: []GeminiContent{{
		Role:  "user",
		Parts: []GeminiPart{{InlineData: &GeminiInlineData{MimeType: "image/png", Data: "Zm9v"}}, {Text: "hi"}},
	}}}
	body, _ := json.Marshal(req)
	conv := &geminiToOpenAIRequest{}
	out, err := conv.Transform(body, "gpt", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "image_url") || !strings.Contains(string(out), "\"type\":\"text\"") {
		t.Fatalf("expected image and text parts")
	}
}

func TestClaudeToOpenAIRequestSystemString(t *testing.T) {
	req := ClaudeRequest{System: "sys", Messages: []ClaudeMessage{{Role: "user", Content: "hi"}}}
	body, _ := json.Marshal(req)
	conv := &claudeToOpenAIRequest{}
	out, err := conv.Transform(body, "gpt", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "sys") {
		t.Fatalf("expected system")
	}
}

func TestCodexToOpenAIRequestRoleDefault(t *testing.T) {
	req := CodexRequest{Input: []interface{}{map[string]interface{}{"type": "message", "content": "hi"}}}
	body, _ := json.Marshal(req)
	conv := &codexToOpenAIRequest{}
	out, err := conv.Transform(body, "gpt", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "\"role\":\"user\"") {
		t.Fatalf("expected role user")
	}
}

func TestCodexToOpenAIRequestBranches(t *testing.T) {
	req := CodexRequest{
		Reasoning: &CodexReasoning{Effort: "high"},
		Input: []interface{}{
			map[string]interface{}{"type": "message", "role": "assistant", "content": "hi"},
			map[string]interface{}{"type": "function_call", "call_id": "call_1", "name": "tool", "arguments": "{}"},
			map[string]interface{}{"type": "function_call_output", "call_id": "call_1", "output": "ok"},
		},
		Tools: []CodexTool{{Name: "tool", Description: "d", Parameters: map[string]interface{}{"type": "object"}}},
	}
	body, _ := json.Marshal(req)
	conv := &codexToOpenAIRequest{}
	out, err := conv.Transform(body, "gpt", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "reasoning_effort") {
		t.Fatalf("expected reasoning_effort")
	}
	if !strings.Contains(string(out), "tool_calls") {
		t.Fatalf("expected tool_calls")
	}
	if !strings.Contains(string(out), "\"role\":\"tool\"") {
		t.Fatalf("expected tool message")
	}
}

func TestClaudeToOpenAIRequestPartsToolsStop(t *testing.T) {
	req := ClaudeRequest{
		Model: "claude-3-5-haiku",
		System: []interface{}{
			map[string]interface{}{"text": "sys"},
		},
		Messages: []ClaudeMessage{{
			Role: "assistant",
			Content: []interface{}{
				map[string]interface{}{"type": "text", "text": "a"},
				map[string]interface{}{"type": "text", "text": "b"},
				map[string]interface{}{"type": "tool_use", "id": "t1", "name": "tool", "input": map[string]interface{}{"x": 1}},
				map[string]interface{}{"type": "tool_result", "tool_use_id": "t1", "content": "ok"},
			},
		}},
		Tools: []ClaudeTool{{
			Name:        "tool",
			Description: "desc",
			InputSchema: map[string]interface{}{"type": "object"},
		}},
		StopSequences: []string{"stop"},
	}
	body, _ := json.Marshal(req)
	conv := &claudeToOpenAIRequest{}
	out, err := conv.Transform(body, "gpt", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "\"type\":\"text\"") {
		t.Fatalf("expected multipart content")
	}
	var openaiReq OpenAIRequest
	if err := json.Unmarshal(out, &openaiReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(openaiReq.Tools) != 1 {
		t.Fatalf("expected tool conversion")
	}
	switch stop := openaiReq.Stop.(type) {
	case []interface{}:
		if len(stop) != 1 || stop[0].(string) != "stop" {
			t.Fatalf("expected stop sequences")
		}
	default:
		t.Fatalf("unexpected stop type")
	}
}

func TestOpenAIToCodexRequestToolNameFallback(t *testing.T) {
	req := OpenAIRequest{
		Model: "gpt",
		Tools: []OpenAITool{{
			Type:     "function",
			Function: OpenAIFunction{Name: "", Description: "noop"},
		}},
		Messages: []OpenAIMessage{{
			Role: "assistant",
			ToolCalls: []OpenAIToolCall{{
				ID:   "call_1",
				Type: "function",
				Function: OpenAIFunctionCall{
					Name:      "toolA",
					Arguments: "{}",
				},
			}},
		}},
	}
	body, _ := json.Marshal(req)
	conv := &openaiToCodexRequest{}
	out, err := conv.Transform(body, "codex", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "function_call") {
		t.Fatalf("expected function_call output")
	}
}

func TestOpenAIToGeminiRequestMaxCompletionAndToolFallback(t *testing.T) {
	req := OpenAIRequest{
		MaxCompletionTokens: 42,
		Messages: []OpenAIMessage{{
			Role:       "tool",
			Content:    "ok",
			ToolCallID: "tool_missing",
		}},
	}
	body, _ := json.Marshal(req)
	conv := &openaiToGeminiRequest{}
	out, err := conv.Transform(body, "gemini", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var gemReq GeminiRequest
	if err := json.Unmarshal(out, &gemReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if gemReq.GenerationConfig.MaxOutputTokens != 42 {
		t.Fatalf("expected max tokens from max_completion_tokens")
	}
	for _, content := range gemReq.Contents {
		for _, part := range content.Parts {
			if part.FunctionResponse != nil && part.FunctionResponse.Name == "tool_missing" {
				t.Fatalf("unexpected tool fallback name")
			}
		}
	}
}

func TestOpenAIToClaudeRequestMaxCompletionStop(t *testing.T) {
	req := OpenAIRequest{
		MaxCompletionTokens: 11,
		Stop:                "stop",
		Messages: []OpenAIMessage{{
			Role:    "user",
			Content: "hi",
		}},
	}
	body, _ := json.Marshal(req)
	conv := &openaiToClaudeRequest{}
	out, err := conv.Transform(body, "claude", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var claudeReq ClaudeRequest
	if err := json.Unmarshal(out, &claudeReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if claudeReq.MaxTokens != 11 {
		t.Fatalf("expected max tokens from max_completion_tokens")
	}
	if len(claudeReq.StopSequences) != 1 || claudeReq.StopSequences[0] != "stop" {
		t.Fatalf("expected stop sequences")
	}
}

func TestCodexToOpenAIRequestInstructions(t *testing.T) {
	req := CodexRequest{
		Instructions: "sys",
		Input:        "hi",
	}
	body, _ := json.Marshal(req)
	conv := &codexToOpenAIRequest{}
	out, err := conv.Transform(body, "gpt", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var openaiReq OpenAIRequest
	if err := json.Unmarshal(out, &openaiReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(openaiReq.Messages) == 0 || openaiReq.Messages[0].Role != "system" {
		t.Fatalf("expected system message from instructions")
	}
}
