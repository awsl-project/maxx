package openrouter

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/flow"
	"github.com/tidwall/gjson"
)

// runOpenRouterExecute drives a request end-to-end through the openrouter adapter
// (Execute → custom core → real HTTP) against a mock upstream (reached by pointing
// MAXX_OPENROUTER_BASE_URL at it), and returns the exact JSON body the upstream
// received. Unlike new-api, OpenRouter's own dialect keeps clarity/aspect at the
// top-level image_config, so these tests assert the fields stay there — not moved
// under extra_body.google.
func runOpenRouterExecute(t *testing.T, requestURI string, reqBody []byte) []byte {
	t.Helper()

	var captured []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"chatcmpl-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()
	t.Setenv("MAXX_OPENROUTER_BASE_URL", server.URL)

	adapter, err := NewAdapter(&domain.Provider{
		Name:                 "test-openrouter",
		Type:                 "openrouter",
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeOpenAI},
		Config: &domain.ProviderConfig{
			OpenRouter: &domain.ProviderConfigOpenRouter{APIKey: "sk-or-test"},
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

// TestIntegration_OpenRouterChatClarityLevels: the 1K / 2K / 4K resolution reaches
// the OpenRouter upstream at the top-level image_config.image_size (its native
// dialect), with the aspect ratio preserved and image output modalities added.
func TestIntegration_OpenRouterChatClarityLevels(t *testing.T) {
	for _, clarity := range []string{"1K", "2K", "4K"} {
		t.Run(clarity, func(t *testing.T) {
			body := []byte(`{"model":"google/gemini-3-pro-image","messages":[{"role":"user","content":"a fox"}],"image_config":{"aspect_ratio":"16:9","image_size":"` + clarity + `"}}`)
			got := runOpenRouterExecute(t, "/v1/chat/completions", body)

			if v := gjson.GetBytes(got, "image_config.image_size").String(); v != clarity {
				t.Errorf("image_config.image_size = %q, want %q\n%s", v, clarity, got)
			}
			if v := gjson.GetBytes(got, "image_config.aspect_ratio").String(); v != "16:9" {
				t.Errorf("image_config.aspect_ratio = %q, want 16:9\n%s", v, got)
			}
			// OpenRouter dialect is top-level; it must NOT grow extra_body.google.
			if gjson.GetBytes(got, "extra_body").Exists() {
				t.Errorf("OpenRouter body must not gain extra_body.google\n%s", got)
			}
			mods := gjson.GetBytes(got, "modalities").Array()
			if len(mods) == 0 || mods[0].String() != "image" {
				t.Errorf("modalities = %v, want image first\n%s", mods, got)
			}
		})
	}
}

// TestIntegration_OpenRouterChatClarityOnly: only a resolution is requested (no
// aspect). Clarity passes through and the request is marked for image output,
// without synthesizing an aspect ratio.
func TestIntegration_OpenRouterChatClarityOnly(t *testing.T) {
	body := []byte(`{"model":"google/gemini-3-pro-image","messages":[{"role":"user","content":"a fox"}],"image_config":{"image_size":"4K"}}`)
	got := runOpenRouterExecute(t, "/v1/chat/completions", body)

	if v := gjson.GetBytes(got, "image_config.image_size").String(); v != "4K" {
		t.Errorf("image_config.image_size = %q, want 4K\n%s", v, got)
	}
	if gjson.GetBytes(got, "image_config.aspect_ratio").Exists() {
		t.Errorf("no aspect requested; must not be synthesized\n%s", got)
	}
	if !gjson.GetBytes(got, "modalities").Exists() {
		t.Errorf("image modality missing for clarity-only request\n%s", got)
	}
}

// TestIntegration_OpenRouterChatPixelSizeDerivesAspect: a pure OpenAI client that
// only sent pixel size gets a nearest-neighbor aspect ratio filled into the
// top-level image_config.
func TestIntegration_OpenRouterChatPixelSizeDerivesAspect(t *testing.T) {
	cases := []struct {
		pixel      string
		wantAspect string
	}{
		{"1920x1080", "16:9"},
		{"1024x1280", "4:5"},
		{"1024x1024", "1:1"},
	}
	for _, c := range cases {
		t.Run(c.pixel, func(t *testing.T) {
			body := []byte(`{"model":"google/gemini-3-pro-image","messages":[{"role":"user","content":"x"}],"size":"` + c.pixel + `"}`)
			got := runOpenRouterExecute(t, "/v1/chat/completions", body)

			if v := gjson.GetBytes(got, "image_config.aspect_ratio").String(); v != c.wantAspect {
				t.Errorf("image_config.aspect_ratio = %q, want %q\n%s", v, c.wantAspect, got)
			}
		})
	}
}

// TestIntegration_OpenRouterImagesEndpoint: the /images path uses the pixel `size`
// (derived from aspect) and must not gain chat-only modalities.
func TestIntegration_OpenRouterImagesEndpoint(t *testing.T) {
	body := []byte(`{"model":"openai/gpt-5-image","prompt":"a fox","image_config":{"aspect_ratio":"9:16"}}`)
	got := runOpenRouterExecute(t, "/v1/images/generations", body)

	if v := gjson.GetBytes(got, "size").String(); v != "1024x1536" {
		t.Errorf("size = %q, want 1024x1536\n%s", v, got)
	}
	if gjson.GetBytes(got, "modalities").Exists() {
		t.Errorf("images endpoint must not get modalities\n%s", got)
	}
}
