package core

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

type proxyRouteSettingsRepo struct {
	values map[string]string
	err    error
}

func (r proxyRouteSettingsRepo) Get(key string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	return r.values[key], nil
}

func (r proxyRouteSettingsRepo) Set(key, value string) error { return nil }

func (r proxyRouteSettingsRepo) GetAll() ([]*domain.SystemSetting, error) { return nil, nil }

func (r proxyRouteSettingsRepo) Delete(key string) error { return nil }

func TestRegisterProxyRoutes_RegistersModelsRoute(t *testing.T) {
	mux := http.NewServeMux()
	calledPaths := make([]string, 0, 2)

	RegisterProxyRoutes(mux, ProxyRouteHandlers{
		ModelsHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calledPaths = append(calledPaths, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		}),
	})

	for _, path := range []string{"/v1/models", "/v1beta/models"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s status = %d, want %d", path, rec.Code, http.StatusNoContent)
		}
	}
	wantPaths := map[string]bool{"/v1/models": false, "/v1beta/models": false}
	for _, path := range calledPaths {
		if _, ok := wantPaths[path]; ok {
			wantPaths[path] = true
		}
	}
	for path, called := range wantPaths {
		if !called {
			t.Fatalf("models handler calls = %v, missing %s", calledPaths, path)
		}
	}
}

func TestRegisterProxyRoutes_RoutesGeminiGenerationToProxy(t *testing.T) {
	mux := http.NewServeMux()
	proxyCalled := false

	RegisterProxyRoutes(mux, ProxyRouteHandlers{
		ProxyHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			proxyCalled = true
			w.WriteHeader(http.StatusNoContent)
		}),
		ModelsHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatalf("did not expect Gemini generation path to hit ModelsHandler")
		}),
		SettingRepo: proxyRouteSettingsRepo{values: map[string]string{
			domain.SettingKeyProxyRouteGeminiEnabled: "true",
		}},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if !proxyCalled {
		t.Fatal("expected Gemini generation path to be routed to ProxyHandler")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestRegisterProxyRoutes_RoutesVideosToProxy(t *testing.T) {
	mux := http.NewServeMux()
	calledPaths := make([]string, 0, 2)

	RegisterProxyRoutes(mux, ProxyRouteHandlers{
		ProxyHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calledPaths = append(calledPaths, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		}),
	})

	for _, path := range []string{"/v1/videos", "/v1/videos/task_abc123"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s status = %d, want %d", path, rec.Code, http.StatusNoContent)
		}
	}
	wantPaths := map[string]bool{"/v1/videos": false, "/v1/videos/task_abc123": false}
	for _, path := range calledPaths {
		if _, ok := wantPaths[path]; ok {
			wantPaths[path] = true
		}
	}
	for path, called := range wantPaths {
		if !called {
			t.Fatalf("proxy handler calls = %v, missing %s", calledPaths, path)
		}
	}
}

func TestRegisterProxyRoutes_GeminiGenerationEnabledByDefault(t *testing.T) {
	mux := http.NewServeMux()
	proxyCalled := false

	RegisterProxyRoutes(mux, ProxyRouteHandlers{
		ProxyHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			proxyCalled = true
			w.WriteHeader(http.StatusNoContent)
		}),
		SettingRepo: proxyRouteSettingsRepo{values: map[string]string{}},
	})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent", nil))

	if !proxyCalled {
		t.Fatal("expected default-enabled Gemini path to hit ProxyHandler")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestRegisterProxyRoutes_DisablesConfiguredProxyRouteGroups(t *testing.T) {
	tests := []struct {
		name        string
		settingKey  string
		disabledURL string
		allowedURL  string
	}{
		{
			name:        "claude messages",
			settingKey:  domain.SettingKeyProxyRouteClaudeMessagesEnabled,
			disabledURL: "/v1/messages",
			allowedURL:  "/v1/chat/completions",
		},
		{
			name:        "openai chat completions",
			settingKey:  domain.SettingKeyProxyRouteOpenAIChatEnabled,
			disabledURL: "/v1/chat/completions",
			allowedURL:  "/v1/messages",
		},
		{
			name:        "responses",
			settingKey:  domain.SettingKeyProxyRouteResponsesEnabled,
			disabledURL: "/v1/responses",
			allowedURL:  "/v1/messages",
		},
		{
			name:        "gemini",
			settingKey:  domain.SettingKeyProxyRouteGeminiEnabled,
			disabledURL: "/v1beta/models/gemini-2.5-pro:generateContent",
			allowedURL:  "/v1/messages",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux := http.NewServeMux()
			proxyCalls := 0

			RegisterProxyRoutes(mux, ProxyRouteHandlers{
				ProxyHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					proxyCalls++
					w.WriteHeader(http.StatusNoContent)
				}),
				SettingRepo: proxyRouteSettingsRepo{values: map[string]string{tt.settingKey: "false"}},
			})

			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, tt.disabledURL, nil))
			if rec.Code != http.StatusNotFound {
				t.Fatalf("disabled route status = %d, want %d", rec.Code, http.StatusNotFound)
			}

			rec = httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, tt.allowedURL, nil))
			if rec.Code != http.StatusNoContent {
				t.Fatalf("allowed route status = %d, want %d", rec.Code, http.StatusNoContent)
			}
			if proxyCalls != 1 {
				t.Fatalf("proxyCalls = %d, want 1", proxyCalls)
			}
		})
	}
}

func TestRegisterProxyRoutes_ProxyRouteGateDefaultsOpen(t *testing.T) {
	for _, settings := range []proxyRouteSettingsRepo{
		{values: map[string]string{}},
		{err: assertAnError{}},
	} {
		mux := http.NewServeMux()
		RegisterProxyRoutes(mux, ProxyRouteHandlers{
			ProxyHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}),
			SettingRepo: settings,
		})

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/messages", nil))
		if rec.Code != http.StatusNoContent {
			t.Fatalf("default-open route status = %d, want %d", rec.Code, http.StatusNoContent)
		}
	}
}

type assertAnError struct{}

func (assertAnError) Error() string { return "assertion error" }
