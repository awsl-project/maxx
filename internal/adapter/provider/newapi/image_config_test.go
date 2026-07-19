package newapi

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestNormalizeImageConfig_ChatTopLevelToExtraBodyGoogle(t *testing.T) {
	// A client (OpenAI- or Gemini-origin) expressing aspect via the top-level
	// image_config → new-api needs it under extra_body.google.image_config plus
	// image output modalities.
	body := []byte(`{"model":"gemini-2.5-flash-image","messages":[{"role":"user","content":"a cat"}],"image_config":{"aspect_ratio":"16:9"}}`)
	out := normalizeImageConfigBody(body, "/v1/chat/completions")

	if got := gjson.GetBytes(out, "extra_body.google.image_config.aspect_ratio").String(); got != "16:9" {
		t.Fatalf("extra_body.google.image_config.aspect_ratio = %q, want 16:9\n%s", got, out)
	}
	mods := gjson.GetBytes(out, "modalities").Array()
	if len(mods) != 2 || mods[0].String() != "image" {
		t.Fatalf("modalities = %v, want [image text]\n%s", mods, out)
	}
}

func TestNormalizeImageConfig_ChatSizeDerivesAspect(t *testing.T) {
	// Standard OpenAI client only sent pixel size → derive a coarse aspect ratio
	// and emit it in new-api's dialect.
	body := []byte(`{"model":"gemini-2.5-flash-image","messages":[{"role":"user","content":"a cat"}],"size":"1536x1024"}`)
	out := normalizeImageConfigBody(body, "/v1/chat/completions")

	if got := gjson.GetBytes(out, "extra_body.google.image_config.aspect_ratio").String(); got != "3:2" {
		t.Fatalf("aspect_ratio = %q, want 3:2\n%s", got, out)
	}
	// Original size stays intact (new-api ignores unknown top-level keys).
	if gjson.GetBytes(out, "size").String() != "1536x1024" {
		t.Fatalf("size should be preserved: %s", out)
	}
	if !gjson.GetBytes(out, "modalities").Exists() {
		t.Fatalf("modalities not added: %s", out)
	}
}

func TestNormalizeImageConfig_ChatImageSizeOnly(t *testing.T) {
	// A request carrying only image_config.image_size still gets translated and
	// gains the image modality (not treated as a non-image request).
	body := []byte(`{"model":"gemini-3-pro-image","messages":[{"role":"user","content":"a cat"}],"image_config":{"image_size":"2K"}}`)
	out := normalizeImageConfigBody(body, "/v1/chat/completions")

	if got := gjson.GetBytes(out, "extra_body.google.image_config.image_size").String(); got != "2K" {
		t.Fatalf("image_size = %q, want 2K\n%s", got, out)
	}
	mods := gjson.GetBytes(out, "modalities").Array()
	hasImage := false
	for _, m := range mods {
		if m.String() == "image" {
			hasImage = true
		}
	}
	if !hasImage {
		t.Fatalf("image modality not added for image_size-only request: %s", out)
	}
}

func TestNormalizeImageConfig_PreservesExistingModalities(t *testing.T) {
	// Existing modalities must be kept, with image prepended — not clobbered.
	body := []byte(`{"model":"x","messages":[],"image_config":{"aspect_ratio":"1:1"},"modalities":["audio","text"]}`)
	out := normalizeImageConfigBody(body, "/v1/chat/completions")
	got := gjson.GetBytes(out, "modalities").Array()
	var vals []string
	for _, m := range got {
		vals = append(vals, m.String())
	}
	if len(vals) != 3 || vals[0] != "image" || vals[1] != "audio" || vals[2] != "text" {
		t.Fatalf("modalities = %v, want [image audio text]", vals)
	}
}

func TestNormalizeImageConfig_ImagesDerivesSize(t *testing.T) {
	// On the images endpoint new-api reads pixel size like plain OpenAI; derive it
	// from an aspect ratio the client expressed as image_config, and do NOT add
	// extra_body.google or modalities (different schema).
	body := []byte(`{"model":"gpt-image-1","prompt":"a cat","image_config":{"aspect_ratio":"9:16"}}`)
	out := normalizeImageConfigBody(body, "/v1/images/generations")

	if got := gjson.GetBytes(out, "size").String(); got != "1024x1536" {
		t.Fatalf("size = %q, want 1024x1536\n%s", got, out)
	}
	if gjson.GetBytes(out, "extra_body").Exists() {
		t.Fatalf("images endpoint must not get extra_body.google: %s", out)
	}
	if gjson.GetBytes(out, "modalities").Exists() {
		t.Fatalf("images endpoint must not get modalities: %s", out)
	}
}

func TestNormalizeImageConfig_NoImageIntentUntouched(t *testing.T) {
	// A plain chat request with no sizing must not gain modalities/extra_body.
	body := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`)
	out := normalizeImageConfigBody(body, "/v1/chat/completions")
	if gjson.GetBytes(out, "modalities").Exists() || gjson.GetBytes(out, "extra_body").Exists() {
		t.Fatalf("non-image request must be left untouched: %s", out)
	}
}

func TestSizeAspectHelpers(t *testing.T) {
	if got := sizeFromAspect("16:9"); got != "1536x1024" {
		t.Errorf("sizeFromAspect(16:9) = %q, want 1536x1024", got)
	}
	if got := sizeFromAspect("9:16"); got != "1024x1536" {
		t.Errorf("sizeFromAspect(9:16) = %q, want 1024x1536", got)
	}
	if got := sizeFromAspect("1:1"); got != "1024x1024" {
		t.Errorf("sizeFromAspect(1:1) = %q, want 1024x1024", got)
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
