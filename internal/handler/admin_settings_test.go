package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/service"
)

type settingsTestRepo struct {
	values map[string]string
}

func (r *settingsTestRepo) Get(key string) (string, error) {
	if r.values == nil {
		return "", nil
	}
	return r.values[key], nil
}

func (r *settingsTestRepo) Set(key, value string) error {
	if r.values == nil {
		r.values = make(map[string]string)
	}
	r.values[key] = value
	return nil
}

func (r *settingsTestRepo) GetAll() ([]*domain.SystemSetting, error) {
	return nil, nil
}

func (r *settingsTestRepo) Delete(key string) error {
	delete(r.values, key)
	return nil
}

func TestHandleSettingsDeleteReturnsBadRequestWhenDeletingLastPublicProxyRoute(t *testing.T) {
	repo := &settingsTestRepo{values: map[string]string{
		domain.SettingKeyProxyRouteClaudeMessagesEnabled: "false",
		domain.SettingKeyProxyRouteOpenAIChatEnabled:     "false",
		domain.SettingKeyProxyRouteResponsesEnabled:      "false",
		domain.SettingKeyProxyRouteGeminiEnabled:         "true",
	}}
	svc := service.NewAdminService(
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
	handler := NewAdminHandler(svc, nil, "")

	req := httptest.NewRequest(
		http.MethodDelete,
		"/admin/settings/"+domain.SettingKeyProxyRouteGeminiEnabled,
		nil,
	)
	rec := httptest.NewRecorder()

	handler.handleSettings(rec, req, []string{"admin", "settings", domain.SettingKeyProxyRouteGeminiEnabled})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if got := repo.values[domain.SettingKeyProxyRouteGeminiEnabled]; got != "true" {
		t.Fatalf("gemini setting = %q, want unchanged true", got)
	}
}
