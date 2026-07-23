package newapi

import (
	"testing"

	"github.com/awsl-project/maxx/internal/adapter/provider"
	"github.com/awsl-project/maxx/internal/domain"
)

func TestNewAdapter_RequiresCustomConfig(t *testing.T) {
	if _, err := NewAdapter(&domain.Provider{Name: "x", Type: "newapi", Config: &domain.ProviderConfig{}}); err == nil {
		t.Fatal("expected error for missing custom config")
	}

	p := &domain.Provider{
		Name:                 "example-relay",
		Type:                 "newapi",
		Config:               &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{BaseURL: "https://example.com"}},
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeOpenAI},
	}
	a, err := NewAdapter(p)
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	if len(a.SupportedClientTypes()) != 1 || a.SupportedClientTypes()[0] != domain.ClientTypeOpenAI {
		t.Fatalf("SupportedClientTypes = %v", a.SupportedClientTypes())
	}
}

func TestNewAPIFactoryRegistered(t *testing.T) {
	if _, ok := provider.GetAdapterFactory("newapi"); !ok {
		t.Fatal("newapi adapter factory not registered")
	}
}
