package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	maxxctx "github.com/awsl-project/maxx/internal/context"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/service"
)

var publicSettingsAllowlist = map[string]struct{}{
	"api_token_auth_enabled": {},
	"force_project_binding":  {},
	"force_project_timeout":  {},
	"auto_sort_antigravity":  {},
	"auto_sort_codex":        {},
}

// SelfServiceHandler exposes tenant-scoped provider/project APIs for authenticated users.
type SelfServiceHandler struct {
	svc *service.AdminService
}

// NewSelfServiceHandler creates a new self-service handler.
func NewSelfServiceHandler(svc *service.AdminService) *SelfServiceHandler {
	return &SelfServiceHandler{svc: svc}
}

func writeSelfServiceInternalError(w http.ResponseWriter, context string, err error) {
	log.Printf("[SelfServiceHandler] %s: %v", context, err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
}

func writeSelfServiceInvalidID(w http.ResponseWriter, resource string) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid " + resource + " id"})
}

func parseSelfServiceID(w http.ResponseWriter, resource, raw string) (uint64, bool) {
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		writeSelfServiceInvalidID(w, resource)
		return 0, false
	}
	return id, true
}

// ServeHTTP routes self-service provider/project requests.
func (h *SelfServiceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	resource := parts[1]

	switch resource {
	case "providers":
		switch {
		case len(parts) == 2:
			h.handleProviders(w, r, 0)
		case len(parts) == 3 && parts[2] == "export":
			h.handleProvidersExport(w, r)
		case len(parts) == 3 && parts[2] == "import":
			h.handleProvidersImport(w, r)
		case len(parts) == 3:
			id, ok := parseSelfServiceID(w, "provider", parts[2])
			if !ok {
				return
			}
			h.handleProviders(w, r, id)
		case len(parts) == 4 && parts[3] == "bedrock-models":
			// Mirror the admin endpoint (/api/admin/providers/{id}/bedrock-models)
			// under /api/providers/{id}/bedrock-models so the frontend's
			// default axios baseURL (/api) can read the discovery catalog
			// without talking to the admin-only surface.
			//
			// GET is readable by any authenticated tenant member (same
			// access posture as the providers list). POST forces a fresh
			// ListInferenceProfiles + ListFoundationModels round-trip,
			// bypassing the in-process TTL and Invalidate() rate-limit —
			// gated on admin so a non-privileged member can't hammer it
			// and burn the provider's AWS API quota.
			id, ok := parseSelfServiceID(w, "provider", parts[2])
			if !ok {
				return
			}
			if r.Method == http.MethodPost && !h.requireAdmin(w, r) {
				return
			}
			// Non-admin GET must not be able to trigger a ListInferenceProfiles
			// refresh by polling past the TTL window — pass the caller's
			// admin status through so DiscoveredModels only lazy-refreshes
			// when an admin is on the other end.
			serveBedrockDiscoveredModels(h.svc, w, r, id, h.isAdmin(r))
		default:
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		}
	case "routes":
		switch {
		case len(parts) == 2:
			h.handleRoutes(w, r, 0)
		case len(parts) == 3 && parts[2] == "batch-positions":
			h.handleBatchUpdateRoutePositions(w, r)
		case len(parts) == 3:
			id, ok := parseSelfServiceID(w, "route", parts[2])
			if !ok {
				return
			}
			h.handleRoutes(w, r, id)
		default:
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		}
	case "projects":
		switch {
		case len(parts) == 2:
			h.handleProjects(w, r, 0, parts)
		case len(parts) >= 3 && parts[2] == "by-slug":
			if len(parts) > 4 {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
				return
			}
			h.handleProjectBySlug(w, r, parts)
		case len(parts) == 3:
			id, ok := parseSelfServiceID(w, "project", parts[2])
			if !ok {
				return
			}
			h.handleProjects(w, r, id, parts)
		default:
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		}
	case "retry-configs":
		switch {
		case len(parts) == 2:
			h.handleRetryConfigs(w, r, 0)
		case len(parts) == 3:
			id, ok := parseSelfServiceID(w, "retry config", parts[2])
			if !ok {
				return
			}
			h.handleRetryConfigs(w, r, id)
		default:
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		}
	case "provider-stats":
		if len(parts) != 2 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		h.handleProviderStats(w, r)
	case "model-mappings":
		switch {
		case len(parts) == 2:
			h.handleModelMappings(w, r, 0)
		case len(parts) == 3 && parts[2] == "clear-all":
			h.handleClearAllModelMappings(w, r)
		case len(parts) == 3 && parts[2] == "reset-defaults":
			h.handleResetModelMappingsToDefaults(w, r)
		case len(parts) == 3:
			id, ok := parseSelfServiceID(w, "model mapping", parts[2])
			if !ok {
				return
			}
			h.handleModelMappings(w, r, id)
		default:
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		}
	case "settings":
		if len(parts) > 3 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		h.handleSettings(w, r, parts)
	case "proxy-status":
		if len(parts) != 2 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		h.handleProxyStatus(w, r)
	case "api-tokens":
		switch {
		case len(parts) == 2:
			h.handleAPITokens(w, r, 0)
		case len(parts) == 3:
			id, ok := parseSelfServiceID(w, "api token", parts[2])
			if !ok {
				return
			}
			h.handleAPITokens(w, r, id)
		default:
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		}
	case "model-prices":
		switch {
		case len(parts) == 2:
			h.handleModelPrices(w, r, 0)
		case len(parts) == 3:
			id, ok := parseSelfServiceID(w, "model price", parts[2])
			if !ok {
				return
			}
			h.handleModelPrices(w, r, id)
		default:
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		}
	case "response-models":
		if len(parts) != 2 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		h.handleResponseModels(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

func (h *SelfServiceHandler) handleProviders(w http.ResponseWriter, r *http.Request, id uint64) {
	tenantID := maxxctx.GetTenantID(r.Context())
	isAdmin := h.isAdmin(r)

	switch r.Method {
	case http.MethodGet:
		if id > 0 {
			provider, err := h.svc.GetProvider(tenantID, id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "provider not found"})
				return
			}
			if !isAdmin {
				provider = sanitizeProvider(provider)
			}
			writeJSON(w, http.StatusOK, provider)
			return
		}

		providers, err := h.svc.GetProviders(tenantID)
		if err != nil {
			writeSelfServiceInternalError(w, "GetProviders failed", err)
			return
		}
		if !isAdmin {
			providers = sanitizeProviders(providers)
		}
		if providers == nil {
			providers = []*domain.Provider{}
		}
		writeJSON(w, http.StatusOK, providers)
	case http.MethodPost:
		if !h.requireAdmin(w, r) {
			return
		}
		var provider domain.Provider
		if err := json.NewDecoder(r.Body).Decode(&provider); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := h.svc.CreateProvider(tenantID, &provider); err != nil {
			writeSelfServiceInternalError(w, "CreateProvider failed", err)
			return
		}
		writeJSON(w, http.StatusCreated, provider)
	case http.MethodPut:
		if !h.requireAdmin(w, r) {
			return
		}
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		existing, err := h.svc.GetProvider(tenantID, id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "provider not found"})
			return
		}

		var provider domain.Provider
		if err := json.NewDecoder(r.Body).Decode(&provider); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		provider.ID = existing.ID
		provider.TenantID = existing.TenantID
		provider.CreatedAt = existing.CreatedAt
		if err := h.svc.UpdateProvider(tenantID, &provider); err != nil {
			writeSelfServiceInternalError(w, "UpdateProvider failed", err)
			return
		}
		writeJSON(w, http.StatusOK, provider)
	case http.MethodDelete:
		if !h.requireAdmin(w, r) {
			return
		}
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		if err := h.svc.DeleteProvider(tenantID, id); err != nil {
			writeSelfServiceInternalError(w, "DeleteProvider failed", err)
			return
		}
		writeJSON(w, http.StatusNoContent, nil)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *SelfServiceHandler) handleProvidersExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if !h.requireAdmin(w, r) {
		return
	}

	tenantID := maxxctx.GetTenantID(r.Context())
	providers, err := h.svc.ExportProviders(tenantID)
	if err != nil {
		writeSelfServiceInternalError(w, "ExportProviders failed", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=providers.json")
	json.NewEncoder(w).Encode(providers)
}

func (h *SelfServiceHandler) handleProvidersImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if !h.requireAdmin(w, r) {
		return
	}

	var providers []*domain.Provider
	if err := json.NewDecoder(r.Body).Decode(&providers); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}

	tenantID := maxxctx.GetTenantID(r.Context())
	result, err := h.svc.ImportProviders(tenantID, providers)
	if err != nil {
		writeSelfServiceInternalError(w, "ImportProviders failed", err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *SelfServiceHandler) handleProjects(w http.ResponseWriter, r *http.Request, id uint64, parts []string) {
	tenantID := maxxctx.GetTenantID(r.Context())

	switch r.Method {
	case http.MethodGet:
		if id > 0 {
			project, err := h.svc.GetProject(tenantID, id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
				return
			}
			writeJSON(w, http.StatusOK, project)
			return
		}

		projects, err := h.svc.GetProjects(tenantID)
		if err != nil {
			writeSelfServiceInternalError(w, "GetProjects failed", err)
			return
		}
		if projects == nil {
			projects = []*domain.Project{}
		}
		writeJSON(w, http.StatusOK, projects)
	case http.MethodPost:
		if !h.requireAdmin(w, r) {
			return
		}
		var project domain.Project
		if err := json.NewDecoder(r.Body).Decode(&project); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := h.svc.CreateProject(tenantID, &project); err != nil {
			writeSelfServiceInternalError(w, "CreateProject failed", err)
			return
		}
		writeJSON(w, http.StatusCreated, project)
	case http.MethodPut:
		if !h.requireAdmin(w, r) {
			return
		}
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		existing, err := h.svc.GetProject(tenantID, id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
			return
		}

		var project domain.Project
		if err := json.NewDecoder(r.Body).Decode(&project); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if project.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		if project.Slug == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "slug is required"})
			return
		}
		project.ID = existing.ID
		project.TenantID = existing.TenantID
		project.CreatedAt = existing.CreatedAt
		if err := h.svc.UpdateProject(tenantID, &project); err != nil {
			writeSelfServiceInternalError(w, "UpdateProject failed", err)
			return
		}
		writeJSON(w, http.StatusOK, project)
	case http.MethodDelete:
		if !h.requireAdmin(w, r) {
			return
		}
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		if err := h.svc.DeleteProject(tenantID, id); err != nil {
			writeSelfServiceInternalError(w, "DeleteProject failed", err)
			return
		}
		writeJSON(w, http.StatusNoContent, nil)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *SelfServiceHandler) handleProjectBySlug(w http.ResponseWriter, r *http.Request, parts []string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if len(parts) < 4 || parts[3] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "slug required"})
		return
	}

	tenantID := maxxctx.GetTenantID(r.Context())
	project, err := h.svc.GetProjectBySlug(tenantID, parts[3])
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "project not found"})
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func (h *SelfServiceHandler) handleRoutes(w http.ResponseWriter, r *http.Request, id uint64) {
	tenantID := maxxctx.GetTenantID(r.Context())

	switch r.Method {
	case http.MethodGet:
		if id > 0 {
			route, err := h.svc.GetRoute(tenantID, id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "route not found"})
				return
			}
			writeJSON(w, http.StatusOK, route)
			return
		}

		routes, err := h.svc.GetRoutes(tenantID)
		if err != nil {
			writeSelfServiceInternalError(w, "GetRoutes failed", err)
			return
		}
		if routes == nil {
			routes = []*domain.Route{}
		}
		writeJSON(w, http.StatusOK, routes)
	case http.MethodPost:
		if !h.requireAdmin(w, r) {
			return
		}
		var route domain.Route
		if err := json.NewDecoder(r.Body).Decode(&route); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := h.svc.CreateRoute(tenantID, &route); err != nil {
			writeSelfServiceInternalError(w, "CreateRoute failed", err)
			return
		}
		writeJSON(w, http.StatusCreated, route)
	case http.MethodPut:
		if !h.requireAdmin(w, r) {
			return
		}
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}

		existing, err := h.svc.GetRoute(tenantID, id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "route not found"})
			return
		}

		var updates map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if v, ok := updates["isEnabled"]; ok {
			if b, ok := v.(bool); ok {
				existing.IsEnabled = b
			}
		}
		if v, ok := updates["isNative"]; ok {
			if b, ok := v.(bool); ok {
				existing.IsNative = b
			}
		}
		if v, ok := updates["projectID"]; ok {
			if f, ok := v.(float64); ok {
				existing.ProjectID = uint64(f)
			}
		}
		if v, ok := updates["clientType"]; ok {
			if s, ok := v.(string); ok {
				existing.ClientType = domain.ClientType(s)
			}
		}
		if v, ok := updates["providerID"]; ok {
			if f, ok := v.(float64); ok {
				existing.ProviderID = uint64(f)
			}
		}
		if v, ok := updates["position"]; ok {
			if f, ok := v.(float64); ok {
				existing.Position = int(f)
			}
		}
		if v, ok := updates["weight"]; ok {
			if f, ok := v.(float64); ok {
				w := int(f)
				if w <= 0 {
					w = 1
				}
				existing.Weight = w
			}
		}
		if v, ok := updates["retryConfigID"]; ok {
			if f, ok := v.(float64); ok {
				existing.RetryConfigID = uint64(f)
			}
		}
		if err := h.svc.UpdateRoute(tenantID, existing); err != nil {
			writeSelfServiceInternalError(w, "UpdateRoute failed", err)
			return
		}
		writeJSON(w, http.StatusOK, existing)
	case http.MethodDelete:
		if !h.requireAdmin(w, r) {
			return
		}
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		if err := h.svc.DeleteRoute(tenantID, id); err != nil {
			writeSelfServiceInternalError(w, "DeleteRoute failed", err)
			return
		}
		writeJSON(w, http.StatusNoContent, nil)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *SelfServiceHandler) handleBatchUpdateRoutePositions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if !h.requireAdmin(w, r) {
		return
	}

	var updates []domain.RoutePositionUpdate
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	tenantID := maxxctx.GetTenantID(r.Context())
	if err := h.svc.BatchUpdateRoutePositions(tenantID, updates); err != nil {
		writeSelfServiceInternalError(w, "BatchUpdateRoutePositions failed", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "positions updated successfully"})
}

