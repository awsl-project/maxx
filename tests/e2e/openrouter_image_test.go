package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// These tests verify that OpenRouter's Gemini image generation works through the
// first-class openrouter provider. OpenRouter does image generation via the
// OpenAI Chat Completions endpoint: the client sends modalities:["image","text"]
// and the upstream returns the generated image in the NON-STANDARD
// choices[].message.images[] array (a data URL), with the image token count in
// usage.completion_tokens_details.image_tokens. Since the openrouter adapter is
// pure passthrough (openai client → openai provider, no format conversion), the
// key questions are: (1) does the request's modalities field survive the model
// rewrite round-trip, (2) does the response's images[] array survive back to the
// client untouched, for both non-stream and stream, and (3) is the image token
// usage extracted so billing is correct.

const orTinyPNG = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

// orImageCostNano is OpenRouter's reported usage.cost ($0.0387) in nanoUSD at the
// default 1× multiplier — what the request should be billed once upstream-cost
// billing is applied end-to-end.
const orImageCostNano uint64 = 38_700_000

// newOpenRouterImageMock returns a mock OpenRouter upstream that shapes an
// image-generation response: message.images[] for non-stream, an images delta
// for stream, both carrying completion_tokens_details.image_tokens.
func newOpenRouterImageMock(t *testing.T, captured *capturedRequest, calls *int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(calls, 1)
		reqBody, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(reqBody))
		captured.Set(r)

		if bytes.Contains(reqBody, []byte(`"stream":true`)) {
			writeOpenRouterImageStream(t, w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "gen_img", "object": "chat.completion",
			"model": "google/gemini-2.5-flash-image", "created": 1700000000,
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "Here is your image",
					"images": []map[string]any{
						{"type": "image_url", "image_url": map[string]any{"url": orTinyPNG}},
					},
				},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{
				"prompt_tokens": 12, "completion_tokens": 1300, "total_tokens": 1312,
				"completion_tokens_details": map[string]any{"image_tokens": 1290},
				// OpenRouter's authoritative charge ($0.0387) — billing source of truth.
				"cost": 0.0387,
			},
		})
	}))
}

// writeOpenRouterImageStream emits OpenRouter's image-generation SSE: the image
// arrives in an images[] delta, then a final chunk carries usage with
// completion_tokens_details.image_tokens, then [DONE].
func writeOpenRouterImageStream(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	flusher, ok := w.(http.Flusher)
	if !ok {
		t.Error("mock upstream: streaming not supported by ResponseWriter")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	frames := []string{
		`data: {"id":"gen_img","object":"chat.completion.chunk","model":"google/gemini-2.5-flash-image","choices":[{"index":0,"delta":{"role":"assistant","images":[{"type":"image_url","image_url":{"url":"` + orTinyPNG + `"}}]}}]}` + "\n\n",
		`data: {"id":"gen_img","object":"chat.completion.chunk","model":"google/gemini-2.5-flash-image","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":1300,"total_tokens":1312,"completion_tokens_details":{"image_tokens":1290},"cost":0.0387}}` + "\n\n",
		"data: [DONE]\n\n",
	}
	for _, f := range frames {
		if _, err := io.WriteString(w, f); err != nil {
			t.Errorf("mock upstream: write SSE frame: %v", err)
			return
		}
		flusher.Flush()
	}
}

// openaiImageRequest is an OpenAI Chat Completions body asking for an image, the
// exact shape a client uses against OpenRouter's Gemini image model.
func openaiImageRequest(model string) map[string]any {
	return map[string]any{
		"model":      model,
		"modalities": []string{"image", "text"},
		"messages": []map[string]any{
			{"role": "user", "content": "draw a cat"},
		},
	}
}

type recordedRequest struct {
	OutputTokenCount uint64 `json:"outputTokenCount"`
	Cost             uint64 `json:"cost"`
}

// waitForRecordedRequest polls the admin requests list for the latest request on a
// provider until it has been billed (nonzero tokens OR cost — usage recording is
// async, and per-image responses carry only a cost), returning that record
// (zero-value if it never lands within the timeout).
func waitForRecordedRequest(t *testing.T, env *ProxyTestEnv, providerID uint64) recordedRequest {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp := env.AdminGet(fmt.Sprintf("/api/admin/requests?limit=5&providerId=%d", providerID))
		var out struct {
			Items []recordedRequest `json:"items"`
		}
		DecodeJSON(t, resp, &out)
		if len(out.Items) > 0 && (out.Items[0].OutputTokenCount > 0 || out.Items[0].Cost > 0) {
			return out.Items[0]
		}
		time.Sleep(50 * time.Millisecond)
	}
	return recordedRequest{}
}

