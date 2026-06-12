package converter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func TestOpenAIToClaudeRequestAndResponse(t *testing.T) {
	req := OpenAIRequest{
		Model: "gpt-x",
		Messages: []OpenAIMessage{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: "ok", ReasoningContent: "think", ToolCalls: []OpenAIToolCall{{
				ID:   "call_1",
				Type: "function",
				Function: OpenAIFunctionCall{
					Name:      "do",
					Arguments: `{"a":1}`,
				},
			}}},
			{Role: "tool", ToolCallID: "call_1", Content: "result"},
		},
		Tools: []OpenAITool{{
			Type: "function",
			Function: OpenAIFunction{
				Name:        "do",
				Description: "d",
				Parameters:  map[string]interface{}{"type": "object"},
			},
		}},
		Stop: []interface{}{"x", "y"},
	}

	body, _ := json.Marshal(req)
	conv := &openaiToClaudeRequest{}
	out, err := conv.Transform(body, "claude-model", false)
	if err != nil {
		t.Fatalf("Transform error: %v", err)
	}

	var got ClaudeRequest
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.System != "sys" {
		t.Fatalf("system mismatch: %v", got.System)
	}
	if len(got.Messages) != 3 {
		t.Fatalf("messages length: %d", len(got.Messages))
	}
	if got.StopSequences == nil || len(got.StopSequences) != 2 {
		t.Fatalf("stop sequences missing")
	}

	resp := OpenAIResponse{
		ID:    "resp_1",
		Model: "gpt-x",
		Usage: OpenAIUsage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3},
		Choices: []OpenAIChoice{{
			Index: 0,
			Message: &OpenAIMessage{
				Role:             "assistant",
				Content:          "answer",
				ReasoningContent: "reason",
				ToolCalls: []OpenAIToolCall{{
					ID:   "call_2",
					Type: "function",
					Function: OpenAIFunctionCall{
						Name:      "tool",
						Arguments: `{"b":2}`,
					},
				}},
			},
			FinishReason: "tool_calls",
		}},
	}
	respBody, _ := json.Marshal(resp)
	respConv := &openaiToClaudeResponse{}
	respOut, err := respConv.Transform(respBody)
	if err != nil {
		t.Fatalf("Transform response: %v", err)
	}
	var claudeResp ClaudeResponse
	if err := json.Unmarshal(respOut, &claudeResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if claudeResp.StopReason != "tool_use" {
		t.Fatalf("stop reason: %v", claudeResp.StopReason)
	}
	if len(claudeResp.Content) < 2 {
		t.Fatalf("claude response content missing")
	}
}

func TestOpenAIToGeminiHelpersAndResponse(t *testing.T) {
	if got := stringifyContent("hi"); got != "hi" {
		t.Fatalf("stringify string: %s", got)
	}
	parts := []interface{}{map[string]interface{}{"text": "a"}, map[string]interface{}{"text": "b"}}
	if got := stringifyContent(parts); got != "ab" {
		t.Fatalf("stringify parts: %s", got)
	}
	if mimeFromExt("png") != "image/png" {
		t.Fatalf("mime png")
	}
	if mimeFromExt("unknown") != "" {
		t.Fatalf("mime unknown")
	}
	inline := parseInlineImage("data:image/png;base64,Zm9v")
	if inline == nil || inline.MimeType != "image/png" {
		t.Fatalf("parseInlineImage failed")
	}
	filePart := map[string]interface{}{
		"file": map[string]interface{}{
			"filename":  "doc.pdf",
			"file_data": "ZGF0YQ==",
		},
	}
	if fp := parseFilePart(filePart); fp == nil || fp.MimeType != "application/pdf" {
		t.Fatalf("parseFilePart failed")
	}

	resp := OpenAIResponse{
		ID:    "resp_2",
		Model: "gpt-y",
		Usage: OpenAIUsage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5},
		Choices: []OpenAIChoice{{
			Index: 0,
			Message: &OpenAIMessage{
				Role:             "assistant",
				ReasoningContent: "thinking",
				Content:          []interface{}{map[string]interface{}{"type": "text", "text": "hi"}},
				ToolCalls: []OpenAIToolCall{{
					ID:   "call_3",
					Type: "function",
					Function: OpenAIFunctionCall{
						Name:      "tool",
						Arguments: `{"x":1}`,
					},
				}},
			},
			FinishReason: "stop",
		}},
	}
	body, _ := json.Marshal(resp)
	conv := &openaiToGeminiResponse{}
	out, err := conv.Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var geminiResp GeminiResponse
	if err := json.Unmarshal(out, &geminiResp); err != nil {
		t.Fatalf("unmarshal gemini: %v", err)
	}
	if len(geminiResp.Candidates) != 1 {
		t.Fatalf("candidates missing")
	}
	if len(geminiResp.Candidates[0].Content.Parts) == 0 {
		t.Fatalf("parts missing")
	}
}

