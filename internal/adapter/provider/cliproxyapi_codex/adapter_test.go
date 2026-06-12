package cliproxyapi_codex

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestNewAdapterSetsBaseURLAttribute(t *testing.T) {
	provider := &domain.Provider{
		Name: "codex",
		Config: &domain.ProviderConfig{
			Codex: &domain.ProviderConfigCodex{
				RefreshToken: "refresh-token",
				BaseURL:      " https://mock.example.test/codex ",
			},
		},
	}

	adapterIface, err := NewAdapter(provider)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}

	adapter, ok := adapterIface.(*CLIProxyAPICodexAdapter)
	if !ok {
		t.Fatalf("expected *CLIProxyAPICodexAdapter, got %T", adapterIface)
	}

	if got := adapter.authObj.Attributes["base_url"]; got != "https://mock.example.test/codex" {
		t.Fatalf("expected base_url attribute to be trimmed provider base URL, got %q", got)
	}
}
