package handler

import (
	"encoding/json"
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

type modelRouteGroup struct {
	RouteID      uint64            `json:"routeID"`
	ProviderID   uint64            `json:"providerID"`
	ProviderName string            `json:"providerName"`
	ClientType   domain.ClientType `json:"clientType"`
	ProjectID    uint64            `json:"projectID"`
	Models       []string          `json:"models"`
}

// ModelsHandler serves model-list endpoints with a lightweight model list.
type ModelsHandler struct {
	responseModelRepo repository.ResponseModelRepository
	providerRepo      repository.ProviderRepository
	modelMappingRepo  repository.ModelMappingRepository
	settingsRepo      repository.SystemSettingRepository
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

// SetSettingsRepository wires settings that can override the public model-list surface.
func (h *ModelsHandler) SetSettingsRepository(settingsRepo repository.SystemSettingRepository) {
	h.settingsRepo = settingsRepo
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
	if models, enabled, err := h.collectConfiguredExternalModelClientTypeList(clientType); err != nil {
		return nil, err
	} else if enabled {
		return sortedModelNames(models), nil
	}

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
	if models, enabled, err := h.collectConfiguredExternalModelList(); err != nil {
		return nil, err
	} else if enabled {
		return models, nil
	}

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
	return len(h.matchAvailableModelRoutes(tenantID, clientType, projectID, providerID, apiTokenID, model)) > 0
}

func (h *ModelsHandler) matchAvailableModelRoutes(tenantID uint64, clientType domain.ClientType, projectID, providerID, apiTokenID uint64, model string) []*router.MatchedRoute {
	if h.router == nil || clientType == "" || model == "" {
		return nil
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
		return nil
	}
	matchedRoutes := make([]*router.MatchedRoute, 0, len(result.Routes))
	for _, matched := range result.Routes {
		if matched == nil || matched.Route == nil || matched.Provider == nil {
			continue
		}
		if providerID != 0 && matched.Provider.ID != providerID {
			continue
		}
		candidates := h.modelCandidatesForRoute(tenantID, model, matched.Route, matched.Provider, clientType, projectID, apiTokenID)
		if isProviderAnyModelExposed(matched.Provider, append([]string{model}, candidates...)) {
			matchedRoutes = append(matchedRoutes, matched)
		}
	}
	return matchedRoutes
}

func (h *ModelsHandler) collectAvailableModelRouteGroups(tenantID uint64, clientType domain.ClientType, projectID, providerID, apiTokenID uint64, userAgent string) ([]modelRouteGroup, error) {
	if groups, enabled, err := h.collectConfiguredExternalModelRouteGroups(tenantID, clientType, projectID, providerID); err != nil {
		return nil, err
	} else if enabled {
		return groups, nil
	}

	candidates, err := h.collectCandidateModelNames(tenantID, userAgent)
	if err != nil {
		return nil, err
	}
	if h.router == nil {
		return nil, nil
	}

	type routeKey struct {
		routeID    uint64
		providerID uint64
	}
	groupsByKey := make(map[routeKey]*modelRouteGroup)
	modelSets := make(map[routeKey]map[string]struct{})

	for name := range candidates {
		for _, matched := range h.matchAvailableModelRoutes(tenantID, clientType, projectID, providerID, apiTokenID, name) {
			key := routeKey{routeID: matched.Route.ID, providerID: matched.Provider.ID}
			group := groupsByKey[key]
			if group == nil {
				group = &modelRouteGroup{
					RouteID:      matched.Route.ID,
					ProviderID:   matched.Provider.ID,
					ProviderName: matched.Provider.Name,
					ClientType:   matched.Route.ClientType,
					ProjectID:    matched.Route.ProjectID,
				}
				groupsByKey[key] = group
				modelSets[key] = make(map[string]struct{})
			}
			modelSets[key][name] = struct{}{}
		}
	}

	groups := make([]modelRouteGroup, 0, len(groupsByKey))
	for key, group := range groupsByKey {
		group.Models = sortedModelNames(modelSets[key])
		if len(group.Models) > 0 {
			groups = append(groups, *group)
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].ProjectID != groups[j].ProjectID {
			return groups[i].ProjectID < groups[j].ProjectID
		}
		if groups[i].ClientType != groups[j].ClientType {
			return groups[i].ClientType < groups[j].ClientType
		}
		return groups[i].RouteID < groups[j].RouteID
	})
	return groups, nil
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

func (h *ModelsHandler) collectConfiguredExternalModelRouteGroups(tenantID uint64, clientType domain.ClientType, projectID, providerID uint64) ([]modelRouteGroup, bool, error) {
	if h == nil || h.settingsRepo == nil {
		return nil, false, nil
	}
	enabledValue, err := h.settingsRepo.Get(domain.SettingKeyExternalModelListEnabled)
	if err != nil {
		return nil, false, err
	}
	if !strings.EqualFold(strings.TrimSpace(enabledValue), "true") {
		return nil, false, nil
	}
	value, err := h.settingsRepo.Get(domain.SettingKeyExternalModelList)
	if err != nil {
		return nil, false, err
	}
	clientTypeModels, hasClientTypes := parseExternalModelClientTypeSetting(value)
	if hasClientTypes {
		groups := make([]modelRouteGroup, 0, len(clientTypeModels))
		for groupClientType, models := range clientTypeModels {
			if clientType != "" && groupClientType != clientType {
				continue
			}
			if providerID != 0 || projectID != 0 {
				continue
			}
			normalizedModels := normalizeExternalModelList(models)
			sort.Strings(normalizedModels)
			if len(normalizedModels) == 0 {
				continue
			}
			groups = append(groups, modelRouteGroup{
				ClientType: groupClientType,
				Models:     normalizedModels,
			})
		}
		sort.Slice(groups, func(i, j int) bool { return groups[i].ClientType < groups[j].ClientType })
		return groups, true, nil
	}
	if h.router == nil {
		return nil, false, nil
	}
	routeModels, categorized := parseExternalModelRouteSetting(value)
	if !categorized {
		return nil, false, nil
	}

	groups := make([]modelRouteGroup, 0, len(routeModels))
	for routeID, models := range routeModels {
		route, provider, err := h.router.GetRouteProviderByID(tenantID, routeID)
		if err != nil || route == nil || provider == nil || !route.IsEnabled {
			continue
		}
		if clientType != "" && route.ClientType != clientType {
			continue
		}
		if projectID != 0 && route.ProjectID != projectID {
			continue
		}
		if providerID != 0 && provider.ID != providerID {
			continue
		}
		normalizedModels := normalizeExternalModelList(models)
		sort.Strings(normalizedModels)
		if len(normalizedModels) == 0 {
			continue
		}
		groups = append(groups, modelRouteGroup{
			RouteID:      route.ID,
			ProviderID:   provider.ID,
			ProviderName: provider.Name,
			ClientType:   route.ClientType,
			ProjectID:    route.ProjectID,
			Models:       normalizedModels,
		})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].ProjectID != groups[j].ProjectID {
			return groups[i].ProjectID < groups[j].ProjectID
		}
		if groups[i].ClientType != groups[j].ClientType {
			return groups[i].ClientType < groups[j].ClientType
		}
		return groups[i].RouteID < groups[j].RouteID
	})
	return groups, true, nil
}

