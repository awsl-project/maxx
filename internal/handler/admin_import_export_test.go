package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/service"
)

type adminTestProviderRepo struct {
	providers []*domain.Provider
}

func (r *adminTestProviderRepo) Create(provider *domain.Provider) error {
	provider.ID = uint64(len(r.providers) + 1)
	r.providers = append(r.providers, provider)
	return nil
}

func (r *adminTestProviderRepo) Update(provider *domain.Provider) error {
	for i, p := range r.providers {
		if p.ID == provider.ID {
			r.providers[i] = provider
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *adminTestProviderRepo) Delete(tenantID uint64, id uint64) error {
	for i, p := range r.providers {
		if p.ID == id {
			r.providers = append(r.providers[:i], r.providers[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (r *adminTestProviderRepo) GetByID(tenantID uint64, id uint64) (*domain.Provider, error) {
	for _, p := range r.providers {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *adminTestProviderRepo) List(tenantID uint64) ([]*domain.Provider, error) {
	cloned := make([]*domain.Provider, len(r.providers))
	copy(cloned, r.providers)
	return cloned, nil
}

func newAdminHandlerForProviderImportExportTests(providerRepo *adminTestProviderRepo) *AdminHandler {
	adminSvc := service.NewAdminService(
		providerRepo,
		nil,
		nil,
		nil,
		nil,
		nil,
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

	return NewAdminHandler(adminSvc, nil, "")
}

func TestAdminHandler_ProvidersImport_WithTrailingSlash(t *testing.T) {
	providerRepo := &adminTestProviderRepo{}
	h := newAdminHandlerForProviderImportExportTests(providerRepo)

	body, err := json.Marshal([]map[string]any{{
		"name": "imported-provider",
		"type": "custom",
	}})
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/providers/import/", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var result service.ImportResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if result.Imported != 1 {
		t.Fatalf("imported = %d, want 1", result.Imported)
	}
	if len(providerRepo.providers) != 1 {
		t.Fatalf("provider count = %d, want 1", len(providerRepo.providers))
	}
}

func TestAdminHandler_ProvidersExport_WithTrailingSlash(t *testing.T) {
	providerRepo := &adminTestProviderRepo{
		providers: []*domain.Provider{{
			ID:   1,
			Name: "exported-provider",
			Type: "custom",
		}},
	}
	h := newAdminHandlerForProviderImportExportTests(providerRepo)

	req := httptest.NewRequest(http.MethodGet, "/admin/providers/export/", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	contentDisposition := rec.Header().Get("Content-Disposition")
	if contentDisposition != "attachment; filename=providers.json" {
		t.Fatalf("Content-Disposition = %q, want attachment header", contentDisposition)
	}

	var providers []domain.Provider
	if err := json.Unmarshal(rec.Body.Bytes(), &providers); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(providers) != 1 || providers[0].Name != "exported-provider" {
		t.Fatalf("providers = %+v, want one exported-provider", providers)
	}
}
