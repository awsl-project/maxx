package openrouter

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestNormalizeImageConfig_ChatDerivesAspectAndModalities(t *testing.T) {
	// OpenAI-ish chat request expressing sizing only via pixel size → on the chat
	// endpoint we derive image_config.aspect_ratio (Gemini honors it) and ensure
	// image output modality.
	body := []byte(`{"model":"google/gemini-2.5-flash-image","messages":[{"role":"user","content":"a cat"}],"size":"1536x1024"}`)
	out := normalizeImageConfigBody(body, "/v1/chat/completions")

	if got := gjson.GetBytes(out, "image_config.aspect_ratio").String(); got != "3:2" {
		t.Fatalf("aspect_ratio = %q, want 3:2\n%s", got, out)
	}
	mods := gjson.GetBytes(out, "modalities").Array()
	if len(mods) != 2 || mods[0].String() != "image" {
		t.Fatalf("modalities = %v, want [image text]\n%s", mods, out)
	}
	// Original size is left intact.
	if gjson.GetBytes(out, "size").String() != "1536x1024" {
		t.Fatalf("size should be preserved: %s", out)
	}
}

func TestNormalizeImageConfig_ChatKeepsAspectAddsModalities(t *testing.T) {
	// Gemini client converted upstream → chat body already carries
	// image_config.aspect_ratio; we only need to guarantee modalities.
	body := []byte(`{"model":"google/gemini-3-pro-image","messages":[{"role":"user","content":"a cat"}],"image_config":{"aspect_ratio":"16:9"}}`)
	out := normalizeImageConfigBody(body, "/v1/chat/completions")

	if got := gjson.GetBytes(out, "image_config.aspect_ratio").String(); got != "16:9" {
		t.Fatalf("aspect_ratio mangled: %s", out)
	}
	if !gjson.GetBytes(out, "modalities").Exists() {
		t.Fatalf("modalities not added: %s", out)
	}
}

func TestNormalizeImageConfig_ImagesDerivesSize(t *testing.T) {
	// On the images endpoint the model reads pixel size; derive it from an
	// aspect ratio the client expressed as image_config.
	body := []byte(`{"model":"openai/gpt-5-image","prompt":"a cat","image_config":{"aspect_ratio":"9:16"}}`)
	out := normalizeImageConfigBody(body, "/v1/images/generations")

	if got := gjson.GetBytes(out, "size").String(); got != "1024x1536" {
		t.Fatalf("size = %q, want 1024x1536\n%s", got, out)
	}
	// Images endpoint must NOT get modalities (different schema).
	if gjson.GetBytes(out, "modalities").Exists() {
		t.Fatalf("modalities must not be added on images endpoint: %s", out)
	}
}

func TestNormalizeImageConfig_ImagesKeepsExplicitSize(t *testing.T) {
	// Standard OpenAI client already sent pixel size on the images endpoint —
	// pass through untouched (OpenRouter maps it to the model's aspect).
	body := []byte(`{"model":"openai/gpt-5-image","prompt":"a cat","size":"1024x1536"}`)
	out := normalizeImageConfigBody(body, "/v1/images/generations")
	if got := gjson.GetBytes(out, "size").String(); got != "1024x1536" {
		t.Fatalf("size should be preserved verbatim: %s", out)
	}
}

func TestNormalizeImageConfig_NoImageIntentUntouched(t *testing.T) {
	// A plain chat request with no sizing must not gain modalities/image_config.
	body := []byte(`{"model":"openai/gpt-4o","messages":[{"role":"user","content":"hi"}]}`)
	out := normalizeImageConfigBody(body, "/v1/chat/completions")
	if gjson.GetBytes(out, "modalities").Exists() || gjson.GetBytes(out, "image_config").Exists() {
		t.Fatalf("non-image request must be left untouched: %s", out)
	}
}

func TestSizeAspectHelpers(t *testing.T) {
	cases := []struct {
		aspect string
		size   string
	}{
		{"16:9", "1536x1024"},
		{"21:9", "1536x1024"},
		{"1:1", "1024x1024"},
		{"9:16", "1024x1536"},
		{"2:3", "1024x1536"},
	}
	for _, c := range cases {
		if got := sizeFromAspect(c.aspect); got != c.size {
			t.Errorf("sizeFromAspect(%q) = %q, want %q", c.aspect, got, c.size)
		}
	}
	if got := sizeFromAspect("garbage"); got != "" {
		t.Errorf("sizeFromAspect(garbage) = %q, want empty", got)
	}
	if got := aspectFromSize("1536x1024"); got != "3:2" {
		t.Errorf("aspectFromSize landscape = %q, want 3:2", got)
	}
	if got := aspectFromSize("1024x1024"); got != "1:1" {
		t.Errorf("aspectFromSize square = %q, want 1:1", got)
	}
}
