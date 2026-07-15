package cliproxyapi_grok

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestNewAdapterAcceptsCPAExportedXAIOAuthJSONShape(t *testing.T) {
	provider := &domain.Provider{
		ID:   42,
		Type: "grok",
		Name: "Grok Test",
		Config: &domain.ProviderConfig{Grok: &domain.ProviderConfigGrok{
			Type:          "xai",
			AuthKind:      "oauth",
			Email:         "xai39b5bb@jh.actionvspot.com",
			Sub:           "88ca5464-ae36-48c5-a7b8-de306101d07f",
			AccessToken:   "access-token",
			RefreshToken:  "refresh-token",
			IDToken:       "id-token",
			TokenType:     "Bearer",
			ExpiresIn:     21600,
			Expired:       "2026-07-11T21:01:55Z",
			LastRefresh:   "2026-07-11T15:01:56Z",
			RedirectURI:   "http://127.0.0.1:56121/callback",
			TokenEndpoint: "https://auth.x.ai/oauth2/token",
			BaseURL:       "https://cli-chat-proxy.grok.com/v1",
			Headers: map[string]string{
				"X-XAI-Token-Auth":      "xai-grok-cli",
				"x-grok-client-version": "0.2.93",
			},
		}},
	}

	adapter, err := NewAdapter(provider)
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}
	grok, ok := adapter.(*CLIProxyAPIGrokAdapter)
	if !ok {
		t.Fatalf("adapter type = %T, want *CLIProxyAPIGrokAdapter", adapter)
	}
	if got := grok.authObj.Provider; got != "xai" {
		t.Fatalf("auth provider = %q, want xai", got)
	}
	if got := grok.authObj.Metadata["access_token"]; got != "access-token" {
		t.Fatalf("access_token metadata = %v", got)
	}
	if got := grok.authObj.Metadata["refresh_token"]; got != "refresh-token" {
		t.Fatalf("refresh_token metadata = %v", got)
	}
	if got := grok.authObj.Metadata["base_url"]; got != "https://cli-chat-proxy.grok.com/v1" {
		t.Fatalf("base_url metadata = %v", got)
	}
}
