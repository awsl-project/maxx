package converter

import (
	"encoding/json"
	"testing"
)

// A Gemini image request converted to OpenAI (e.g. a Gemini client routed to an
// OpenAI-speaking provider like OpenRouter) must keep its output modality and
// image sizing, mirroring the openai->gemini direction.
func TestGeminiToOpenAIRequest_ImageConfigAndModalities(t *testing.T) {
	gReq := GeminiRequest{
		Contents: []GeminiContent{{
			Role:  "user",
			Parts: []GeminiPart{{Text: "a cat"}},
		}},
		GenerationConfig: &GeminiGenerationConfig{
			ResponseModalities: []string{"TEXT", "IMAGE"},
			ImageConfig:        &GeminiImageConfig{AspectRatio: "16:9", ImageSize: "2K"},
		},
	}
	body, _ := json.Marshal(gReq)

	out, err := (&geminiToOpenAIRequest{}).Transform(body, "google/gemini-3-pro-image", false)
	if err != nil {
		t.Fatalf("transform failed: %v", err)
	}
	var got OpenAIRequest
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ImageConfig == nil || got.ImageConfig.AspectRatio != "16:9" || got.ImageConfig.ImageSize != "2K" {
		t.Fatalf("image_config not carried: %#v", got.ImageConfig)
	}
	if len(got.Modalities) != 2 || got.Modalities[0] != "text" || got.Modalities[1] != "image" {
		t.Fatalf("modalities = %v, want [text image]", got.Modalities)
	}
}
