package service

import (
	"errors"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

type publicProxyRouteSettingRepo struct {
	values map[string]string
}

func (r *publicProxyRouteSettingRepo) Get(key string) (string, error) {
	if r.values == nil {
		return "", nil
	}
	return r.values[key], nil
}

func (r *publicProxyRouteSettingRepo) Set(key, value string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	r.values[key] = value
	return nil
}

func (r *publicProxyRouteSettingRepo) GetAll() ([]*domain.SystemSetting, error) {
	return nil, nil
}

func (r *publicProxyRouteSettingRepo) Delete(key string) error {
	delete(r.values, key)
	return nil
}

func newPublicProxyRouteSettingsService(repo *publicProxyRouteSettingRepo) *AdminService {
	return NewAdminService(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		repo,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		"",
		nil,
		nil,
		nil,
	)
}

func TestUpdateSettingRejectsDisablingLastPublicProxyRoute(t *testing.T) {
	repo := &publicProxyRouteSettingRepo{values: map[string]string{
		domain.SettingKeyProxyRouteClaudeMessagesEnabled: "false",
		domain.SettingKeyProxyRouteOpenAIChatEnabled:     "false",
		domain.SettingKeyProxyRouteResponsesEnabled:      "false",
		domain.SettingKeyProxyRouteGeminiEnabled:         "true",
	}}
	svc := newPublicProxyRouteSettingsService(repo)

	err := svc.UpdateSetting(domain.SettingKeyProxyRouteGeminiEnabled, "false")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("error = %v, want ErrInvalidInput", err)
	}
	if got := repo.values[domain.SettingKeyProxyRouteGeminiEnabled]; got != "true" {
		t.Fatalf("gemini setting = %q, want unchanged true", got)
	}
}

func TestUpdateSettingTreatsUnsetGeminiAsDisabledByDefault(t *testing.T) {
	repo := &publicProxyRouteSettingRepo{values: map[string]string{
		domain.SettingKeyProxyRouteClaudeMessagesEnabled: "false",
		domain.SettingKeyProxyRouteOpenAIChatEnabled:     "false",
		domain.SettingKeyProxyRouteResponsesEnabled:      "true",
	}}
	svc := newPublicProxyRouteSettingsService(repo)

	err := svc.UpdateSetting(domain.SettingKeyProxyRouteResponsesEnabled, "false")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("error = %v, want ErrInvalidInput", err)
	}
	if got := repo.values[domain.SettingKeyProxyRouteResponsesEnabled]; got != "true" {
		t.Fatalf("responses setting = %q, want unchanged true", got)
	}
}

func TestDeleteSettingRejectsDeletingExplicitLastEnabledGemini(t *testing.T) {
	repo := &publicProxyRouteSettingRepo{values: map[string]string{
		domain.SettingKeyProxyRouteClaudeMessagesEnabled: "false",
		domain.SettingKeyProxyRouteOpenAIChatEnabled:     "false",
		domain.SettingKeyProxyRouteResponsesEnabled:      "false",
		domain.SettingKeyProxyRouteGeminiEnabled:         "true",
	}}
	svc := newPublicProxyRouteSettingsService(repo)

	err := svc.DeleteSetting(domain.SettingKeyProxyRouteGeminiEnabled)
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("error = %v, want ErrInvalidInput", err)
	}
	if got := repo.values[domain.SettingKeyProxyRouteGeminiEnabled]; got != "true" {
		t.Fatalf("gemini setting = %q, want unchanged true", got)
	}
}

func TestDeleteSettingAllowsDeletingDisabledRouteWhenDefaultsKeepAnotherEnabled(t *testing.T) {
	repo := &publicProxyRouteSettingRepo{values: map[string]string{
		domain.SettingKeyProxyRouteClaudeMessagesEnabled: "false",
		domain.SettingKeyProxyRouteGeminiEnabled:         "false",
	}}
	svc := newPublicProxyRouteSettingsService(repo)

	if err := svc.DeleteSetting(domain.SettingKeyProxyRouteClaudeMessagesEnabled); err != nil {
		t.Fatalf("DeleteSetting returned error: %v", err)
	}
	if _, ok := repo.values[domain.SettingKeyProxyRouteClaudeMessagesEnabled]; ok {
		t.Fatalf("claude setting still exists after delete")
	}
}

func TestUpdateSettingAllowsDisablingPublicProxyRouteWhenAnotherRemainsEnabled(t *testing.T) {
	repo := &publicProxyRouteSettingRepo{values: map[string]string{
		domain.SettingKeyProxyRouteClaudeMessagesEnabled: "false",
		domain.SettingKeyProxyRouteOpenAIChatEnabled:     "true",
		domain.SettingKeyProxyRouteResponsesEnabled:      "false",
		domain.SettingKeyProxyRouteGeminiEnabled:         "true",
	}}
	svc := newPublicProxyRouteSettingsService(repo)

	if err := svc.UpdateSetting(domain.SettingKeyProxyRouteGeminiEnabled, "false"); err != nil {
		t.Fatalf("UpdateSetting returned error: %v", err)
	}
	if got := repo.values[domain.SettingKeyProxyRouteGeminiEnabled]; got != "false" {
		t.Fatalf("gemini setting = %q, want false", got)
	}
}

func TestUpdateSettingAllowsRestoringPublicProxyRoute(t *testing.T) {
	repo := &publicProxyRouteSettingRepo{values: map[string]string{
		domain.SettingKeyProxyRouteClaudeMessagesEnabled: "false",
		domain.SettingKeyProxyRouteOpenAIChatEnabled:     "false",
		domain.SettingKeyProxyRouteResponsesEnabled:      "false",
		domain.SettingKeyProxyRouteGeminiEnabled:         "true",
	}}
	svc := newPublicProxyRouteSettingsService(repo)

	if err := svc.UpdateSetting(domain.SettingKeyProxyRouteClaudeMessagesEnabled, "true"); err != nil {
		t.Fatalf("UpdateSetting returned error: %v", err)
	}
	if got := repo.values[domain.SettingKeyProxyRouteClaudeMessagesEnabled]; got != "true" {
		t.Fatalf("claude setting = %q, want true", got)
	}
}
