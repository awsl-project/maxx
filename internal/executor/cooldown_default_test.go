package executor

import (
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/cooldown"
	"github.com/awsl-project/maxx/internal/domain"
)

type fakeSystemSettingRepo struct{ values map[string]string }

func (r fakeSystemSettingRepo) Get(key string) (string, error) { return r.values[key], nil }
func (r fakeSystemSettingRepo) Set(key, value string) error    { return nil }
func (r fakeSystemSettingRepo) GetAll() ([]*domain.SystemSetting, error) {
	return nil, nil
}
func (r fakeSystemSettingRepo) Delete(key string) error { return nil }

func TestHandleCooldownDefaultDoesNotOverrideQuotaFallback(t *testing.T) {
	provider := &domain.Provider{ID: 99001}
	cooldown.Default().ClearCooldown(provider.ID, string(domain.ClientTypeGemini), "")
	defer cooldown.Default().ClearCooldown(provider.ID, string(domain.ClientTypeGemini), "")

	e := &Executor{settingsRepo: fakeSystemSettingRepo{values: map[string]string{
		domain.SettingKeyRateLimitCooldownDefaultSeconds: "15",
	}}}
	fallbackUntil := time.Now().Add(time.Minute)
	proxyErr := &domain.ProxyError{
		Scope:         domain.ScopeKey,
		Reason:        domain.CooldownReasonQuotaExhausted,
		CooldownUntil: &fallbackUntil,
	}

	e.handleCooldown(proxyErr, provider, domain.ClientTypeGemini, "gemini-2.5-pro")

	until := cooldown.Default().GetCooldownUntil(provider.ID, string(domain.ClientTypeGemini), "gemini-2.5-pro")
	remaining := time.Until(until)
	if remaining < 55*time.Second || remaining > 65*time.Second {
		t.Fatalf("remaining cooldown = %v, want original quota fallback around 60s", remaining)
	}
}

func TestHandleCooldownRateLimitFallbackUsesSettingWhenNoRetryAfter(t *testing.T) {
	provider := &domain.Provider{ID: 99002}
	cooldown.Default().ClearCooldown(provider.ID, string(domain.ClientTypeGemini), "")
	defer cooldown.Default().ClearCooldown(provider.ID, string(domain.ClientTypeGemini), "")

	e := &Executor{settingsRepo: fakeSystemSettingRepo{values: map[string]string{
		domain.SettingKeyRateLimitCooldownDefaultSeconds: "15",
	}}}
	proxyErr := &domain.ProxyError{
		Scope:  domain.ScopeKey,
		Reason: domain.CooldownReasonRateLimitExceeded,
	}

	e.handleCooldown(proxyErr, provider, domain.ClientTypeGemini, "gemini-2.5-pro")

	until := cooldown.Default().GetCooldownUntil(provider.ID, string(domain.ClientTypeGemini), "gemini-2.5-pro")
	remaining := time.Until(until)
	if remaining < 10*time.Second || remaining > 20*time.Second {
		t.Fatalf("remaining cooldown = %v, want configurable rate-limit fallback around 15s", remaining)
	}
}

func TestHandleCooldownConcurrentLimitFallbackUsesSettingWhenNoRetryAfter(t *testing.T) {
	provider := &domain.Provider{ID: 99003}
	cooldown.Default().ClearCooldown(provider.ID, string(domain.ClientTypeGemini), "")
	defer cooldown.Default().ClearCooldown(provider.ID, string(domain.ClientTypeGemini), "")

	e := &Executor{settingsRepo: fakeSystemSettingRepo{values: map[string]string{
		domain.SettingKeyRateLimitCooldownDefaultSeconds: "15",
	}}}
	proxyErr := &domain.ProxyError{
		Scope:  domain.ScopeKey,
		Reason: domain.CooldownReasonConcurrentLimit,
	}

	e.handleCooldown(proxyErr, provider, domain.ClientTypeGemini, "gemini-2.5-pro")

	until := cooldown.Default().GetCooldownUntil(provider.ID, string(domain.ClientTypeGemini), "gemini-2.5-pro")
	remaining := time.Until(until)
	if remaining < 10*time.Second || remaining > 20*time.Second {
		t.Fatalf("remaining cooldown = %v, want configurable concurrent-limit fallback around 15s", remaining)
	}
}

func TestHandleCooldownRateLimitUsesRetryAfterBeforeSettingsDefault(t *testing.T) {
	provider := &domain.Provider{ID: 99004}
	cooldown.Default().ClearCooldown(provider.ID, string(domain.ClientTypeGemini), "")
	defer cooldown.Default().ClearCooldown(provider.ID, string(domain.ClientTypeGemini), "")

	e := &Executor{settingsRepo: fakeSystemSettingRepo{values: map[string]string{
		domain.SettingKeyRateLimitCooldownDefaultSeconds: "15",
	}}}
	proxyErr := &domain.ProxyError{
		Scope:      domain.ScopeKey,
		Reason:     domain.CooldownReasonRateLimitExceeded,
		RetryAfter: 3 * time.Second,
	}

	e.handleCooldown(proxyErr, provider, domain.ClientTypeGemini, "gemini-2.5-pro")

	until := cooldown.Default().GetCooldownUntil(provider.ID, string(domain.ClientTypeGemini), "gemini-2.5-pro")
	remaining := time.Until(until)
	if remaining < 2*time.Second || remaining > 5*time.Second {
		t.Fatalf("remaining cooldown = %v, want RetryAfter around 3s and not settings default", remaining)
	}
}