func TestClaudeToOpenAIHelpersAndResponse(t *testing.T) {
	if extractClaudeThinkingText(map[string]interface{}{"thinking": "t"}) != "t" {
		t.Fatalf("extract thinking")
	}
	if extractClaudeThinkingText(map[string]interface{}{"text": "t2"}) != "t2" {
		t.Fatalf("extract text")
	}
	if convertClaudeToolResultContentToString("ok") != "ok" {
		t.Fatalf("tool result string")
	}
	toolParts := []interface{}{map[string]interface{}{"text": "a"}, map[string]interface{}{"text": "b"}}
	if convertClaudeToolResultContentToString(toolParts) != "ab" {
		t.Fatalf("tool result parts")
	}
	if s := convertClaudeToolResultContentToString(map[string]interface{}{"k": "v"}); !strings.Contains(s, "k") {
		t.Fatalf("tool result json")
	}

	openaiReq := &OpenAIRequest{}
	claudeReq := &ClaudeRequest{OutputConfig: &ClaudeOutputConfig{Effort: "high"}}
	applyClaudeThinkingToOpenAI(openaiReq, claudeReq)
	if openaiReq.ReasoningEffort != "high" {
		t.Fatalf("effort from output config")
	}

	openaiReq = &OpenAIRequest{}
	claudeReq = &ClaudeRequest{Thinking: map[string]interface{}{"type": "enabled", "budget_tokens": float64(0)}}
	applyClaudeThinkingToOpenAI(openaiReq, claudeReq)
	if openaiReq.ReasoningEffort != "none" {
		t.Fatalf("effort from budget")
	}

	if v, ok := asInt(float64(3)); !ok || v != 3 {
		t.Fatalf("asInt float64")
	}
	if mapBudgetToEffort(9000) != "high" {
		t.Fatalf("mapBudgetToEffort high")
	}

	resp := ClaudeResponse{
		ID:   "msg_1",
		Role: "assistant",
		Usage: ClaudeUsage{
			InputTokens:  1,
			OutputTokens: 2,
		},
		Content: []ClaudeContentBlock{{
			Type: "text",
			Text: "hello",
		}, {
			Type:  "tool_use",
			ID:    "call_1",
			Name:  "tool",
			Input: map[string]interface{}{"x": 1},
		}},
		StopReason: "max_tokens",
	}
	respBody, _ := json.Marshal(resp)
	conv := &claudeToOpenAIResponse{}
	out, err := conv.Transform(respBody)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var openaiResp OpenAIResponse
	if err := json.Unmarshal(out, &openaiResp); err != nil {
		t.Fatalf("unmarshal openai: %v", err)
	}
	if openaiResp.Choices[0].FinishReason != "length" {
		t.Fatalf("finish reason: %v", openaiResp.Choices[0].FinishReason)
	}
}

