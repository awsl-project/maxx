package e2e_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	cliproxycodex "github.com/awsl-project/maxx/internal/adapter/provider/cliproxyapi_codex"
	"github.com/awsl-project/maxx/internal/domain"
)

func TestCodexRequestFallsBackWithoutPersistedCodexConfig(t *testing.T) {
	captured := &capturedRequest{}
	mock := newMockCodexUpstream(t, captured)
	defer mock.Close()

	env := NewProxyTestEnv(t)
	providerID := createCodexProvider(t, env, map[string]any{
		"baseURL": mock.URL,
	})
	createRoute(t, env, "codex", providerID)

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

	stored := getCodexProvider(t, env, providerID)
	if stored.Config == nil || stored.Config.Codex == nil {
		t.Fatalf("expected codex config to exist after fallback")
	}
	if stored.Config.Codex.AccessToken == "" {
		t.Fatalf("expected fallback access token to be persisted")
	}
	if stored.Config.Codex.ExpiresAt == "" {
		t.Fatalf("expected fallback token to have a short expiry sentinel")
	}
}

func TestCLIProxyAPICodexFallbackRefreshRecovery(t *testing.T) {
	refreshHits := 0
	refreshServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refreshHits++
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse refresh form: %v", err)
		}
		if got := r.Form.Get("refresh_token"); got != "refresh-after-fallback" {
			t.Fatalf("unexpected refresh token: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"real-access-token","refresh_token":"refresh-after-fallback","expires_in":3600,"token_type":"Bearer"}`)
	}))
	defer refreshServer.Close()

	originalTokenURL := cliproxycodex.GetOpenAITokenURLForTest()
	cliproxycodex.SetOpenAITokenURLForTest(refreshServer.URL)
	defer cliproxycodex.SetOpenAITokenURLForTest(originalTokenURL)

	provider := &domain.Provider{
		ID:   42,
		Name: "Codex CLIProxy Fallback",
		Type: "codex",
		Config: &domain.ProviderConfig{
			Codex: &domain.ProviderConfigCodex{
				UseCLIProxyAPI: true,
			},
		},
	}

	adapterIface, err := cliproxycodex.NewAdapter(provider)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	adapter := adapterIface.(*cliproxycodex.CLIProxyAPICodexAdapter)
	adapter.SetProviderUpdateFunc(func(updated *domain.Provider) error {
		provider = updated
		return nil
	})

	if err := adapter.WarmToken(t.Context()); err != nil {
		t.Fatalf("warm fallback token: %v", err)
	}
	if provider.Config.Codex.AccessToken == "" || provider.Config.Codex.ExpiresAt == "" {
		t.Fatalf("expected persisted fallback token with short expiry sentinel, got %+v", provider.Config.Codex)
	}

	provider.Config.Codex.RefreshToken = "refresh-after-fallback"
	provider.Config.Codex.ExpiresAt = time.Now().Add(-time.Minute).Format(time.RFC3339)

	if err := adapter.WarmToken(t.Context()); err != nil {
		t.Fatalf("warm refreshed token: %v", err)
	}
	if refreshHits == 0 {
		t.Fatalf("expected CLIProxyAPI adapter to refresh token after refresh token was restored")
	}
	if provider.Config.Codex.AccessToken != "real-access-token" {
		t.Fatalf("expected refreshed access token to replace fallback token, got %q", provider.Config.Codex.AccessToken)
	}
}

func createCodexProvider(t *testing.T, env *ProxyTestEnv, codexConfig map[string]any) uint64 {
	t.Helper()
	resp := env.AdminPost("/api/admin/providers", map[string]any{
		"name": "Codex Fallback",
		"type": "codex",
		"config": map[string]any{
			"codex": codexConfig,
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
	return provider.ID
}

func getCodexProvider(t *testing.T, env *ProxyTestEnv, providerID uint64) domain.Provider {
	t.Helper()
	getResp := env.doRequest(http.MethodGet, "/api/admin/providers/"+itoa(providerID), nil, env.Token)
	if getResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(getResp.Body)
		t.Fatalf("get provider failed: status=%d body=%s", getResp.StatusCode, body)
	}
	var stored domain.Provider
	if err := json.NewDecoder(getResp.Body).Decode(&stored); err != nil {
		t.Fatalf("decode stored provider: %v", err)
	}
	getResp.Body.Close()
	return stored
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