func (h *ModelsHandler) collectConfiguredExternalModelClientTypeList(clientType domain.ClientType) (map[string]struct{}, bool, error) {
	result := make(map[string]struct{})
	if h == nil || h.settingsRepo == nil || clientType == "" {
		return result, false, nil
	}
	enabledValue, err := h.settingsRepo.Get(domain.SettingKeyExternalModelListEnabled)
	if err != nil {
		return nil, false, err
	}
	if !strings.EqualFold(strings.TrimSpace(enabledValue), "true") {
		return result, false, nil
	}
	value, err := h.settingsRepo.Get(domain.SettingKeyExternalModelList)
	if err != nil {
		return nil, false, err
	}
	clientTypeModels, categorized := parseExternalModelClientTypeSetting(value)
	if !categorized {
		return result, false, nil
	}
	for _, name := range clientTypeModels[clientType] {
		addModelName(result, name)
	}
	return result, true, nil
}

func (h *ModelsHandler) collectConfiguredExternalModelList() (map[string]struct{}, bool, error) {
	result := make(map[string]struct{})
	if h == nil || h.settingsRepo == nil {
		return result, false, nil
	}
	enabledValue, err := h.settingsRepo.Get(domain.SettingKeyExternalModelListEnabled)
	if err != nil {
		return nil, false, err
	}
	if !strings.EqualFold(strings.TrimSpace(enabledValue), "true") {
		return result, false, nil
	}

	value, err := h.settingsRepo.Get(domain.SettingKeyExternalModelList)
	if err != nil {
		return nil, false, err
	}
	for _, name := range parseExternalModelListSetting(value) {
		addModelName(result, name)
	}
	return result, true, nil
}

