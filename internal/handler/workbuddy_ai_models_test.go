package handler

import (
	"encoding/json"
	"fmt"
	"github.com/awsl-project/maxx/internal/adapter/provider/workbuddyai"
	"github.com/awsl-project/maxx/internal/domain"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type workBuddyAdminStub struct {
	provider *domain.Provider
	updates  int
}

func (s *workBuddyAdminStub) GetProvider(tenant, id uint64) (*domain.Provider, error) {
	return s.provider, nil
}
func (s *workBuddyAdminStub) UpdateProvider(tenant uint64, p *domain.Provider) error {
	s.provider = p
	s.updates++
	return nil
}

func TestWorkBuddyModelsProtectSavedCredentials(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/plugin/auth/token/refresh" {
			fmt.Fprint(w, `{"data":{"accessToken":"new-dummy","refreshToken":"new-refresh","expiresIn":3600}}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer new-dummy" {
			t.Error("discovery did not use refreshed credentials")
		}
		fmt.Fprint(w, `{"data":{"agents":[{"name":"cli","models":["test-model"]}]}}`)
	}))
	defer upstream.Close()
	for _, mode := range []string{"normal", "blackbox", "excluded", "manual"} {
		t.Run(mode, func(t *testing.T) {
			cfg := &domain.ProviderConfigWorkBuddyAI{AccessToken: "old-dummy", RefreshToken: "dummy-" + mode, ExpiresAt: 1}
			admin := &workBuddyAdminStub{provider: &domain.Provider{ID: 1, Type: "workbuddy-ai", BlackBox: mode == "blackbox", ExcludeFromExport: mode == "excluded", Config: &domain.ProviderConfig{WorkBuddyAI: cfg}}}
			h := NewWorkBuddyAIHandler(workbuddyai.NewClient(upstream.URL, upstream.Client())).SetAdminService(admin)
			body := `{"providerID":1}`
			if mode == "manual" {
				b, _ := json.Marshal(cfg)
				body = string(b)
			}
			w := httptest.NewRecorder()
			NoAuthMiddleware(h).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/admin/workbuddy-ai/models", strings.NewReader(body)))
			if w.Code != http.StatusOK {
				t.Fatal(w.Code, w.Body.String())
			}
			var result map[string]json.RawMessage
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			_, hasConfig := result["config"]
			if hasConfig != (mode == "normal" || mode == "manual") {
				t.Fatal("credential disclosure policy", w.Body.String())
			}
			if string(result["models"]) != `["test-model"]` {
				t.Fatal(w.Body.String())
			}
			if mode != "manual" && (admin.updates != 1 || admin.provider.Config.WorkBuddyAI.RefreshToken != "new-refresh") {
				t.Fatal("rotation not persisted")
			}
		})
	}
}
