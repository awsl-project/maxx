package converter

import (
	"encoding/json"
	"testing"

	"github.com/tidwall/gjson"
)

const tinyPNGDataURL = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

// A generated image in the OpenAI response's non-standard message.images[] array
// must convert to a Gemini inlineData part so a Gemini client receives it.
func TestOpenAIToGeminiResponse_CarriesGeneratedImage(t *testing.T) {
	body := []byte(`{
		"id":"gen","object":"chat.completion","model":"google/gemini-2.5-flash-image",
		"choices":[{"index":0,"message":{"role":"assistant","content":"here",
			"images":[{"type":"image_url","image_url":{"url":"` + tinyPNGDataURL + `"}}]},
			"finish_reason":"stop"}],
		"usage":{"prompt_tokens":3,"completion_tokens":1290,"total_tokens":1293}
	}`)

	out, err := (&openaiToGeminiResponse{}).Transform(body)
	if err != nil {
		t.Fatalf("transform: %v", err)
	}

	mime := gjson.GetBytes(out, "candidates.0.content.parts.#(inlineData.mimeType!=).inlineData.mimeType").String()
	data := gjson.GetBytes(out, "candidates.0.content.parts.#(inlineData.data!=).inlineData.data").String()
	if mime != "image/png" {
		t.Errorf("inlineData.mimeType = %q, want image/png\n%s", mime, out)
	}
	if data == "" {
		t.Errorf("generated image dropped; body=%s", out)
	}
	// The text part is still present.
	if !gjson.GetBytes(out, `candidates.0.content.parts.#(text=="here")`).Exists() {
		t.Errorf("text part lost: %s", out)
	}
}

// The streaming variant must emit the image delta as a Gemini inlineData part.
func TestOpenAIToGeminiStream_CarriesGeneratedImage(t *testing.T) {
	chunk := []byte("data: " + `{"id":"gen","object":"chat.completion.chunk","model":"google/gemini-2.5-flash-image","choices":[{"index":0,"delta":{"role":"assistant","images":[{"type":"image_url","image_url":{"url":"` + tinyPNGDataURL + `"}}]}}]}` + "\n\n")

	state := NewTransformState()
	out, err := (&openaiToGeminiResponse{}).TransformChunk(chunk, state)
	if err != nil {
		t.Fatalf("transform chunk: %v", err)
	}
	// The emitted SSE frame must carry the inlineData image.
	if !gjson.ValidBytes(extractSSEData(t, out)) {
		t.Fatalf("no valid gemini frame emitted: %s", out)
	}
	frame := extractSSEData(t, out)
	if got := gjson.GetBytes(frame, "candidates.0.content.parts.0.inlineData.data").String(); got == "" {
		t.Fatalf("stream dropped generated image; frame=%s", frame)
	}
	if got := gjson.GetBytes(frame, "candidates.0.content.parts.0.inlineData.mimeType").String(); got != "image/png" {
		t.Fatalf("stream inlineData.mimeType = %q, want image/png", got)
	}
}

// extractSSEData returns the JSON payload of the first "data:" SSE line.
func extractSSEData(t *testing.T, sse []byte) []byte {
	t.Helper()
	for _, line := range splitLines(string(sse)) {
		if len(line) > 5 && line[:5] == "data:" {
			var js json.RawMessage
			payload := []byte(line[5:])
			if json.Unmarshal(payload, &js) == nil {
				return payload
			}
		}
	}
	return nil
}

func splitLines(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
			continue
		}
		if r == '\r' {
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
