package cooldown

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestModelLevelCooldownIsolation(t *testing.T) {
	cm := NewManager()
	providerID := uint64(1)

	// Record model-level failure
	cm.RecordFailure(providerID, "gemini", "gemini-2.5-flash-image", ReasonServerError, domain.ScopeModel, nil)

	// This model should be in cooldown
	if !cm.IsInCooldown(providerID, "gemini", "gemini-2.5-flash-image") {
		t.Fatal("expected model to be in cooldown")
	}

	// Different model should NOT be in cooldown
	if cm.IsInCooldown(providerID, "gemini", "gemini-2.5-pro") {
		t.Fatal("different model should not be in cooldown")
	}
}

func TestProviderLevelCooldownBlocksAllModels(t *testing.T) {
	cm := NewManager()
	providerID := uint64(1)

	// Record provider-level failure (network error)
	cm.RecordFailure(providerID, "gemini", "gemini-2.5-flash-image", ReasonNetworkError, domain.ScopeProvider, nil)

	// All models should be in cooldown
	if !cm.IsInCooldown(providerID, "gemini", "gemini-2.5-flash-image") {
		t.Fatal("expected model to be in cooldown")
	}
	if !cm.IsInCooldown(providerID, "gemini", "gemini-2.5-pro") {
		t.Fatal("expected different model to also be in cooldown")
	}
	if !cm.IsInCooldown(providerID, "openai", "gpt-4") {
		t.Fatal("expected different client type to also be in cooldown")
	}
}

func TestKeyLevelCooldownBlocksAllModelsForClientType(t *testing.T) {
	cm := NewManager()
	providerID := uint64(1)

	// Record key-level failure (rate limit)
	cm.RecordFailure(providerID, "gemini", "gemini-2.5-flash-image", ReasonRateLimit, domain.ScopeKey, nil)

	// Same client type, different model should be in cooldown
	if !cm.IsInCooldown(providerID, "gemini", "gemini-2.5-pro") {
		t.Fatal("expected different model same client type to be in cooldown")
	}

	// Different client type should NOT be in cooldown
	if cm.IsInCooldown(providerID, "openai", "gpt-4") {
		t.Fatal("different client type should not be in cooldown")
	}
}

func TestScopeRequestNoCooldown(t *testing.T) {
	cm := NewManager()
	providerID := uint64(1)

	// Record request-level error
	until := cm.RecordFailure(providerID, "gemini", "gemini-2.5-flash-image", ReasonUnknown, domain.ScopeRequest, nil)

	// Should return zero time
	if !until.IsZero() {
		t.Fatal("ScopeRequest should return zero time")
	}

	// Should NOT be in cooldown
	if cm.IsInCooldown(providerID, "gemini", "gemini-2.5-flash-image") {
		t.Fatal("ScopeRequest should not create cooldown")
	}
}

func TestSuccessClearsOnlyModelLevel(t *testing.T) {
	cm := NewManager()
	providerID := uint64(1)

	// Set both model-level and key-level cooldowns
	cm.RecordFailure(providerID, "gemini", "gemini-2.5-flash-image", ReasonServerError, domain.ScopeModel, nil)
	cm.RecordFailure(providerID, "gemini", "", ReasonRateLimit, domain.ScopeKey, nil)

	// Record success for the model
	cm.RecordSuccess(providerID, "gemini", "gemini-2.5-flash-image")

	// Model-level cooldown should be cleared, but key-level should remain
	// So the model is still effectively in cooldown due to key-level
	if !cm.IsInCooldown(providerID, "gemini", "gemini-2.5-flash-image") {
		t.Fatal("expected to still be in cooldown due to key-level")
	}
}