func TestCodexToClaudeAndOpenAIResponses(t *testing.T) {
	req := CodexRequest{
		Instructions: "sys",
		Input: []interface{}{
			map[string]interface{}{"type": "message", "role": "user", "content": "hi"},
			map[string]interface{}{"type": "function_call", "name": "tool", "call_id": "call_1", "arguments": `{"x":1}`},
			map[string]interface{}{"type": "function_call_output", "call_id": "call_1", "output": "ok"},
		},
		Tools: []CodexTool{{Name: "tool", Description: "d", Parameters: map[string]interface{}{"type": "object"}}},
	}
	body, _ := json.Marshal(req)
	conv := &codexToClaudeRequest{}
	out, err := conv.Transform(body, "claude", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var claudeReq ClaudeRequest
	if err := json.Unmarshal(out, &claudeReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(claudeReq.Messages) < 2 {
		t.Fatalf("messages missing")
	}

	codexResp := CodexResponse{
		ID:    "resp",
		Model: "m",
		Usage: CodexUsage{InputTokens: 1, OutputTokens: 2, TotalTokens: 3},
		Output: []CodexOutput{{Type: "message", Content: "hi"}, {
			Type:      "function_call",
			ID:        "call_2",
			Name:      "tool",
			Arguments: `{"y":2}`,
		}},
	}
	respBody, _ := json.Marshal(codexResp)
	respConv := &codexToClaudeResponse{}
	respOut, err := respConv.Transform(respBody)
	if err != nil {
		t.Fatalf("Transform response: %v", err)
	}
	var claudeResp ClaudeResponse
	if err := json.Unmarshal(respOut, &claudeResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if claudeResp.StopReason != "tool_use" {
		t.Fatalf("stop reason: %v", claudeResp.StopReason)
	}

	openaiRespConv := &codexToOpenAIResponse{}
	openaiOut, err := openaiRespConv.Transform(respBody)
	if err != nil {
		t.Fatalf("Transform openai: %v", err)
	}
	var openaiResp OpenAIResponse
	if err := json.Unmarshal(openaiOut, &openaiResp); err != nil {
		t.Fatalf("unmarshal openai: %v", err)
	}
	if openaiResp.Choices[0].FinishReason != "" {
		t.Fatalf("finish reason: %v", openaiResp.Choices[0].FinishReason)
	}
}

func TestGeminiToOpenAIResponseInline(t *testing.T) {
	resp := GeminiResponse{Candidates: []GeminiCandidate{{
		Content: GeminiContent{Role: "model", Parts: []GeminiPart{{InlineData: &GeminiInlineData{MimeType: "image/png", Data: "Zm9v"}}}},
		Index:   0,
	}}}
	body, _ := json.Marshal(resp)
	conv := &geminiToOpenAIResponse{}
	out, err := conv.Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var openaiResp OpenAIResponse
	if err := json.Unmarshal(out, &openaiResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if openaiResp.Choices[0].Message == nil {
		t.Fatalf("message missing")
	}
}

func TestGeminiToOpenAIResponseWithToolCall(t *testing.T) {
	resp := GeminiResponse{Candidates: []GeminiCandidate{{
		Content: GeminiContent{Role: "model", Parts: []GeminiPart{{
			Thought: true,
			Text:    "think",
		}, {
			InlineData: &GeminiInlineData{MimeType: "image/png", Data: "Zm9v"},
		}, {
			Text: "hello",
		}, {
			FunctionCall: &GeminiFunctionCall{Name: "tool", Args: map[string]interface{}{"x": 1}},
		}}},
		FinishReason: "STOP",
	}}}
	body, _ := json.Marshal(resp)
	conv := &geminiToOpenAIResponse{}
	out, err := conv.Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var openaiResp OpenAIResponse
	if err := json.Unmarshal(out, &openaiResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if openaiResp.Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("finish reason mismatch")
	}
}

func TestOpenAIToClaudeResponseArrayContent(t *testing.T) {
	resp := OpenAIResponse{Choices: []OpenAIChoice{{
		Message:      &OpenAIMessage{Content: []interface{}{map[string]interface{}{"type": "text", "text": "hi"}}},
		FinishReason: "length",
	}}}
	body, _ := json.Marshal(resp)
	conv := &openaiToClaudeResponse{}
	out, err := conv.Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var claudeResp ClaudeResponse
	if err := json.Unmarshal(out, &claudeResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if claudeResp.StopReason != "max_tokens" {
		t.Fatalf("stop reason mismatch")
	}
}

func TestOpenAIToCodexResponseContent(t *testing.T) {
	resp := OpenAIResponse{Choices: []OpenAIChoice{{
		Message: &OpenAIMessage{Content: "hi"},
	}}}
	body, _ := json.Marshal(resp)
	conv := &openaiToCodexResponse{}
	out, err := conv.Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !gjson.GetBytes(out, "output").Exists() {
		t.Fatalf("expected output in response")
	}
}

func TestGeminiToOpenAIRequestToolResponse(t *testing.T) {
	req := GeminiRequest{
		Contents: []GeminiContent{{
			Role: "model",
			Parts: []GeminiPart{{
				FunctionResponse: &GeminiFunctionResponse{Name: "tool_call_1", Response: map[string]interface{}{"ok": true}},
			}},
		}},
	}
	body, _ := json.Marshal(req)
	conv := &geminiToOpenAIRequest{}
	out, err := conv.Transform(body, "gpt", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var openaiReq OpenAIRequest
	if err := json.Unmarshal(out, &openaiReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	foundTool := false
	for _, msg := range openaiReq.Messages {
		if msg.Role == "tool" {
			foundTool = true
		}
	}
	if !foundTool {
		t.Fatalf("expected tool message")
	}
}

func TestGeminiToOpenAIRequestFunctionResponseIDs(t *testing.T) {
	req := GeminiRequest{Contents: []GeminiContent{{
		Role: "model",
		Parts: []GeminiPart{{
			FunctionResponse: &GeminiFunctionResponse{Name: "tool_call_1", Response: map[string]interface{}{"ok": true}},
		}, {
			FunctionResponse: &GeminiFunctionResponse{Name: "tool", ID: "call_2", Response: map[string]interface{}{"ok": true}},
		}},
	}}}
	body, _ := json.Marshal(req)
	conv := &geminiToOpenAIRequest{}
	out, err := conv.Transform(body, "gpt", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "tool_call_id") {
		t.Fatalf("expected tool_call_id in output")
	}
}

func TestGeminiToOpenAIResponseMaxTokens(t *testing.T) {
	resp := GeminiResponse{Candidates: []GeminiCandidate{{
		Content:      GeminiContent{Role: "model", Parts: []GeminiPart{{Text: "hi"}}},
		FinishReason: "MAX_TOKENS",
	}}}
	body, _ := json.Marshal(resp)
	conv := &geminiToOpenAIResponse{}
	out, err := conv.Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "length") {
		t.Fatalf("expected length finish reason")
	}
}

func TestGeminiToOpenAITransformToolResponseSplit(t *testing.T) {
	req := GeminiRequest{Contents: []GeminiContent{{
		Role: "model",
		Parts: []GeminiPart{{
			FunctionResponse: &GeminiFunctionResponse{Name: "tool_call_9", ID: "call_10", Response: map[string]interface{}{"ok": true}},
		}},
	}}}
	body, _ := json.Marshal(req)
	conv := &geminiToOpenAIRequest{}
	out, err := conv.Transform(body, "gpt", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "tool_call_id") {
		t.Fatalf("expected tool_call_id")
	}
}

func TestOpenAIToGeminiResponseArrayContent(t *testing.T) {
	resp := OpenAIResponse{Choices: []OpenAIChoice{{
		Message:      &OpenAIMessage{Content: []interface{}{map[string]interface{}{"type": "text", "text": "hi"}}},
		FinishReason: "length",
	}}}
	body, _ := json.Marshal(resp)
	conv := &openaiToGeminiResponse{}
	out, err := conv.Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "MAX_TOKENS") {
		t.Fatalf("expected MAX_TOKENS")
	}
}

func TestClaudeToOpenAIResponseNoContent(t *testing.T) {
	resp := ClaudeResponse{StopReason: "end_turn"}
	body, _ := json.Marshal(resp)
	conv := &claudeToOpenAIResponse{}
	out, err := conv.Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "\"finish_reason\":\"stop\"") {
		t.Fatalf("expected stop finish reason")
	}
}

func TestOpenAIToGeminiResponseToolCallsFinish(t *testing.T) {
	resp := OpenAIResponse{Choices: []OpenAIChoice{{
		Message:      &OpenAIMessage{ToolCalls: []OpenAIToolCall{{ID: "call_1", Type: "function", Function: OpenAIFunctionCall{Name: "tool", Arguments: "{}"}}}},
		FinishReason: "tool_calls",
	}}}
	body, _ := json.Marshal(resp)
	conv := &openaiToGeminiResponse{}
	out, err := conv.Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "functionCall") {
		t.Fatalf("expected functionCall")
	}
}

func TestGeminiToOpenAIResponseInlineAndText(t *testing.T) {
	resp := GeminiResponse{Candidates: []GeminiCandidate{{
		Content:      GeminiContent{Role: "model", Parts: []GeminiPart{{InlineData: &GeminiInlineData{MimeType: "image/png", Data: "Zm9v"}}, {Text: "hi"}}},
		FinishReason: "STOP",
	}}}
	body, _ := json.Marshal(resp)
	conv := &geminiToOpenAIResponse{}
	out, err := conv.Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "image_url") {
		t.Fatalf("expected image_url")
	}
	if !strings.Contains(string(out), "hi") {
		t.Fatalf("expected text")
	}
}

func TestCodexToOpenAIResponseNoToolCalls(t *testing.T) {
	resp := CodexResponse{Output: []CodexOutput{{Type: "message", Content: "hi"}}}
	body, _ := json.Marshal(resp)
	conv := &codexToOpenAIResponse{}
	out, err := conv.Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "finish_reason\":null") {
		t.Fatalf("expected empty finish reason")
	}
}

func TestGeminiToOpenAIResponseToolCallsFinishStop(t *testing.T) {
	resp := GeminiResponse{Candidates: []GeminiCandidate{{
		Content:      GeminiContent{Role: "model", Parts: []GeminiPart{{FunctionCall: &GeminiFunctionCall{Name: "tool", Args: map[string]interface{}{"x": 1}}}}},
		FinishReason: "STOP",
	}}}
	body, _ := json.Marshal(resp)
	conv := &geminiToOpenAIResponse{}
	out, err := conv.Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "tool_calls") {
		t.Fatalf("expected tool_calls finish reason")
	}
}

