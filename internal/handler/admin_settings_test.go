package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestHandleSettingsReturnsBadRequestForInvalidPayloadOverrideRules(t *testing.T) {
	repo := &settingsTestRepo{}
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
		"",
		nil,
		nil,
		nil,
	)
	handler := NewAdminHandler(svc, nil, "")

	req := httptest.NewRequest(
		http.MethodPut,
		"/admin/settings/"+domain.SettingKeyPayloadOverrideRules,
		strings.NewReader(`{"value":"[{\"models\":[{\"name\":\"gpt-5.4\",\"protocol\":\"codex\"}],\"params\":{}}]"}`),
	)
	rec := httptest.NewRecorder()

	handler.handleSettings(rec, req, []string{"admin", "settings", domain.SettingKeyPayloadOverrideRules})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}
