package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

type fakeResponseModelRepo struct {
	names []string
	err   error
}

func (f *fakeResponseModelRepo) Upsert(name string) error               { return nil }
func (f *fakeResponseModelRepo) BatchUpsert(names []string) error       { return nil }
func (f *fakeResponseModelRepo) List() ([]*domain.ResponseModel, error) { return nil, f.err }
func (f *fakeResponseModelRepo) ListNames() ([]string, error) {
	return append([]string(nil), f.names...), f.err
}

type fakeProviderRepo struct {
	providers []*domain.Provider
	err       error
}

func (f *fakeProviderRepo) Create(provider *domain.Provider) error { return nil }
func (f *fakeProviderRepo) Update(provider *domain.Provider) error { return nil }
func (f *fakeProviderRepo) Delete(tenantID uint64, id uint64) error { return nil }
func (f *fakeProviderRepo) GetByID(tenantID uint64, id uint64) (*domain.Provider, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeProviderRepo) List(tenantID uint64) ([]*domain.Provider, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]*domain.Provider(nil), f.providers...), nil
}

type fakeModelMappingRepo struct {
	mappings []*domain.ModelMapping
	err      error
}

func (f *fakeModelMappingRepo) Create(mapping *domain.ModelMapping) error { return nil }
func (f *fakeModelMappingRepo) Update(mapping *domain.ModelMapping) error { return nil }
func (f *fakeModelMappingRepo) Delete(tenantID uint64, id uint64) error   { return nil }
func (f *fakeModelMappingRepo) GetByID(tenantID uint64, id uint64) (*domain.ModelMapping, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeModelMappingRepo) List(tenantID uint64) ([]*domain.ModelMapping, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]*domain.ModelMapping(nil), f.mappings...), nil
}
func (f *fakeModelMappingRepo) ListEnabled(tenantID uint64) ([]*domain.ModelMapping, error) {
	return f.List(tenantID)
}
func (f *fakeModelMappingRepo) ListByClientType(tenantID uint64, clientType domain.ClientType) ([]*domain.ModelMapping, error) {
	return f.List(tenantID)
}
func (f *fakeModelMappingRepo) ListByQuery(tenantID uint64, query *domain.ModelMappingQuery) ([]*domain.ModelMapping, error) {
	return f.List(tenantID)
}
func (f *fakeModelMappingRepo) Count(tenantID uint64) (int, error) { return len(f.mappings), f.err }
func (f *fakeModelMappingRepo) DeleteAll(tenantID uint64) error    { return nil }
func (f *fakeModelMappingRepo) ClearAll(tenantID uint64) error     { return nil }
func (f *fakeModelMappingRepo) SeedDefaults(tenantID uint64) error { return nil }

func containsModel(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func TestCollectModelNames(t *testing.T) {
	responseRepo := &fakeResponseModelRepo{names: []string{"gpt-1", "gpt-2"}}
	providerRepo := &fakeProviderRepo{
		providers: []*domain.Provider{
			{SupportModels: []string{"gpt-3", "*", " "}},
		},
	}
	mappingRepo := &fakeModelMappingRepo{
		mappings: []*domain.ModelMapping{
			{Pattern: "gpt-4", Target: "gpt-4o"},
			{Pattern: "gpt-*", Target: "gpt-5"},
		},
	}

	handler := NewModelsHandler(responseRepo, providerRepo, mappingRepo)
	names, err := handler.collectModelNames(0)
	if err != nil {
		t.Fatalf("collectModelNames error: %v", err)
	}

	want := []string{"gpt-1", "gpt-2", "gpt-3", "gpt-4", "gpt-4o", "gpt-5"}
	sort.Strings(want)
	if len(names) != len(want) {
		t.Fatalf("model count = %d, want %d", len(names), len(want))
	}
	for i, name := range want {
		if names[i] != name {
			t.Fatalf("names[%d] = %q, want %q", i, names[i], name)
		}
	}
}

func TestModelsHandlerFormats(t *testing.T) {
	responseRepo := &fakeResponseModelRepo{names: []string{"gpt-1"}}
	handler := NewModelsHandler(responseRepo, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("User-Agent", "claude-cli/2.0")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var claudeResp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &claudeResp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := claudeResp["has_more"]; !ok {
		t.Fatalf("claude response missing has_more")
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var openaiResp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &openaiResp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if openaiResp["object"] != "list" {
		t.Fatalf("openai response object = %v, want list", openaiResp["object"])
	}
}

func TestModelsHandlerPricingSupplementByUserAgent(t *testing.T) {
	handler := NewModelsHandler(nil, nil, nil)

	openAIReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	openAIReq.Header.Set("User-Agent", "codex_cli_rs/0.98.0")
	openAIRec := httptest.NewRecorder()
	handler.ServeHTTP(openAIRec, openAIReq)
	if openAIRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", openAIRec.Code)
	}
	var openAIPayload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(openAIRec.Body.Bytes(), &openAIPayload); err != nil {
		t.Fatalf("invalid openai payload: %v", err)
	}
	openAIIDs := make([]string, 0, len(openAIPayload.Data))
	for _, item := range openAIPayload.Data {
		openAIIDs = append(openAIIDs, item.ID)
	}
	if !containsModel(openAIIDs, "gpt-5.3") {
		t.Fatalf("expected gpt-5.3 in openai model list")
	}
	if containsModel(openAIIDs, "claude-opus-4-6") {
		t.Fatalf("did not expect claude pricing-only model in codex model list")
	}

	claudeReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	claudeReq.Header.Set("User-Agent", "claude-cli/2.1.17")
	claudeRec := httptest.NewRecorder()
	handler.ServeHTTP(claudeRec, claudeReq)
	if claudeRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", claudeRec.Code)
	}
	var claudePayload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(claudeRec.Body.Bytes(), &claudePayload); err != nil {
		t.Fatalf("invalid claude payload: %v", err)
	}
	claudeIDs := make([]string, 0, len(claudePayload.Data))
	for _, item := range claudePayload.Data {
		claudeIDs = append(claudeIDs, item.ID)
	}
	if !containsModel(claudeIDs, "claude-opus-4-6") {
		t.Fatalf("expected claude-opus-4-6 in claude model list")
	}
	if containsModel(claudeIDs, "gpt-5.3") {
		t.Fatalf("did not expect gpt-5.3 in claude model list")
	}
}

func TestShouldIncludePricingModelForUserAgentOpenAIOSeriesMatching(t *testing.T) {
	if !shouldIncludePricingModelForUserAgent("o1-mini", "codex_cli_rs/0.98.0") {
		t.Fatalf("expected o1-mini to be included")
	}
	if !shouldIncludePricingModelForUserAgent("o3-mini", "codex_cli_rs/0.98.0") {
		t.Fatalf("expected o3-mini to be included")
	}
	if !shouldIncludePricingModelForUserAgent("o4-mini", "codex_cli_rs/0.98.0") {
		t.Fatalf("expected o4-mini to be included")
	}
	if shouldIncludePricingModelForUserAgent("ollama-foo", "codex_cli_rs/0.98.0") {
		t.Fatalf("did not expect ollama-foo to be included")
	}
}