func TestGeminiToOpenAIRequestFunctionResponseOnly(t *testing.T) {
	req := GeminiRequest{Contents: []GeminiContent{{
		Role:  "model",
		Parts: []GeminiPart{{FunctionResponse: &GeminiFunctionResponse{Name: "tool_call_1", Response: map[string]interface{}{"ok": true}}}},
	}}}
	body, _ := json.Marshal(req)
	conv := &geminiToOpenAIRequest{}
	out, err := conv.Transform(body, "gpt", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "\"role\":\"tool\"") {
		t.Fatalf("expected tool message")
	}
}

func TestOpenAIToGeminiResponseReasoningArray(t *testing.T) {
	resp := OpenAIResponse{Choices: []OpenAIChoice{{
		Message:      &OpenAIMessage{ReasoningContent: []interface{}{map[string]interface{}{"text": "a"}, map[string]interface{}{"text": "b"}}},
		FinishReason: "stop",
	}}}
	body, _ := json.Marshal(resp)
	conv := &openaiToGeminiResponse{}
	out, err := conv.Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "thought") {
		t.Fatalf("expected thought")
	}
}

func TestCodexToOpenAIResponseToolCalls(t *testing.T) {
	resp := CodexResponse{Output: []CodexOutput{{
		Type:      "function_call",
		ID:        "call_1",
		Name:      "tool",
		Arguments: `{"x":1}`,
	}}}
	body, _ := json.Marshal(resp)
	conv := &codexToOpenAIResponse{}
	out, err := conv.Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "tool_calls") {
		t.Fatalf("expected tool_calls")
	}
}

