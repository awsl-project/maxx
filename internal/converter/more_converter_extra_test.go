package converter

import (
	"encoding/json"
	"testing"

	"github.com/tidwall/gjson"
)

func TestCodexToOpenAIResponse_ToolCallsFinishReason(t *testing.T) {
	resp := CodexResponse{
		ID:     "resp_1",
		Object: "response",
		Model:  "codex-test",
		Status: "completed",
		Usage:  CodexUsage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
		Output: []CodexOutput{{
			Type:      "function_call",
			ID:        "call_1",
			CallID:    "call_1",
			Name:      "do_work",
			Arguments: `{"a":1}`,
		}},
	}
	body, _ := json.Marshal(resp)

	conv := &codexToOpenAIResponse{}
	out, err := conv.Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}

	var got OpenAIResponse
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Choices) == 0 || got.Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("expected finish_reason tool_calls, got %#v", got.Choices)
	}
}

func TestOpenAIToCodexResponse_ToolCallsOutput(t *testing.T) {
	resp := OpenAIResponse{
		ID:      "chatcmpl_1",
		Object:  "chat.completion",
		Model:   "gpt-test",
		Created: 1,
		Usage:   OpenAIUsage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
		Choices: []OpenAIChoice{{
			Index: 0,
			Message: &OpenAIMessage{
				Role: "assistant",
				ToolCalls: []OpenAIToolCall{{
					ID:   "call_1",
					Type: "function",
					Function: OpenAIFunctionCall{
						Name:      "do_work",
						Arguments: `{"a":1}`,
					},
				}},
			},
			FinishReason: "tool_calls",
		}},
	}
	body, _ := json.Marshal(resp)

	conv := &openaiToCodexResponse{}
	out, err := conv.Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}
	if !gjson.GetBytes(out, "output").Exists() {
		t.Fatalf("expected output in response")
	}
	found := false
	if outputs := gjson.GetBytes(out, "output"); outputs.IsArray() {
		outputs.ForEach(func(_, item gjson.Result) bool {
			if item.Get("type").String() == "function_call" && item.Get("name").String() == "do_work" {
				found = true
				return false
			}
			return true
		})
	}
	if !found {
		t.Fatalf("expected function_call in response output")
	}
}

func TestGeminiToClaudeResponse_ThinkingSignature(t *testing.T) {
	resp := GeminiResponse{
		Candidates: []GeminiCandidate{{
			Content: GeminiContent{
				Role: "model",
				Parts: []GeminiPart{{
					Text:             "think",
					Thought:          true,
					ThoughtSignature: "sig1234567",
				}},
			},
			Index: 0,
		}},
	}
	body, _ := json.Marshal(resp)

	conv := &geminiToClaudeResponse{}
	out, err := conv.Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}

	var got ClaudeResponse
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Content) == 0 || got.Content[0].Type != "thinking" || got.Content[0].Signature != "sig1234567" {
		t.Fatalf("expected thinking block with signature, got %#v", got.Content)
	}
}

func TestOpenAIToGeminiResponse_ToolCallsFinishReason(t *testing.T) {
	resp := OpenAIResponse{
		ID:      "chatcmpl_1",
		Object:  "chat.completion",
		Model:   "gpt-test",
		Created: 1,
		Usage:   OpenAIUsage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
		Choices: []OpenAIChoice{{
			Index: 0,
			Message: &OpenAIMessage{
				Role: "assistant",
				ToolCalls: []OpenAIToolCall{{
					ID:   "call_1",
					Type: "function",
					Function: OpenAIFunctionCall{
						Name:      "do_work",
						Arguments: `{"a":1}`,
					},
				}},
			},
			FinishReason: "tool_calls",
		}},
	}
	body, _ := json.Marshal(resp)

	conv := &openaiToGeminiResponse{}
	out, err := conv.Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}

	var got GeminiResponse
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Candidates) == 0 || got.Candidates[0].FinishReason == "" {
		t.Fatalf("expected finishReason, got %#v", got.Candidates)
	}
}
