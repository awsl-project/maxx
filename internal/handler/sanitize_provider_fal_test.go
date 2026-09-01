package handler

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

// TestSanitizeProviderRedactsFalAPIKey guards against leaking the fal provider's
// full "id:secret" API key through the admin provider API. Before the fal branch
// existed in sanitizeProvider, GET /api/admin/providers/{id} returned
// config.fal.apiKey in cleartext.
func TestSanitizeProviderRedactsFalAPIKey(t *testing.T) {
	provider := &domain.Provider{
		Type:                 "fal",
		SupportModels:        []string{"fal-ai/flux/dev"},
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeVideo},
		Config: &domain.ProviderConfig{
			Fal: &domain.ProviderConfigFal{APIKey: "id:secret"},
		},
	}

	sanitized := sanitizeProvider(provider)
	if sanitized == nil {
		t.Fatal("sanitizeProvider returned nil")
	}
	if sanitized.Config == nil || sanitized.Config.Fal == nil {
		t.Fatal("sanitized fal config missing")
	}
	if got := sanitized.Config.Fal.APIKey; got != "" {
		t.Fatalf("expected fal APIKey to be redacted, got %q", got)
	}

	// Non-secret fields must survive sanitization.
	if sanitized.Type != "fal" {
		t.Fatalf("expected type %q, got %q", "fal", sanitized.Type)
	}
	if len(sanitized.SupportModels) != 1 || sanitized.SupportModels[0] != "fal-ai/flux/dev" {
		t.Fatalf("supportModels not preserved: %v", sanitized.SupportModels)
	}
	if len(sanitized.SupportedClientTypes) != 1 || sanitized.SupportedClientTypes[0] != domain.ClientTypeVideo {
		t.Fatalf("supportedClientTypes not preserved: %v", sanitized.SupportedClientTypes)
	}

	// Original provider must not be mutated.
	if provider.Config.Fal.APIKey != "id:secret" {
		t.Fatalf("original provider APIKey mutated: %q", provider.Config.Fal.APIKey)
	}
}
