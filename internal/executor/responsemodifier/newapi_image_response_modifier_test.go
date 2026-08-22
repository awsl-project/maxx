package responsemodifier

import (
	"net/http/httptest"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/tidwall/gjson"
)

const dataURL = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

func newapiProvider() *domain.Provider { return &domain.Provider{Type: "newapi"} }

func TestNewapiImageModifier_Gating(t *testing.T) {
	cases := []struct {
		name       string
		provider   *domain.Provider
		clientType domain.ClientType
		wantNil    bool
	}{
		{"newapi + openai -> active", newapiProvider(), domain.ClientTypeOpenAI, false},
		{"newapi + gemini -> nil (native inlineData, no markdown)", newapiProvider(), domain.ClientTypeGemini, true},
		{"custom provider -> nil", &domain.Provider{Type: "custom"}, domain.ClientTypeOpenAI, true},
		{"openrouter provider -> nil", &domain.Provider{Type: "openrouter"}, domain.ClientTypeOpenAI, true},
		{"nil provider -> nil", nil, domain.ClientTypeOpenAI, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := newNewapiImageResponseModifier(tc.provider, tc.clientType)
			if (got == nil) != tc.wantNil {
				t.Fatalf("newNewapiImageResponseModifier nil=%v, want nil=%v", got == nil, tc.wantNil)
			}
		})
	}
}

func TestNewapiImageModifier_LiftsMarkdownImageFromContent(t *testing.T) {
	m := &newapiImageResponseModifier{}
	body := []byte(`{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"Here's a red apple!\n![image](` + dataURL + `)"},"finish_reason":"stop"}]}`)

	out := m.modifyBody(body)

	// image lifted into the standard images[] array
	if got := gjson.GetBytes(out, "choices.0.message.images.0.image_url.url").String(); got != dataURL {
		t.Fatalf("images[0].image_url.url = %q, want the data URL\n%s", got, out)
	}
	if got := gjson.GetBytes(out, "choices.0.message.images.0.type").String(); got != "image_url" {
		t.Fatalf("images[0].type = %q, want image_url", got)
	}
	// prose preserved, markdown image removed
	if got := gjson.GetBytes(out, "choices.0.message.content").String(); got != "Here's a red apple!" {
		t.Fatalf("content = %q, want prose without the markdown image", got)
	}
}

func TestNewapiImageModifier_MultipleImagesAndExistingImages(t *testing.T) {
	m := &newapiImageResponseModifier{}
	// one image already present + two markdown images in content
	body := []byte(`{"choices":[{"message":{"role":"assistant","content":"a ![image](` + dataURL + `) b ![image](` + dataURL + `)","images":[{"type":"image_url","image_url":{"url":"pre"}}]}}]}`)

	out := m.modifyBody(body)

	imgs := gjson.GetBytes(out, "choices.0.message.images").Array()
	if len(imgs) != 3 {
		t.Fatalf("images len = %d, want 3 (1 pre-existing + 2 lifted)\n%s", len(imgs), out)
	}
	if imgs[0].Get("image_url.url").String() != "pre" {
		t.Fatalf("pre-existing image must be preserved first, got %s", imgs[0].Raw)
	}
	if got := gjson.GetBytes(out, "choices.0.message.content").String(); got != "a  b" {
		t.Fatalf("content = %q, want prose with both images removed", got)
	}
}

func TestNewapiImageModifier_NoImageIsPassthrough(t *testing.T) {
	m := &newapiImageResponseModifier{}
	body := []byte(`{"choices":[{"message":{"role":"assistant","content":"just text, no image"}}]}`)
	if out := m.modifyBody(body); string(out) != string(body) {
		t.Fatalf("body without an image must pass through unchanged\n got: %s\nwant: %s", out, body)
	}
}

func TestNewapiImageModifier_MalformedBodyIsPassthrough(t *testing.T) {
	m := &newapiImageResponseModifier{}
	// contains the trigger substring but is not valid JSON
	body := []byte(`not json ![image](data:image/png;base64,AAAA)`)
	if out := m.modifyBody(body); string(out) != string(body) {
		t.Fatalf("malformed body must pass through unchanged, got %s", out)
	}
}

func TestNewapiImageModifier_StreamEventLiftsCompleteDelta(t *testing.T) {
	m := &newapiImageResponseModifier{}
	event := []byte("data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"![image](" + dataURL + ")\"}}]}\n\n")

	out := m.modifyStreamEvent(event)

	url := gjson.GetBytes([]byte(sseData(string(out))), "choices.0.delta.images.0.image_url.url").String()
	if url != dataURL {
		t.Fatalf("stream delta image not lifted; url=%q\n%s", url, out)
	}
	// SSE framing preserved
	if got := string(out); got[:6] != "data: " || got[len(got)-2:] != "\n\n" {
		t.Fatalf("SSE framing not preserved: %q", got)
	}
}

func TestNewapiImageModifier_StreamDoneAndPlainPassthrough(t *testing.T) {
	m := &newapiImageResponseModifier{}
	for _, ev := range []string{"data: [DONE]\n\n", "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"} {
		if got := string(m.modifyStreamEvent([]byte(ev))); got != ev {
			t.Fatalf("event should pass through unchanged\n got: %q\nwant: %q", got, ev)
		}
	}
}

// End-to-end through the writer: the factory must select the newapi modifier for a
// newapi+openai response and Finalize must emit the normalized body to the client.
func TestResponseModifierWriter_NewapiImageNonStream(t *testing.T) {
	rr := httptest.NewRecorder()
	writer := NewResponseModifierWriter(rr, newapiProvider(), domain.ClientTypeOpenAI, false)
	if writer == nil {
		t.Fatal("expected a writer (newapi+openai should select the image modifier)")
	}
	writer.WriteHeader(200)
	_, _ = writer.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"apple\n![image](` + dataURL + `)"}}]}`))
	if err := writer.Finalize(); err != nil {
		t.Fatalf("finalize failed: %v", err)
	}
	got := rr.Body.Bytes()
	if url := gjson.GetBytes(got, "choices.0.message.images.0.image_url.url").String(); url != dataURL {
		t.Fatalf("client did not receive lifted image; body=%s", got)
	}
	if c := gjson.GetBytes(got, "choices.0.message.content").String(); c != "apple" {
		t.Fatalf("content = %q, want \"apple\"", c)
	}
}

// sseData returns the JSON payload of the first SSE data line.
func sseData(s string) string {
	for _, line := range splitSSELines(s) {
		if len(line) > 6 && line[:6] == "data: " {
			return line[6:]
		}
	}
	return ""
}

func splitSSELines(s string) []string {
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
