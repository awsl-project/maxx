package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "github.com/awsl-project/maxx/internal/adapter/provider/custom"
	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
)

func TestUserPanelModelsIncludesCodexOnlyVisibleRoute(t *testing.T) {
	provider := &domain.Provider{
		ID:                   10,
		TenantID:             1,
		Name:                 "codex-only-provider",
		Type:                 "custom",
		SupportedClientTypes: []domain.ClientType{domain.ClientTypeCodex},
		SupportModels:        []string{"codex-visible-model"},
		ExposedModelsEnabled: true,
		ExposedModels:        []string{"codex-visible-model"},
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL: "https://example.invalid/v1",
			APIKey:  "test-key",
		}},
	}
	route := &domain.Route{
		ID:         1,
		TenantID:   1,
		ProviderID: 10,
		ClientType: domain.ClientTypeCodex,
		ProjectID:  0,
		IsEnabled: true,
		Position:  1,
		Weight:    1,
	}
	r, providerRepo := newModelAvailabilityRouter(t, []*domain.Provider{provider}, []*domain.Route{route}, nil)

	apiTokenRepo := &selfServiceAPITokenRepo{tokens: []*domain.APIToken{{
		ID:          1000,
		TenantID:    1,
		Token:       "maxx_user_panel_token",
		TokenPrefix: "maxx_user...",
		Name:        userPanelAPITokenName(9),
		Description: userPanelAPITokenDescription(9),
		IsEnabled:   true,
		CreatedAt:   time.Now(),
	}}}
	handler := newSelfServiceHandlerForTests(selfServiceTestDeps{
		providerRepo: providerRepo,
		settingsRepo: &selfServiceSettingsRepo{values: map[string]string{
			"ui_multitenant_enabled":                         "true",
			"ui_multitenant_layout":                          "user_panel",
			domain.SettingKeyProxyRouteOpenAIChatEnabled:     "false",
			domain.SettingKeyProxyRouteResponsesEnabled:      "true",
			domain.SettingKeyProxyRouteClaudeMessagesEnabled: "false",
			domain.SettingKeyProxyRouteGeminiEnabled:         "false",
		}},
		apiTokenRepo: apiTokenRepo,
	})
	handler.modelsHandler = NewModelsHandler(nil, providerRepo, nil, r)

	rec := httptest.NewRecorder()
	req := newSelfServiceRequest(http.MethodGet, "/user-panel/models")
	req = req.WithContext(maxxctx.WithUserID(req.Context(), 9))
	handler.handleUserPanelModels(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var models []string
	if err := json.Unmarshal(rec.Body.Bytes(), &models); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(models) != 1 || models[0] != "codex-visible-model" {
		t.Fatalf("models = %#v, want codex-visible-model", models)
	}
}
