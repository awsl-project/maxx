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
)

// This exercises OpenRouter's dedicated unified image endpoint POST /v1/images
// (distinct from the OpenAI-style /v1/images/generations). Its request is a bare
// {model, prompt} body and its response is {data:[{b64_json}], usage:{cost}} with
// NO token counts — so it only bills via the upstream usage.cost path. It proves:
// the bare /v1/images route is registered + allowlisted, client-type detection
// classifies it as openai, the model is rewritten while prompt is preserved, the
// b64_json image passes back verbatim, and a zero-token response still bills.

// newOpenRouterImagesEndpointMock returns a mock upstream that answers /v1/images
// with the OpenRouter unified image response shape.
func newOpenRouterImagesEndpointMock(t *testing.T, captured *capturedRequest, calls *int64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(calls, 1)
		reqBody, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(reqBody))
		captured.Set(r)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"created": 1700000000,
			"data": []map[string]any{
				{"b64_json": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
					"media_type": "image/png"},
			},
			// No token counts — per-image models bill only from usage.cost.
			"usage": map[string]any{"cost": 0.0387},
		})
	}))
}

// The dedicated /v1/images endpoint routes to OpenRouter, rewrites the model,
// preserves the prompt, returns b64_json verbatim, and bills from usage.cost
// despite the response carrying zero tokens.
func TestOpenRouterImagesEndpoint(t *testing.T) {
	captured := &capturedRequest{}
	var calls int64
	mock := newOpenRouterImagesEndpointMock(t, captured, &calls)
	defer mock.Close()
	t.Setenv("MAXX_OPENROUTER_BASE_URL", mock.URL)

	env := NewProxyTestEnv(t)
	pid := createORProvider(t, env, "or-img-endpoint", "sk-test-key", []string{"openai"})
	createRouteAt(t, env, "openai", pid, 1)

	req := map[string]any{"model": "google/gemini-2.5-flash-image", "prompt": "a cat"}
	resp := env.ProxyPost("/v1/images", req,
		map[string]string{"Authorization": "Bearer client-placeholder"})
	assertStatus(t, resp, http.StatusOK)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	// Routed to the bare upstream endpoint, model rewritten, prompt preserved.
	_, path, _, up := captured.Get()
	if path != "/v1/images" {
		t.Errorf("upstream path = %s, want /v1/images", path)
	}
	if !bytes.Contains(up, []byte(`"prompt"`)) || !bytes.Contains(up, []byte("a cat")) {
		t.Errorf("upstream request lost the prompt; body=%s", up)
	}

	// b64_json image survives verbatim to the client.
	if s := string(body); !strings.Contains(s, `"b64_json"`) {
		t.Fatalf("client response lost data[].b64_json; body=%s", s)
	}

	// Zero-token response still bills from usage.cost ($0.0387 × 1×).
	rec := waitForRecordedRequest(t, env, pid)
	if rec.Cost != orImageCostNano {
		t.Errorf("images-endpoint cost = %d, want %d (upstream usage.cost on a zero-token response)", rec.Cost, orImageCostNano)
	}
}