func TestOpenAIToGeminiResponseReasoningAndToolCalls(t *testing.T) {
	resp := OpenAIResponse{Choices: []OpenAIChoice{{
		Message: &OpenAIMessage{
			ReasoningContent: "think",
			ToolCalls:        []OpenAIToolCall{{ID: "call_1", Type: "function", Function: OpenAIFunctionCall{Name: "tool", Arguments: "{}"}}},
		},
		FinishReason: "tool_calls",
	}}}
	body, _ := json.Marshal(resp)
	conv := &openaiToGeminiResponse{}
	out, err := conv.Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "functionCall") {
		t.Fatalf("expected functionCall")
	}
	if !strings.Contains(string(out), "thought") {
		t.Fatalf("expected thought")
	}
}

func TestCodexToOpenAIResponseMessageOnly(t *testing.T) {
	resp := CodexResponse{Output: []CodexOutput{{Type: "message", Content: "hi"}}}
	body, _ := json.Marshal(resp)
	conv := &codexToOpenAIResponse{}
	out, err := conv.Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "\"finish_reason\":null") {
		t.Fatalf("expected empty finish")
	}
}

func TestOpenAIToGeminiResponseNoChoices(t *testing.T) {
	resp := OpenAIResponse{Usage: OpenAIUsage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3}}
	body, _ := json.Marshal(resp)
	conv := &openaiToGeminiResponse{}
	out, err := conv.Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "usageMetadata") {
		t.Fatalf("expected usage")
	}
}

func TestOpenAIToGeminiResponseFinishLength(t *testing.T) {
	resp := OpenAIResponse{Choices: []OpenAIChoice{{
		Message:      &OpenAIMessage{Content: "hi"},
		FinishReason: "length",
	}}}
	body, _ := json.Marshal(resp)
	conv := &openaiToGeminiResponse{}
	out, err := conv.Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "MAX_TOKENS") {
		t.Fatalf("expected MAX_TOKENS")
	}
}

func TestOpenAIToGeminiResponseTextOnly(t *testing.T) {
	resp := OpenAIResponse{Choices: []OpenAIChoice{{
		Message:      &OpenAIMessage{Content: "hi"},
		FinishReason: "stop",
	}}}
	body, _ := json.Marshal(resp)
	conv := &openaiToGeminiResponse{}
	out, err := conv.Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "hi") {
		t.Fatalf("expected text")
	}
}

