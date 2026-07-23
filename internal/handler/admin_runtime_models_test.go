package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestFetchProviderRuntimeModels_CustomUsesOpenAIClientOverride(t *testing.T) {
	baseHit := false
	base := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		baseHit = true
		t.Fatalf("base URL should not be used when openai client override is set")
	}))
	defer base.Close()

	openAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %s, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Fatalf("Authorization = %q, want Bearer sk-test", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4o"},{"id":"gpt-4.1"}]}`))
	}))
	defer openAI.Close()

	provider := &domain.Provider{
		Type: "custom",
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{
			BaseURL: base.URL,
			APIKey:  "sk-test",
			ClientBaseURL: map[domain.ClientType]string{
				domain.ClientTypeOpenAI: openAI.URL,
			},
		}},
	}

	result := (&AdminHandler{}).fetchProviderRuntimeModels(httptest.NewRequest(http.MethodPost, "/", nil), provider)
	if baseHit {
		t.Fatal("base URL was hit")
	}
	if !result.Available {
		t.Fatalf("result not available: %#v", result)
	}
	if len(result.Models) != 2 || result.Models[0] != "gpt-4.1" || result.Models[1] != "gpt-4o" {
		t.Fatalf("models = %#v", result.Models)
	}
}

func TestFetchProviderRuntimeModels_CustomFallsBackToBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %s, want /v1/models", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"base-model"}]}`))
	}))
	defer server.Close()

	provider := &domain.Provider{
		Type:   "custom",
		Config: &domain.ProviderConfig{Custom: &domain.ProviderConfigCustom{BaseURL: server.URL}},
	}

	result := (&AdminHandler{}).fetchProviderRuntimeModels(httptest.NewRequest(http.MethodPost, "/", nil), provider)
	if !result.Available {
		t.Fatalf("result not available: %#v", result)
	}
	if len(result.Models) != 1 || result.Models[0] != "base-model" {
		t.Fatalf("models = %#v", result.Models)
	}
}
