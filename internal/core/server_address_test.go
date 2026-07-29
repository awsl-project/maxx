package core

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

type serverAddressSettingsRepo struct {
	values map[string]string
}

func (r serverAddressSettingsRepo) Get(key string) (string, error) {
	return r.values[key], nil
}
func (r serverAddressSettingsRepo) Set(key, value string) error              { return nil }
func (r serverAddressSettingsRepo) GetAll() ([]*domain.SystemSetting, error) { return nil, nil }
func (r serverAddressSettingsRepo) Delete(key string) error                  { return nil }

func TestLANAccessEnabledDefaultsToTrue(t *testing.T) {
	if !LANAccessEnabled(serverAddressSettingsRepo{}) {
		t.Fatal("LANAccessEnabled() = false, want true for missing setting")
	}
}

func TestResolveServerBindAddressUsesLoopbackWhenLANDisabled(t *testing.T) {
	settings := serverAddressSettingsRepo{values: map[string]string{domain.SettingKeyLANAccessEnabled: "false"}}
	if got, want := ResolveServerBindAddress(":9880", settings, false), "127.0.0.1:9880"; got != want {
		t.Fatalf("ResolveServerBindAddress() = %q, want %q", got, want)
	}
}

func TestResolveServerBindAddressPreservesExplicitAddress(t *testing.T) {
	settings := serverAddressSettingsRepo{values: map[string]string{domain.SettingKeyLANAccessEnabled: "false"}}
	if got, want := ResolveServerBindAddress(":9880", settings, true), ":9880"; got != want {
		t.Fatalf("ResolveServerBindAddress() = %q, want %q", got, want)
	}
}

func TestResolveServerBindAddressKeepsLANDefaultBind(t *testing.T) {
	settings := serverAddressSettingsRepo{values: map[string]string{domain.SettingKeyLANAccessEnabled: "true"}}
	if got, want := ResolveServerBindAddress(":9880", settings, false), ":9880"; got != want {
		t.Fatalf("ResolveServerBindAddress() = %q, want %q", got, want)
	}
}
