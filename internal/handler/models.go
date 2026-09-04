package handler

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/pricing"
	"github.com/awsl-project/maxx/internal/repository"
	"github.com/awsl-project/maxx/internal/router"
)

// ModelsHandler serves model-list endpoints with a lightweight model list.
type ModelsHandler struct {
	responseModelRepo repository.ResponseModelRepository
	providerRepo      repository.ProviderRepository
	modelMappingRepo  repository.ModelMappingRepository
	router            *router.Router
}

// NewModelsHandler creates a new ModelsHandler.
func NewModelsHandler(
	responseModelRepo repository.ResponseModelRepository,
	providerRepo repository.ProviderRepository,
	modelMappingRepo repository.ModelMappingRepository,
	availabilityRouter ...*router.Router,
) *ModelsHandler {
	var r *router.Router
	if len(availabilityRouter) > 0 {
		r = availabilityRouter[0]
	}
	return &ModelsHandler{
		responseModelRepo: responseModelRepo,
		providerRepo:      providerRepo,
		modelMappingRepo:  modelMappingRepo,
		router:            r,
	}
}

// ServeHTTP handles model-list requests.
func (h *ModelsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	tenantID := maxxctx.GetTenantID(r.Context())
	userAgent := r.Header.Get("User-Agent")
	isGeminiModels := isGeminiModelsPath(r.URL.Path)
	clientType := modelListClientType(r)
	projectID := modelListProjectID(r)
	providerID := modelListProviderID(r)
	apiTokenID := maxxctx.GetAPITokenID(r.Context())

	var names []string
	var err error
	if isGeminiModels {
		names, err = h.collectAvailableModelNames(tenantID, clientType, projectID, providerID, apiTokenID, "")
	} else {
		names, err = h.collectAvailableModelNames(tenantID, clientType, projectID, providerID, apiTokenID, userAgent)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if isGeminiModels {
		writeJSON(w, http.StatusOK, buildGeminiModelsResponse(names))
		return
	}

	if strings.HasPrefix(userAgent, "claude-cli") {
		writeJSON(w, http.StatusOK, buildClaudeModelsResponse(names))
		return
	}

	writeJSON(w, http.StatusOK, buildOpenAIModelsResponse(names))
}

func isModelListAPIPath(path string) bool {
	return path == "/v1/models" || isGeminiModelsPath(path)
}

func isGeminiModelsPath(path string) bool {
	return path == "/v1beta/models"
}

func (h *ModelsHandler) collectModelNames(tenantID uint64) ([]string, error) {
	return h.collectModelNamesForUserAgent(tenantID, "")
}

func (h *ModelsHandler) collectModelNamesForUserAgent(tenantID uint64, userAgent string) ([]string, error) {
	candidates, err := h.collectCandidateModelNames(tenantID, userAgent)
	if err != nil {
		return nil, err
	}
	return sortedModelNames(candidates), nil
}

func (h *ModelsHandler) collectAvailableModelNames(tenantID uint64, clientType domain.ClientType, projectID, providerID, apiTokenID uint64, userAgent string) ([]string, error) {
	candidates, err := h.collectCandidateModelNames(tenantID, userAgent)
	if err != nil {
		return nil, err
	}
	if h.router == nil {
		return sortedModelNames(candidates), nil
	}

	available := make(map[string]struct{})
	for name := range candidates {
		if h.isModelAvailable(tenantID, clientType, projectID, providerID, apiTokenID, name) {
			available[name] = struct{}{}
		}
	}
	return sortedModelNames(available), nil
}

func (h *ModelsHandler) collectCandidateModelNames(tenantID uint64, userAgent string) (map[string]struct{}, error) {
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
			if provider.ExposedModelsEnabled {
				for _, name := range provider.ExposedModels {
					addModelName(result, name)
				}
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
	return result, nil
}

func (h *ModelsHandler) isModelAvailable(tenantID uint64, clientType domain.ClientType, projectID, providerID, apiTokenID uint64, model string) bool {
	if h.router == nil || clientType == "" || model == "" {
		return false
	}
	result, err := h.router.Match(&router.MatchContext{
		TenantID:     tenantID,
		ClientType:   clientType,
		ProjectID:    projectID,
		RequestModel: model,
		ModelCandidates: func(route *domain.Route, provider *domain.Provider, clientType domain.ClientType, requestModel string) []string {
			return h.modelCandidatesForRoute(tenantID, requestModel, route, provider, clientType, projectID, apiTokenID)
		},
		APITokenID:          apiTokenID,
		StrictSupportModels: true,
	})
	if err != nil || result == nil {
		return false
	}
	for _, matched := range result.Routes {
		if matched == nil || matched.Provider == nil {
			continue
		}
		if providerID != 0 && matched.Provider.ID != providerID {
			continue
		}
		candidates := h.modelCandidatesForRoute(tenantID, model, matched.Route, matched.Provider, clientType, projectID, apiTokenID)
		if isProviderAnyModelExposed(matched.Provider, append([]string{model}, candidates...)) {
			return true
		}
	}
	return false
}

func (h *ModelsHandler) modelCandidatesForRoute(tenantID uint64, requestModel string, route *domain.Route, provider *domain.Provider, clientType domain.ClientType, projectID, apiTokenID uint64) []string {
	if h == nil || h.modelMappingRepo == nil || route == nil || provider == nil {
		return []string{requestModel}
	}
	mappings, err := h.modelMappingRepo.ListByQuery(tenantID, &domain.ModelMappingQuery{
		ClientType:   clientType,
		ProviderType: provider.Type,
		ProviderID:   provider.ID,
		ProjectID:    projectID,
		RouteID:      route.ID,
		APITokenID:   apiTokenID,
	})
	if err != nil {
		return []string{requestModel}
	}
	candidates := make([]string, 0, len(mappings))
	seen := make(map[string]struct{}, len(mappings))
	for _, mapping := range mappings {
		if mapping == nil || !domain.MatchWildcard(mapping.Pattern, requestModel) {
			continue
		}
		if _, exists := seen[mapping.Target]; exists {
			continue
		}
		seen[mapping.Target] = struct{}{}
		candidates = append(candidates, mapping.Target)
	}
	if len(candidates) == 0 {
		return []string{requestModel}
	}
	return candidates
}

func isProviderAnyModelExposed(provider *domain.Provider, models []string) bool {
	if provider == nil || !provider.ExposedModelsEnabled {
		return true
	}
	for _, model := range models {
		if isProviderModelExposed(provider, model) {
			return true
		}
	}
	return false
}

func isProviderModelExposed(provider *domain.Provider, model string) bool {
	if provider == nil || !provider.ExposedModelsEnabled {
		return true
	}
	if len(provider.ExposedModels) == 0 {
		return false
	}
	for _, pattern := range provider.ExposedModels {
		if domain.MatchWildcard(pattern, model) {
			return true
		}
	}
	return false
}

func sortedModelNames(result map[string]struct{}) []string {
	names := make([]string, 0, len(result))
	for name := range result {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func modelListProjectID(r *http.Request) uint64 {
	if r == nil {
		return 0
	}
	return parseUintHeader(r, "X-Maxx-Project-ID")
}

func modelListProviderID(r *http.Request) uint64 {
	if r == nil {
		return 0
	}
	return parseUintHeader(r, "X-Maxx-Provider-ID")
}

func parseUintHeader(r *http.Request, name string) uint64 {
	if r == nil {
		return 0
	}
	value := strings.TrimSpace(r.Header.Get(name))
	if value == "" {
		return 0
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
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

func buildGeminiModelsResponse(names []string) map[string]interface{} {
	models := make([]map[string]interface{}, 0, len(names))
	for _, name := range names {
		modelName := name
		if !strings.HasPrefix(modelName, "models/") {
			modelName = "models/" + modelName
		}
		baseModelID := strings.TrimPrefix(modelName, "models/")
		models = append(models, map[string]interface{}{
			"name":                       modelName,
			"baseModelId":                baseModelID,
			"version":                    "",
			"displayName":                baseModelID,
			"description":                "",
			"inputTokenLimit":            0,
			"outputTokenLimit":           0,
			"supportedGenerationMethods": []string{"generateContent", "streamGenerateContent"},
		})
	}

	return map[string]interface{}{
		"models": models,
	}
}
