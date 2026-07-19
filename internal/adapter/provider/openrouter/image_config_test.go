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

func TestNormalizeImageConfig_ChatImageSizeOnlyAddsModalities(t *testing.T) {
	// A Gemini request converted upstream may carry only image_config.image_size
	// (no aspect_ratio, no size). On chat it must still get the image modality
	// rather than being treated as a non-image request.
	body := []byte(`{"model":"google/gemini-3-pro-image","messages":[{"role":"user","content":"a cat"}],"image_config":{"image_size":"2K"}}`)
	out := normalizeImageConfigBody(body, "/v1/chat/completions")

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
	if gjson.GetBytes(out, "image_config.image_size").String() != "2K" {
		t.Fatalf("image_size should be preserved: %s", out)
	}
}

func TestEnsureImageModalities_PreservesExisting(t *testing.T) {
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
	// aspectFromSize picks the nearest supported Gemini ratio, not a coarse
	// landscape/portrait/square bucket.
	aspectCases := []struct {
		size string
		want string
	}{
		{"1024x1024", "1:1"},
		{"1536x1024", "3:2"},  // exact 1.5
		{"1024x1536", "2:3"},  // exact 0.667
		{"1920x1080", "16:9"}, // widescreen no longer collapses to 3:2
		{"1080x1920", "9:16"},
		{"2560x1080", "21:9"}, // ultrawide
		{"1280x1024", "5:4"},  // 1.25 exact
		{"1024x1280", "4:5"},  // 0.8 exact
		{"1024x768", "4:3"},   // 1.333 exact
		{"768x1024", "3:4"},   // 0.75 exact
		{"garbage", ""},
	}
	for _, c := range aspectCases {
		if got := aspectFromSize(c.size); got != c.want {
			t.Errorf("aspectFromSize(%q) = %q, want %q", c.size, got, c.want)
		}
	}
}

// TestAspectFromSize_ResolutionInvariant proves the mapping is scale-free: the
// same aspect ratio at different pixel resolutions (SD → 1K → 2K → 4K) must map
// to the same ratio. This is the whole point of matching in log space — clarity
// and framing are independent, so bumping resolution must never flip the aspect.
func TestAspectFromSize_ResolutionInvariant(t *testing.T) {
	groups := []struct {
		want  string
		sizes []string
	}{
		{"16:9", []string{"1280x720", "1920x1080", "2560x1440", "3840x2160"}},
		{"9:16", []string{"720x1280", "1080x1920", "1440x2560", "2160x3840"}},
		{"1:1", []string{"512x512", "1024x1024", "2048x2048", "4096x4096"}},
		{"4:3", []string{"1024x768", "1600x1200", "2048x1536", "4096x3072"}},
		{"3:4", []string{"768x1024", "1200x1600", "1536x2048"}},
		{"3:2", []string{"1536x1024", "3000x2000", "6000x4000"}},
		{"2:3", []string{"1024x1536", "2000x3000", "4000x6000"}},
		{"21:9", []string{"2560x1080", "3440x1440", "5120x2160"}},
		{"4:5", []string{"1024x1280", "2048x2560", "3200x4000"}},
		{"5:4", []string{"1280x1024", "2560x2048", "5000x4000"}},
	}
	for _, g := range groups {
		for _, size := range g.sizes {
			if got := aspectFromSize(size); got != g.want {
				t.Errorf("aspectFromSize(%q) = %q, want %q (resolution must not change aspect)", size, got, g.want)
			}
		}
	}
}
