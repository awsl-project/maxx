package converter

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOpenAIToGeminiRequest_ModalitiesAndImageConfigAndFile(t *testing.T) {
	req := OpenAIRequest{
		Model:      "gpt-test",
		Modalities: []string{"text", "image"},
		ImageConfig: &OpenAIImageConfig{
			AspectRatio: "1:1",
			ImageSize:   "1024x1024",
		},
		Messages: []OpenAIMessage{{
			Role: "user",
			Content: []interface{}{
				map[string]interface{}{
					"type": "file",
					"file": map[string]interface{}{
						"filename":  "test.png",
						"file_data": "aGVsbG8=",
					},
				},
			},
		}},
	}
	body, _ := json.Marshal(req)

	conv := &openaiToGeminiRequest{}
	out, err := conv.Transform(body, "gemini-test", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}

	var got GeminiRequest
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.GenerationConfig == nil {
		t.Fatalf("expected generationConfig")
	}
	if len(got.GenerationConfig.ResponseModalities) != 2 {
		t.Fatalf("expected responseModalities, got %#v", got.GenerationConfig.ResponseModalities)
	}
	if got.GenerationConfig.ImageConfig == nil || got.GenerationConfig.ImageConfig.AspectRatio != "1:1" {
		t.Fatalf("expected imageConfig aspect ratio, got %#v", got.GenerationConfig.ImageConfig)
	}
	if len(got.Contents) == 0 || len(got.Contents[0].Parts) == 0 || got.Contents[0].Parts[0].InlineData == nil {
		t.Fatalf("expected inlineData from file part")
	}
}

func TestOpenAIToGeminiRequest_ToolChoiceFunction(t *testing.T) {
	req := OpenAIRequest{
		Model: "gpt-test",
		ToolChoice: map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name": "do_work",
			},
		},
		Messages: []OpenAIMessage{{
			Role:    "user",
			Content: "hi",
		}},
	}
	body, _ := json.Marshal(req)

	conv := &openaiToGeminiRequest{}
	out, err := conv.Transform(body, "gemini-test", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}

	var got GeminiRequest
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ToolConfig == nil || got.ToolConfig.FunctionCallingConfig == nil {
		t.Fatalf("expected toolConfig")
	}
	if got.ToolConfig.FunctionCallingConfig.Mode != "ANY" {
		t.Fatalf("expected mode ANY, got %q", got.ToolConfig.FunctionCallingConfig.Mode)
	}
	if len(got.ToolConfig.FunctionCallingConfig.AllowedFunctionNames) != 1 || got.ToolConfig.FunctionCallingConfig.AllowedFunctionNames[0] != "do_work" {
		t.Fatalf("unexpected allowed names: %#v", got.ToolConfig.FunctionCallingConfig.AllowedFunctionNames)
	}
}

func TestGeminiToOpenAIResponse_InlineDataToImageURL(t *testing.T) {
	resp := GeminiResponse{
		Candidates: []GeminiCandidate{{
			Content: GeminiContent{
				Role: "model",
				Parts: []GeminiPart{{
					InlineData: &GeminiInlineData{
						MimeType: "image/png",
						Data:     "aGVsbG8=",
					},
				}},
			},
			Index: 0,
		}},
	}
	body, _ := json.Marshal(resp)

	conv := &geminiToOpenAIResponse{}
	out, err := conv.Transform(body)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}

	var got OpenAIResponse
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Choices) == 0 || got.Choices[0].Message == nil {
		t.Fatalf("expected choice message")
	}
	contentParts, ok := got.Choices[0].Message.Content.([]interface{})
	if !ok || len(contentParts) == 0 {
		t.Fatalf("expected content parts array, got %#v", got.Choices[0].Message.Content)
	}
	raw, _ := json.Marshal(contentParts[0])
	if !strings.Contains(string(raw), "data:image/png;base64,aGVsbG8=") {
		t.Fatalf("expected image_url data, got %s", string(raw))
	}
}

func TestOpenAIToGeminiRequest_SystemOnlyUsesSystemInstruction(t *testing.T) {
	req := OpenAIRequest{
		Model: "gpt-test",
		Messages: []OpenAIMessage{{
			Role:    "system",
			Content: "SYS_ONLY",
		}},
	}
	body, _ := json.Marshal(req)

	conv := &openaiToGeminiRequest{}
	out, err := conv.Transform(body, "gemini-test", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}

	var got GeminiRequest
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.SystemInstruction == nil || len(got.SystemInstruction.Parts) == 0 {
		t.Fatalf("expected systemInstruction parts")
	}
	found := false
	for _, p := range got.SystemInstruction.Parts {
		if p.Text == "SYS_ONLY" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected systemInstruction to contain SYS_ONLY, got %#v", got.SystemInstruction.Parts)
	}
	if len(got.Contents) != 1 || got.Contents[0].Role != "user" {
		t.Fatalf("expected one user content, got %#v", got.Contents)
	}
	if len(got.Contents[0].Parts) != 1 || got.Contents[0].Parts[0].Text != geminiSystemOnlyPlaceholderText {
		t.Fatalf("expected space user content part, got %#v", got.Contents[0].Parts)
	}
}

func TestOpenAIToGeminiRequest_SystemAndUser(t *testing.T) {
	req := OpenAIRequest{
		Model: "gpt-test",
		Messages: []OpenAIMessage{
			{
				Role:    "system",
				Content: "SYS_AND_USER",
			},
			{
				Role:    "user",
				Content: "hi",
			},
		},
	}
	body, _ := json.Marshal(req)

	conv := &openaiToGeminiRequest{}
	out, err := conv.Transform(body, "gemini-test", false)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}

	var got GeminiRequest
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.SystemInstruction == nil || len(got.SystemInstruction.Parts) == 0 {
		t.Fatalf("expected systemInstruction parts")
	}
	var fullText strings.Builder
	for _, part := range got.SystemInstruction.Parts {
		fullText.WriteString(part.Text)
	}
	if !strings.Contains(fullText.String(), "SYS_AND_USER") {
		t.Fatalf("expected SystemInstruction to contain SYS_AND_USER, got: %q", fullText.String())
	}
	if len(got.Contents) != 1 {
		t.Fatalf("expected 1 content, got %#v", got.Contents)
	}
	if got.Contents[0].Role != "user" {
		t.Fatalf("expected user role, got %q", got.Contents[0].Role)
	}
	if len(got.Contents[0].Parts) != 1 || got.Contents[0].Parts[0].Text != "hi" {
		t.Fatalf("expected user content 'hi', got %#v", got.Contents[0].Parts)
	}
	for _, content := range got.Contents {
		for _, part := range content.Parts {
			if part.Text == "SYS_AND_USER" {
				t.Fatalf("system text leaked into contents: %#v", got.Contents)
			}
		}
	}
}
