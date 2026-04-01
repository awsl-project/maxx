package e2e_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestCodexRequestFallsBackWithoutPersistedCodexConfig(t *testing.T) {
	captured := &capturedRequest{}
	mock := newMockCodexUpstream(t, captured)
	defer mock.Close()

	env := NewProxyTestEnv(t)

	resp := env.AdminPost("/api/admin/providers", map[string]any{
		"name": "Codex Fallback",
		"type": "codex",
		"config": map[string]any{
			"codex": map[string]any{
				"baseURL": mock.URL,
			},
		},
		"supportedClientTypes": []string{"codex"},
		"supportModels":        []string{"*"},
	})
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create provider failed: status=%d body=%s", resp.StatusCode, body)
	}
	var provider struct {
		ID uint64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&provider); err != nil {
		t.Fatalf("decode provider: %v", err)
	}
	resp.Body.Close()

	createRoute(t, env, "codex", provider.ID)

	proxyResp := env.ProxyPost("/responses", codexRequest("gpt-4o"), nil)
	defer proxyResp.Body.Close()
	assertStatus(t, proxyResp, http.StatusOK)

	_, path, headers, _ := captured.Get()
	if path != "/responses" && path != "/responses/compact" {
		t.Fatalf("expected upstream /responses or /responses/compact, got %s", path)
	}
	if got := headers.Get("Authorization"); got == "" {
		t.Fatalf("expected Authorization header to be synthesized for fallback flow")
	}

	getResp := env.doRequest(http.MethodGet, "/api/admin/providers/"+itoa(provider.ID), nil, env.Token)
	if getResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(getResp.Body)
		t.Fatalf("get provider failed: status=%d body=%s", getResp.StatusCode, body)
	}
	var stored domain.Provider
	if err := json.NewDecoder(getResp.Body).Decode(&stored); err != nil {
		t.Fatalf("decode stored provider: %v", err)
	}
	getResp.Body.Close()

	if stored.Config == nil || stored.Config.Codex == nil {
		t.Fatalf("expected codex config to exist after fallback")
	}
	if stored.Config.Codex.AccessToken == "" {
		t.Fatalf("expected fallback access token to be persisted")
	}
}

func itoa(n uint64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