func TestClaudeToOpenAIResponseToolUseStop(t *testing.T) {
	resp := ClaudeResponse{
		ID:         "msg",
		Model:      "claude-3-5-haiku",
		Usage:      ClaudeUsage{InputTokens: 1, OutputTokens: 2},
		StopReason: "tool_use",
		Content: []ClaudeContentBlock{{
			Type:  "tool_use",
			ID:    "call_1",
			Name:  "tool",
			Input: map[string]interface{}{"x": 1},
		}},
	}
	body, _ := json.Marshal(resp)
	conv := &claudeToOpenAIResponse{}
	out, err := conv.Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	var openaiResp OpenAIResponse
	if err := json.Unmarshal(out, &openaiResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if openaiResp.Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("expected tool_calls finish reason")
	}
}

func TestOpenAIToClaudeResponseStopReason(t *testing.T) {
	resp := OpenAIResponse{Choices: []OpenAIChoice{{
		Message:      &OpenAIMessage{Content: "hi"},
		FinishReason: "stop",
	}}}
	body, _ := json.Marshal(resp)
	conv := &openaiToClaudeResponse{}
	out, err := conv.Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "\"stop_reason\":\"end_turn\"") {
		t.Fatalf("expected end_turn stop reason")
	}
}

func TestCodexToOpenAIResponseInvalidJSON(t *testing.T) {
	input := []byte("{")
	out, err := (&codexToOpenAIResponse{}).Transform(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != string(input) {
		t.Fatalf("expected passthrough of original body, got: %s", out)
	}
}

func TestOpenAIToGeminiResponseInvalidJSON(t *testing.T) {
	_, err := (&openaiToGeminiResponse{}).Transform([]byte("{"))
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestGeminiToOpenAIResponseUsage(t *testing.T) {
	resp := GeminiResponse{
		UsageMetadata: &GeminiUsageMetadata{
			PromptTokenCount:     1,
			CandidatesTokenCount: 2,
			TotalTokenCount:      3,
		},
		Candidates: []GeminiCandidate{{
			Content: GeminiContent{Parts: []GeminiPart{{Text: "hi"}}},
		}},
	}
	body, _ := json.Marshal(resp)
	conv := &geminiToOpenAIResponse{}
	out, err := conv.Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "\"prompt_tokens\":1") {
		t.Fatalf("expected usage metadata")
	}
}

func TestGeminiToOpenAIRequestFunctionResponseFallback(t *testing.T) {
	req := GeminiRequest{
		Contents: []GeminiContent{{
			Role: "user",
			Parts: []GeminiPart{{
				FunctionResponse: &GeminiFunctionResponse{
					Name:     "tool",
					ID:       "",
					Response: map[string]interface{}{"result": "ok"},
				},
			}},
		}},
	}
	body, _ := json.Marshal(req)
	conv := &geminiToOpenAIRequest{}
	out, err := conv.Transform(body, "gpt", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), "\"tool_call_id\":\"tool\"") {
		t.Fatalf("expected tool_call_id fallback to name")
	}
}

func TestCodexToOpenAIResponseJoinsMultipleOutputTextAndRefusal(t *testing.T) {
	body := []byte(`{"id":"resp_1","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"a"},{"type":"output_text","text":"b"},{"type":"refusal","refusal":"c"}]}]}`)
	out, err := (&codexToOpenAIResponse{}).Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), `"content":"abc"`) {
		t.Fatalf("expected joined content/refusal text: %s", string(out))
	}
}

func TestCodexToOpenAIResponseCompletedToolCallsFinishReason(t *testing.T) {
	body := []byte(`{"id":"resp_1","status":"completed","output":[{"type":"function_call","call_id":"call_1","name":"tool","arguments":"{}"}]}`)
	out, err := (&codexToOpenAIResponse{}).Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), `"finish_reason":"tool_calls"`) {
		t.Fatalf("expected tool_calls finish reason: %s", string(out))
	}
}

func TestCodexToOpenAIResponseIncompleteFinishReason(t *testing.T) {
	body := []byte(`{"id":"resp_1","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"message","content":[{"type":"output_text","text":"partial"}]}]}`)
	out, err := (&codexToOpenAIResponse{}).Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !strings.Contains(string(out), `"finish_reason":"length"`) {
		t.Fatalf("expected length finish reason: %s", string(out))
	}
}
