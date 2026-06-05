package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

type fakeProjectRepo struct {
	project *domain.Project
}

func (f *fakeProjectRepo) Create(project *domain.Project) error    { return nil }
func (f *fakeProjectRepo) Update(project *domain.Project) error    { return nil }
func (f *fakeProjectRepo) Delete(tenantID uint64, id uint64) error { return nil }
func (f *fakeProjectRepo) GetByID(tenantID uint64, id uint64) (*domain.Project, error) {
	return f.project, nil
}
func (f *fakeProjectRepo) GetBySlug(tenantID uint64, slug string) (*domain.Project, error) {
	return f.project, nil
}
func (f *fakeProjectRepo) List(tenantID uint64) ([]*domain.Project, error) {
	if f.project == nil {
		return nil, nil
	}
	return []*domain.Project{f.project}, nil
}

type fakeProviderByIDRepo struct {
	provider *domain.Provider
}

func (f *fakeProviderByIDRepo) Create(provider *domain.Provider) error  { return nil }
func (f *fakeProviderByIDRepo) Update(provider *domain.Provider) error  { return nil }
func (f *fakeProviderByIDRepo) Delete(tenantID uint64, id uint64) error { return nil }
func (f *fakeProviderByIDRepo) GetByID(tenantID uint64, id uint64) (*domain.Provider, error) {
	return f.provider, nil
}
func (f *fakeProviderByIDRepo) List(tenantID uint64) ([]*domain.Provider, error) {
	if f.provider == nil {
		return nil, nil
	}
	return []*domain.Provider{f.provider}, nil
}

func assertGeminiModelsPayload(t *testing.T, body []byte) {
	t.Helper()
	var payload struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("invalid Gemini models payload: %v", err)
	}
	if len(payload.Models) != 1 || payload.Models[0].Name != "models/gpt-1" {
		t.Fatalf("Gemini models payload = %+v, want only models/gpt-1", payload.Models)
	}
}

func TestParseProviderPath(t *testing.T) {
	h := &ProviderProxyHandler{}
	providerID, apiPath, ok := h.parseProviderPath("/provider/1/v1/chat/completions")
	if !ok {
		t.Fatal("expected provider path to parse")
	}
	if providerID != "1" {
		t.Fatalf("providerID = %q, want 1", providerID)
	}
	if apiPath != "/v1/chat/completions" {
		t.Fatalf("apiPath = %q, want /v1/chat/completions", apiPath)
	}
}

func TestParseProviderPath_TrimsProviderID(t *testing.T) {
	h := &ProviderProxyHandler{}
	providerID, apiPath, ok := h.parseProviderPath("/provider/ 1 /v1/messages")
	if !ok {
		t.Fatal("expected provider path to parse")
	}
	if providerID != "1" {
		t.Fatalf("providerID = %q, want 1", providerID)
	}
	if apiPath != "/v1/messages" {
		t.Fatalf("apiPath = %q, want /v1/messages", apiPath)
	}
}

func TestIsValidProviderAPIPath_AllowsExactAndSubpathsOnly(t *testing.T) {
	valid := []string{
		"/v1/messages",
		"/v1/messages/stream",
		"/v1/chat/completions",
		"/v1/chat/completions/extra",
		"/responses",
		"/responses/items",
		"/v1/responses",
		"/v1/responses/abc",
		"/v1/models",
		"/v1/models/list",
		"/v1beta/models",
		"/v1beta/models/gemini-2.5-pro",
	}
	for _, path := range valid {
		if !isValidProviderAPIPath(path) {
			t.Fatalf("expected %q to be valid", path)
		}
	}

	invalid := []string{
		"/v1/messages-debug",
		"/v1/chat/completionsXYZ",
		"/responses123",
		"/v1/responsesXYZ",
		"/v1/models-debug",
		"/v1beta/modelsX",
	}
	for _, path := range invalid {
		if isValidProviderAPIPath(path) {
			t.Fatalf("expected %q to be invalid", path)
		}
	}
}

