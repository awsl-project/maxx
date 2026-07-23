package e2e_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tidwall/gjson"
)

// newORImageConfigMock captures the upstream request and returns a minimal valid
// response per endpoint (data[] for /images, a plain chat completion otherwise),
// so these tests can assert how maxx shapes the OUTBOUND image sizing.
func newORImageConfigMock(t *testing.T, captured *capturedRequest, calls *int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(calls, 1)
		reqBody, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(reqBody))
		captured.Set(r)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/images") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"created": 1700000000,
				"data":    []map[string]any{{"b64_json": "aGVsbG8=", "media_type": "image/png"}},
				"usage":   map[string]any{"cost": 0.01},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl_or", "object": "chat.completion",
			"model": "google/gemini-2.5-flash-image", "created": 1700000000,
			"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 1, "total_tokens": 4},
		})
	}))
}

// On the chat endpoint, an OpenAI client that expresses sizing only via the
// standard pixel `size` must reach OpenRouter as image_config.aspect_ratio (the
// form Gemini image models honor on chat) plus modalities:["image"].
func TestOpenRouterImageConfig_ChatSizeToAspect(t *testing.T) {
	captured := &capturedRequest{}
	var calls int64
	mock := newORImageConfigMock(t, captured, &calls)
	defer mock.Close()
	t.Setenv("MAXX_OPENROUTER_BASE_URL", mock.URL)

	env := NewProxyTestEnv(t)
	pid := createORProvider(t, env, "or-imgcfg-chat", "sk-test-key", []string{"openai"})
	createRouteAt(t, env, "openai", pid, 1)

	req := map[string]any{
		"model":    "google/gemini-2.5-flash-image",
		"messages": []map[string]any{{"role": "user", "content": "a cat"}},
		"size":     "1536x1024",
	}
	resp := env.ProxyPost("/v1/chat/completions", req, map[string]string{"Authorization": "Bearer client-placeholder"})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	_, _, _, up := captured.Get()
	if got := gjson.GetBytes(up, "image_config.aspect_ratio").String(); got != "3:2" {
		t.Errorf("upstream image_config.aspect_ratio = %q, want 3:2; body=%s", got, up)
	}
	if !gjson.GetBytes(up, "modalities").Exists() {
		t.Errorf("upstream lost modalities; body=%s", up)
	}
}

// On the images endpoint, an aspect ratio expressed as image_config must reach
// OpenRouter as a pixel `size` (the form both Gemini and GPT image models honor
// there).
func TestOpenRouterImageConfig_ImagesAspectToSize(t *testing.T) {
	captured := &capturedRequest{}
	var calls int64
	mock := newORImageConfigMock(t, captured, &calls)
	defer mock.Close()
	t.Setenv("MAXX_OPENROUTER_BASE_URL", mock.URL)

	env := NewProxyTestEnv(t)
	pid := createORProvider(t, env, "or-imgcfg-images", "sk-test-key", []string{"openai"})
	createRouteAt(t, env, "openai", pid, 1)

	req := map[string]any{
		"model":        "openai/gpt-5-image",
		"prompt":       "a cat",
		"image_config": map[string]any{"aspect_ratio": "9:16"},
	}
	resp := env.ProxyPost("/v1/images/generations", req, map[string]string{"Authorization": "Bearer client-placeholder"})
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	_, _, _, up := captured.Get()
	if got := gjson.GetBytes(up, "size").String(); got != "1024x1536" {
		t.Errorf("upstream size = %q, want 1024x1536; body=%s", got, up)
	}
}

// Full pipeline: a native Gemini client (generationConfig.imageConfig) routed to
// the OpenAI-speaking OpenRouter provider. The forced Gemini->OpenAI conversion
// (layer 1) must carry the aspect ratio into image_config, and the provider
// (layer 2) must guarantee modalities — so the image request survives end to end.
func TestOpenRouterImageConfig_GeminiClientToOpenRouter(t *testing.T) {
	captured := &capturedRequest{}
	var calls int64
	mock := newORImageConfigMock(t, captured, &calls)
	defer mock.Close()
	t.Setenv("MAXX_OPENROUTER_BASE_URL", mock.URL)

	env := NewProxyTestEnv(t)
	enableGeminiPublicProxyRoute(t, env)
	// openai-only support forces the gemini request to be converted to OpenAI.
	pid := createORProvider(t, env, "or-imgcfg-gem", "sk-test-key", []string{"openai"})
	createRouteAt(t, env, "gemini", pid, 1)

	req := map[string]any{
		"contents": []map[string]any{{"role": "user", "parts": []map[string]any{{"text": "a cat"}}}},
		"generationConfig": map[string]any{
			"responseModalities": []string{"TEXT", "IMAGE"},
			"imageConfig":        map[string]any{"aspectRatio": "16:9"},
		},
	}
	resp := env.ProxyPost("/v1beta/models/gemini-2.0-flash:generateContent", req, nil)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	_, path, _, up := captured.Get()
	if !strings.Contains(path, "/chat/completions") {
		t.Fatalf("expected conversion to chat/completions, upstream path=%s", path)
	}
	if got := gjson.GetBytes(up, "image_config.aspect_ratio").String(); got != "16:9" {
		t.Errorf("aspect ratio lost across gemini->openai; got %q, body=%s", got, up)
	}
	mods := gjson.GetBytes(up, "modalities").Array()
	hasImage := false
	for _, m := range mods {
		if m.String() == "image" {
			hasImage = true
		}
	}
	if !hasImage {
		t.Errorf("upstream lost image modality; body=%s", up)
	}
}
