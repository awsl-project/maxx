package executor

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/awsl-project/maxx/internal/converter"
	"github.com/awsl-project/maxx/internal/domain"
)

func TestOpenRouterCodexBridge_EndToEndMockUpstream(t *testing.T) {
	var capturedPath string
	var capturedBody []byte
	mockOpenRouter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedBody, _ = io.ReadAll(r.Body)
		if capturedPath != "/v1/chat/completions" {
			t.Fatalf("upstream path = %q, want /v1/chat/completions", capturedPath)
		}

		var got converter.OpenAIRequest
		if err := json.Unmarshal(capturedBody, &got); err != nil {
			t.Fatalf("upstream received invalid OpenAI request JSON: %v\n%s", err, capturedBody)
		}
		if got.Model != "openrouter/codex-target" || !got.Stream {
			t.Fatalf("unexpected model/stream at mock upstream: %#v", got)
		}
		if len(got.Messages) != 3 {
			t.Fatalf("messages len = %d, want 3: %#v", len(got.Messages), got.Messages)
		}
		wantRoles := []string{"system", "system", "user"}
		for i, want := range wantRoles {
			if got.Messages[i].Role != want {
				t.Fatalf("message[%d].role = %q, want %q; messages=%#v", i, got.Messages[i].Role, want, got.Messages)
			}
		}
		if len(got.Tools) != 1 {
			t.Fatalf("tools len = %d, want only one function tool after dropping Responses-only built-ins: %#v", len(got.Tools), got.Tools)
		}
		if got.Tools[0].Type != "function" || got.Tools[0].Function.Name != "shell" {
			t.Fatalf("unexpected function tool at mock upstream: %#v", got.Tools[0])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_mock","object":"chat.completion","created":1,"model":"openrouter/codex-target","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9}}`))
	}))
	defer mockOpenRouter.Close()

	provider := &domain.Provider{
		Type: "custom",
		Name: "openrouter",
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL: mockOpenRouter.URL,
			ClientBaseURL: map[domain.ClientType]string{
				domain.ClientTypeOpenAI: mockOpenRouter.URL,
				domain.ClientTypeCodex:  "https://openrouter.ai/api/v1",
			},
		}},
	}
	if !shouldBridgeCustomCodexViaOpenAI(provider, domain.ClientTypeCodex, []domain.ClientType{domain.ClientTypeCodex, domain.ClientTypeOpenAI}) {
		t.Fatal("expected OpenRouter-compatible custom provider to bridge Codex through OpenAI")
	}

	codexReq := converter.CodexRequest{
		Model:        "codex-source",
		Instructions: "be careful",
		Input: []interface{}{
			map[string]interface{}{"type": "message", "role": "developer", "content": "follow project rules"},
			map[string]interface{}{"type": "message", "role": "user", "content": "run pwd"},
		},
		Tools: []converter.CodexTool{{
			Type:        "function",
			Name:        "shell",
			Description: "run a shell command",
			Parameters:  map[string]interface{}{"type": "object"},
		}, {
			Type: "web_search",
		}, {
			Type: "image_generation",
		}},
	}
	body, err := json.Marshal(codexReq)
	if err != nil {
		t.Fatal(err)
	}

	convertedBody, err := converter.GetGlobalRegistry().TransformRequest(domain.ClientTypeCodex, domain.ClientTypeOpenAI, body, "openrouter/codex-target", true)
	if err != nil {
		t.Fatalf("TransformRequest: %v", err)
	}
	convertedURI := ConvertRequestURI("/responses", domain.ClientTypeCodex, domain.ClientTypeOpenAI, "openrouter/codex-target", true)
	if convertedURI != "/v1/chat/completions" {
		t.Fatalf("converted URI = %q, want /v1/chat/completions", convertedURI)
	}

	resp, err := http.Post(mockOpenRouter.URL+convertedURI, "application/json", bytes.NewReader(convertedBody))
	if err != nil {
		t.Fatalf("mock upstream post: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("mock upstream status = %d body=%s", resp.StatusCode, respBody)
	}
	if len(capturedBody) == 0 {
		t.Fatal("mock upstream did not capture converted request body")
	}
}
