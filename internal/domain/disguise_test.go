package domain

import (
	"encoding/json"
	"testing"
)

// TestProviderConfigCustomLegacyCloakRoundTrips proves that an old persisted
// custom-provider config (with the legacy `cloak` JSON field but no `disguise`)
// still survives unmarshalling and that ResolveDisguise migrates it onto the
// new shape — preserving user-set fields like StrictMode and SensitiveWords.
func TestProviderConfigCustomLegacyCloakRoundTrips(t *testing.T) {
	raw := []byte(`{
        "baseURL": "https://relay.example.com",
        "apiKey": "sk-old",
        "cloak": {
            "mode": "always",
            "strictMode": true,
            "sensitiveWords": ["secret", "internal"]
        }
    }`)

	var cfg ProviderConfigCustom
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cfg.LegacyCloak == nil {
		t.Fatal("expected LegacyCloak to be populated from legacy `cloak` JSON field")
	}
	if cfg.LegacyCloak.Mode != "always" {
		t.Errorf("LegacyCloak.Mode = %q, want always", cfg.LegacyCloak.Mode)
	}
	if !cfg.LegacyCloak.StrictMode {
		t.Error("LegacyCloak.StrictMode should be true")
	}
	if len(cfg.LegacyCloak.SensitiveWords) != 2 {
		t.Errorf("LegacyCloak.SensitiveWords len = %d, want 2", len(cfg.LegacyCloak.SensitiveWords))
	}

	// Disguise itself is not set in legacy data.
	if cfg.Disguise != nil {
		t.Error("legacy config should leave Disguise nil")
	}

	// ResolveDisguise must migrate it transparently.
	resolved := cfg.ResolveDisguise()
	if resolved == nil {
		t.Fatal("ResolveDisguise returned nil for a legacy cloak config")
	}
	if resolved.Type != DisguiseTypeClaudeCode {
		t.Errorf("ResolveDisguise type = %q, want %q", resolved.Type, DisguiseTypeClaudeCode)
	}
	if resolved.ClaudeCode == nil {
		t.Fatal("ResolveDisguise should expose ClaudeCode sub-options")
	}
	if resolved.ClaudeCode.Mode != "always" {
		t.Errorf("migrated Mode = %q, want always", resolved.ClaudeCode.Mode)
	}
	if !resolved.ClaudeCode.StrictMode {
		t.Error("migrated StrictMode should be true")
	}
	if len(resolved.ClaudeCode.SensitiveWords) != 2 {
		t.Errorf("migrated SensitiveWords len = %d, want 2", len(resolved.ClaudeCode.SensitiveWords))
	}
}

// TestProviderConfigCustomDisguisePreferredOverLegacy verifies that when both
// `disguise` and the legacy `cloak` field are present, ResolveDisguise prefers
// the new field. This matters for the half-migrated state where a new save
// would normally drop `cloak`, but a manual edit (or external tool) might leave
// both behind.
func TestProviderConfigCustomDisguisePreferredOverLegacy(t *testing.T) {
	cfg := ProviderConfigCustom{
		Disguise: &ProviderConfigCustomDisguise{Type: DisguiseTypeBedrock},
		LegacyCloak: &DisguiseClaudeCodeOptions{
			Mode:       "always",
			StrictMode: true,
		},
	}
	resolved := cfg.ResolveDisguise()
	if resolved == nil || resolved.Type != DisguiseTypeBedrock {
		t.Errorf("expected bedrock disguise to win over legacy cloak, got %+v", resolved)
	}
}

func TestProviderConfigCustomResolveDisguiseNilSafe(t *testing.T) {
	var cfg *ProviderConfigCustom
	if got := cfg.ResolveDisguise(); got != nil {
		t.Errorf("ResolveDisguise on nil receiver should return nil, got %+v", got)
	}

	empty := &ProviderConfigCustom{}
	if got := empty.ResolveDisguise(); got != nil {
		t.Errorf("ResolveDisguise on empty config should return nil, got %+v", got)
	}
}