// Non-stream image generation: the request's modalities survives to the upstream,
// and the response's non-standard images[] data URL is passed back to the client
// untouched. Also asserts image token usage is captured (output tokens recorded).
func TestOpenRouterImageNonStream(t *testing.T) {
	captured := &capturedRequest{}
	var calls int64
	mock := newOpenRouterImageMock(t, captured, &calls)
	defer mock.Close()
	t.Setenv("MAXX_OPENROUTER_BASE_URL", mock.URL)

	env := NewProxyTestEnv(t)
	pid := createORProvider(t, env, "or-img", "sk-test-key", []string{"openai"})
	createRouteAt(t, env, "openai", pid, 1)

	resp := env.ProxyPost("/v1/chat/completions", openaiImageRequest("google/gemini-2.5-flash-image"),
		map[string]string{"Authorization": "Bearer client-placeholder"})
	assertStatus(t, resp, http.StatusOK)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	// Request passthrough: modalities must survive the model-rewrite round-trip,
	// otherwise OpenRouter never generates an image.
	_, path, _, up := captured.Get()
	if path != "/v1/chat/completions" {
		t.Errorf("upstream path = %s, want /v1/chat/completions", path)
	}
	if !bytes.Contains(up, []byte(`"modalities"`)) || !bytes.Contains(up, []byte(`"image"`)) {
		t.Errorf("upstream request lost modalities:[image]; body=%s", up)
	}

	// Response passthrough: the non-standard images[] array with the data URL
	// must reach the client untouched.
	s := string(body)
	if !strings.Contains(s, `"images"`) {
		t.Fatalf("client response lost message.images[]; body=%s", s)
	}
	if !strings.Contains(s, orTinyPNG) {
		t.Fatalf("client response lost the image data URL; body=%s", s)
	}

	// Usage + billing: the image response's token usage is recorded AND the
	// image-token usage is recorded AND the cost equals OpenRouter's authoritative
	// usage.cost ($0.0387) at the default 1× multiplier — proving upstream-cost
	// billing end-to-end (extract → attempt column → FinalizeAttemptCost → mirror).
	rec := waitForRecordedRequest(t, env, pid)
	if rec.OutputTokenCount == 0 {
		t.Errorf("image response output token usage was not recorded")
	}
	if rec.Cost != orImageCostNano {
		t.Errorf("image response cost = %d, want %d (upstream usage.cost $0.0387 × 1×)", rec.Cost, orImageCostNano)
	}
}

// Streaming image generation: the image arrives in an SSE images[] delta and must
// stream back to the client untouched, ending in [DONE]; usage is recorded.
func TestOpenRouterImageStream(t *testing.T) {
	captured := &capturedRequest{}
	var calls int64
	mock := newOpenRouterImageMock(t, captured, &calls)
	defer mock.Close()
	t.Setenv("MAXX_OPENROUTER_BASE_URL", mock.URL)

	env := NewProxyTestEnv(t)
	pid := createORProvider(t, env, "or-img-stream", "sk-test-key", []string{"openai"})
	createRouteAt(t, env, "openai", pid, 1)

	req := openaiImageRequest("google/gemini-2.5-flash-image")
	req["stream"] = true
	resp := env.ProxyPost("/v1/chat/completions", req,
		map[string]string{"Authorization": "Bearer client-placeholder"})
	assertStatus(t, resp, http.StatusOK)
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("stream Content-Type = %q, want text/event-stream", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if _, _, _, up := captured.Get(); !bytes.Contains(up, []byte(`"modalities"`)) {
		t.Errorf("upstream stream request lost modalities; body=%s", up)
	}
	s := string(body)
	if !strings.Contains(s, `"images"`) || !strings.Contains(s, orTinyPNG) {
		t.Fatalf("streamed image delta did not reach the client; body=%s", s)
	}
	if !strings.Contains(s, "[DONE]") {
		t.Fatalf("expected SSE ending in [DONE]; body=%s", s)
	}
	rec := waitForRecordedRequest(t, env, pid)
	if rec.OutputTokenCount == 0 {
		t.Errorf("streamed image response output token usage was not recorded")
	}
	if rec.Cost != orImageCostNano {
		t.Errorf("streamed image response cost = %d, want %d (upstream usage.cost from final SSE chunk)", rec.Cost, orImageCostNano)
	}
}