func TestIsProviderProxyPath(t *testing.T) {
	if !isProviderProxyPath("/provider/1/v1/messages") {
		t.Fatal("expected provider path to be detected")
	}
	if isProviderProxyPath("/project/demo/v1/messages") {
		t.Fatal("did not expect project path to be detected as provider path")
	}
	if isProviderProxyPath("/providers") {
		t.Fatal("did not expect regular web route to be detected")
	}
}

func TestIsModelListAPIPath(t *testing.T) {
	for _, path := range []string{"/v1/models", "/v1beta/models"} {
		if !isModelListAPIPath(path) {
			t.Fatalf("expected %q to be a model-list API path", path)
		}
	}

	for _, path := range []string{
		"/v1/models/list",
		"/v1beta/models/gemini-2.5-pro:generateContent",
		"/v1beta/models/gemini-2.5-pro:streamGenerateContent",
	} {
		if isModelListAPIPath(path) {
			t.Fatalf("did not expect %q to be a model-list API path", path)
		}
	}
}

func TestProjectAPIPathAllowsExactGeminiModelList(t *testing.T) {
	if !isValidAPIPath("/v1beta/models") {
		t.Fatal("expected exact Gemini model-list path to be valid for project proxy URLs")
	}
}

// TestProjectAPIPathAllowsImagesEndpoints pins the contract that
// /v1/images/generations and /v1/images/edits work under the /project/<slug>/
// prefix and nothing else under /v1/images/ leaks through. proxy_routes.go
// only registers those two endpoints at the root mux; this whitelist must
// stay equally tight, otherwise project-scoped routes become more permissive
// than the root contract.
func TestProjectAPIPathAllowsImagesEndpoints(t *testing.T) {
	for _, path := range []string{"/v1/images/generations", "/v1/images/edits"} {
		if !isValidAPIPath(path) {
			t.Fatalf("expected %q to be valid for project proxy URLs", path)
		}
	}
	for _, path := range []string{
		"/v1/images",
		"/v1/images/",
		"/v1/images/variations",
		"/v1/images/generations/extra",
		"/v1/images/random",
	} {
		if isValidAPIPath(path) {
			t.Fatalf("did not expect %q to pass the project proxy whitelist", path)
		}
	}
}

// TestProviderAPIPathAllowsImagesEndpoints pins the same contract for the
// sibling /provider/<id>/ prefix: only the two registered endpoints, no
// broader prefix match.
func TestProviderAPIPathAllowsImagesEndpoints(t *testing.T) {
	for _, path := range []string{"/v1/images/generations", "/v1/images/edits"} {
		if !isValidProviderAPIPath(path) {
			t.Fatalf("expected %q to be valid for provider proxy URLs", path)
		}
	}
	for _, path := range []string{
		"/v1/images",
		"/v1/images/",
		"/v1/images/variations",
		"/v1/images/generations/extra",
		"/v1/images/random",
	} {
		if isValidProviderAPIPath(path) {
			t.Fatalf("did not expect %q to pass the provider proxy whitelist", path)
		}
	}
}

func TestProjectProxyRoutesGeminiModelListToModelsHandler(t *testing.T) {
	modelsHandler := NewModelsHandler(&fakeResponseModelRepo{names: []string{"gpt-1"}}, nil, nil)
	handler := NewProjectProxyHandler(nil, modelsHandler, &fakeProjectRepo{
		project: &domain.Project{ID: 42, Name: "Demo", Slug: "demo"},
	})

	req := httptest.NewRequest(http.MethodGet, "/project/demo/v1beta/models", nil)
	req.Header.Set("User-Agent", "claude-cli/2.0")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	assertGeminiModelsPayload(t, rec.Body.Bytes())
}

func TestProviderProxyRoutesGeminiModelListToModelsHandler(t *testing.T) {
	modelsHandler := NewModelsHandler(&fakeResponseModelRepo{names: []string{"gpt-1"}}, nil, nil)
	handler := NewProviderProxyHandler(nil, modelsHandler, &fakeProviderByIDRepo{
		provider: &domain.Provider{ID: 1, Name: "Provider"},
	}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/provider/1/v1beta/models", nil)
	req.Header.Set("User-Agent", "claude-cli/2.0")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	assertGeminiModelsPayload(t, rec.Body.Bytes())
}