func parseExternalModelClientTypeSetting(value string) (map[domain.ClientType][]string, bool) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "{") {
		return nil, false
	}
	var categorized struct {
		ClientTypes map[string][]string `json:"clientTypes"`
	}
	if json.Unmarshal([]byte(trimmed), &categorized) != nil || len(categorized.ClientTypes) == 0 {
		return nil, false
	}
	result := make(map[domain.ClientType][]string, len(categorized.ClientTypes))
	valid := map[domain.ClientType]struct{}{
		domain.ClientTypeOpenAI: {},
		domain.ClientTypeCodex:  {},
		domain.ClientTypeClaude: {},
		domain.ClientTypeGemini: {},
	}
	for key, models := range categorized.ClientTypes {
		clientType := domain.ClientType(strings.TrimSpace(key))
		if _, ok := valid[clientType]; !ok {
			continue
		}
		if normalized := normalizeExternalModelList(models); len(normalized) > 0 {
			result[clientType] = normalized
		}
	}
	return result, true
}

func parseExternalModelRouteSetting(value string) (map[uint64][]string, bool) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "{") {
		return nil, false
	}
	var categorized struct {
		Routes map[string][]string `json:"routes"`
	}
	if json.Unmarshal([]byte(trimmed), &categorized) != nil || len(categorized.Routes) == 0 {
		return nil, false
	}
	result := make(map[uint64][]string, len(categorized.Routes))
	for key, models := range categorized.Routes {
		routeID, err := strconv.ParseUint(strings.TrimSpace(key), 10, 64)
		if err != nil || routeID == 0 {
			continue
		}
		if normalized := normalizeExternalModelList(models); len(normalized) > 0 {
			result[routeID] = normalized
		}
	}
	return result, true
}

func parseExternalModelListSetting(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	var parsed []string
	if strings.HasPrefix(trimmed, "[") && json.Unmarshal([]byte(trimmed), &parsed) == nil {
		return normalizeExternalModelList(parsed)
	}
	if strings.HasPrefix(trimmed, "{") {
		var categorized struct {
			ClientTypes   map[string][]string `json:"clientTypes"`
			Routes        map[string][]string `json:"routes"`
			Uncategorized []string            `json:"uncategorized"`
		}
		if json.Unmarshal([]byte(trimmed), &categorized) == nil {
			values := make([]string, 0, len(categorized.Uncategorized))
			for _, models := range categorized.ClientTypes {
				values = append(values, models...)
			}
			for _, models := range categorized.Routes {
				values = append(values, models...)
			}
			values = append(values, categorized.Uncategorized...)
			return normalizeExternalModelList(values)
		}
	}
	fields := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ','
	})
	return normalizeExternalModelList(fields)
}

func normalizeExternalModelList(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		model := strings.TrimSpace(value)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		result = append(result, model)
	}
	return result
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
