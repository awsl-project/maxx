package handler

import (
	"net/http"
	"sort"
	"strings"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/pricing"
	"github.com/awsl-project/maxx/internal/repository"
)

// ModelsHandler serves GET /v1/models with a lightweight model list.
type ModelsHandler struct {
	responseModelRepo repository.ResponseModelRepository
	providerRepo      repository.ProviderRepository
	modelMappingRepo  repository.ModelMappingRepository
}

// NewModelsHandler creates a new ModelsHandler.
func NewModelsHandler(
	responseModelRepo repository.ResponseModelRepository,
	providerRepo repository.ProviderRepository,
	modelMappingRepo repository.ModelMappingRepository,
) *ModelsHandler {
	return &ModelsHandler{
		responseModelRepo: responseModelRepo,
		providerRepo:      providerRepo,
		modelMappingRepo:  modelMappingRepo,
	}
}

// ServeHTTP handles GET /v1/models.
func (h *ModelsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	tenantID := maxxctx.GetTenantID(r.Context())
	userAgent := r.Header.Get("User-Agent")
	names, err := h.collectModelNamesForUserAgent(tenantID, userAgent)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if strings.HasPrefix(userAgent, "claude-cli") {
		writeJSON(w, http.StatusOK, buildClaudeModelsResponse(names))
		return
	}

	writeJSON(w, http.StatusOK, buildOpenAIModelsResponse(names))
}

func (h *ModelsHandler) collectModelNames(tenantID uint64) ([]string, error) {
	return h.collectModelNamesForUserAgent(tenantID, "")
}

func (h *ModelsHandler) collectModelNamesForUserAgent(tenantID uint64, userAgent string) ([]string, error) {
	result := make(map[string]struct{})

	if h.responseModelRepo != nil {
		names, err := h.responseModelRepo.ListNames()
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			addModelName(result, name)
		}
	}

	if h.providerRepo != nil {
		providers, err := h.providerRepo.List(tenantID)
		if err != nil {
			return nil, err
		}
		for _, provider := range providers {
			for _, name := range provider.SupportModels {
				addModelName(result, name)
			}
		}
	}

	if h.modelMappingRepo != nil {
		mappings, err := h.modelMappingRepo.ListEnabled(tenantID)
		if err != nil {
			return nil, err
		}
		for _, mapping := range mappings {
			addModelName(result, mapping.Target)
			addModelName(result, mapping.Pattern)
		}
	}

	appendPricingModelNames(result, userAgent)

	names := make([]string, 0, len(result))
	for name := range result {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func appendPricingModelNames(target map[string]struct{}, userAgent string) {
	for _, modelPricing := range pricing.DefaultPriceTable().All() {
		modelID := strings.TrimSpace(modelPricing.ModelID)
		if modelID == "" {
			continue
		}
		if !shouldIncludePricingModelForUserAgent(modelID, userAgent) {
			continue
		}
		addModelName(target, modelID)
	}
}

func shouldIncludePricingModelForUserAgent(modelID, userAgent string) bool {
	modelIDLower := strings.ToLower(strings.TrimSpace(modelID))
	if modelIDLower == "" {
		return false
	}

	userAgentLower := strings.ToLower(strings.TrimSpace(userAgent))
	if userAgentLower == "" {
		return false
	}
	if strings.HasPrefix(userAgentLower, "claude-cli") {
		return strings.HasPrefix(modelIDLower, "claude-")
	}

	return strings.HasPrefix(modelIDLower, "gpt-") ||
		strings.HasPrefix(modelIDLower, "o1") ||
		strings.HasPrefix(modelIDLower, "o3") ||
		strings.HasPrefix(modelIDLower, "o4") ||
		strings.Contains(modelIDLower, "codex")
}

func addModelName(target map[string]struct{}, name string) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return
	}
	if strings.Contains(trimmed, "*") {
		return
	}
	target[trimmed] = struct{}{}
}

func buildOpenAIModelsResponse(names []string) map[string]interface{} {
	data := make([]map[string]interface{}, 0, len(names))
	for _, name := range names {
		data = append(data, map[string]interface{}{
			"id":       name,
			"object":   "model",
			"created":  0,
			"owned_by": "maxx",
		})
	}

	return map[string]interface{}{
		"object": "list",
		"data":   data,
	}
}

func buildClaudeModelsResponse(names []string) map[string]interface{} {
	data := make([]map[string]interface{}, 0, len(names))
	for _, name := range names {
		data = append(data, map[string]interface{}{
			"id":           name,
			"display_name": name,
			"type":         "model",
		})
	}

	return map[string]interface{}{
		"data":     data,
		"has_more": false,
	}
}
