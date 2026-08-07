package e2e_test

import (
	"io"
	"net/http"
	"testing"

	"github.com/tidwall/gjson"
)

func TestProviderForceHTTP502EndToEnd(t *testing.T) {
	captured := &capturedRequest{}
	mock := newMockOpenAIUpstream(t, captured)
	defer mock.Close()

	env := NewProxyTestEnv(t)
	providerID := createProvider(t, env, "force-502-provider", mock.URL, []string{"openai"})
	createRoute(t, env, "openai", providerID)

	resp := env.ProxyPost("/v1/chat/completions", openaiRequest("gpt-4o"), nil)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
	_, path, _, _ := captured.Get()
	if path != "/v1/chat/completions" {
		t.Fatalf("upstream path before force 502 = %q, want /v1/chat/completions", path)
	}

	updateResp := env.AdminPut("/api/admin/providers/"+itoa(providerID), map[string]any{
		"id":                   providerID,
		"name":                 "force-502-provider",
		"type":                 "custom",
		"supportedClientTypes": []string{"openai"},
		"forceHTTP502":         true,
		"config": map[string]any{
			"custom": map[string]any{
				"baseURL": mock.URL,
				"apiKey":  "sk-mock-test-key",
			},
		},
	})
	defer updateResp.Body.Close()
	assertStatus(t, updateResp, http.StatusOK)

	resp = env.ProxyPost("/v1/chat/completions", openaiRequest("gpt-4o"), nil)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusBadGateway)
	body, _ := io.ReadAll(resp.Body)
	if got := gjson.GetBytes(body, "error.code").String(); got != "provider_forced_502" {
		t.Fatalf("error.code = %q, want provider_forced_502; body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "error.message").String(); got == "" {
		t.Fatalf("missing error.message; body=%s", body)
	}

	resp = env.ProxyPost("/v1/chat/completions", openaiRequest("gpt-4o"), nil)
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusBadGateway)

	_, pathAfterForce, _, _ := captured.Get()
	if pathAfterForce != path {
		t.Fatalf("upstream was called while force 502 enabled: path changed from %q to %q", path, pathAfterForce)
	}
}
