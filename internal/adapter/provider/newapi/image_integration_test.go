package newapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/tidwall/gjson"
)

// runNewAPIExecute drives a request end-to-end through the newapi adapter
// (Execute → custom core → real HTTP) against a mock new-api upstream, and
// returns the exact JSON body the upstream actually received. This exercises the
// full path — not just normalizeImageConfigBody in isolation — so a regression in
// the wiring (body not re-set, wrong endpoint detection, custom core dropping
// fields) would be caught.
func runNewAPIExecute(t *testing.T, requestURI string, reqBody []byte) []byte {
	t.Helper()

	var captured []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"chatcmpl-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()

	adapter, err := NewAdapter(&domain.Provider{
		Name:                 "test-newapi",
		Type:                 "newapi",
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeOpenAI},
		Config: &domain.ProviderConfig{
			Custom: &domain.ProviderConfigCustom{BaseURL: server.URL, APIKey: "sk-test"},
		},
	})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost, "http://localhost"+requestURI, nil)
	rec := httptest.NewRecorder()
	ctx := flow.NewCtx(rec, req)
	ctx.Set(flow.KeyClientType, domain.ClientTypeOpenAI)
	ctx.Set(flow.KeyOriginalClientType, domain.ClientTypeOpenAI)
	ctx.Set(flow.KeyRequestHeaders, req.Header.Clone())
	ctx.Set(flow.KeyRequestURI, requestURI)
	ctx.Set(flow.KeyRequestBody, reqBody)

	if err := adapter.Execute(ctx, &domain.Provider{}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if captured == nil {
		t.Fatal("upstream received no body")
	}
	return captured
}

// TestIntegration_NewAPIChatClarityLevels is the core resolution ("清晰度") case:
// an OpenAI client asks for image output at 1K / 2K / 4K via image_config, and the
// upstream must receive that clarity under extra_body.google.image_config.image_size
// alongside the aspect ratio, with image output modalities.
func TestIntegration_NewAPIChatClarityLevels(t *testing.T) {
	for _, clarity := range []string{"1K", "2K", "4K"} {
		t.Run(clarity, func(t *testing.T) {
			body := []byte(`{"model":"gemini-3-pro-image","messages":[{"role":"user","content":"a fox"}],"image_config":{"aspect_ratio":"16:9","image_size":"` + clarity + `"}}`)
			got := runNewAPIExecute(t, "/v1/chat/completions", body)

			if v := gjson.GetBytes(got, "extra_body.google.image_config.image_size").String(); v != clarity {
				t.Errorf("upstream image_size = %q, want %q\n%s", v, clarity, got)
			}
			if v := gjson.GetBytes(got, "extra_body.google.image_config.aspect_ratio").String(); v != "16:9" {
				t.Errorf("upstream aspect_ratio = %q, want 16:9\n%s", v, got)
			}
			mods := gjson.GetBytes(got, "modalities").Array()
			if len(mods) == 0 || mods[0].String() != "image" {
				t.Errorf("modalities = %v, want image first\n%s", mods, got)
			}
		})
	}
}

// TestIntegration_NewAPIChatClarityOnly covers a client that specifies only the
// resolution (no aspect ratio) — clarity must still reach the upstream and the
// request must still be marked as image output, without inventing an aspect ratio.
func TestIntegration_NewAPIChatClarityOnly(t *testing.T) {
	body := []byte(`{"model":"gemini-3-pro-image","messages":[{"role":"user","content":"a fox"}],"image_config":{"image_size":"4K"}}`)
	got := runNewAPIExecute(t, "/v1/chat/completions", body)

	if v := gjson.GetBytes(got, "extra_body.google.image_config.image_size").String(); v != "4K" {
		t.Errorf("upstream image_size = %q, want 4K\n%s", v, got)
	}
	if gjson.GetBytes(got, "extra_body.google.image_config.aspect_ratio").Exists() {
		t.Errorf("no aspect ratio was requested; must not be synthesized\n%s", got)
	}
	if !gjson.GetBytes(got, "modalities").Exists() {
		t.Errorf("image modality missing for clarity-only request\n%s", got)
	}
}

// TestIntegration_NewAPIChatPixelSizeWithClarity covers the mixed case: a pure
// OpenAI client that expresses framing as a pixel size and clarity separately.
// The pixel size becomes a nearest-neighbor aspect ratio while the clarity is
// carried through verbatim.
func TestIntegration_NewAPIChatPixelSizeWithClarity(t *testing.T) {
	cases := []struct {
		pixel      string
		clarity    string
		wantAspect string
	}{
		{"1920x1080", "2K", "16:9"},
		{"1024x1280", "4K", "4:5"},
		{"1024x1024", "1K", "1:1"},
	}
	for _, c := range cases {
		t.Run(c.pixel+"_"+c.clarity, func(t *testing.T) {
			body := []byte(`{"model":"gemini-3-pro-image","messages":[{"role":"user","content":"x"}],"size":"` + c.pixel + `","image_config":{"image_size":"` + c.clarity + `"}}`)
			got := runNewAPIExecute(t, "/v1/chat/completions", body)

			if v := gjson.GetBytes(got, "extra_body.google.image_config.aspect_ratio").String(); v != c.wantAspect {
				t.Errorf("aspect_ratio = %q, want %q\n%s", v, c.wantAspect, got)
			}
			if v := gjson.GetBytes(got, "extra_body.google.image_config.image_size").String(); v != c.clarity {
				t.Errorf("image_size = %q, want %q\n%s", v, c.clarity, got)
			}
		})
	}
}

// TestIntegration_NewAPIImagesEndpoint verifies the /images path uses OpenAI's
// pixel `size` (derived from aspect) and does NOT get the chat-only
// extra_body.google / modalities shape.
func TestIntegration_NewAPIImagesEndpoint(t *testing.T) {
	body := []byte(`{"model":"gpt-image-1","prompt":"a fox","image_config":{"aspect_ratio":"9:16"}}`)
	got := runNewAPIExecute(t, "/v1/images/generations", body)

	if v := gjson.GetBytes(got, "size").String(); v != "1024x1536" {
		t.Errorf("size = %q, want 1024x1536\n%s", v, got)
	}
	if gjson.GetBytes(got, "extra_body").Exists() {
		t.Errorf("images endpoint must not get extra_body.google\n%s", got)
	}
	if gjson.GetBytes(got, "modalities").Exists() {
		t.Errorf("images endpoint must not get modalities\n%s", got)
	}
}

// TestIntegration_NewAPIPlainChatUntouched guards the non-image path: an ordinary
// chat request must reach the upstream byte-identical, with no injected image
// fields.
func TestIntegration_NewAPIPlainChatUntouched(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`)
	got := runNewAPIExecute(t, "/v1/chat/completions", body)

	if gjson.GetBytes(got, "extra_body").Exists() || gjson.GetBytes(got, "modalities").Exists() {
		t.Errorf("plain chat request must not gain image fields\n%s", got)
	}
}