func (h *SelfServiceHandler) handleRetryConfigs(w http.ResponseWriter, r *http.Request, id uint64) {
	tenantID := maxxctx.GetTenantID(r.Context())

	switch r.Method {
	case http.MethodGet:
		if id > 0 {
			config, err := h.svc.GetRetryConfig(tenantID, id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "retry config not found"})
				return
			}
			writeJSON(w, http.StatusOK, config)
			return
		}

		configs, err := h.svc.GetRetryConfigs(tenantID)
		if err != nil {
			writeSelfServiceInternalError(w, "GetRetryConfigs failed", err)
			return
		}
		writeJSON(w, http.StatusOK, configs)
	case http.MethodPost:
		if !h.requireAdmin(w, r) {
			return
		}
		var config domain.RetryConfig
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := h.svc.CreateRetryConfig(tenantID, &config); err != nil {
			writeSelfServiceInternalError(w, "CreateRetryConfig failed", err)
			return
		}
		writeJSON(w, http.StatusCreated, config)
	case http.MethodPut:
		if !h.requireAdmin(w, r) {
			return
		}
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}

		existing, err := h.svc.GetRetryConfig(tenantID, id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "retry config not found"})
			return
		}

		var config domain.RetryConfig
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		config.ID = existing.ID
		config.TenantID = existing.TenantID
		config.CreatedAt = existing.CreatedAt
		if err := h.svc.UpdateRetryConfig(tenantID, &config); err != nil {
			writeSelfServiceInternalError(w, "UpdateRetryConfig failed", err)
			return
		}
		writeJSON(w, http.StatusOK, config)
	case http.MethodDelete:
		if !h.requireAdmin(w, r) {
			return
		}
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		if err := h.svc.DeleteRetryConfig(tenantID, id); err != nil {
			writeSelfServiceInternalError(w, "DeleteRetryConfig failed", err)
			return
		}
		writeJSON(w, http.StatusNoContent, nil)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *SelfServiceHandler) handleProviderStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	tenantID := maxxctx.GetTenantID(r.Context())
	clientType := r.URL.Query().Get("client_type")
	var projectID uint64
	if pidStr := r.URL.Query().Get("project_id"); pidStr != "" {
		var err error
		projectID, err = strconv.ParseUint(pidStr, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid project_id query parameter"})
			return
		}
	}

	stats, err := h.svc.GetProviderStats(tenantID, clientType, projectID)
	if err != nil {
		writeSelfServiceInternalError(w, "GetProviderStats failed", err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (h *SelfServiceHandler) handleModelMappings(w http.ResponseWriter, r *http.Request, id uint64) {
	tenantID := maxxctx.GetTenantID(r.Context())

	switch r.Method {
	case http.MethodGet:
		if id > 0 {
			mapping, err := h.svc.GetModelMapping(tenantID, id)
			if err != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "mapping not found"})
				return
			}
			writeJSON(w, http.StatusOK, mapping)
			return
		}

		mappings, err := h.svc.GetModelMappings(tenantID)
		if err != nil {
			writeSelfServiceInternalError(w, "GetModelMappings failed", err)
			return
		}
		writeJSON(w, http.StatusOK, mappings)
	case http.MethodPost:
		if !h.requireAdmin(w, r) {
			return
		}
		var mapping domain.ModelMapping
		if err := json.NewDecoder(r.Body).Decode(&mapping); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if mapping.Pattern == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "pattern is required"})
			return
		}
		if mapping.Target == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target is required"})
			return
		}
		if err := h.svc.CreateModelMapping(tenantID, &mapping); err != nil {
			writeSelfServiceInternalError(w, "CreateModelMapping failed", err)
			return
		}
		writeJSON(w, http.StatusCreated, mapping)
	case http.MethodPut:
		if !h.requireAdmin(w, r) {
			return
		}
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}

		existing, err := h.svc.GetModelMapping(tenantID, id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "mapping not found"})
			return
		}

		var body struct {
			ClientType *string `json:"clientType"`
			Pattern    *string `json:"pattern"`
			Target     *string `json:"target"`
			Priority   *int    `json:"priority"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if body.ClientType != nil {
			existing.ClientType = domain.ClientType(*body.ClientType)
		}
		if body.Pattern != nil {
			if *body.Pattern == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "pattern cannot be empty"})
				return
			}
			existing.Pattern = *body.Pattern
		}
		if body.Target != nil {
			if *body.Target == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "target cannot be empty"})
				return
			}
			existing.Target = *body.Target
		}
		if body.Priority != nil {
			existing.Priority = *body.Priority
		}
		if err := h.svc.UpdateModelMapping(tenantID, existing); err != nil {
			writeSelfServiceInternalError(w, "UpdateModelMapping failed", err)
			return
		}
		writeJSON(w, http.StatusOK, existing)
	case http.MethodDelete:
		if !h.requireAdmin(w, r) {
			return
		}
		if id == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		if err := h.svc.DeleteModelMapping(tenantID, id); err != nil {
			writeSelfServiceInternalError(w, "DeleteModelMapping failed", err)
			return
		}
		writeJSON(w, http.StatusNoContent, nil)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *SelfServiceHandler) handleClearAllModelMappings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if !h.requireAdmin(w, r) {
		return
	}

	tenantID := maxxctx.GetTenantID(r.Context())
	if err := h.svc.ClearAllModelMappings(tenantID); err != nil {
		writeSelfServiceInternalError(w, "ClearAllModelMappings failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "all mappings cleared"})
}

func (h *SelfServiceHandler) handleResetModelMappingsToDefaults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if !h.requireAdmin(w, r) {
		return
	}

	tenantID := maxxctx.GetTenantID(r.Context())
	if err := h.svc.ResetModelMappingsToDefaults(tenantID); err != nil {
		writeSelfServiceInternalError(w, "ResetModelMappingsToDefaults failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "mappings reset to defaults"})
}

func (h *SelfServiceHandler) handleSettings(w http.ResponseWriter, r *http.Request, parts []string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var key string
	if len(parts) > 2 {
		key = parts[2]
	}

	if key != "" {
		if _, ok := publicSettingsAllowlist[key]; !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "setting not found"})
			return
		}
		value, err := h.svc.GetSetting(key)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "setting not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"key": key, "value": value})
		return
	}

	settings, err := h.svc.GetSettings()
	if err != nil {
		writeSelfServiceInternalError(w, "GetSettings failed", err)
		return
	}

	filtered := make(map[string]string, len(publicSettingsAllowlist))
	for allowKey := range publicSettingsAllowlist {
		if value, ok := settings[allowKey]; ok {
			filtered[allowKey] = value
		}
	}
	writeJSON(w, http.StatusOK, filtered)
}

func (h *SelfServiceHandler) handleAPITokens(w http.ResponseWriter, r *http.Request, id uint64) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	tenantID := maxxctx.GetTenantID(r.Context())
	if id > 0 {
		token, err := h.svc.GetAPIToken(tenantID, id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "token not found"})
			return
		}
		writeJSON(w, http.StatusOK, sanitizeAPIToken(token))
		return
	}

	tokens, err := h.svc.GetAPITokens(tenantID)
	if err != nil {
		writeSelfServiceInternalError(w, "GetAPITokens failed", err)
		return
	}
	tokens = sanitizeAPITokens(tokens)
	if tokens == nil {
		tokens = []*domain.APIToken{}
	}
	writeJSON(w, http.StatusOK, tokens)
}

func (h *SelfServiceHandler) handleProxyStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	writeJSON(w, http.StatusOK, h.svc.GetProxyStatus(r))
}

func sanitizeAPIToken(token *domain.APIToken) *domain.APIToken {
	if token == nil {
		return nil
	}
	sanitized := *token
	sanitized.Token = ""
	return &sanitized
}

func sanitizeAPITokens(tokens []*domain.APIToken) []*domain.APIToken {
	if len(tokens) == 0 {
		return tokens
	}
	sanitized := make([]*domain.APIToken, 0, len(tokens))
	for _, token := range tokens {
		sanitized = append(sanitized, sanitizeAPIToken(token))
	}
	return sanitized
}

func sanitizeProvider(provider *domain.Provider) *domain.Provider {
	if provider == nil {
		return nil
	}
	sanitized := *provider
	if provider.Config == nil {
		return &sanitized
	}

	config := *provider.Config
	if config.Custom != nil {
		custom := *config.Custom
		custom.APIKey = ""
		if custom.Disguise != nil {
			disguise := *custom.Disguise
			if disguise.ClaudeCode != nil {
				cc := *disguise.ClaudeCode
				disguise.ClaudeCode = &cc
			}
			custom.Disguise = &disguise
		}
		config.Custom = &custom
	}
	if config.Antigravity != nil {
		antigravity := *config.Antigravity
		antigravity.RefreshToken = ""
		config.Antigravity = &antigravity
	}
	if config.Kiro != nil {
		kiro := *config.Kiro
		kiro.RefreshToken = ""
		kiro.ClientSecret = ""
		config.Kiro = &kiro
	}
	if config.Codex != nil {
		codex := *config.Codex
		codex.RefreshToken = ""
		codex.AccessToken = ""
		config.Codex = &codex
	}
	if config.Claude != nil {
		claude := *config.Claude
		claude.RefreshToken = ""
		claude.AccessToken = ""
		config.Claude = &claude
	}
	sanitized.Config = &config
	return &sanitized
}

func sanitizeProviders(providers []*domain.Provider) []*domain.Provider {
	if len(providers) == 0 {
		return providers
	}
	sanitized := make([]*domain.Provider, 0, len(providers))
	for _, provider := range providers {
		sanitized = append(sanitized, sanitizeProvider(provider))
	}
	return sanitized
}

func (h *SelfServiceHandler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if h.isAdmin(r) {
		return true
	}
	writeJSON(w, http.StatusForbidden, map[string]string{"error": "admin access required"})
	return false
}

func (h *SelfServiceHandler) isAdmin(r *http.Request) bool {
	return maxxctx.GetUserRole(r.Context()) == string(domain.UserRoleAdmin)
}

func (h *SelfServiceHandler) handleModelPrices(w http.ResponseWriter, r *http.Request, id uint64) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	if id > 0 {
		price, err := h.svc.GetModelPrice(id)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "model price not found"})
			return
		}
		writeJSON(w, http.StatusOK, price)
		return
	}

	prices, err := h.svc.GetModelPrices()
	if err != nil {
		writeSelfServiceInternalError(w, "GetModelPrices failed", err)
		return
	}
	if prices == nil {
		prices = []*domain.ModelPrice{}
	}
	writeJSON(w, http.StatusOK, prices)
}

func (h *SelfServiceHandler) handleResponseModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	names, err := h.svc.GetResponseModelNames()
	if err != nil {
		writeSelfServiceInternalError(w, "GetResponseModelNames failed", err)
		return
	}
	if names == nil {
		names = []string{}
	}
	writeJSON(w, http.StatusOK, names)
}
