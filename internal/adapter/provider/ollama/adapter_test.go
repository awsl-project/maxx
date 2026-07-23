package ollama

import (
	"testing"

	"github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/domain"
)

func TestNewAdapter_ForcesBackendWithoutMutatingOriginal(t *testing.T) {
	p := &domain.Provider{
		Name: "local-ollama",
		Type: "ollama",
		Config: &domain.ProviderConfig{
			Custom: &domain.ProviderConfigCustom{
				BaseURL: "http://localhost:11434",
			},
		},
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeClaude},
	}

	a, err := NewAdapter(p)
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}

	// The original provider entity must be untouched (it may be shared/cached).
	if p.Config.Custom.Backend != "" {
		t.Fatalf("original provider backend mutated: %q", p.Config.Custom.Backend)
	}

	// The synthesized provider drives the custom core with backend=ollama.
	got := a.(*Adapter)
	if got.synth.Config.Custom.Backend != ollamaBackend {
		t.Fatalf("synth backend = %q, want %q", got.synth.Config.Custom.Backend, ollamaBackend)
	}
	if got.synth.Config.Custom == p.Config.Custom {
		t.Fatalf("synth custom config must be a copy, not the shared pointer")
	}
}

func TestNewAdapter_MissingCustomConfig(t *testing.T) {
	if _, err := NewAdapter(&domain.Provider{Name: "x", Type: "ollama", Config: &domain.ProviderConfig{}}); err == nil {
		t.Fatal("expected error for missing custom config")
	}
}

func TestOllamaFactoryRegistered(t *testing.T) {
	if _, ok := provider.GetAdapterFactory("ollama"); !ok {
		t.Fatal("ollama adapter factory not registered")
	}
}
